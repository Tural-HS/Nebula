package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"testing"
)

const (
	viewsPath = "/api/v1/workspaces/acme/projects/ACME/views"
	sumsPath  = items + "/sums"
)

// viewSetup adds number fields "monthly" and "remaining" and a select field
// "plan" (choices Plan 2027 / Other), all bound to the task type.
func (e *env) viewSetup() (plan map[string]string) {
	e.t.Helper()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "monthly", "name": "Aylıq ödəniş", "type": "number"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "remaining", "name": "Qalıq", "type": "number"})
	p := e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "plan", "name": "Plan", "type": "select",
		"options": map[string]any{"choices": []map[string]any{{"label": "Plan 2027"}, {"label": "Other"}}}})
	for i, k := range []string{"monthly", "remaining", "plan"} {
		e.must(200, "alice", "PUT", bindPath("task", k), map[string]any{"position": i + 1})
	}
	return choiceIDs(e.t, p)
}

func (e *env) statusIDs() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	for _, it := range e.must(200, "alice", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			m := s.(map[string]any)
			out[m["name"].(string)] = m["id"].(string)
		}
	}
	return out
}

func viewNames(r resp) []string {
	var out []string
	for _, v := range r.Body["views"].([]any) {
		out = append(out, v.(map[string]any)["name"].(string))
	}
	return out
}

