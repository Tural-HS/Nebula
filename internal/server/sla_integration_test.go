package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

const slaPath = wsBase + "/projects/ACME/sla"

func (e *p1env) slaOf(n string) map[string]any {
	e.t.Helper()
	s, _ := e.must(200, "carol", "GET", items+"/"+n, nil).Body["sla"].(map[string]any)
	return s
}

func (e *p1env) slaKeys(filter string) []string {
	e.t.Helper()
	var out []string
	for _, it := range e.must(200, "carol", "GET", items+"?sla="+filter, nil).Body["items"].([]any) {
		out = append(out, it.(map[string]any)["key"].(string))
	}
	return out
}

func TestSLAPolicyAccessAndValidation(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	if r := e.must(200, "bob", "GET", slaPath, nil); r.Body["resolution_days"] != nil {
		t.Fatalf("empty policy: %v", r.Body)
	}
	for _, who := range []string{"bob", "carol"} { // guest, member
		if r := e.call(who, "PUT", slaPath, map[string]any{"resolution_days": 3}); r.Status != 403 {
			t.Fatalf("%s sets the policy: %d", who, r.Status)
		}
	}
	for _, bad := range []map[string]any{
		{"resolution_days": 0}, {"resolution_days": 121}, {"response_days": 61},
		{"response_days": 5, "resolution_days": 3}, {"resolution_days": "3"},
	} {
		if r := e.call("alice", "PUT", slaPath, bad); r.Status != 422 && r.Status != 400 {
			t.Errorf("%v: %d %s", bad, r.Status, r.Raw)
		}
	}
	p := e.must(200, "alice", "PUT", slaPath, map[string]any{"response_days": 1, "resolution_days": 3}).Body
	if p["response_days"] != 1.0 || p["resolution_days"] != 3.0 || p["since"] == nil {
		t.Fatalf("policy: %v", p)
	}
	if n := e.countEvents(`action = 'project.sla_updated'`); n != 1 {
		t.Fatalf("audit events: %d", n)
	}
	// Both null removes the policy and the items' clocks.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Sorğu"})
	if e.slaOf("1") == nil {
		t.Fatal("new item has no SLA clock")
	}
	e.must(200, "alice", "PUT", slaPath, map[string]any{"response_days": nil, "resolution_days": nil})
	if e.slaOf("1") != nil {
		t.Fatal("clock kept after the policy was removed")
	}
	if r := e.call("carol", "GET", items+"?sla=late", nil); r.Status != 422 {
		t.Fatalf("bad sla filter: %d", r.Status)
	}
}

