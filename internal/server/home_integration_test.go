package server

import (
	"context"
	"slices"
	"testing"
	"time"
)

// The home page "Mənim işlərim" (ADR-0037). Synthetic people as in the
// approval tests: alice owner, bob guest, carol member (the preparer), eve
// and erin members (approvers), dave outsider.

const homePath = "/api/v1/me/home"

type homeResp struct {
	counters   map[string]float64
	keys       []string // "<ws>/<key>:<reason>" in list order
	total      float64
	workspaces map[string]map[string]any
	watching   []string
}

func (e *p1env) home(user, query string) homeResp {
	e.t.Helper()
	b := e.must(200, user, "GET", homePath+query, nil).Body
	out := homeResp{counters: map[string]float64{}, workspaces: map[string]map[string]any{}, total: b["total"].(float64)}
	for k, v := range b["counters"].(map[string]any) {
		out.counters[k] = v.(float64)
	}
	for _, x := range b["items"].([]any) {
		it := x.(map[string]any)
		out.keys = append(out.keys, it["workspace"].(map[string]any)["slug"].(string)+"/"+it["key"].(string)+":"+it["reason"].(string))
	}
	for _, x := range b["workspaces"].([]any) {
		w := x.(map[string]any)
		out.workspaces[w["slug"].(string)] = w
	}
	for _, x := range b["watching"].([]any) {
		out.watching = append(out.watching, x.(map[string]any)["key"].(string))
	}
	return out
}