func TestSavedViewsCRUDAndPermissions(t *testing.T) {
	e := newEnv(t)
	e.setup()
	plan := e.viewSetup()
	statuses := e.statusIDs()
	var anyStatus string
	for _, id := range statuses {
		anyStatus = id
		break
	}
	audit := func() string { return e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw }

	cfg := map[string]any{
		"filters": map[string]any{"q": "ödəniş", "assignee": "me", "due": "overdue", "status": []string{anyStatus},
			"fields": map[string]any{"plan": plan["Plan 2027"]}},
		"columns":  []string{"key", "title", "field.monthly", "field.remaining", "status"},
		"sort":     "due",
		"group_by": "category",
	}
	mine := e.must(201, "alice", "POST", viewsPath, map[string]any{"name": "  Gecikənlər  ", "kind": "table", "config": cfg}).Body
	if mine["name"] != "Gecikənlər" || mine["shared"] != false || mine["can_edit"] != true {
		t.Fatalf("created: %v", mine)
	}
	c := mine["config"].(map[string]any)
	if c["sort"] != "due" || len(c["columns"].([]any)) != 5 || c["filters"].(map[string]any)["fields"].(map[string]any)["plan"] != plan["Plan 2027"] {
		t.Fatalf("config round trip: %v", c)
	}
	mineID := mine["id"].(string)
	if contains(audit(), "view.created") {
		t.Fatal("personal views must not be audited")
	}

	// Every reader of the project may save personal views, guests included.
	carolView := e.must(201, "carol", "POST", viewsPath, map[string]any{"name": "Mənim", "config": map[string]any{}}).Body
	if carolView["kind"] != "table" || carolView["config"].(map[string]any)["sort"] != "number" {
		t.Fatalf("defaults: %v", carolView)
	}
	e.must(201, "bob", "POST", viewsPath, map[string]any{"name": "Qonaq", "kind": "board"})
	if r := e.call("dave", "POST", viewsPath, map[string]any{"name": "x"}); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}

	// Sharing is admin+.
	if r := e.call("carol", "POST", viewsPath, map[string]any{"name": "Hamı üçün", "shared": true}); r.Status != 403 {
		t.Fatalf("member creates shared view: %d", r.Status)
	}
	if r := e.call("carol", "PATCH", viewsPath+"/"+carolView["id"].(string), map[string]any{"shared": true}); r.Status != 403 {
		t.Fatalf("member shares own view: %d", r.Status)
	}
	shared := e.must(201, "alice", "POST", viewsPath, map[string]any{"name": "Plan 2027", "shared": true, "kind": "board",
		"config": map[string]any{"group_by": "status", "filters": map[string]any{"fields": map[string]any{"plan": plan["Plan 2027"]}}}}).Body
	sharedID := shared["id"].(string)
	if !contains(audit(), "view.created") {
		t.Fatal("shared view creation not audited")
	}

	// Visibility: own personal views plus shared ones, sorted by name.
	if got := viewNames(e.must(200, "carol", "GET", viewsPath, nil)); !slices.Equal(got, []string{"Mənim", "Plan 2027"}) {
		t.Fatalf("carol sees %v", got)
	}
	if got := viewNames(e.must(200, "alice", "GET", viewsPath, nil)); !slices.Equal(got, []string{"Gecikənlər", "Plan 2027"}) {
		t.Fatalf("alice sees %v", got)
	}
	// Someone else's personal view does not exist, even for the owner role.
	for _, m := range []string{"GET", "DELETE"} {
		if r := e.call("carol", m, viewsPath+"/"+mineID, nil); r.Status != 404 {
			t.Fatalf("carol %s alice's personal view: %d", m, r.Status)
		}
		if r := e.call("alice", m, viewsPath+"/"+carolView["id"].(string), nil); r.Status != 404 {
			t.Fatalf("alice %s carol's personal view: %d", m, r.Status)
		}
	}
	if got := e.must(200, "carol", "GET", viewsPath+"/"+sharedID, nil).Body; got["can_edit"] != false {
		t.Fatalf("member can_edit on shared: %v", got)
	}
	if r := e.call("carol", "PATCH", viewsPath+"/"+sharedID, map[string]any{"name": "Mine now"}); r.Status != 403 {
		t.Fatalf("member edits shared: %d", r.Status)
	}
	if r := e.call("carol", "DELETE", viewsPath+"/"+sharedID, nil); r.Status != 403 {
		t.Fatalf("member deletes shared: %d", r.Status)
	}
	upd := e.must(200, "alice", "PATCH", viewsPath+"/"+sharedID, map[string]any{"name": "Plan 2027 (aktiv)"}).Body
	if upd["name"] != "Plan 2027 (aktiv)" || upd["kind"] != "board" || upd["config"].(map[string]any)["filters"].(map[string]any)["fields"] == nil {
		t.Fatalf("patch kept other fields? %v", upd)
	}
	if !contains(audit(), "view.updated") {
		t.Fatal("shared view update not audited")
	}

	// The owner keeps control of their own shared view after losing admin.
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	e.must(200, "alice", "PATCH", wsBase+"/members/"+carolID, map[string]any{"role": "admin"})
	carolShared := e.must(201, "carol", "POST", viewsPath, map[string]any{"name": "Komanda", "shared": true}).Body["id"].(string)
	e.must(200, "alice", "PATCH", wsBase+"/members/"+carolID, map[string]any{"role": "member"})
	e.must(200, "carol", "PATCH", viewsPath+"/"+carolShared, map[string]any{"name": "Komanda 2"})
	e.must(200, "carol", "PATCH", viewsPath+"/"+carolShared, map[string]any{"shared": false})
	if r := e.call("carol", "PATCH", viewsPath+"/"+carolShared, map[string]any{"shared": true}); r.Status != 403 {
		t.Fatalf("re-sharing as member: %d", r.Status)
	}
	// Admins may edit others' shared views, but an unshared one is private again.
	if r := e.call("alice", "GET", viewsPath+"/"+carolShared, nil); r.Status != 404 {
		t.Fatalf("unshared view still visible: %d", r.Status)
	}

	e.must(204, "alice", "DELETE", viewsPath+"/"+sharedID, nil)
	if !contains(audit(), "view.deleted") {
		t.Fatal("shared view deletion not audited")
	}
	e.must(404, "alice", "GET", viewsPath+"/"+sharedID, nil)
	e.must(404, "alice", "GET", viewsPath+"/not-a-uuid", nil)
}

