package server

import (
	"fmt"
	"testing"
	"time"
)

// ADR-0065: category filter, new queue counts, per-project counter choice.
func TestCategoryFilterAndQueues(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	wf := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/workflows", nil).Body["workflows"].([]any)[0].(map[string]any)
	var done string
	for _, s := range wf["statuses"].([]any) {
		if st := s.(map[string]any); st["category"] == "done" && done == "" {
			done = st["id"].(string)
		}
	}
	thisMonth := time.Now().Format("2006-01") + "-15"
	e.must(201, "carol", "POST", items, map[string]any{"title": "Open"})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Due", "due_date": thisMonth})
	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Closed"}).Body
	e.must(200, "carol", "PATCH", items+"/"+fmt.Sprint(it["number"]), map[string]any{"version": it["version"], "status_id": done})

	count := func(q string) int {
		return len(e.must(200, "carol", "GET", items+"?"+q, nil).Body["items"].([]any))
	}
	if n := count("category=done"); n != 1 {
		t.Fatalf("category=done: %d", n)
	}
	if n := count("category=todo"); n != 2 {
		t.Fatalf("category=todo: %d", n)
	}
	if r := e.call("carol", "GET", items+"?category=nope", nil); r.Status != 422 && r.Status != 400 {
		t.Fatalf("bad category: %d", r.Status)
	}
	qc := e.must(200, "carol", "GET", items+"/queue-counts", nil).Body
	if qc["todo"] != 2.0 || qc["done"] != 1.0 || qc["month"] != 1.0 {
		t.Fatalf("queue counts: %v", qc)
	}

	views := "/api/v1/workspaces/acme/projects/ACME/views"
	if r := e.call("carol", "POST", views, map[string]any{"name": "x", "kind": "table", "config": map[string]any{"filters": map[string]any{"category": "nope"}}}); r.Status != 422 {
		t.Fatalf("view bad category: %d %s", r.Status, r.Raw)
	}
	v := e.must(201, "carol", "POST", views, map[string]any{"name": "Done", "kind": "table", "config": map[string]any{"filters": map[string]any{"category": "done"}}}).Body
	if v["config"].(map[string]any)["filters"].(map[string]any)["category"] != "done" {
		t.Fatalf("view category: %v", v["config"])
	}

	rules := "/api/v1/workspaces/acme/projects/ACME/rules"
	for _, bad := range [][]string{{"nope"}, {"all", "all"}} {
		if r := e.call("alice", "PUT", rules, map[string]any{"queues": bad}); r.Status != 422 {
			t.Errorf("queues %v: %d", bad, r.Status)
		}
	}
	got := e.must(200, "alice", "PUT", rules, map[string]any{"queues": []string{"all", "todo", "overdue", "done", "month"}}).Body
	if q := got["queues"].([]any); len(q) != 5 || q[1] != "todo" {
		t.Fatalf("queues: %v", got)
	}
}