func TestSLAClockBreachRiskAndReminders(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	wf := e.defaultWorkflow()
	inProgress, done := statusID(t, wf, "In Progress"), statusID(t, wf, "Done")
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)

	e.must(200, "alice", "PUT", slaPath, map[string]any{"response_days": 1, "resolution_days": 3})
	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş sorğusu", "assignee_id": carolID}).Body
	s := it["sla"].(map[string]any)
	if s["state"] != "on_track" || s["response_due"] == nil || s["resolution_due"] == nil {
		t.Fatalf("new item clock: %v", s)
	}
	before := s["resolution_due"].(string)

	// A holiday inside the window moves the due by one working day once the
	// worker recomputes (calendar change marks the policy stale).
	day := e.today()
	for found := 0; found < 2; {
		day = day.AddDate(0, 0, 1)
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			found++
		}
	}
	e.must(200, "alice", "PUT", wsBase+"/calendar/days/"+ymd(day), map[string]any{"kind": "holiday", "name": "Sınaq bayramı"})
	if _, err := e.st.RecomputeSLA(ctx, 100); err != nil {
		t.Fatal(err)
	}
	b, _ := time.Parse(time.RFC3339, before)
	a, _ := time.Parse(time.RFC3339, e.slaOf("1")["resolution_due"].(string))
	if !a.After(b) {
		t.Fatalf("holiday did not move the due: %s -> %s", b, a)
	}

	// Backdate item 1 past both targets; item 2 into the last quarter.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Risk", "assignee_id": carolID})
	if _, err := e.pool.Exec(ctx, `UPDATE item_sla SET started_at = now() - interval '30 days',
		response_due = now() - interval '20 days', resolution_due = now() - interval '1 hour'
		WHERE item_id = (SELECT id FROM items WHERE number = 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE item_sla SET started_at = now() - interval '10 days',
		response_due = NULL, resolution_due = now() + interval '1 day'
		WHERE item_id = (SELECT id FROM items WHERE number = 2)`); err != nil {
		t.Fatal(err)
	}
	if st := e.slaOf("1")["state"]; st != "breached" {
		t.Fatalf("state: %v", st)
	}
	if got := e.slaKeys("breached"); len(got) != 1 || got[0] != "ACME-1" {
		t.Fatalf("breached queue: %v", got)
	}
	if got := e.slaKeys("at_risk"); len(got) != 1 || got[0] != "ACME-2" {
		t.Fatalf("at-risk queue: %v", got)
	}
	qc := e.must(200, "carol", "GET", items+"/queue-counts", nil).Body
	if qc["sla_breached"] != 1.0 || qc["sla_at_risk"] != 1.0 {
		t.Fatalf("queue counts: %v", qc)
	}

	// Two replicas tick at once: response + resolution breach of item 1,
	// at-risk of item 2, each exactly once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for range 2 {
		wg.Go(func() {
			n, err := e.st.EmitSLA(ctx, 100)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		})
	}
	wg.Wait()
	if total != 3 {
		t.Fatalf("emitted %d, want 3", total)
	}
	if n, err := e.st.EmitSLA(ctx, 100); err != nil || n != 0 {
		t.Fatalf("second tick: %d %v", n, err)
	}
	e.drain()
	count := func(user, kind string) int {
		n := 0
		for _, x := range e.notes(user, "") {
			if x["kind"] == kind {
				n++
			}
		}
		return n
	}
	if count("carol", "sla_breached") != 2 || count("carol", "sla_at_risk") != 1 || count("alice", "sla_breached") != 2 {
		t.Fatalf("notifications: carol %v, alice %v", kinds(e.notes("carol", "")), kinds(e.notes("alice", "")))
	}
	if count("bob", "sla_breached") != 0 || count("alice", "sla_at_risk") != 0 {
		t.Fatal("guest or admin got an assignee-only reminder")
	}

	// Archived projects get no reminders.
	if _, err := e.pool.Exec(ctx, `UPDATE item_sla SET resolution_due = now() - interval '1 minute' WHERE item_id = (SELECT id FROM items WHERE number = 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE projects SET archived_at = now() WHERE key = 'ACME'`); err != nil {
		t.Fatal(err)
	}
	if n, err := e.st.EmitSLA(ctx, 100); err != nil || n != 0 {
		t.Fatalf("archived project reminded: %d %v", n, err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE projects SET archived_at = NULL WHERE key = 'ACME'`); err != nil {
		t.Fatal(err)
	}

	// Transitions: a comment by someone other than the requester is the
	// first response; done resolves (late: still breached, but off the
	// queue); reopening clears the resolution.
	e.must(201, "carol", "POST", items+"/2/comments", map[string]any{"body": "Öz şərhim"})
	if e.slaOf("2")["responded_at"] != nil {
		t.Fatal("the requester's own comment counted as a response")
	}
	e.must(201, "alice", "POST", items+"/2/comments", map[string]any{"body": "Baxırıq"})
	if e.slaOf("2")["responded_at"] == nil {
		t.Fatal("comment by the team is not a response")
	}
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": e.version("1"), "status_id": done})
	s = e.slaOf("1")
	if s["resolved_at"] == nil || s["responded_at"] == nil || s["state"] != "breached" {
		t.Fatalf("resolved late: %v", s)
	}
	if got := e.slaKeys("breached"); len(got) != 1 || got[0] != "ACME-2" {
		t.Fatalf("breached queue after resolving: %v", got)
	}
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": e.version("1"), "status_id": inProgress})
	if e.slaOf("1")["resolved_at"] != nil {
		t.Fatal("reopening kept the resolution")
	}
}