func TestSavedViewsValidation(t *testing.T) {
	e := newEnv(t)
	e.setup()
	plan := e.viewSetup()
	daveID := e.must(200, "dave", "GET", "/api/v1/me", nil).Body["id"].(string)
	many := make([]string, 31)
	for i := range many {
		many[i] = fmt.Sprintf("field.f%d", i)
	}

	bad422 := map[string]map[string]any{
		"no name":            {"kind": "table"},
		"blank name":         {"name": "   "},
		"long name":          {"name": strings.Repeat("ə", 101)},
		"null name":          {"name": nil},
		"kind":               {"name": "x", "kind": "list"},
		"sort":               {"name": "x", "config": map[string]any{"sort": "titel"}},
		"group":              {"name": "x", "config": map[string]any{"group_by": "assignee"}},
		"due":                {"name": "x", "config": map[string]any{"filters": map[string]any{"due": "yesterday"}}},
		"assignee":           {"name": "x", "config": map[string]any{"filters": map[string]any{"assignee": "someone"}}},
		"non-member":         {"name": "x", "config": map[string]any{"filters": map[string]any{"assignee": daveID}}},
		"unknown status":     {"name": "x", "config": map[string]any{"filters": map[string]any{"status": []string{"01a20727-0000-7000-8000-000000000000"}}}},
		"unknown field":      {"name": "x", "config": map[string]any{"filters": map[string]any{"fields": map[string]any{"nope": "1"}}}},
		"bad choice":         {"name": "x", "config": map[string]any{"filters": map[string]any{"fields": map[string]any{"plan": "zzz"}}}},
		"bad number":         {"name": "x", "config": map[string]any{"filters": map[string]any{"fields": map[string]any{"monthly": "abc"}}}},
		"unknown column":     {"name": "x", "config": map[string]any{"columns": []string{"key", "salary"}}},
		"unknown field col":  {"name": "x", "config": map[string]any{"columns": []string{"field.nope"}}},
		"duplicate column":   {"name": "x", "config": map[string]any{"columns": []string{"title", "title"}}},
		"too many columns":   {"name": "x", "config": map[string]any{"columns": many}},
		"long query":         {"name": "x", "config": map[string]any{"filters": map[string]any{"q": strings.Repeat("a", 501)}}},
		"select filter type": {"name": "x", "config": map[string]any{"filters": map[string]any{"fields": map[string]any{"plan": plan["Other"] + "x"}}}},
	}
	for name, body := range bad422 {
		if r := e.call("carol", "POST", viewsPath, body); r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	for name, body := range map[string]any{
		"unknown key":        map[string]any{"name": "x", "colour": "red"},
		"unknown config key": map[string]any{"name": "x", "config": map[string]any{"colums": []string{"title"}}},
		"unknown filter key": map[string]any{"name": "x", "config": map[string]any{"filters": map[string]any{"owner": "me"}}},
		"wrong type":         map[string]any{"name": "x", "config": map[string]any{"columns": "title"}},
	} {
		if r := e.call("carol", "POST", viewsPath, body); r.Status != 400 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	// Archived fields cannot become new columns.
	e.must(200, "alice", "PATCH", fieldsPath+"/remaining", map[string]any{"archived": true})
	if r := e.call("carol", "POST", viewsPath, map[string]any{"name": "x", "config": map[string]any{"columns": []string{"field.remaining"}}}); r.Status != 422 {
		t.Errorf("archived column: %d", r.Status)
	}

	// At most 100 views per user and project.
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO saved_views (id, workspace_id, project_id, owner_id, name, kind)
		SELECT gen_random_uuid(), p.workspace_id, p.id, $1, 'v' || g, 'table' FROM projects p, generate_series(1, 99) g WHERE p.key = 'ACME'`, carolID); err != nil {
		t.Fatal(err)
	}
	e.must(201, "carol", "POST", viewsPath, map[string]any{"name": "100th"})
	if r := e.call("carol", "POST", viewsPath, map[string]any{"name": "101st"}); r.Status != 409 || errCode(r) != "view_limit" {
		t.Fatalf("limit: %d %s", r.Status, r.Raw)
	}
	e.must(201, "alice", "POST", viewsPath, map[string]any{"name": "others are not limited by carol"})
}

func TestSavedViewsTenantIsolation(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.viewSetup()
	ctx := context.Background()
	// dave owns a second workspace with its own project and statuses.
	e.must(201, "dave", "POST", "/api/v1/workspaces", map[string]any{"slug": "beta", "name": "Beta"})
	e.must(201, "dave", "POST", "/api/v1/workspaces/beta/projects", map[string]any{"key": "ACME", "name": "Same key"})
	var betaStatus string
	for _, it := range e.must(200, "dave", "GET", "/api/v1/workspaces/beta/item-types", nil).Body["item_types"].([]any) {
		betaStatus = it.(map[string]any)["statuses"].([]any)[0].(map[string]any)["id"].(string)
	}
	acme := e.must(201, "alice", "POST", viewsPath, map[string]any{"name": "Shared", "shared": true}).Body["id"].(string)

	// Same project key, other workspace: the view is not there.
	betaViews := "/api/v1/workspaces/beta/projects/ACME/views"
	e.must(404, "dave", "GET", betaViews+"/"+acme, nil)
	e.must(404, "dave", "PATCH", betaViews+"/"+acme, map[string]any{"name": "hijack"})
	e.must(404, "dave", "DELETE", betaViews+"/"+acme, nil)
	if got := e.must(200, "dave", "GET", betaViews, nil).Body["views"].([]any); len(got) != 0 {
		t.Fatalf("beta lists acme views: %v", got)
	}
	// Statuses and fields of another workspace are unknown here.
	if r := e.call("alice", "POST", viewsPath, map[string]any{"name": "x", "config": map[string]any{"filters": map[string]any{"status": []string{betaStatus}}}}); r.Status != 422 {
		t.Fatalf("foreign status: %d", r.Status)
	}
	e.must(201, "dave", "POST", "/api/v1/workspaces/beta/fields", map[string]any{"key": "betaonly", "name": "B", "type": "number"})
	if r := e.call("alice", "POST", viewsPath, map[string]any{"name": "x", "config": map[string]any{"columns": []string{"field.betaonly"}}}); r.Status != 422 {
		t.Fatalf("foreign field column: %d", r.Status)
	}
	if r := e.call("alice", "GET", sumsPath+"?fields=betaonly", nil); r.Status != 422 {
		t.Fatalf("foreign field sum: %d", r.Status)
	}
	// The composite foreign key rejects a view pointing at another tenant's project.
	_, err := e.pool.Exec(ctx, `INSERT INTO saved_views (id, workspace_id, project_id, owner_id, name, kind)
		SELECT gen_random_uuid(), a.id, p.id, u.id, 'x', 'table'
		FROM workspaces a, projects p JOIN workspaces b ON b.id = p.workspace_id, users u
		WHERE a.slug = 'acme' AND b.slug = 'beta' AND u.email = 'alice@example.com'`)
	if err == nil || !strings.Contains(err.Error(), "foreign key") {
		t.Fatalf("cross-tenant view inserted: %v", err)
	}
}

func TestItemsSortByDuePagination(t *testing.T) {
	e := newEnv(t)
	e.setup()
	// Ties (same due date) and undated items; numbers 1..9 in creation order.
	dues := []any{"2026-12-01", nil, "2026-11-30", "2026-12-01", nil, "2026-11-30", "2026-12-05", "2026-12-01", nil}
	for i, d := range dues {
		e.must(201, "carol", "POST", items, map[string]any{"title": fmt.Sprintf("Item %d", i+1), "due_date": d})
	}
	walk := func(query string, during func(page int)) []float64 {
		t.Helper()
		var seen []float64
		cursor := ""
		for page := 0; page < 30; page++ {
			path := items + "?sort=due&limit=2" + query
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			r := e.must(200, "bob", "GET", path, nil)
			for _, it := range r.Body["items"].([]any) {
				seen = append(seen, it.(map[string]any)["number"].(float64))
			}
			if during != nil {
				during(page)
			}
			next, _ := r.Body["next_cursor"].(string)
			if next == "" {
				break
			}
			cursor = next
		}
		return seen
	}
	// 11-30: 3, 6 · 12-01: 1, 4, 8 · 12-05: 7 · undated: 2, 5, 9
	want := []float64{3, 6, 1, 4, 8, 7, 2, 5, 9}
	if got := walk("", nil); !slices.Equal(got, want) {
		t.Fatalf("due order = %v, want %v", got, want)
	}

	// Items created while paging: one sorting after the cursor shows up once,
	// one sorting before it does not (and nothing repeats or goes missing).
	got := walk("", func(page int) {
		if page == 1 { // after 3, 6, 1, 4
			e.must(201, "carol", "POST", items, map[string]any{"title": "Later", "due_date": "2026-12-02"}) // 10
			e.must(201, "carol", "POST", items, map[string]any{"title": "Earlier", "due_date": "2026-11-01"})
			e.must(201, "carol", "POST", items, map[string]any{"title": "Undated"}) // 12
		}
	})
	if want := []float64{3, 6, 1, 4, 8, 10, 7, 2, 5, 9, 12}; !slices.Equal(got, want) {
		t.Fatalf("with concurrent inserts = %v, want %v", got, want)
	}
	// Filters apply together with the sort.
	if got := walk("&due=none", nil); !slices.Equal(got, []float64{2, 5, 9, 12}) {
		t.Fatalf("undated only = %v", got)
	}

	// Cursors are bound to their sort.
	numCursor := e.must(200, "bob", "GET", items+"?limit=1", nil).Body["next_cursor"].(string)
	dueCursor := e.must(200, "bob", "GET", items+"?sort=due&limit=1", nil).Body["next_cursor"].(string)
	for _, bad := range []string{
		"sort=due&cursor=" + numCursor,
		"cursor=" + dueCursor,
		"sort=number&cursor=" + dueCursor,
		"sort=titel",
		"sort=due&cursor=" + b64("d1:2026-13-01:3"),
		"sort=due&cursor=" + b64("d1:2026-12-01:0"),
		"sort=due&cursor=" + b64("d1:2026-12-01"),
		"sort=due&cursor=" + b64("d1:x:3"),
		"cursor=" + b64("v1:3:4"),
	} {
		if r := e.call("bob", "GET", items+"?"+bad, nil); r.Status != 400 {
			t.Errorf("%s: %d %s", bad, r.Status, r.Raw)
		}
	}
	// A hand-made but valid undated cursor works.
	r := e.must(200, "bob", "GET", items+"?sort=due&cursor="+b64("d1::5"), nil)
	if n := len(r.Body["items"].([]any)); n != 2 { // 9, 12
		t.Fatalf("after undated 5: %d items", n)
	}
}

func TestItemSums(t *testing.T) {
	e := newEnv(t)
	e.setup()
	plan := e.viewSetup()
	ctx := context.Background()
	statuses := e.statusIDs()
	create := func(title string, fields map[string]any) {
		e.must(201, "carol", "POST", items, map[string]any{"title": title, "fields": fields})
	}
	create("Lizinq ödəniş", map[string]any{"monthly": 100.5, "remaining": 1000, "plan": plan["Plan 2027"]})
	create("Əliyev ödəniş", map[string]any{"monthly": 200, "plan": plan["Plan 2027"]})
	create("Kiçik", map[string]any{"monthly": 0.25, "plan": plan["Other"]})
	create("Boş", map[string]any{})
	create("Pis data", map[string]any{"remaining": 5})
	// Non-numeric data that bypassed validation (old type, manual SQL) is ignored.
	if _, err := e.pool.Exec(ctx, `UPDATE items SET fields = fields || '{"monthly": "abc"}' WHERE title = 'Pis data'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE items SET fields = fields || '{"remaining": {"x": 1}}' WHERE title = 'Boş'`); err != nil {
		t.Fatal(err)
	}

	sums := func(query string) map[string]any {
		t.Helper()
		return e.must(200, "bob", "GET", sumsPath+"?"+query, nil).Body["sums"].(map[string]any)
	}
	check := func(got map[string]any, key, sum string, count float64) {
		t.Helper()
		f, _ := got[key].(map[string]any)
		if f["sum"] != sum || f["count"] != count {
			t.Errorf("%s = %v, want sum %s count %v", key, f, sum, count)
		}
	}
	all := sums("fields=monthly,remaining")
	check(all, "monthly", "300.75", 3)
	check(all, "remaining", "1005", 2)
	// The same filters as the list: field, search, status, assignee.
	check(sums("fields=monthly&field.plan="+plan["Plan 2027"]), "monthly", "300.5", 2)
	check(sums("fields=monthly&q=ödəniş"), "monthly", "300.5", 2)
	check(sums("fields=monthly&status="+statuses["Done"]), "monthly", "0", 0)
	check(sums("fields=monthly&assignee=none&limit=1"), "monthly", "300.75", 3) // limit does not cut the total

	// Exact decimal arithmetic: float64 would lose the .1.
	create("Böyük", map[string]any{"monthly": 1e15})
	create("Böyük 2", map[string]any{"monthly": 1e15})
	create("Kəsr", map[string]any{"monthly": 0.1})
	check(sums("fields=monthly&q=Böyük%20OR%20Kəsr"), "monthly", "2000000000000000.1", 3)

	// The footer count: every matching item, with or without amount fields.
	listed := len(e.must(200, "bob", "GET", items+"?limit=200&q=Böyük%20OR%20Kəsr", nil).Body["items"].([]any))
	for _, q := range []string{"q=Böyük%20OR%20Kəsr", "fields=monthly&q=Böyük%20OR%20Kəsr"} {
		if got := e.must(200, "bob", "GET", sumsPath+"?"+q, nil).Body["count"]; got != float64(listed) || listed != 3 {
			t.Errorf("%s: count %v, listed %d", q, got, listed)
		}
	}

	for q, want := range map[string]int{
		"fields=plan":                  422, // not a number field
		"fields=nope":                  422,
		"fields=a,b,c,d,e,f,g,h,i,j,k": 400,
		"fields=monthly&due=bad":       422,
		"fields=monthly&sort=x":        400,
	} {
		if r := e.call("bob", "GET", sumsPath+"?"+q, nil); r.Status != want {
			t.Errorf("%q: %d, want %d (%s)", q, r.Status, want, r.Raw)
		}
	}
	e.must(404, "dave", "GET", sumsPath+"?fields=monthly", nil)
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
