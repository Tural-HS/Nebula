package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const wsBase = "/api/v1/workspaces/acme"

func TestWorkspaceSettings(t *testing.T) {
	e := newEnv(t)
	e.setup()

	for _, who := range []string{"bob", "carol"} { // guest, member
		if r := e.call(who, "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku"}); r.Status != 403 {
			t.Fatalf("%s changes settings: %d", who, r.Status)
		}
	}
	for _, bad := range []map[string]any{
		{"timezone": "Mars/Olympus"}, {"timezone": "Local"}, {"timezone": ""}, {"timezone": nil},
		{"overdue_digest_hour": 24}, {"overdue_digest_hour": -1}, {"name": ""},
	} {
		if r := e.call("alice", "PATCH", wsBase, bad); r.Status != 422 {
			t.Errorf("%v: %d %s", bad, r.Status, r.Raw)
		}
	}
	ws := e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku", "overdue_digest_hour": 9}).Body
	if ws["timezone"] != "Asia/Baku" || ws["overdue_digest_hour"] != 9.0 || ws["role"] != "owner" {
		t.Fatalf("settings not applied: %v", ws)
	}
	if ws = e.must(200, "alice", "PATCH", wsBase, map[string]any{"overdue_digest_hour": nil}).Body; ws["overdue_digest_hour"] != nil {
		t.Fatalf("digest not turned off: %v", ws)
	}
	// Changes are audited.
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw
	if !json.Valid([]byte(audit)) || !contains(audit, "workspace.updated") {
		t.Fatalf("audit log lacks workspace.updated: %s", audit)
	}
}

func TestDueDatesFilterAndOverdueFlag(t *testing.T) {
	e := newEnv(t)
	e.setup()

	past := e.must(201, "carol", "POST", items, map[string]any{"title": "Late", "due_date": "2020-01-15"}).Body
	if past["due_date"] != "2020-01-15" || past["overdue"] != true {
		t.Fatalf("past due item: %v", past)
	}
	future := e.must(201, "carol", "POST", items, map[string]any{"title": "Later", "due_date": "2199-12-31"}).Body
	if future["overdue"] != false {
		t.Fatalf("future item overdue: %v", future)
	}
	e.must(201, "carol", "POST", items, map[string]any{"title": "No date"})
	for _, bad := range []string{"15.01.2020", "2020-02-30", "1800-01-01", "tomorrow"} {
		if r := e.call("carol", "POST", items, map[string]any{"title": "x", "due_date": bad}); r.Status != 422 {
			t.Errorf("due_date %q: %d", bad, r.Status)
		}
	}

	count := func(q string) int {
		t.Helper()
		return len(e.must(200, "bob", "GET", items+"?"+q, nil).Body["items"].([]any))
	}
	if n := count("due=overdue"); n != 1 {
		t.Errorf("due=overdue: %d", n)
	}
	if n := count("due=none"); n != 1 {
		t.Errorf("due=none: %d", n)
	}
	if n := count("due=today"); n != 0 {
		t.Errorf("due=today: %d", n)
	}
	if r := e.call("bob", "GET", items+"?due=soon", nil); r.Status != 422 && r.Status != 400 {
		t.Errorf("due=soon: %d", r.Status)
	}
	if c := e.must(200, "bob", "GET", items+"/counts?due=overdue", nil).Body["total"]; c != 1.0 {
		t.Errorf("counts due=overdue: %v", c)
	}

	// Edit, then clear: the change is in the activity history.
	upd := e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "due_date": "2199-01-01"}).Body
	if upd["due_date"] != "2199-01-01" || upd["overdue"] != false {
		t.Fatalf("update: %v", upd)
	}
	upd = e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 2, "due_date": nil}).Body
	if upd["due_date"] != nil {
		t.Fatalf("clear: %v", upd)
	}
	if act := e.must(200, "carol", "GET", items+"/1/activity", nil).Raw; !contains(act, "due_date") {
		t.Fatalf("activity lacks due_date change: %s", act)
	}

	// Done items are never overdue.
	wf := e.must(200, "alice", "GET", wsBase+"/workflows", nil).Body["workflows"].([]any)[0].(map[string]any)
	done := statusID(t, wf, "Done")
	e.must(200, "alice", "PATCH", wsBase+"/workflows/"+wf["id"].(string), map[string]any{"enforce_transitions": false})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Finished late", "due_date": "2020-01-01"})
	if it := e.must(200, "carol", "PATCH", items+"/4", map[string]any{"version": 1, "status_id": done}).Body; it["overdue"] != false {
		t.Fatalf("done item overdue: %v", it)
	}
}

