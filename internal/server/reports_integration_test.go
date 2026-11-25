package server

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Workspace reports (ADR-0041). Synthetic people as in the approval tests:
// alice owner, bob guest, carol member (the preparer), eve and erin members
// (approvers), dave outsider. History is backdated with the schema owner's
// pool so every number of every widget is known exactly.

const reportsPath = "/api/v1/workspaces/acme/reports"

type reportsFixture struct {
	e     *p1env
	owner *pgxpool.Pool
	baku  *time.Location
}

// at is a wall-clock time in the workspace's zone (Asia/Baku, UTC+4).
func (f *reportsFixture) at(s string) time.Time {
	f.e.t.Helper()
	t, err := time.ParseInLocation("2006-01-02 15:04", s, f.baku)
	if err != nil {
		f.e.t.Fatal(err)
	}
	return t
}

func (f *reportsFixture) exec(sql string, args ...any) {
	f.e.t.Helper()
	if _, err := f.owner.Exec(context.Background(), sql, args...); err != nil {
		f.e.t.Fatalf("%v\n%s", err, sql)
	}
}

// itemID of <project>-<number> in acme.
func (f *reportsFixture) itemID(key string, n int) string {
	f.e.t.Helper()
	var id string
	if err := f.owner.QueryRow(context.Background(), `SELECT i.id::text FROM items i JOIN projects p ON p.id = i.project_id
		WHERE p.key = $1 AND i.number = $2`, key, n).Scan(&id); err != nil {
		f.e.t.Fatal(err)
	}
	return id
}

func (f *reportsFixture) created(key string, n int, at string) {
	f.exec(`UPDATE items SET created_at = $2 WHERE id = $1`, f.itemID(key, n), f.at(at))
}

// request backdates the item's last approval request and, for a closed
// one, its decision events (the item.updated of an applied approval is
// the completion).
func (f *reportsFixture) request(key string, n int, opened, closed string, dueOn string) {
	id := f.itemID(key, n)
	var due any
	if dueOn != "" {
		due = dueOn
	}
	f.exec(`UPDATE approval_requests SET created_at = $2, due_on = $3::date WHERE id =
		(SELECT id FROM approval_requests WHERE item_id = $1 ORDER BY created_at DESC LIMIT 1)`, id, f.at(opened), due)
	if closed == "" {
		return
	}
	f.exec(`UPDATE approval_requests SET closed_at = $2 WHERE item_id = $1 AND closed_at IS NOT NULL`, id, f.at(closed))
	f.exec(`ALTER TABLE events DISABLE TRIGGER events_no_update`)
	f.exec(`UPDATE events SET occurred_at = $2 WHERE entity_id = $1 AND action = 'item.updated'`, id, f.at(closed))
	f.exec(`ALTER TABLE events ENABLE TRIGGER events_no_update`)
}

func (e *p1env) reports(user, query string) map[string]any {
	e.t.Helper()
	return e.must(200, user, "GET", reportsPath+query, nil).Body
}

func num(m map[string]any, k string) float64 {
	v, _ := m[k].(float64)
	return v
}

func kpis(b map[string]any) map[string]float64 {
	out := map[string]float64{}
	for k, v := range b["kpis"].(map[string]any) {
		out[k] = v.(float64)
	}
	return out
}

func counts(xs []any, key string) []float64 {
	var out []float64
	for _, x := range xs {
		out = append(out, x.(map[string]any)[key].(float64))
	}
	return out
}

const janPeriod = "?from=2026-01-05&to=2026-01-25"

