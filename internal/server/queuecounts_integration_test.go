package server

import (
	"testing"
	"time"
)

// Quick-filter counts of the item list (ADR-0035): one request, and each
// number equals the list that the matching filter returns.
func TestQueueCounts(t *testing.T) {
	e := newEnv(t)
	e.setup()
	carol := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	soon := time.Now().UTC().AddDate(0, 0, 3).Format(time.DateOnly)

	e.must(201, "carol", "POST", items, map[string]any{"title": "Late payment", "due_date": "2020-01-15", "assignee_id": carol}) // ACME-1
	e.must(201, "carol", "POST", items, map[string]any{"title": "Soon report", "due_date": soon})                                // ACME-2
	e.must(201, "carol", "POST", items, map[string]any{"title": "Plain task"})                                                   // ACME-3
	e.must(201, "carol", "POST", items+"/3/links", map[string]any{"type": "blocked_by", "target": "ACME-1"})

	qc := items + "/queue-counts"
	got := e.must(200, "carol", "GET", qc, nil).Body
	want := map[string]float64{"all": 3, "mine": 1, "overdue": 1, "approval": 0, "blocked": 1, "week": 1}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v (%v)", k, got[k], v, got)
		}
	}
	// Each count matches its list filter.
	for k, q := range map[string]string{"all": "", "mine": "assignee=me", "overdue": "due=overdue", "approval": "approval=pending", "blocked": "blocked=true", "week": "due=week"} {
		if n := e.must(200, "carol", "GET", items+"/counts?"+q, nil).Body["total"]; n != got[k] {
			t.Errorf("%s: queue count %v, list count %v", k, got[k], n)
		}
	}
	// Search applies; the preset dimensions in the query are replaced.
	got = e.must(200, "carol", "GET", qc+"?q=Late&due=none&assignee=none&blocked=true", nil).Body
	if got["all"] != 1.0 || got["mine"] != 1.0 || got["overdue"] != 1.0 || got["week"] != 0.0 || got["blocked"] != 0.0 {
		t.Fatalf("with search: %v", got)
	}
	// Guests read; outsiders do not learn the project exists.
	e.must(200, "bob", "GET", qc, nil)
	if r := e.call("dave", "GET", qc, nil); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}
	if r := e.call("carol", "GET", qc+"?blocked=maybe", nil); r.Status != 400 {
		t.Fatalf("bad blocked: %d", r.Status)
	}
}
