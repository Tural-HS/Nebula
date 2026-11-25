package server

import (
	"net/url"
	"sort"
	"testing"
)

// Field filter operators (ADR-0048): "~text" contains on text fields,
// "min..max" ranges on number and date fields; counts follow the list.
func TestFieldFilterOperators(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for _, f := range []map[string]any{
		{"key": "phone", "name": "Telefon", "type": "text"},
		{"key": "bal", "name": "Qalıq", "type": "number"},
		{"key": "loan", "name": "Kredit tarixi", "type": "date"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
		e.must(200, "alice", "PUT", bindPath("task", f["key"].(string)), map[string]any{})
	}
	mk := func(title string, fields map[string]any) {
		e.must(201, "alice", "POST", items, map[string]any{"title": title, "fields": fields})
	}
	mk("Əliyev", map[string]any{"phone": "+994 50 123 45 67", "bal": 100, "loan": "2026-01-15"})
	mk("İsmayılov", map[string]any{"phone": "0551234567", "bal": 750.5, "loan": "2026-06-30"})
	mk("Wild_card", map[string]any{"phone": "50%_off", "bal": 2000, "loan": "2027-02-01"})
	mk("Boş", map[string]any{})

	titles := func(user, q string) []string {
		t.Helper()
		r := e.must(200, user, "GET", items+"?"+q, nil)
		var out []string
		for _, it := range r.Body["items"].([]any) {
			out = append(out, it.(map[string]any)["title"].(string))
		}
		sort.Strings(out)
		return out
	}
	enc := func(k, v string) string { return url.Values{"field." + k: {v}}.Encode() }
	check := func(q string, want ...string) {
		t.Helper()
		got := titles("carol", q) // a member
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", q, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", q, got, want)
			}
		}
	}

	// Contains: substring, case and Azerbaijani letters folded; wildcards literal.
	check(enc("phone", "~123"), "Əliyev", "İsmayılov")
	check(enc("phone", "~ 50 "), "Əliyev", "Wild_card")
	check(enc("phone", "~%"), "Wild_card")
	check(enc("phone", "~_"), "Wild_card")
	check(enc("phone", "+994 50 123 45 67"), "Əliyev") // exact still works

	// Number ranges, open on either side, inclusive.
	check(enc("bal", "100..750.5"), "Əliyev", "İsmayılov")
	check(enc("bal", "751.."), "Wild_card")
	check(enc("bal", "..100"), "Əliyev")
	check(enc("bal", "750.5"), "İsmayılov")

	// Date ranges.
	check(enc("loan", "2026-01-01..2026-12-31"), "Əliyev", "İsmayılov")
	check(enc("loan", "2027-01-01.."), "Wild_card")

	// Combined with another filter: AND.
	check(enc("bal", "..1000")+"&"+enc("phone", "~055"), "İsmayılov")

	// Counts and sums use the same filters.
	c := e.must(200, "carol", "GET", items+"/counts?"+enc("phone", "~123"), nil)
	if c.Body["total"] != float64(2) {
		t.Fatalf("counts with contains filter: %v", c.Body)
	}

	// Empty / set on any type: "Boş" has none of the fields.
	check(enc("phone", "is:empty"), "Boş")
	check(enc("bal", "is:set"), "Əliyev", "İsmayılov", "Wild_card")
	check(enc("loan", "is:empty"), "Boş")

	// Any of several choices (select and multi_select).
	sel := e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "branch", "name": "Filial", "type": "select",
		"options": map[string]any{"choices": []map[string]any{{"label": "Baş ofis"}, {"label": "Gəncə"}, {"label": "Sumqayıt"}}}}).Body
	e.must(200, "alice", "PUT", bindPath("task", "branch"), map[string]any{})
	tags := e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "tags", "name": "Etiketlər", "type": "multi_select",
		"options": map[string]any{"choices": []map[string]any{{"label": "VIP"}, {"label": "Gecikən"}}}}).Body
	e.must(200, "alice", "PUT", bindPath("task", "tags"), map[string]any{})
	cid := func(f map[string]any, i int) string {
		return f["options"].(map[string]any)["choices"].([]any)[i].(map[string]any)["id"].(string)
	}
	mk("Gəncə müştəri", map[string]any{"branch": cid(sel, 1), "tags": []string{cid(tags, 0)}})
	mk("Sumqayıt müştəri", map[string]any{"branch": cid(sel, 2), "tags": []string{cid(tags, 1)}})
	mk("Baş ofis müştəri", map[string]any{"branch": cid(sel, 0), "tags": []string{cid(tags, 0), cid(tags, 1)}})
	check(enc("branch", "in:"+cid(sel, 1)+","+cid(sel, 2)), "Gəncə müştəri", "Sumqayıt müştəri")
	check(enc("tags", "in:"+cid(tags, 1)), "Sumqayıt müştəri", "Baş ofis müştəri")
	check(enc("branch", "is:empty")+"&"+enc("bal", "is:set"), "Əliyev", "İsmayılov", "Wild_card")
	for _, bad := range []string{"in:", "in:not-a-choice", "in:" + cid(tags, 0)} { // last: a tags id on branch
		if r := e.call("carol", "GET", items+"?"+enc("branch", bad), nil); r.Status != 422 {
			t.Fatalf("branch %q: want 422, got %d", bad, r.Status)
		}
	}

	// Validation.
	for _, q := range []string{
		enc("phone", "~"),           // empty contains
		enc("bal", ".."),            // empty range
		enc("bal", "a..b"),          // not numbers
		enc("loan", "2026-13-01.."), // not a date
		enc("phone", "1..2"),        // range on text: exact "1..2", no match, but valid
	} {
		r := e.call("carol", "GET", items+"?"+q, nil)
		if q == enc("phone", "1..2") {
			if r.Status != 200 {
				t.Fatalf("%s: %d", q, r.Status)
			}
			continue
		}
		if r.Status != 422 {
			t.Fatalf("%s: want 422, got %d %s", q, r.Status, r.Raw)
		}
	}
	long := "~"
	for range 101 {
		long += "x"
	}
	if r := e.call("carol", "GET", items+"?"+enc("phone", long), nil); r.Status != 422 {
		t.Fatalf("long contains: %d", r.Status)
	}

	// A guest sees the same filtered list (read access).
	if got := titles("bob", enc("bal", "751..")); len(got) != 1 {
		t.Fatalf("guest range: %v", got)
	}

	// Saved views accept the operators and reject bad ones.
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects/ACME/views", map[string]any{"name": "Böyük qalıq", "kind": "table", "shared": true,
		"config": map[string]any{"filters": map[string]any{"fields": map[string]string{"bal": "500..", "phone": "~50"}}}})
	if r := e.call("alice", "POST", "/api/v1/workspaces/acme/projects/ACME/views", map[string]any{"name": "Səhv", "kind": "table", "shared": true,
		"config": map[string]any{"filters": map[string]any{"fields": map[string]string{"bal": "x.."}}}}); r.Status != 422 {
		t.Fatalf("bad view range: %d", r.Status)
	}
}
