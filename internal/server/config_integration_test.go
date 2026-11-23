package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

const wfBase = "/api/v1/workspaces/acme/workflows"

func statusID(t *testing.T, wf map[string]any, name string) string {
	t.Helper()
	for _, s := range wf["statuses"].([]any) {
		if s.(map[string]any)["name"] == name {
			return s.(map[string]any)["id"].(string)
		}
	}
	t.Fatalf("status %q not found in %v", name, wf)
	return ""
}

func TestWorkflowManagement(t *testing.T) {
	e := newEnv(t)
	e.setup()

	created := e.must(201, "alice", "POST", wfBase, map[string]any{
		"name": "Hiring", "statuses": []map[string]any{
			{"name": "Applied", "category": "todo"},
			{"name": "Interview", "category": "in_progress"},
			{"name": "Hired", "category": "done"},
		}}).Body
	wfID := created["id"].(string)
	sts := created["statuses"].([]any)
	if len(sts) != 3 || sts[0].(map[string]any)["is_initial"] != true || sts[1].(map[string]any)["is_initial"] != false {
		t.Fatalf("created workflow: %v", created)
	}
	path := wfBase + "/" + wfID

	t.Run("validation and conflicts", func(t *testing.T) {
		cases := []struct {
			body any
			want int
		}{
			{map[string]any{"name": "Hiring", "statuses": []map[string]any{{"name": "A", "category": "todo"}}}, 409},
			{map[string]any{"name": "Empty", "statuses": []map[string]any{}}, 422},
			{map[string]any{"name": "Dup", "statuses": []map[string]any{{"name": "A", "category": "todo"}, {"name": "a", "category": "done"}}}, 422},
			{map[string]any{"name": "Cat", "statuses": []map[string]any{{"name": "A", "category": "maybe"}}}, 422},
		}
		for i, c := range cases {
			if r := e.call("alice", "POST", wfBase, c.body); r.Status != c.want {
				t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
			}
		}
	})

	t.Run("permissions", func(t *testing.T) {
		e.must(200, "bob", "GET", wfBase, nil) // guests may read
		for _, tok := range []string{"bob", "carol"} {
			if r := e.call(tok, "POST", path+"/statuses", map[string]any{"name": "X", "category": "todo"}); r.Status != 403 {
				t.Errorf("%s add status: %d", tok, r.Status)
			}
		}
		if r := e.call("dave", "GET", wfBase, nil); r.Status != 404 {
			t.Errorf("non-member: %d", r.Status)
		}
		if r := e.call("alice", "PATCH", wfBase+"/not-a-uuid", map[string]any{"name": "x"}); r.Status != 404 {
			t.Errorf("bad id: %d", r.Status)
		}
	})

	t.Run("statuses", func(t *testing.T) {
		wf := e.must(201, "alice", "POST", path+"/statuses", map[string]any{"name": "Rejected", "category": "canceled"}).Body
		rejected := statusID(t, wf, "Rejected")
		if r := e.call("alice", "POST", path+"/statuses", map[string]any{"name": "Rejected", "category": "done"}); r.Status != 409 {
			t.Errorf("duplicate status: %d", r.Status)
		}

		// Move the initial marker; exactly one status stays initial.
		wf = e.must(200, "alice", "PATCH", path+"/statuses/"+statusID(t, wf, "Interview"), map[string]any{"is_initial": true}).Body
		initial := 0
		for _, s := range wf["statuses"].([]any) {
			if s.(map[string]any)["is_initial"] == true {
				initial++
				if s.(map[string]any)["name"] != "Interview" {
					t.Errorf("wrong initial: %v", s)
				}
			}
		}
		if initial != 1 {
			t.Fatalf("initial statuses = %d", initial)
		}
		if r := e.call("alice", "PATCH", path+"/statuses/"+rejected, map[string]any{"is_initial": false}); r.Status != 422 {
			t.Errorf("unset initial: %d", r.Status)
		}
		if r := e.call("alice", "DELETE", path+"/statuses/"+statusID(t, wf, "Interview"), nil); r.Status != 409 || errCode(r) != "status_is_initial" {
			t.Errorf("delete initial: %d %s", r.Status, r.Raw)
		}
		wf = e.must(200, "alice", "PATCH", path+"/statuses/"+rejected, map[string]any{"name": "Declined", "position": 0}).Body
		if wf["statuses"].([]any)[0].(map[string]any)["name"] != "Applied" && wf["statuses"].([]any)[0].(map[string]any)["name"] != "Declined" {
			t.Errorf("reorder: %v", wf["statuses"])
		}
		e.must(200, "alice", "DELETE", path+"/statuses/"+rejected, nil)
		e.must(404, "alice", "DELETE", path+"/statuses/"+rejected, nil)
	})

	t.Run("item type on a custom workflow, status in use", func(t *testing.T) {
		it := e.must(201, "alice", "POST", "/api/v1/workspaces/acme/item-types",
			map[string]any{"key": "candidate", "name": "Candidate", "icon": "user", "workflow_id": wfID}).Body
		if it["workflow_id"] != wfID || it["icon"] != "user" || len(it["statuses"].([]any)) != 3 {
			t.Fatalf("item type: %v", it)
		}
		item := e.must(201, "carol", "POST", items, map[string]any{"title": "Jane Doe", "type": "candidate"}).Body
		if item["status"].(map[string]any)["name"] != "Interview" {
			t.Fatalf("new item must start in the initial status: %v", item["status"])
		}
		wf := e.must(200, "carol", "GET", wfBase, nil).Body["workflows"].([]any)
		var hiring map[string]any
		for _, w := range wf {
			if w.(map[string]any)["id"] == wfID {
				hiring = w.(map[string]any)
			}
		}
		// Make another status initial so Interview is deletable except for being in use.
		e.must(200, "alice", "PATCH", path+"/statuses/"+statusID(t, hiring, "Applied"), map[string]any{"is_initial": true})
		if r := e.call("alice", "DELETE", path+"/statuses/"+statusID(t, hiring, "Interview"), nil); r.Status != 409 || errCode(r) != "status_in_use" {
			t.Fatalf("delete used status: %d %s", r.Status, r.Raw)
		}
		// A status from another workflow cannot be set on this item type.
		var todo string
		for _, w := range wf {
			if w.(map[string]any)["name"] == "Default" {
				todo = statusID(t, w.(map[string]any), "To Do")
			}
		}
		if r := e.call("carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": todo}); r.Status != 422 {
			t.Fatalf("cross-workflow status: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("rename", func(t *testing.T) {
		wf := e.must(200, "alice", "PATCH", path, map[string]any{"name": "Recruiting"}).Body
		if wf["name"] != "Recruiting" {
			t.Fatalf("rename: %v", wf)
		}
		if r := e.call("alice", "PATCH", path, map[string]any{"name": "Default"}); r.Status != 409 {
			t.Errorf("rename to taken name: %d", r.Status)
		}
	})
}

func TestItemTypeManagement(t *testing.T) {
	e := newEnv(t)
	e.setup()
	types := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any)
	wfID := types[0].(map[string]any)["workflow_id"].(string)
	base := "/api/v1/workspaces/acme/item-types"

	for name, c := range map[string]struct {
		body any
		want int
	}{
		"bad key":          {map[string]any{"key": "Epic", "name": "Epic", "workflow_id": wfID}, 422},
		"taken key":        {map[string]any{"key": "task", "name": "Task 2", "workflow_id": wfID}, 409},
		"unknown workflow": {map[string]any{"key": "epic", "name": "Epic", "workflow_id": "01a20727-0000-7000-8000-000000000000"}, 422},
		"long icon":        {map[string]any{"key": "epic", "name": "Epic", "workflow_id": wfID, "icon": string(make([]byte, 51))}, 422},
	} {
		if r := e.call("alice", "POST", base, c.body); r.Status != c.want {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	if r := e.call("carol", "POST", base, map[string]any{"key": "epic", "name": "Epic", "workflow_id": wfID}); r.Status != 403 {
		t.Errorf("member create: %d", r.Status)
	}

	e.must(201, "alice", "POST", base, map[string]any{"key": "epic", "name": "Epic", "workflow_id": wfID})
	it := e.must(200, "alice", "PATCH", base+"/epic", map[string]any{"name": "Initiative", "icon": "flag"}).Body
	if it["name"] != "Initiative" || it["icon"] != "flag" || it["key"] != "epic" {
		t.Fatalf("update: %v", it)
	}
	it = e.must(200, "alice", "PATCH", base+"/epic", map[string]any{"icon": nil}).Body
	if it["icon"] != nil {
		t.Fatalf("clear icon: %v", it)
	}
	e.must(404, "alice", "PATCH", base+"/nope", map[string]any{"name": "x"})

	// Archive: hidden from lists, rejected for new items, existing items untouched.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Big thing", "type": "epic"})
	e.must(204, "alice", "DELETE", base+"/epic", nil)
	e.must(404, "alice", "DELETE", base+"/epic", nil)
	if r := e.call("carol", "POST", items, map[string]any{"title": "x", "type": "epic"}); r.Status != 422 {
		t.Errorf("create with archived type: %d", r.Status)
	}
	if got := e.must(200, "carol", "GET", items+"/1", nil).Body["type"].(map[string]any)["key"]; got != "epic" {
		t.Errorf("existing item lost its type: %v", got)
	}
	if r := e.call("alice", "POST", base, map[string]any{"key": "epic", "name": "Again", "workflow_id": wfID}); r.Status != 409 {
		t.Errorf("reuse archived key: %d", r.Status)
	}

	// The last active type cannot be archived.
	e.must(204, "alice", "DELETE", base+"/bug", nil)
	if r := e.call("alice", "DELETE", base+"/task", nil); r.Status != 409 || errCode(r) != "last_item_type" {
		t.Fatalf("archive last type: %d %s", r.Status, r.Raw)
	}
}

func TestConfigChangesAreAudited(t *testing.T) {
	e := newEnv(t)
	e.setup()
	wf := e.must(201, "alice", "POST", wfBase, map[string]any{"name": "Ops", "statuses": []map[string]any{{"name": "Open", "category": "todo"}}}).Body
	e.must(201, "alice", "POST", wfBase+"/"+wf["id"].(string)+"/statuses", map[string]any{"name": "Closed", "category": "done"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/item-types", map[string]any{"key": "incident", "name": "Incident", "workflow_id": wf["id"]})
	var n int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE action IN
		('workflow.created', 'status.created', 'item_type.created') AND actor_id IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("audit events = %d, want 3", n)
	}
}

func TestRateLimiting(t *testing.T) {
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:        observability.NewMetrics(),
		DB:             pool,
		Store:          store.New(pool),
		Verifier:       testUsers,
		RequestTimeout: 10 * time.Second,
		RateLimit:      config.RateLimit{UserRPS: 1, UserBurst: 3, IPRPS: 1000, IPBurst: 1000},
	})
	call := func(token, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/me", nil)
		req.RemoteAddr = ip + ":40000"
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := range 3 {
		if rec := call("alice", "192.0.2.1"); rec.Code != 200 {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	rec := call("alice", "192.0.2.2") // a different IP does not reset the user's budget
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over user limit: %d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := call("bob", "192.0.2.1"); rec.Code != 200 {
		t.Fatalf("other user must not be limited: %d", rec.Code)
	}

	// Per-IP limit applies before authentication (protects token verification).
	h = NewAPIHandler(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(),
		DB: pool, Store: store.New(pool), Verifier: testUsers, RequestTimeout: 10 * time.Second,
		RateLimit: config.RateLimit{IPRPS: 1, IPBurst: 2},
	})
	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for range 10 {
		wg.Go(func() { codes <- call("forged-token", "198.51.100.9").Code })
	}
	wg.Wait()
	close(codes)
	limited := 0
	for c := range codes {
		if c == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 8 {
		t.Fatalf("per-IP limit: %d of 10 limited, want 8", limited)
	}
	// Health checks are never rate limited.
	for range 5 {
		req := httptest.NewRequestWithContext(t.Context(), "GET", "/healthz", nil)
		req.RemoteAddr = "198.51.100.9:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("healthz limited: %d", rec.Code)
		}
	}
}

// Behind a trusted proxy each client gets its own IP bucket; a client that
// talks to the API directly cannot pick a bucket by forging X-Forwarded-For.
func TestRateLimitingBehindTrustedProxy(t *testing.T) {
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(),
		DB: pool, Store: store.New(pool), Verifier: testUsers, RequestTimeout: 10 * time.Second,
		RateLimit:      config.RateLimit{IPRPS: 0.001, IPBurst: 1},
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
	})
	call := func(peer, xff string) int {
		req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/me", nil)
		req.RemoteAddr = peer + ":40000"
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		req.Header.Set("Authorization", "Bearer alice")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// Two clients through the same proxy: separate buckets.
	if c := call("10.0.0.2", "198.51.100.1"); c != 200 {
		t.Fatalf("client 1: %d", c)
	}
	if c := call("10.0.0.2", "198.51.100.2"); c != 200 {
		t.Fatalf("client 2 shares the proxy but not the bucket: %d", c)
	}
	if c := call("10.0.0.2", "6.6.6.6, 198.51.100.1"); c != http.StatusTooManyRequests {
		t.Fatalf("client 1 prepending a fake hop must stay in its bucket: %d", c)
	}
	// Direct (untrusted) peer: XFF is ignored, the peer's bucket is used.
	if c := call("203.0.113.5", "198.51.100.3"); c != 200 {
		t.Fatalf("direct client: %d", c)
	}
	if c := call("203.0.113.5", "198.51.100.4"); c != http.StatusTooManyRequests {
		t.Fatalf("forged XFF from an untrusted peer must not grant a new bucket: %d", c)
	}
}