func (e *p1env) wsToday(slug string) time.Time {
	e.t.Helper()
	var d time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT (now() AT TIME ZONE timezone)::date FROM workspaces WHERE slug = $1`, slug).Scan(&d); err != nil {
		e.t.Fatal(err)
	}
	return d
}

func (e *p1env) workflowOf(ws, name string) map[string]any {
	e.t.Helper()
	for _, w := range e.must(200, "alice", "GET", "/api/v1/workspaces/"+ws+"/workflows", nil).Body["workflows"].([]any) {
		if w.(map[string]any)["name"] == name {
			return w.(map[string]any)
		}
	}
	e.t.Fatalf("workflow %q missing in %s", name, ws)
	return nil
}

// betaSetup: workspace "beta" owned by alice with project BETA; carol, eve,
// erin members; Done needs min approvals from members.
func (e *p1env) betaSetup(minApprovals int) (betaItems, done string) {
	e.t.Helper()
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "beta", "name": "Beta MMC"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/beta/projects", map[string]any{"key": "BETA", "name": "Xəzinədarlıq"})
	for _, u := range []string{"carol", "eve", "erin"} {
		e.must(201, "alice", "POST", "/api/v1/workspaces/beta/members", map[string]any{"email": u + "@example.com", "role": "member"})
	}
	wf := e.workflowOf("beta", "Default")
	done = statusID(e.t, wf, "Done")
	e.must(200, "alice", "PUT", "/api/v1/workspaces/beta/workflows/"+wf["id"].(string)+"/statuses/"+done+"/approval-policy",
		map[string]any{"min_approvals": minApprovals, "approver_roles": []string{"member"}})
	return "/api/v1/workspaces/beta/projects/BETA/items", done
}

func TestHomeMergeOrderingPrivacyAndCards(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	canceled := statusID(t, wf, "Canceled")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_roles": []string{"member"}})
	erin := e.userID("erin")
	betaItems, betaDone := e.betaSetup(2)
	aToday, bToday := e.wsToday("acme"), e.wsToday("beta")
	day := func(base time.Time, n int) string { return ymd(base.AddDate(0, 0, n)) }

	// The oldest approval request is in beta: requests are ordered by age
	// across workspaces.
	e.must(201, "carol", "POST", betaItems, map[string]any{"title": "Ödəniş sorğusu"}) // BETA-1
	e.must(201, "carol", "POST", betaItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": betaDone})
	e.must(201, "carol", "POST", betaItems, map[string]any{"title": "Bank üzləşməsi", "assignee_id": erin, "due_date": day(bToday, -9)}) // BETA-2

	for _, it := range []map[string]any{
		{"title": "Akt-üzləşmə", "assignee_id": erin, "due_date": day(aToday, -5)},                         // ACME-1 overdue
		{"title": "ƏDV bəyannaməsi", "assignee_id": erin, "due_date": day(aToday, -2)},                     // ACME-2 overdue
		{"title": "Kassa hesabatı", "assignee_id": erin, "due_date": day(aToday, 3)},                       // ACME-3 this week
		{"title": "İllik audit", "assignee_id": erin, "due_date": day(aToday, 10)},                         // ACME-4 later
		{"title": "Arxiv", "assignee_id": erin},                                                            // ACME-5 no due date
		{"title": "Ləğv olunmuş", "assignee_id": erin, "due_date": day(aToday, -1), "status_id": canceled}, // ACME-6 closed
		{"title": "Avans", "assignee_id": "", "due_date": day(aToday, -3)},                                 // ACME-7 not mine
	} {
		if it["assignee_id"] == "" {
			delete(it, "assignee_id")
		}
		e.must(201, "carol", "POST", items, it)
	}
	e.must(201, "carol", "POST", items+"/7/approvals", map[string]any{"version": 1, "to_status_id": done})

	// A project that becomes private: erin's overdue item and a request she
	// could decide on must disappear with it (ADR-0013).
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq"})
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli müqavilə", "assignee_id": erin, "due_date": day(aToday, -30)})
	e.must(201, "carol", "POST", secItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})
	before := e.home("erin", "")
	if before.counters["overdue"] != 4 || before.counters["approvals"] != 3 {
		t.Fatalf("before private: %v", before.counters)
	}
	e.must(200, "alice", "PATCH", secPath, map[string]any{"visibility": "private"})
	e.must(204, "alice", "PUT", secPath+"/members/"+e.userID("carol"), nil)

	h := e.home("erin", "")
	if want := map[string]float64{"overdue": 3, "approvals": 2, "week": 1, "assigned": 6}; !mapsEqual(h.counters, want) {
		t.Fatalf("counters %v, want %v", h.counters, want)
	}
	wantOrder := []string{
		"beta/BETA-2:overdue", "acme/ACME-1:overdue", "acme/ACME-2:overdue", // oldest due first, across workspaces
		"beta/BETA-1:approval", "acme/ACME-7:approval", // oldest request first
		"acme/ACME-3:week",
	}
	if !slices.Equal(h.keys, wantOrder) || h.total != 6 {
		t.Fatalf("attention list %v (total %v), want %v", h.keys, h.total, wantOrder)
	}
	for _, k := range h.keys {
		if k == "acme/SEC-1:overdue" || k == "acme/SEC-1:approval" {
			t.Fatalf("private item listed: %v", h.keys)
		}
	}
	acme, beta := h.workspaces["acme"], h.workspaces["beta"]
	if acme["projects"] != 1.0 || acme["assigned"] != 5.0 || acme["overdue"] != 2.0 || acme["approvals"] != 1.0 || acme["role"] != "member" {
		t.Fatalf("acme card: %v", acme)
	}
	if beta["projects"] != 1.0 || beta["assigned"] != 1.0 || beta["overdue"] != 1.0 || beta["approvals"] != 1.0 || beta["last_activity"] == nil {
		t.Fatalf("beta card: %v", beta)
	}

	t.Run("filtered lists and limit", func(t *testing.T) {
		for _, c := range []struct {
			query string
			want  []string
		}{
			{"?show=overdue", wantOrder[:3]},
			{"?show=approval", wantOrder[3:5]},
			{"?show=week", wantOrder[5:]},
			{"?show=assigned", []string{"beta/BETA-2:overdue", "acme/ACME-1:overdue", "acme/ACME-2:overdue", "acme/ACME-3:week", "acme/ACME-4:assigned", "acme/ACME-5:assigned"}},
		} {
			if got := e.home("erin", c.query).keys; !slices.Equal(got, c.want) {
				t.Errorf("%s: %v, want %v", c.query, got, c.want)
			}
		}
		if got := e.home("erin", "?limit=2"); len(got.keys) != 2 || got.total != 6 {
			t.Errorf("limit 2: %v total %v", got.keys, got.total)
		}
		for _, q := range []string{"?show=all", "?limit=0", "?limit=201", "?limit=x", "?show=%zz"} {
			if r := e.call("erin", "GET", homePath+q, nil); r.Status != 400 {
				t.Errorf("%s: %d", q, r.Status)
			}
		}
	})

	t.Run("an admin sees the private project, its member too", func(t *testing.T) {
		if got := e.home("alice", "").workspaces["acme"]["projects"]; got != 2.0 {
			t.Errorf("alice projects: %v", got)
		}
		// carol is a member of SEC but requested every approval herself.
		if got := e.home("carol", "").counters["approvals"]; got != 0 {
			t.Errorf("requester counts her own requests: %v", got)
		}
		if got := e.home("eve", "").counters["approvals"]; got != 2 {
			t.Errorf("eve approvals: %v", got)
		}
	})

	t.Run("a decision removes the request from my count only", func(t *testing.T) {
		pending := e.pending(items + "/7")
		e.must(200, "erin", "POST", items+"/7/approvals/"+pending["id"].(string)+"/approve", map[string]any{})
		if got := e.home("erin", "").counters["approvals"]; got != 1 {
			t.Errorf("erin after voting: %v", got)
		}
		if got := e.home("eve", "").counters["approvals"]; got != 2 {
			t.Errorf("eve still has to decide: %v", got)
		}
	})

	t.Run("closed and deleted items drop out", func(t *testing.T) {
		e.must(204, "carol", "DELETE", items+"/1", nil)
		h := e.home("erin", "")
		if h.counters["overdue"] != 2 || slices.Contains(h.keys, "acme/ACME-1:overdue") {
			t.Errorf("deleted item still counted: %v %v", h.counters, h.keys)
		}
	})

	t.Run("watched items are listed, private ones never", func(t *testing.T) {
		// Her own items (assignees watch them) are not repeated here.
		e.must(200, "erin", "PUT", items+"/7/watchers/me", nil)
		if got := e.home("erin", "").watching; !slices.Equal(got, []string{"ACME-7"}) {
			t.Errorf("watching: %v", got)
		}
		e.must(200, "alice", "PUT", secItems+"/1/watchers/me", nil)
		if got := e.home("alice", "").watching; !slices.Contains(got, "SEC-1") {
			t.Errorf("alice watching: %v", got)
		}
	})
}

func TestHomeGuestsAndEmptyUsers(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş"})
	e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Gizli", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli iş", "due_date": ymd(e.today().AddDate(0, 0, -1))})

	// A guest never decides on approvals and sees only workspace projects.
	g := e.home("bob", "")
	if g.counters["approvals"] != 0 || g.counters["overdue"] != 0 || len(g.keys) != 0 {
		t.Fatalf("guest: %v %v", g.counters, g.keys)
	}
	if w := g.workspaces["acme"]; w == nil || w["projects"] != 1.0 || w["role"] != "guest" || w["approvals"] != 0.0 {
		t.Fatalf("guest card: %v", w)
	}

	// An outsider has no workspaces at all; dave has never been added anywhere.
	d := e.home("dave", "")
	if len(d.workspaces) != 0 || len(d.keys) != 0 || d.total != 0 || d.counters["assigned"] != 0 {
		t.Fatalf("outsider: %+v", d)
	}
	// A member with nothing assigned: cards, zero counters, an empty list.
	c := e.home("carol", "")
	if len(c.workspaces) != 1 || len(c.keys) != 0 || c.counters["approvals"] != 0 {
		t.Fatalf("empty member: %+v", c)
	}
	if r := e.call("", "GET", homePath, nil); r.Status != 401 {
		t.Fatalf("anonymous: %d", r.Status)
	}
}

// "This week" and "overdue" use each workspace's own local date: the same
// calendar date is overdue in a workspace far east and due today far west.
func TestHomeTimezoneBoundaries(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(200, "alice", "PATCH", "/api/v1/workspaces/acme", map[string]any{"timezone": "Pacific/Kiritimati"}) // UTC+14
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "west", "name": "West"})
	e.must(200, "alice", "PATCH", "/api/v1/workspaces/west", map[string]any{"timezone": "Pacific/Pago_Pago"}) // UTC-11
	e.must(201, "alice", "POST", "/api/v1/workspaces/west/projects", map[string]any{"key": "WST", "name": "West"})
	westItems := "/api/v1/workspaces/west/projects/WST/items"
	alice := e.userID("alice")
	east, west := e.wsToday("acme"), e.wsToday("west")
	if !east.After(west) {
		t.Fatalf("time zones: east %v, west %v", east, west)
	}

	// West's today: due today there (this week), already past in the east.
	e.must(201, "alice", "POST", items, map[string]any{"title": "Şərq", "assignee_id": alice, "due_date": ymd(west)})     // ACME-1
	e.must(201, "alice", "POST", westItems, map[string]any{"title": "Qərb", "assignee_id": alice, "due_date": ymd(west)}) // WST-1
	// The edges of the 7-day window: today+6 is in, today+7 is not.
	e.must(201, "alice", "POST", items, map[string]any{"title": "Son gün", "assignee_id": alice, "due_date": ymd(east.AddDate(0, 0, 6))}) // ACME-2
	e.must(201, "alice", "POST", items, map[string]any{"title": "Sonra", "assignee_id": alice, "due_date": ymd(east.AddDate(0, 0, 7))})   // ACME-3

	h := e.home("alice", "")
	if want := map[string]float64{"overdue": 1, "approvals": 0, "week": 2, "assigned": 4}; !mapsEqual(h.counters, want) {
		t.Fatalf("counters %v, want %v", h.counters, want)
	}
	if want := []string{"acme/ACME-1:overdue", "west/WST-1:week", "acme/ACME-2:week"}; !slices.Equal(h.keys, want) {
		t.Fatalf("list %v, want %v", h.keys, want)
	}
	if h.workspaces["west"]["today"] != ymd(west) || h.workspaces["acme"]["today"] != ymd(east) {
		t.Fatalf("local dates: %v / %v", h.workspaces["west"]["today"], h.workspaces["acme"]["today"])
	}
}

func mapsEqual(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
