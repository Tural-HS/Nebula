package server

import (
	"fmt"
	"testing"
)

// ADR-0061: complete fields gate moves into and out of done; late fields
// are stored for the table.
func TestProjectRules(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	for _, k := range []string{"paid", "portal", "poa"} {
		e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": k, "name": k, "type": "boolean"})
		e.must(200, "alice", "PUT", bindPath("task", k), map[string]any{})
	}
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "balance", "name": "Balance", "type": "number"})
	rules := "/api/v1/workspaces/acme/projects/ACME/rules"

	for _, bad := range []map[string]any{
		{"complete_fields": []string{"balance"}}, {"complete_fields": []string{"nope"}},
		{"complete_fields": []string{"paid", "paid"}}, {"late_fields": []string{"nope"}},
	} {
		if r := e.call("alice", "PUT", rules, bad); r.Status != 422 {
			t.Errorf("%v: %d %s", bad, r.Status, r.Raw)
		}
	}
	if r := e.call("carol", "PUT", rules, map[string]any{"complete_fields": []string{"paid"}}); r.Status != 403 {
		t.Fatalf("member saved rules: %d", r.Status)
	}
	got := e.must(200, "alice", "PUT", rules, map[string]any{"complete_fields": []string{"paid", "portal", "poa"}, "late_fields": []string{"balance"}}).Body
	if len(got["complete_fields"].([]any)) != 3 || len(got["late_fields"].([]any)) != 1 {
		t.Fatalf("rules: %v", got)
	}
	if r := e.must(200, "carol", "GET", rules, nil).Body; len(r["late_fields"].([]any)) != 1 {
		t.Fatalf("member read: %v", r)
	}

	wf := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/workflows", nil).Body["workflows"].([]any)[0].(map[string]any)
	var todo, done, canceled string
	for _, s := range wf["statuses"].([]any) {
		st := s.(map[string]any)
		switch {
		case st["category"] == "todo" && todo == "":
			todo = st["id"].(string)
		case st["category"] == "done" && done == "":
			done = st["id"].(string)
		case st["category"] == "canceled" && canceled == "":
			canceled = st["id"].(string)
		}
	}
	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Customer", "fields": map[string]any{"paid": true}}).Body
	n := fmt.Sprint(it["number"])
	path := items + "/" + n

	// Not all ticked: cannot complete (anyone), can still cancel.
	for _, who := range []string{"carol", "alice"} {
		r := e.call(who, "PATCH", path, map[string]any{"version": it["version"], "status_id": done})
		if r.Status != 409 || errCode(r) != "complete_requires" {
			t.Fatalf("%s completed without all fields: %d %s", who, r.Status, r.Raw)
		}
	}
	// Ticking the rest in the same change completes it.
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": it["version"], "status_id": done,
		"fields": map[string]any{"portal": true, "poa": true}}).Body

	// All ticked: nobody reopens.
	for _, who := range []string{"carol", "alice"} {
		r := e.call(who, "PATCH", path, map[string]any{"version": it["version"], "status_id": todo})
		if r.Status != 409 || errCode(r) != "close_lock" {
			t.Fatalf("%s reopened a locked item: %d %s", who, r.Status, r.Raw)
		}
	}
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": it["version"], "title": "Customer A"}).Body
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": it["version"], "status_id": todo,
		"fields": map[string]any{"portal": false}}).Body
	if it["status"].(map[string]any)["id"] != todo {
		t.Fatalf("not reopened: %v", it["status"])
	}
	if canceled != "" {
		e.must(200, "carol", "PATCH", path, map[string]any{"version": it["version"], "status_id": canceled})
	}

	// Off: no rules.
	e.must(200, "alice", "PUT", rules, map[string]any{"complete_fields": []string{}, "late_fields": []string{}})
	if r := e.must(200, "carol", "GET", rules, nil).Body; len(r["complete_fields"].([]any)) != 0 {
		t.Fatalf("rules not cleared: %v", r)
	}
}
