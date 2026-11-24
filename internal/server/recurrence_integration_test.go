package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/store"
)

const recPath = "/api/v1/workspaces/acme/projects/ACME/recurrences"

// today in the workspace's zone (UTC unless changed), like the scheduler.
func (e *p1env) today() time.Time {
	e.t.Helper()
	var d time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT (now() AT TIME ZONE timezone)::date FROM workspaces WHERE slug = 'acme'`).Scan(&d); err != nil {
		e.t.Fatal(err)
	}
	return d
}

func ymd(t time.Time) string { return t.Format(time.DateOnly) }

func TestRecurrenceAPIValidationAndAccess(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	today := e.today()
	daily := map[string]any{"title": "Gündəlik yoxlama", "freq": "daily", "every": 1, "start_date": ymd(today)}

	if r := e.call("bob", "GET", recPath, nil); r.Status != 403 { // guest
		t.Fatalf("guest lists rules: %d", r.Status)
	}
	e.must(200, "carol", "GET", recPath, nil) // member reads
	if r := e.call("carol", "POST", recPath, daily); r.Status != 403 {
		t.Fatalf("member creates rule: %d", r.Status)
	}
	if r := e.call("dave", "GET", recPath, nil); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}

	bad := []map[string]any{
		{"title": "x", "freq": "hourly", "every": 1, "start_date": ymd(today)},
		{"title": "x", "freq": "daily", "every": 0, "start_date": ymd(today)},
		{"title": "x", "freq": "weekly", "every": 1, "start_date": ymd(today)},
		{"title": "x", "freq": "monthly", "every": 1, "month_day": 32, "start_date": ymd(today)},
		{"title": "x", "freq": "daily", "every": 1, "start_date": "09.11.2026"},
		{"title": "x", "freq": "daily", "every": 1, "start_date": ymd(today), "end_date": ymd(today.AddDate(0, 0, -1))},
		{"title": "", "freq": "daily", "every": 1, "start_date": ymd(today)},
		{"title": "x", "freq": "daily", "every": 1, "start_date": ymd(today), "type": "nope"},
		{"title": "x", "freq": "daily", "every": 1, "start_date": ymd(today), "lead_days": 400},
		{"title": "x", "freq": "daily", "every": 1, "start_date": ymd(today), "assignee_id": "01a20727-0000-7000-8000-000000000000"},
	}
	for i, b := range bad {
		if r := e.call("alice", "POST", recPath, b); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}

	// Preview: the 9th of every month, from today.
	prev := e.must(200, "carol", "POST", recPath+"/preview", map[string]any{"freq": "monthly", "every": 1, "month_day": 9, "start_date": ymd(today)}).Body
	up := prev["upcoming"].([]any)
	if len(up) != 5 {
		t.Fatalf("preview: %v", prev)
	}
	for _, d := range up {
		if s := d.(string); s[8:] != "09" || s < ymd(today) {
			t.Fatalf("preview date %s", s)
		}
	}

	rec := e.must(201, "alice", "POST", recPath, map[string]any{
		"title": "Ofis icarəsi — {{month}} ödənişi", "freq": "monthly", "every": 1, "month_day": 9,
		"start_date": ymd(today), "lead_days": 3,
	}).Body
	if rec["enabled"] != true || rec["next_due"] == nil || len(rec["upcoming"].([]any)) != 5 || rec["type"].(map[string]any)["key"] != "task" {
		t.Fatalf("created: %v", rec)
	}
	id := rec["id"].(string)
	e.must(200, "carol", "GET", recPath+"/"+id, nil)
	off := e.must(200, "alice", "PUT", recPath+"/"+id, map[string]any{
		"title": "Ofis icarəsi — {{month}} ödənişi", "freq": "monthly", "every": 1, "month_day": 9,
		"start_date": ymd(today), "enabled": false,
	}).Body
	if off["enabled"] != false {
		t.Fatalf("disable: %v", off)
	}
	if audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw; !contains(audit, "recurrence.created") || !contains(audit, "recurrence.updated") {
		t.Fatalf("audit: %s", audit)
	}
	e.must(204, "alice", "DELETE", recPath+"/"+id, nil)
	if r := e.call("alice", "GET", recPath+"/"+id, nil); r.Status != 404 {
		t.Fatalf("deleted rule: %d", r.Status)
	}
}

func TestRecurrenceSchedulerCreatesCatchesUpAndFailsSafely(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	today := e.today()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)

	rc := newReceiver(t, 200)
	e.must(201, "alice", "POST", wsBase+"/webhooks", map[string]any{"url": rc.srv.URL + "/hook", "events": []string{"item.created"}})

	rec := e.must(201, "alice", "POST", recPath, map[string]any{
		"title": "Kassa yoxlaması {{date}}", "freq": "daily", "every": 1, "start_date": ymd(today),
		"lead_days": 0, "assignee_id": carolID,
	}).Body
	id := rec["id"].(string)
	if rec["next_due"] != ymd(today) {
		t.Fatalf("next_due = %v, want today", rec["next_due"])
	}

	n, err := e.st.EmitRecurrences(ctx, 100)
	if err != nil || n != 1 {
		t.Fatalf("first run: n=%d err=%v", n, err)
	}
	if n, _ = e.st.EmitRecurrences(ctx, 100); n != 0 {
		t.Fatalf("second run created again: %d", n)
	}
	it := e.must(200, "alice", "GET", items+"/1", nil).Body
	if it["title"] != "Kassa yoxlaması "+today.Format("02.01.2006") || it["due_date"] != ymd(today) ||
		it["assignee_id"] != carolID || it["reporter_id"] != nil {
		t.Fatalf("created item: %v", it)
	}
	if next := e.must(200, "alice", "GET", recPath+"/"+id, nil).Body["next_due"]; next != ymd(today.AddDate(0, 0, 1)) {
		t.Fatalf("next_due not advanced: %v", next)
	}

	// The item is an ordinary item.created from the system: webhooks see it.
	e.drain()
	var got struct {
		Type   string         `json:"type"`
		Source string         `json:"source"`
		Data   map[string]any `json:"data"`
	}
	rc.mu.Lock()
	err = json.Unmarshal(rc.bodies[len(rc.bodies)-1], &got)
	rc.mu.Unlock()
	if err != nil || got.Type != "item.created" || got.Source != "system" || got.Data["recurrence_id"] != id {
		t.Fatalf("webhook: %+v %v", got, err)
	}

	// The server was down for three days: the missed dates are created (and
	// are overdue), the one that already has an item is not duplicated.
	// (A rule that started 10 days ago, last advanced 3 days ago.)
	if _, err := e.pool.Exec(ctx, `UPDATE recurrences SET start_date = $1, next_due = $2 WHERE id = $3`,
		today.AddDate(0, 0, -10), today.AddDate(0, 0, -3), id); err != nil {
		t.Fatal(err)
	}
	if n, _ = e.st.EmitRecurrences(ctx, 100); n != 3 {
		t.Fatalf("catch-up created %d, want 3 (days -3..-1; today exists)", n)
	}
	if c := e.must(200, "alice", "GET", items+"/counts?due=overdue", nil).Body["total"]; c != 3.0 {
		t.Fatalf("overdue after catch-up: %v", c)
	}
	var runs int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM recurrence_runs WHERE recurrence_id = $1`, id).Scan(&runs)
	if runs != 4 {
		t.Fatalf("runs: %d", runs)
	}

	// Editing the rule never reschedules a date that already has an item.
	upd := e.must(200, "alice", "PUT", recPath+"/"+id, map[string]any{
		"title": "Kassa yoxlaması {{date}}", "freq": "daily", "every": 1, "start_date": ymd(today.AddDate(0, 0, -10)), "lead_days": 0,
	}).Body
	if upd["next_due"] != ymd(today.AddDate(0, 0, 1)) {
		t.Fatalf("after edit next_due = %v", upd["next_due"])
	}

	// A rule that can no longer create a valid item is disabled once, with the reason.
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "branch", "name": "Filial", "type": "text"})
	e.must(200, "alice", "PUT", bindPath("task", "branch"), map[string]any{"required": true, "position": 0})
	if _, err := e.pool.Exec(ctx, `UPDATE recurrences SET next_due = $1 WHERE id = $2`, today.AddDate(0, 0, 5), id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE recurrences SET lead_days = 10 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, err = e.st.EmitRecurrences(ctx, 100); err != nil || n != 0 {
		t.Fatalf("broken rule: n=%d err=%v", n, err)
	}
	failed := e.must(200, "alice", "GET", recPath+"/"+id, nil).Body
	if failed["enabled"] != false || failed["last_error"] == nil {
		t.Fatalf("broken rule not disabled: %v", failed)
	}
	if audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw; !contains(audit, "recurrence.failed") {
		t.Fatalf("no recurrence.failed event")
	}

	// Deleting the rule keeps its items.
	e.must(204, "alice", "DELETE", recPath+"/"+id, nil)
	if c := e.must(200, "alice", "GET", items+"/counts", nil).Body["total"]; c != 4.0 {
		t.Fatalf("items after rule deletion: %v", c)
	}
}

func TestRenderRecurrenceTitle(t *testing.T) {
	d := time.Date(2026, time.November, 9, 0, 0, 0, 0, time.UTC)
	if got := store.RenderRecurrenceTitle("{{month}} {{year}} — {{date}}", d); got != "noyabr 2026 — 09.11.2026" {
		t.Fatal(got)
	}
}