func TestOverdueScanTriggersRulesWebhooksAndDigest(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	bobID := e.must(200, "bob", "GET", "/api/v1/me", nil).Body["id"].(string)
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)

	rc := newReceiver(t, 200)
	e.must(201, "alice", "POST", wsBase+"/webhooks", map[string]any{
		"url": rc.srv.URL + "/hook", "events": []string{"item.overdue", "workspace.overdue_digest"},
	})
	e.must(201, "alice", "POST", wsBase+"/automations", map[string]any{
		"name": "Gecikənləri Carol-a", "trigger": "item.overdue",
		"conditions": []any{map[string]any{"field": "due_date", "op": "is_not_empty"}},
		"actions":    []any{map[string]any{"type": "set_assignee", "user_id": carolID}},
	})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Ödəniş gecikir", "due_date": "2020-03-01", "assignee_id": bobID})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Vaxt var", "due_date": "2199-03-01"})
	e.drain() // creation events (no webhook subscription for them)
	base := rc.count()

	n, err := e.st.EmitOverdue(ctx, 100)
	if err != nil || n != 1 {
		t.Fatalf("first scan: n=%d err=%v", n, err)
	}
	if n, _ = e.st.EmitOverdue(ctx, 100); n != 0 {
		t.Fatalf("second scan emitted again: %d", n)
	}
	e.drain()

	if a := e.must(200, "alice", "GET", items+"/1", nil).Body["assignee_id"]; a != carolID {
		t.Fatalf("overdue rule not applied: assignee=%v", a)
	}
	if rc.count() != base+1 {
		t.Fatalf("webhook deliveries: %d", rc.count()-base)
	}
	var got struct {
		Type   string         `json:"type"`
		Source string         `json:"source"`
		Data   map[string]any `json:"data"`
	}
	rc.mu.Lock()
	err = json.Unmarshal(rc.bodies[len(rc.bodies)-1], &got)
	rc.mu.Unlock()
	if err != nil || got.Type != "item.overdue" || got.Source != "system" ||
		got.Data["key"] != "ACME-1" || got.Data["due_date"] != "2020-03-01" || got.Data["days_overdue"].(float64) < 1 {
		t.Fatalf("overdue webhook body: %+v err=%v", got, err)
	}

	// A new due date that is also past is a new reminder.
	v := e.must(200, "alice", "GET", items+"/1", nil).Body["version"]
	e.must(200, "alice", "PATCH", items+"/1", map[string]any{"version": v, "due_date": "2020-04-01"})
	if n, _ = e.st.EmitOverdue(ctx, 100); n != 1 {
		t.Fatalf("re-dated item: %d", n)
	}

	// Digest: off by default, then once per local day.
	if n, _ = e.st.EmitOverdueDigests(ctx); n != 0 {
		t.Fatalf("digest while off: %d", n)
	}
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"overdue_digest_hour": 0})
	if n, _ = e.st.EmitOverdueDigests(ctx); n != 1 {
		t.Fatalf("digest: %d", n)
	}
	if n, _ = e.st.EmitOverdueDigests(ctx); n != 0 {
		t.Fatalf("second digest the same day: %d", n)
	}
	e.drain()
	rc.mu.Lock()
	last := rc.bodies[len(rc.bodies)-1]
	rc.mu.Unlock()
	if err := json.Unmarshal(last, &got); err != nil || got.Type != "workspace.overdue_digest" || got.Data["total"] != 1.0 {
		t.Fatalf("digest body: %s", last)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