// reportsSetup seeds acme (Asia/Baku) with a Done approval policy:
//
//	ACME-1 erin  created 01-06, approved 01-07 (2 h)                 → done
//	ACME-2 erin  created 01-13, rejected 01-14 (6 h), due today-10   → open, overdue 8-30
//	ACME-3 carol created 01-20, approved 01-22 (24 h, deadline 01-21) → done, late decision
//	ACME-4 —     created now, due today-3, pending request due yesterday
//	ACME-5 carol created 01-25 23:30 local, request canceled 01-25    → open
//	ACME-6 carol created 01-26 00:30 local (01-25 in UTC!), type bug  → open
//	SEC-1  erin  created 01-08, approved 01-09 — private project
//	SEC-2  erin  created 01-09, pending
func reportsSetup(t *testing.T) *reportsFixture {
	e, owner := newP1EnvOwner(t)
	baku, _ := time.LoadLocation("Asia/Baku")
	f := &reportsFixture{e: e, owner: owner, baku: baku}
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku"})
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	erin, carol := e.userID("erin"), e.userID("carol")
	today := e.wsToday("acme")
	day := func(n int) string { return ymd(today.AddDate(0, 0, n)) }
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq"})

	for _, it := range []map[string]any{
		{"title": "Akt-üzləşmə", "assignee_id": erin},
		{"title": "ƏDV bəyannaməsi", "assignee_id": erin, "due_date": day(-10)},
		{"title": "Kassa hesabatı", "assignee_id": carol},
		{"title": "Avans", "due_date": day(-3)},
		{"title": "Bank çıxarışı", "assignee_id": carol},
		{"title": "Arxiv", "assignee_id": carol, "type": "bug"},
	} {
		e.must(201, "carol", "POST", items, it)
	}
	ask := func(path string, n string) string {
		e.must(201, "carol", "POST", path+"/"+n+"/approvals", map[string]any{"version": 1, "to_status_id": done})
		return e.pending(path + "/" + n)["id"].(string)
	}
	r1, r2, r3 := ask(items, "1"), ask(items, "2"), ask(items, "3")
	ask(items, "4")
	r5 := ask(items, "5")
	e.must(200, "eve", "POST", items+"/1/approvals/"+r1+"/approve", map[string]any{})
	e.must(200, "eve", "POST", items+"/2/approvals/"+r2+"/reject", map[string]any{"reason": "Məbləğ uyğun deyil"})
	e.must(200, "eve", "POST", items+"/3/approvals/"+r3+"/approve", map[string]any{})
	e.must(200, "carol", "POST", items+"/5/approvals/"+r5+"/cancel", map[string]any{})

	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli müqavilə", "assignee_id": erin})
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli əlavə", "assignee_id": erin})
	s1 := ask(secItems, "1")
	ask(secItems, "2")
	e.must(200, "eve", "POST", secItems+"/1/approvals/"+s1+"/approve", map[string]any{})
	e.must(200, "alice", "PATCH", secPath, map[string]any{"visibility": "private"})
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)

	f.created("ACME", 1, "2026-01-06 10:00")
	f.request("ACME", 1, "2026-01-07 09:00", "2026-01-07 11:00", "")
	f.created("ACME", 2, "2026-01-13 10:00")
	f.request("ACME", 2, "2026-01-14 08:00", "2026-01-14 14:00", "")
	f.created("ACME", 3, "2026-01-20 10:00")
	f.request("ACME", 3, "2026-01-21 00:00", "2026-01-22 00:00", "2026-01-21")
	f.request("ACME", 4, "2026-11-01 10:00", "", day(-1))
	f.created("ACME", 5, "2026-01-25 23:30")
	f.request("ACME", 5, "2026-01-25 23:40", "2026-01-25 23:50", "")
	f.created("ACME", 6, "2026-01-26 00:30")
	f.created("SEC", 1, "2026-01-08 10:00")
	f.request("SEC", 1, "2026-01-09 09:00", "2026-01-09 10:00", "")
	f.created("SEC", 2, "2026-01-09 10:00")
	return f
}

