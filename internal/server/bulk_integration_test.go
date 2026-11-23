package server

import (
	"strings"
	"testing"
)

const bulkPath = items + "/bulk"

func targets(pairs ...int) []map[string]int {
	out := []map[string]int{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]int{"number": pairs[i], "version": pairs[i+1]})
	}
	return out
}

func bulkErrors(r resp) map[float64]string {
	out := map[float64]string{}
	e, _ := r.Body["error"].(map[string]any)
	list, _ := e["items"].([]any)
	for _, it := range list {
		m := it.(map[string]any)
		out[m["number"].(float64)] = m["code"].(string)
	}
	return out
}

func (e *env) version(n string) float64 {
	e.t.Helper()
	return e.must(200, "carol", "GET", items+"/"+n, nil).Body["version"].(float64)
}

func TestBulkItems(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for _, title := range []string{"One", "Two", "Three", "Four", "Five"} {
		e.must(201, "carol", "POST", items, map[string]any{"title": title})
	}
	wf := e.must(200, "alice", "GET", wsBase+"/workflows", nil).Body["workflows"].([]any)[0].(map[string]any)
	todo, inProgress, done := statusID(t, wf, "To Do"), statusID(t, wf, "In Progress"), statusID(t, wf, "Done")
	daveID := e.must(200, "dave", "GET", "/api/v1/me", nil).Body["id"].(string)
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)

	t.Run("validation", func(t *testing.T) {
		many := make([]int, 0, 1002)
		for i := 1; i <= 501; i++ {
			many = append(many, i, 1)
		}
		cases := map[string]map[string]any{
			"empty":        {"items": targets(), "status_id": inProgress},
			"too many":     {"items": targets(many...), "status_id": inProgress},
			"duplicate":    {"items": targets(1, 1, 1, 1), "status_id": inProgress},
			"no version":   {"items": []map[string]int{{"number": 1}}, "status_id": inProgress},
			"no change":    {"items": targets(1, 1)},
			"delete+edit":  {"items": targets(1, 1), "status_id": inProgress, "delete": true},
			"null status":  {"items": targets(1, 1), "status_id": nil},
			"bad due date": {"items": targets(1, 1), "due_date": "2026-02-30"},
		}
		for name, body := range cases {
			if r := e.call("alice", "POST", bulkPath, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
		if r := e.call("alice", "POST", bulkPath, map[string]any{"items": targets(1, 1), "evil": 1}); r.Status != 400 {
			t.Errorf("unknown key: %d", r.Status)
		}
	})

	t.Run("roles", func(t *testing.T) {
		if r := e.call("bob", "POST", bulkPath, map[string]any{"items": targets(1, 1), "status_id": inProgress}); r.Status != 403 {
			t.Errorf("guest bulk edit: %d", r.Status)
		}
		if r := e.call("carol", "POST", bulkPath, map[string]any{"items": targets(1, 1), "delete": true}); r.Status != 403 {
			t.Errorf("member bulk delete: %d", r.Status)
		}
		if r := e.call("dave", "POST", bulkPath, map[string]any{"items": targets(1, 1), "status_id": inProgress}); r.Status != 404 {
			t.Errorf("outsider bulk edit: %d", r.Status)
		}
	})

	// One change, three items, one transaction, one event per item.
	body := map[string]any{"items": targets(1, 1, 2, 1, 3, 1), "status_id": inProgress, "assignee_id": carolID, "due_date": "2026-12-15"}
	res := e.must(200, "carol", "POST", bulkPath, body).Body
	if res["changed"] != 3.0 || res["unchanged"] != 0.0 || len(res["items"].([]any)) != 3 || res["bulk_id"] == "" {
		t.Fatalf("bulk result: %v", res)
	}
	for _, it := range res["items"].([]any) {
		m := it.(map[string]any)
		if m["version"] != 2.0 || m["status"].(map[string]any)["id"] != inProgress || m["assignee_id"] != carolID || m["due_date"] != "2026-12-15" {
			t.Fatalf("updated item: %v", m)
		}
	}
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100&action=item.updated", nil).Raw
	if n := strings.Count(audit, res["bulk_id"].(string)); n != 3 {
		t.Fatalf("want 3 item.updated events tagged with the bulk id, got %d: %s", n, audit)
	}
	if act := e.must(200, "carol", "GET", items+"/2/activity", nil).Raw; !contains(act, "status_id") || !contains(act, "due_date") {
		t.Fatalf("per-item activity: %s", act)
	}

	t.Run("retry after commit is harmless", func(t *testing.T) {
		r := e.call("carol", "POST", bulkPath, body)
		if r.Status != 409 || errCode(r) != "bulk_failed" || len(bulkErrors(r)) != 3 {
			t.Fatalf("replayed request: %d %s", r.Status, r.Raw)
		}
		if e.version("1") != 2 {
			t.Fatal("replay changed an item")
		}
	})

	t.Run("all or nothing", func(t *testing.T) {
		// Item 4 is current, item 1 is stale, 99 does not exist: nothing changes.
		r := e.call("carol", "POST", bulkPath, map[string]any{"items": targets(4, 1, 1, 1, 99, 1), "status_id": done})
		errs := bulkErrors(r)
		if r.Status != 409 || errs[1] != "version_conflict" || errs[99] != "not_found" || len(errs) != 2 {
			t.Fatalf("partial failure: %d %s", r.Status, r.Raw)
		}
		if e.version("4") != 1 {
			t.Fatal("item 4 changed although the bulk change failed")
		}
		// A non-member assignee fails every item with a validation error (422).
		r = e.call("carol", "POST", bulkPath, map[string]any{"items": targets(4, 1, 5, 1), "assignee_id": daveID})
		if r.Status != 422 || bulkErrors(r)[4] != "validation_failed" {
			t.Fatalf("bad assignee: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("workflow transitions apply per item", func(t *testing.T) {
		path := wsBase + "/workflows/" + wf["id"].(string)
		e.must(201, "alice", "POST", path+"/transitions", map[string]any{"name": "Start", "from_status_id": todo, "to_status_id": inProgress})
		e.must(201, "alice", "POST", path+"/transitions", map[string]any{"name": "Finish", "from_status_id": inProgress, "to_status_id": done})
		e.must(200, "alice", "PATCH", path, map[string]any{"enforce_transitions": true})
		// 1 is In Progress (may finish), 4 is To Do (may not): all rolled back.
		r := e.call("carol", "POST", bulkPath, map[string]any{"items": targets(1, 2, 4, 1), "status_id": done})
		if r.Status != 422 || len(bulkErrors(r)) != 1 || bulkErrors(r)[4] != "validation_failed" {
			t.Fatalf("transition rules: %d %s", r.Status, r.Raw)
		}
		if e.version("1") != 2 {
			t.Fatal("item 1 changed although item 4 failed")
		}
		e.must(200, "carol", "POST", bulkPath, map[string]any{"items": targets(1, 2, 2, 2), "status_id": done})
		e.must(200, "alice", "PATCH", path, map[string]any{"enforce_transitions": false})
	})

	t.Run("unchanged items write no event", func(t *testing.T) {
		res := e.must(200, "carol", "POST", bulkPath, map[string]any{"items": targets(1, 3, 4, 1), "due_date": "2026-12-15"}).Body
		if res["changed"] != 1.0 || res["unchanged"] != 1.0 {
			t.Fatalf("unchanged count: %v", res)
		}
		if e.version("1") != 3 || e.version("4") != 2 {
			t.Fatalf("versions: 1=%v 4=%v", e.version("1"), e.version("4"))
		}
	})

	t.Run("field values", func(t *testing.T) {
		e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "points", "name": "Points", "type": "number"})
		e.must(200, "alice", "PUT", bindPath("task", "points"), map[string]any{"position": 1})
		res := e.must(200, "carol", "POST", bulkPath, map[string]any{"items": targets(4, 2, 5, 1), "fields": map[string]any{"points": 8}}).Body
		for _, it := range res["items"].([]any) {
			if it.(map[string]any)["fields"].(map[string]any)["points"] != 8.0 {
				t.Fatalf("field not set: %v", it)
			}
		}
		if r := e.call("carol", "POST", bulkPath, map[string]any{"items": targets(4, 3), "fields": map[string]any{"points": "many"}}); r.Status != 422 {
			t.Fatalf("bad field value: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("admin bulk delete", func(t *testing.T) {
		r := e.call("alice", "POST", bulkPath, map[string]any{"items": targets(4, 1, 5, 2), "delete": true})
		if r.Status != 409 || bulkErrors(r)[4] != "version_conflict" {
			t.Fatalf("stale delete: %d %s", r.Status, r.Raw)
		}
		e.must(200, "carol", "GET", items+"/5", nil)
		res := e.must(200, "alice", "POST", bulkPath, map[string]any{"items": targets(4, 3, 5, 2), "delete": true}).Body
		if res["changed"] != 2.0 || len(res["items"].([]any)) != 0 {
			t.Fatalf("delete result: %v", res)
		}
		e.must(404, "carol", "GET", items+"/4", nil)
		e.must(404, "carol", "GET", items+"/5", nil)
		if a := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100&action=item.deleted", nil).Raw; strings.Count(a, res["bulk_id"].(string)) != 2 {
			t.Fatalf("delete events: %s", a)
		}
	})
}

func TestDueRangeFilter(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for _, due := range []any{"2026-11-30", "2026-12-01", "2026-12-31", "2027-01-01", nil} {
		e.must(201, "carol", "POST", items, map[string]any{"title": "Item", "due_date": due})
	}
	numbers := func(q string) []float64 {
		var out []float64
		for _, it := range e.must(200, "bob", "GET", items+"?"+q, nil).Body["items"].([]any) {
			out = append(out, it.(map[string]any)["number"].(float64))
		}
		return out
	}
	if got := numbers("sort=due&due_from=2026-12-01&due_to=2026-12-31"); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("december: %v", got)
	}
	if got := numbers("due_from=2026-12-31"); len(got) != 2 {
		t.Fatalf("open end: %v", got)
	}
	if got := numbers("due_to=2026-11-30"); len(got) != 1 || got[0] != 1 {
		t.Fatalf("open start: %v", got)
	}
	if got := numbers("due_from=2026-12-01&due_to=2026-12-01"); len(got) != 1 {
		t.Fatalf("single day: %v", got)
	}
	if c := e.must(200, "bob", "GET", items+"/counts?due_from=2026-12-01&due_to=2027-01-31", nil).Body; c["total"] != 3.0 {
		t.Fatalf("counts with range: %v", c)
	}
	for q, want := range map[string]int{
		"due_from=2026-13-01":                     422,
		"due_to=yesterday":                        422,
		"due_from=2026-12-02&due_to=2026-12-01":   422,
		"due_from=2026-01-01&due_to=2027-01-02":   422, // 367 days
		"due_from=2026-01-01&due_from=2026-02-01": 400,
	} {
		if r := e.call("bob", "GET", items+"?"+q, nil); r.Status != want {
			t.Errorf("%s: %d %s", q, r.Status, r.Raw)
		}
	}
	e.must(200, "bob", "GET", items+"?due_from=2026-01-01&due_to=2027-01-01", nil) // 366 days
}

func TestCalendarSavedView(t *testing.T) {
	e := newEnv(t)
	e.setup()
	v := e.must(201, "carol", "POST", viewsPath, map[string]any{"name": "Bu ay", "kind": "calendar",
		"config": map[string]any{"filters": map[string]any{"assignee": "me"}}}).Body
	if v["kind"] != "calendar" {
		t.Fatalf("kind: %v", v)
	}
	id := v["id"].(string)
	if got := e.must(200, "carol", "PATCH", viewsPath+"/"+id, map[string]any{"kind": "board"}).Body; got["kind"] != "board" {
		t.Fatalf("patched: %v", got)
	}
	if r := e.call("carol", "POST", viewsPath, map[string]any{"name": "x", "kind": "gantt"}); r.Status != 422 {
		t.Fatalf("unknown kind: %d %s", r.Status, r.Raw)
	}
}
