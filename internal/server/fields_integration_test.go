package server

import (
	"fmt"
	"reflect"
	"testing"
)

const fieldsPath = "/api/v1/workspaces/acme/fields"

func bindPath(typeKey, field string) string {
	return fmt.Sprintf("/api/v1/workspaces/acme/item-types/%s/fields/%s", typeKey, field)
}

func errField(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["field"].(string)
		return s
	}
	return ""
}

func choiceIDs(t *testing.T, field resp) map[string]string {
	t.Helper()
	out := map[string]string{}
	opts, _ := field.Body["options"].(map[string]any)
	list, _ := opts["choices"].([]any)
	for _, c := range list {
		m := c.(map[string]any)
		out[m["label"].(string)] = m["id"].(string)
	}
	if len(out) == 0 {
		t.Fatalf("no choices in %s", field.Raw)
	}
	return out
}

func TestCustomFields(t *testing.T) {
	e := newEnv(t)
	e.setup()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	daveID := e.must(200, "dave", "GET", "/api/v1/me", nil).Body["id"].(string) // not a member

	// --- Definitions: admin only, validated, keys unique -------------------
	sevBody := map[string]any{"key": "severity", "name": "Severity", "type": "select",
		"options": map[string]any{"choices": []map[string]any{{"label": "High", "color": "#ff0000"}, {"label": "Low"}}}}
	if r := e.call("carol", "POST", fieldsPath, sevBody); r.Status != 403 {
		t.Fatalf("member created a field: %d %s", r.Status, r.Raw)
	}
	sev := e.must(201, "alice", "POST", fieldsPath, sevBody)
	choices := choiceIDs(t, sev)
	if r := e.call("alice", "POST", fieldsPath, sevBody); r.Status != 409 || errCode(r) != "field_key_taken" {
		t.Fatalf("duplicate key: %d %s", r.Status, r.Raw)
	}
	for name, body := range map[string]map[string]any{
		"bad type":            {"key": "x", "name": "X", "type": "currency"},
		"bad key":             {"key": "Bad-Key", "name": "X", "type": "text"},
		"choices on text":     {"key": "x", "name": "X", "type": "text", "options": map[string]any{"choices": []any{map[string]any{"label": "a"}}}},
		"duplicate choice":    {"key": "x", "name": "X", "type": "select", "options": map[string]any{"choices": []any{map[string]any{"label": "a"}, map[string]any{"label": "A"}}}},
		"client-chosen id":    {"key": "x", "name": "X", "type": "select", "options": map[string]any{"choices": []any{map[string]any{"id": "abc", "label": "a"}}}},
		"invalid color value": {"key": "x", "name": "X", "type": "select", "options": map[string]any{"choices": []any{map[string]any{"label": "a", "color": "red"}}}},
	} {
		if r := e.call("alice", "POST", fieldsPath, body); r.Status != 422 {
			t.Errorf("%s: status %d, want 422; %s", name, r.Status, r.Raw)
		}
	}
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "estimate", "name": "Estimate", "type": "number"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "reviewer", "name": "Reviewer", "type": "user"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "tags", "name": "Tags", "type": "multi_select",
		"options": map[string]any{"choices": []map[string]any{{"label": "ui"}, {"label": "api"}}}})
	tags := choiceIDs(t, e.must(200, "bob", "GET", fieldsPath+"/tags", nil)) // guests can read

	list := e.must(200, "bob", "GET", fieldsPath, nil)
	if n := len(list.Body["fields"].([]any)); n != 4 {
		t.Fatalf("listed %d fields, want 4", n)
	}

	// key and type are immutable: the strict decoder rejects them.
	if r := e.call("alice", "PATCH", fieldsPath+"/severity", map[string]any{"type": "text"}); r.Status != 400 {
		t.Fatalf("type change: %d %s", r.Status, r.Raw)
	}

	// --- Bindings ------------------------------------------------------------
	if r := e.call("carol", "PUT", bindPath("bug", "severity"), map[string]any{"required": true}); r.Status != 403 {
		t.Fatalf("member bound a field: %d", r.Status)
	}
	e.must(200, "alice", "PUT", bindPath("bug", "severity"), map[string]any{"required": true, "position": 1})
	e.must(200, "alice", "PUT", bindPath("task", "estimate"), map[string]any{"position": 1})
	e.must(200, "alice", "PUT", bindPath("task", "reviewer"), map[string]any{"position": 2})
	e.must(200, "alice", "PUT", bindPath("task", "tags"), map[string]any{"position": 3})
	e.must(404, "alice", "PUT", bindPath("nope", "estimate"), map[string]any{})
	e.must(404, "alice", "PUT", bindPath("task", "nope"), map[string]any{})

	// --- Item values -----------------------------------------------------------
	task := e.must(201, "carol", "POST", items, map[string]any{"title": "With fields", "fields": map[string]any{
		"estimate": 3, "reviewer": carolID, "tags": []string{tags["ui"], tags["api"], tags["ui"]}}})
	fields := task.Body["fields"].(map[string]any)
	wantTags := []any{tags["api"], tags["ui"]}
	if tags["ui"] < tags["api"] {
		wantTags = []any{tags["ui"], tags["api"]}
	}
	if fields["estimate"] != 3.0 || fields["reviewer"] != carolID || !reflect.DeepEqual(fields["tags"], wantTags) {
		t.Fatalf("stored fields not canonical: %v", fields)
	}

	cases := []struct {
		name, field string
		body        map[string]any
	}{
		{"unbound field", "fields.severity", map[string]any{"title": "x", "fields": map[string]any{"severity": choices["High"]}}},
		{"required missing", "fields.severity", map[string]any{"title": "x", "type": "bug"}},
		{"required blank", "fields.severity", map[string]any{"title": "x", "type": "bug", "fields": map[string]any{"severity": " "}}},
		{"non-member user", "fields.reviewer", map[string]any{"title": "x", "fields": map[string]any{"reviewer": daveID}}},
		{"wrong type", "fields.estimate", map[string]any{"title": "x", "fields": map[string]any{"estimate": "3"}}},
		{"unknown field", "fields.nope", map[string]any{"title": "x", "fields": map[string]any{"nope": 1}}},
	}
	for _, c := range cases {
		r := e.call("carol", "POST", items, c.body)
		if r.Status != 422 || errField(r) != c.field {
			t.Errorf("%s: %d field=%q, want 422 %q; %s", c.name, r.Status, errField(r), c.field, r.Raw)
		}
	}
	bug := e.must(201, "carol", "POST", items, map[string]any{"title": "Crash", "type": "bug",
		"fields": map[string]any{"severity": choices["High"]}})
	bugPath := fmt.Sprintf("%s/%v", items, bug.Body["number"])
	taskPath := fmt.Sprintf("%s/%v", items, task.Body["number"])

	// PATCH merges: null clears, absent keys are kept, required cannot be cleared.
	upd := e.must(200, "carol", "PATCH", taskPath, map[string]any{"version": task.Body["version"],
		"fields": map[string]any{"estimate": nil}})
	f := upd.Body["fields"].(map[string]any)
	if _, ok := f["estimate"]; ok || f["reviewer"] != carolID {
		t.Fatalf("merge wrong: %v", f)
	}
	if r := e.call("carol", "PATCH", bugPath, map[string]any{"version": bug.Body["version"],
		"fields": map[string]any{"severity": nil}}); r.Status != 422 || errField(r) != "fields.severity" {
		t.Fatalf("cleared a required field: %d %s", r.Status, r.Raw)
	}
	// Unchanged values do not bump the version.
	same := e.must(200, "carol", "PATCH", taskPath, map[string]any{"version": upd.Body["version"],
		"fields": map[string]any{"reviewer": carolID}})
	if same.Body["version"] != upd.Body["version"] {
		t.Fatalf("no-op field update bumped version: %v -> %v", upd.Body["version"], same.Body["version"])
	}

	var changes int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM events
		WHERE action = 'item.updated' AND payload->'changes' ? 'fields.estimate'`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if changes != 1 {
		t.Fatalf("field change events = %d, want 1", changes)
	}

	// --- Filters (index-backed containment) -------------------------------------
	count := func(query string) int {
		t.Helper()
		r := e.must(200, "bob", "GET", items+"?"+query, nil)
		return len(r.Body["items"].([]any))
	}
	if n := count("field.severity=" + choices["High"]); n != 1 {
		t.Fatalf("severity filter: %d", n)
	}
	if n := count("field.tags=" + tags["api"]); n != 1 {
		t.Fatalf("multi_select contains filter: %d", n)
	}
	if n := count("field.reviewer=" + carolID + "&field.tags=" + tags["ui"]); n != 1 {
		t.Fatalf("combined filter: %d", n)
	}
	if n := count("field.severity=" + choices["Low"]); n != 0 {
		t.Fatalf("low filter: %d", n)
	}
	for _, q := range []string{"field.nope=1", "field.estimate=abc", "field.severity=zzz"} {
		if r := e.call("bob", "GET", items+"?"+q, nil); r.Status != 422 {
			t.Errorf("%s: status %d, want 422; %s", q, r.Status, r.Raw)
		}
	}

	// --- Choice and field lifecycle ----------------------------------------------
	// Choices cannot be removed; archiving blocks new use but keeps filters working.
	if r := e.call("alice", "PATCH", fieldsPath+"/severity", map[string]any{"options": map[string]any{
		"choices": []map[string]any{{"id": choices["High"], "label": "High"}}}}); r.Status != 422 {
		t.Fatalf("choice removal: %d %s", r.Status, r.Raw)
	}
	e.must(200, "alice", "PATCH", fieldsPath+"/severity", map[string]any{"name": "Bug severity", "options": map[string]any{
		"choices": []map[string]any{{"id": choices["High"], "label": "Critical"}, {"id": choices["Low"], "label": "Low", "archived": true}}}})
	if r := e.call("carol", "POST", items, map[string]any{"title": "x", "type": "bug",
		"fields": map[string]any{"severity": choices["Low"]}}); r.Status != 422 {
		t.Fatalf("archived choice accepted: %d %s", r.Status, r.Raw)
	}
	// The relabel did not touch stored values.
	if got := e.must(200, "bob", "GET", bugPath, nil).Body["fields"].(map[string]any)["severity"]; got != choices["High"] {
		t.Fatalf("severity after relabel = %v", got)
	}

	// Archived fields: no new values, no longer required, value kept.
	e.must(200, "alice", "PATCH", fieldsPath+"/severity", map[string]any{"archived": true})
	e.must(201, "carol", "POST", items, map[string]any{"title": "No severity needed now", "type": "bug"})
	if r := e.call("carol", "POST", items, map[string]any{"title": "x", "type": "bug",
		"fields": map[string]any{"severity": choices["High"]}}); r.Status != 422 {
		t.Fatalf("archived field accepted: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "PUT", bindPath("task", "severity"), map[string]any{}); r.Status != 422 {
		t.Fatalf("bound an archived field: %d %s", r.Status, r.Raw)
	}

	// Unbinding keeps values but rejects new ones.
	e.must(204, "alice", "DELETE", bindPath("task", "tags"), nil)
	e.must(404, "alice", "DELETE", bindPath("task", "tags"), nil)
	if r := e.call("carol", "POST", items, map[string]any{"title": "x", "fields": map[string]any{"tags": []string{tags["ui"]}}}); r.Status != 422 {
		t.Fatalf("unbound field accepted: %d %s", r.Status, r.Raw)
	}
	if _, ok := e.must(200, "bob", "GET", taskPath, nil).Body["fields"].(map[string]any)["tags"]; !ok {
		t.Fatal("unbinding dropped stored values")
	}
}

func TestFieldKeyAndTypeImmutableInDatabase(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "estimate", "name": "Estimate", "type": "number"})
	for _, stmt := range []string{
		`UPDATE field_definitions SET key = 'renamed' WHERE key = 'estimate'`,
		`UPDATE field_definitions SET field_type = 'text' WHERE key = 'estimate'`,
	} {
		if _, err := e.pool.Exec(t.Context(), stmt); err == nil {
			t.Errorf("%s: succeeded, want trigger error", stmt)
		}
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE field_definitions SET name = 'Points' WHERE key = 'estimate'`); err != nil {
		t.Fatalf("renaming must stay allowed: %v", err)
	}
}