func TestReportsExactNumbers(t *testing.T) {
	f := reportsSetup(t)
	e := f.e
	b := e.reports("erin", janPeriod)

	rng := b["range"].(map[string]any)
	if rng["from"] != "2026-01-05" || rng["to"] != "2026-01-25" || rng["bucket"] != "week" || rng["timezone"] != "Asia/Baku" ||
		rng["today"] != ymd(e.wsToday("acme")) {
		t.Fatalf("range %v", rng)
	}
	want := map[string]float64{"open": 4, "created": 4, "completed": 2, "overdue": 2, "awaiting_approval": 1, "awaiting_approval_late": 1}
	if got := kpis(b); !mapsEqual(got, want) {
		t.Fatalf("kpis %v, want %v", got, want)
	}

	tp := b["throughput"].([]any)
	var starts []string
	for _, x := range tp {
		starts = append(starts, x.(map[string]any)["start"].(string))
	}
	if !slices.Equal(starts, []string{"2026-01-05", "2026-01-12", "2026-01-19"}) ||
		!slices.Equal(counts(tp, "created"), []float64{1, 1, 2}) || !slices.Equal(counts(tp, "completed"), []float64{1, 0, 1}) {
		t.Fatalf("throughput %v", tp)
	}

	aging := b["aging"].(map[string]any)
	if got := counts(aging["age"].([]any), "count"); !slices.Equal(got, []float64{1, 0, 0, 3}) {
		t.Errorf("age %v", got)
	}
	if got := counts(aging["late"].([]any), "count"); !slices.Equal(got, []float64{1, 1, 0, 0}) {
		t.Errorf("late %v", got)
	}

	ap := b["approvals"].(map[string]any)
	total := ap["total"].(map[string]any)
	if num(total, "approved") != 2 || num(total, "rejected") != 1 || num(total, "canceled") != 1 || num(total, "late") != 1 ||
		num(total, "median_seconds") != 6*3600 || math.Abs(num(total, "p90_seconds")-73440) > 0.5 {
		t.Errorf("approval total %v", total)
	}
	by := ap["by_transition"].([]any)
	if len(by) != 1 || by[0].(map[string]any)["to_status"].(map[string]any)["name"] != "Done" ||
		by[0].(map[string]any)["to_status"].(map[string]any)["workflow"] != "Default" || num(by[0].(map[string]any), "approved") != 2 {
		t.Errorf("by transition %v", by)
	}

	var rows []string
	for _, x := range b["workload"].([]any) {
		w := x.(map[string]any)
		name, _ := w["name"].(string)
		rows = append(rows, name+":"+ftoa(num(w, "open"))+"/"+ftoa(num(w, "overdue"))+"/"+ftoa(num(w, "completed")))
	}
	if !slices.Equal(rows, []string{"Erin:1/1/1", ":1/1/0", "Carol:2/0/1"}) {
		t.Errorf("workload %v", rows)
	}

	st := b["status"].([]any)
	if len(st) != 1 || st[0].(map[string]any)["key"] != "ACME" || num(st[0].(map[string]any), "total") != 6 {
		t.Fatalf("status %v", st)
	}
	var dist []string
	for _, x := range st[0].(map[string]any)["statuses"].([]any) {
		s := x.(map[string]any)
		dist = append(dist, s["name"].(string)+"="+ftoa(num(s, "count")))
	}
	if !slices.Equal(dist, []string{"To Do=4", "Done=2"}) {
		t.Errorf("distribution %v", dist)
	}
	if ps := b["projects"].([]any); len(ps) != 1 {
		t.Errorf("projects in scope for erin: %v", ps)
	}

	t.Run("privacy: the private project counts only for those who see it", func(t *testing.T) {
		if got := kpis(e.reports("bob", janPeriod)); !mapsEqual(got, want) {
			t.Errorf("guest %v", got)
		}
		a := e.reports("alice", janPeriod)
		wantA := map[string]float64{"open": 5, "created": 6, "completed": 3, "overdue": 2, "awaiting_approval": 2, "awaiting_approval_late": 1}
		if got := kpis(a); !mapsEqual(got, wantA) {
			t.Errorf("owner %v, want %v", got, wantA)
		}
		if n := len(a["status"].([]any)); n != 2 {
			t.Errorf("owner status projects %d", n)
		}
		// Asking for the private project by key is just an empty scope.
		if got := kpis(e.reports("erin", janPeriod+"&projects=SEC")); got["open"] != 0 || got["created"] != 0 {
			t.Errorf("erin projects=SEC %v", got)
		}
		if got := kpis(e.reports("carol", janPeriod+"&projects=SEC")); got["open"] != 1 || got["completed"] != 1 {
			t.Errorf("carol (SEC member) %v", got)
		}
		if r := e.call("dave", "GET", reportsPath+janPeriod, nil); r.Status != 404 {
			t.Errorf("outsider: %d", r.Status)
		}
	})

	t.Run("filters", func(t *testing.T) {
		if got := kpis(e.reports("alice", janPeriod+"&projects=ACME")); !mapsEqual(got, want) {
			t.Errorf("projects=ACME %v", got)
		}
		if got := kpis(e.reports("carol", janPeriod+"&assignee=me")); got["open"] != 2 || got["completed"] != 1 || got["created"] != 2 {
			t.Errorf("assignee=me %v", got)
		}
		if got := kpis(e.reports("erin", janPeriod+"&assignee=none")); got["open"] != 1 || got["awaiting_approval"] != 1 {
			t.Errorf("assignee=none %v", got)
		}
		if got := kpis(e.reports("erin", janPeriod+"&assignee="+e.userID("erin"))); got["open"] != 1 || got["overdue"] != 1 {
			t.Errorf("assignee=<erin> %v", got)
		}
		if got := kpis(e.reports("erin", janPeriod+"&type=bug")); got["open"] != 1 || got["created"] != 0 {
			t.Errorf("type=bug %v", got)
		}
		if got := kpis(e.reports("erin", janPeriod+"&type=nope")); got["open"] != 0 {
			t.Errorf("unknown type %v", got)
		}
		// One day later in Baku: ACME-6 (00:30 on the 26th) is created in period.
		if got := kpis(e.reports("erin", "?from=2026-01-05&to=2026-01-26")); got["created"] != 5 {
			t.Errorf("tz boundary %v", got)
		}
		year := e.reports("erin", "?from=2026-01-01&to=2026-12-31")
		tp := year["throughput"].([]any)
		if year["range"].(map[string]any)["bucket"] != "month" || len(tp) != 12 || tp[0].(map[string]any)["start"] != "2026-01-01" ||
			num(tp[0].(map[string]any), "created") != 5 {
			t.Errorf("monthly %v", year["range"])
		}
		def := e.reports("erin", "")["range"].(map[string]any)
		if today := ymd(e.wsToday("acme")); def["to"] != today || def["from"] != today[:8]+"01" {
			t.Errorf("default range %v", def)
		}
	})

	t.Run("validation and headers", func(t *testing.T) {
		for _, q := range []string{"?from=2026-01-05", "?to=2026-01-05", "?from=2026-02-01&to=2026-01-01", "?from=2024-01-01&to=2026-01-02",
			"?from=2026-13-01&to=2026-12-01", "?from=x&to=y", janPeriod + "&projects=bad-key", janPeriod + "&assignee=xyz",
			janPeriod + "&type=Bad", janPeriod + "&from=2026-01-01", "?%zz"} {
			// 400: malformed parameters; 422: the store's range and key rules.
			if r := e.call("erin", "GET", reportsPath+q, nil); r.Status != 400 && r.Status != 422 {
				t.Errorf("%s: %d %s", q, r.Status, r.Raw)
			}
		}
		req := httptest.NewRequestWithContext(t.Context(), "GET", reportsPath+janPeriod, nil)
		req.Header.Set("Authorization", "Bearer erin")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "private, max-age=60" || rec.Header().Get("Vary") != "Authorization" {
			t.Errorf("headers %d %v", rec.Code, rec.Header())
		}
	})

	t.Run("workload CSV is the table, audited", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), "GET", reportsPath+"/workload.csv"+janPeriod+"&lang=en&delimiter=semicolon", nil)
		req.Header.Set("Authorization", "Bearer bob")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		body := strings.TrimPrefix(rec.Body.String(), "\ufeff")
		wantCSV := "Assignee;Open;Overdue;Completed in period\r\nErin;1;1;1\r\nUnassigned;1;1;0\r\nCarol;2;0;1\r\n"
		if rec.Code != 200 || body != wantCSV || !strings.Contains(rec.Header().Get("Content-Disposition"), "acme-workload-2026-01-05_2026-01-25.csv") {
			t.Fatalf("csv %d %q %v", rec.Code, body, rec.Header())
		}
		if n := e.countEvents(`action = 'reports.exported' AND payload->>'rows' = '3' AND payload->>'assignee' = 'all'`); n != 1 {
			t.Errorf("audit events %d", n)
		}
		if r := e.call("bob", "GET", reportsPath+"/workload.csv"+janPeriod+"&lang=xx", nil); r.Status != 400 {
			t.Errorf("bad lang %d", r.Status)
		}
	})
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
