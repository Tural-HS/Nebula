package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// tokenVerifier maps opaque test tokens to identities. Real JWT verification
// is covered in the auth package; here we test everything behind it.
type tokenVerifier map[string]auth.Claims

func (v tokenVerifier) Verify(_ context.Context, raw string) (auth.Claims, error) {
	c, ok := v[raw]
	if !ok {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	return c, nil
}

var testUsers = tokenVerifier{
	"alice": {Issuer: "https://idp.test", Subject: "sub-alice", Email: "alice@example.com", Name: "Alice", EmailVerified: true},
	"bob":   {Issuer: "https://idp.test", Subject: "sub-bob", Email: "bob@example.com", Name: "Bob", EmailVerified: true},
	"carol": {Issuer: "https://idp.test", Subject: "sub-carol", Email: "carol@example.com", Name: "Carol", EmailVerified: true},
	"dave":  {Issuer: "https://idp.test", Subject: "sub-dave", Email: "dave@example.com", Name: "Dave", EmailVerified: true},
	// Invitation tests: erin is invited, eve gets a forwarded link, frank's
	// address is not verified by the IdP.
	"erin":  {Issuer: "https://idp.test", Subject: "sub-erin", Email: "Erin@Example.com", Name: "Erin", EmailVerified: true},
	"eve":   {Issuer: "https://idp.test", Subject: "sub-eve", Email: "eve@example.com", Name: "Eve", EmailVerified: true},
	"frank": {Issuer: "https://idp.test", Subject: "sub-frank", Email: "frank@example.com", Name: "Frank"},
	// Same email as alice, different identity: must be rejected.
	"mallory": {Issuer: "https://idp.test", Subject: "sub-mallory", Email: "alice@example.com", Name: "Mallory", EmailVerified: true},
}

type env struct {
	t    *testing.T
	h    http.Handler
	pool *pgxpool.Pool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:        observability.NewMetrics(),
		DB:             pool,
		Store:          store.New(pool),
		Verifier:       testUsers,
		Version:        "test",
		RequestTimeout: 10 * time.Second,
	})
	return &env{t: t, h: h, pool: pool}
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (e *env) call(token, method, path string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		buf, _ := json.Marshal(b)
		rd = bytes.NewReader(buf)
	}
	req := httptest.NewRequestWithContext(e.t.Context(), method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	r := resp{Status: rec.Code, Raw: rec.Body.String()}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &r.Body)
	}
	return r
}

// must asserts the status code and returns the response.
func (e *env) must(want int, token, method, path string, body any) resp {
	e.t.Helper()
	r := e.call(token, method, path, body)
	if r.Status != want {
		e.t.Fatalf("%s %s as %q: status %d, want %d; body: %s", method, path, token, r.Status, want, r.Raw)
	}
	return r
}

func errCode(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

// setup: alice owns workspace "acme" with project ACME; bob is guest, carol is member.
func (e *env) setup() {
	e.t.Helper()
	for _, u := range []string{"alice", "bob", "carol", "dave"} {
		e.must(200, u, "GET", "/api/v1/me", nil) // first sign-in creates the user
	}
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "acme", "name": "Acme Inc"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "ACME", "name": "Acme"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "bob@example.com", "role": "guest"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "carol@example.com", "role": "member"})
}

const items = "/api/v1/workspaces/acme/projects/ACME/items"

func TestAuthentication(t *testing.T) {
	e := newEnv(t)
	if r := e.call("", "GET", "/api/v1/me", nil); r.Status != 401 || errCode(r) != "unauthenticated" {
		t.Fatalf("no token: %d %s", r.Status, r.Raw)
	}
	if r := e.call("forged", "GET", "/api/v1/me", nil); r.Status != 401 || errCode(r) != "invalid_token" {
		t.Fatalf("bad token: %d %s", r.Status, r.Raw)
	}
	first := e.must(200, "alice", "GET", "/api/v1/me", nil)
	second := e.must(200, "alice", "GET", "/api/v1/me", nil)
	if first.Body["id"] != second.Body["id"] || first.Body["email"] != "alice@example.com" {
		t.Fatalf("identity not stable: %v vs %v", first.Body, second.Body)
	}
	if r := e.call("mallory", "GET", "/api/v1/me", nil); r.Status != 403 || errCode(r) != "identity_conflict" {
		t.Fatalf("email takeover must be blocked: %d %s", r.Status, r.Raw)
	}

	// Deactivation must take effect on the very next request.
	e.must(200, "dave", "GET", "/api/v1/me", nil)
	if _, err := e.pool.Exec(context.Background(), `UPDATE users SET is_active = false WHERE email = 'dave@example.com'`); err != nil {
		t.Fatal(err)
	}
	if r := e.call("dave", "GET", "/api/v1/me", nil); r.Status != 403 || errCode(r) != "user_disabled" {
		t.Fatalf("disabled user: %d %s", r.Status, r.Raw)
	}
}

func TestWorkspacesAndTenantIsolation(t *testing.T) {
	e := newEnv(t)
	e.setup()

	if r := e.call("alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "acme", "name": "Again"}); r.Status != 409 || errCode(r) != "slug_taken" {
		t.Fatalf("duplicate slug: %d %s", r.Status, r.Raw)
	}
	for _, slug := range []string{"A", "-bad", "has space", strings.Repeat("x", 64)} {
		if r := e.call("alice", "POST", "/api/v1/workspaces", map[string]any{"slug": slug, "name": "x"}); r.Status != 422 {
			t.Errorf("slug %q: status %d", slug, r.Status)
		}
	}

	members := e.must(200, "bob", "GET", "/api/v1/workspaces/acme/members", nil).Body["members"].([]any)
	if len(members) != 3 {
		t.Fatalf("members: %v", members)
	}
	if r := e.call("dave", "GET", "/api/v1/workspaces/acme/members", nil); r.Status != 404 {
		t.Fatalf("non-member lists members: %d", r.Status)
	}

	list := e.must(200, "alice", "GET", "/api/v1/workspaces", nil)
	ws := list.Body["workspaces"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("alice workspaces: %s", list.Raw)
	}

	// dave is not a member: every acme URL must look nonexistent (404, not 403).
	for _, path := range []string{"/api/v1/workspaces/acme", "/api/v1/workspaces/acme/projects", items, items + "/1"} {
		if r := e.call("dave", "GET", path, nil); r.Status != 404 {
			t.Errorf("non-member GET %s: %d", path, r.Status)
		}
	}
	if r := e.must(200, "dave", "GET", "/api/v1/workspaces", nil); len(r.Body["workspaces"].([]any)) != 0 {
		t.Errorf("dave must see no workspaces: %s", r.Raw)
	}

	types := e.must(200, "bob", "GET", "/api/v1/workspaces/acme/item-types", nil)
	tl := types.Body["item_types"].([]any)
	if len(tl) != 2 || len(tl[0].(map[string]any)["statuses"].([]any)) != 4 {
		t.Fatalf("default template: %s", types.Raw)
	}
}

func TestRolesAndPermissions(t *testing.T) {
	e := newEnv(t)
	e.setup()

	tests := []struct {
		name, token, method, path string
		body                      any
		want                      int
	}{
		{"member cannot create project", "carol", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"}, 403},
		{"member cannot add members", "carol", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "dave@example.com", "role": "member"}, 403},
		{"guest cannot create items", "bob", "POST", items, map[string]any{"title": "x"}, 403},
		{"guest can list items", "bob", "GET", items, nil, 200},
		{"duplicate member", "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "bob@example.com", "role": "member"}, 409},
		{"unknown user", "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "nobody@example.com", "role": "member"}, 422},
		{"invalid role", "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "dave@example.com", "role": "god"}, 422},
		{"duplicate project key", "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "ACME", "name": "x"}, 409},
		{"invalid project key", "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "a-1", "name": "x"}, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if r := e.call(tt.token, tt.method, tt.path, tt.body); r.Status != tt.want {
				t.Fatalf("status %d, want %d: %s", r.Status, tt.want, r.Raw)
			}
		})
	}

	// Only an owner may grant owner.
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "dave@example.com", "role": "admin"})
	if r := e.call("dave", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "dave@example.com", "role": "owner"}); r.Status != 403 {
		t.Fatalf("admin granted owner: %d %s", r.Status, r.Raw)
	}
}

func TestItemLifecycle(t *testing.T) {
	e := newEnv(t)
	e.setup()
	bobID := e.must(200, "bob", "GET", "/api/v1/me", nil).Body["id"].(string)
	daveID := e.must(200, "dave", "GET", "/api/v1/me", nil).Body["id"].(string)

	created := e.must(201, "carol", "POST", items, map[string]any{"title": "  Fix login page  ", "description": "SSO loop"})
	item := created.Body
	if item["key"] != "ACME-1" || item["title"] != "Fix login page" || item["version"] != 1.0 {
		t.Fatalf("created item: %s", created.Raw)
	}
	if item["status"].(map[string]any)["name"] != "To Do" || item["type"].(map[string]any)["key"] != "task" {
		t.Fatalf("defaults: %s", created.Raw)
	}

	t.Run("create validation", func(t *testing.T) {
		cases := map[string]any{
			"empty title":       map[string]any{"title": "   "},
			"long title":        map[string]any{"title": strings.Repeat("x", 501)},
			"unknown type":      map[string]any{"title": "x", "type": "epic"},
			"unknown field":     map[string]any{"title": "x", "fields": map[string]any{"priority": 1}},
			"non-member assign": map[string]any{"title": "x", "assignee_id": "00000000-0000-0000-0000-000000000001"},
		}
		for name, body := range cases {
			if r := e.call("carol", "POST", items, body); r.Status != 422 {
				t.Errorf("%s: status %d %s", name, r.Status, r.Raw)
			}
		}
		if r := e.call("carol", "POST", items, map[string]any{"title": "x", "assignee_id": daveID}); r.Status != 422 {
			t.Errorf("assign to non-member dave: %d", r.Status)
		}
		if r := e.call("carol", "POST", items, map[string]any{"title": "x", "evil": true}); r.Status != 400 {
			t.Errorf("unknown JSON key: %d", r.Status)
		}
		if r := e.call("carol", "POST", items, `{"title":"x"}{"title":"y"}`); r.Status != 400 {
			t.Errorf("two JSON objects: %d", r.Status)
		}
		if r := e.call("carol", "POST", items, `{"title":"`+strings.Repeat("x", 2<<20)+`"}`); r.Status != 413 {
			t.Errorf("2 MiB body: %d", r.Status)
		}
	})

	t.Run("get", func(t *testing.T) {
		e.must(200, "bob", "GET", items+"/1", nil)
		for _, p := range []string{"/999", "/0", "/-1", "/abc"} {
			if r := e.call("bob", "GET", items+p, nil); r.Status != 404 {
				t.Errorf("GET %s: %d", p, r.Status)
			}
		}
	})

	// Status "Done" id from the item types endpoint.
	var doneID string
	for _, it := range e.must(200, "carol", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			if s.(map[string]any)["name"] == "Done" && it.(map[string]any)["key"] == "task" {
				doneID = s.(map[string]any)["id"].(string)
			}
		}
	}

	t.Run("update with optimistic locking", func(t *testing.T) {
		r := e.must(200, "carol", "PATCH", items+"/1", map[string]any{
			"version": 1, "title": "Fix SSO login", "status_id": doneID, "assignee_id": bobID})
		if r.Body["version"] != 2.0 || r.Body["status"].(map[string]any)["category"] != "done" || r.Body["assignee_id"] != bobID {
			t.Fatalf("update: %s", r.Raw)
		}
		stale := e.call("carol", "PATCH", items+"/1", map[string]any{"version": 1, "title": "lost update"})
		if stale.Status != 409 || errCode(stale) != "version_conflict" {
			t.Fatalf("stale write must conflict: %d %s", stale.Status, stale.Raw)
		}
		if r := e.call("carol", "PATCH", items+"/1", map[string]any{"title": "no version"}); r.Status != 422 {
			t.Errorf("missing version: %d", r.Status)
		}
		if r := e.call("carol", "PATCH", items+"/1", map[string]any{"version": 2, "title": nil}); r.Status != 422 {
			t.Errorf("null title: %d", r.Status)
		}
		if r := e.call("carol", "PATCH", items+"/1", map[string]any{"version": 2, "status_id": "00000000-0000-0000-0000-000000000001"}); r.Status != 422 {
			t.Errorf("foreign status: %d", r.Status)
		}
		// Clearing nullable fields.
		r = e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 2, "description": nil, "assignee_id": nil})
		if r.Body["description"] != nil || r.Body["assignee_id"] != nil || r.Body["version"] != 3.0 {
			t.Fatalf("clear: %s", r.Raw)
		}
		// A no-op PATCH does not bump the version or write an event.
		r = e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 3, "title": "Fix SSO login"})
		if r.Body["version"] != 3.0 {
			t.Fatalf("no-op bumped version: %s", r.Raw)
		}
	})

	t.Run("every mutation is in the event log", func(t *testing.T) {
		var actions []string
		rows, err := e.pool.Query(context.Background(), `SELECT action FROM events WHERE entity_type = 'item' ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a string
			_ = rows.Scan(&a)
			actions = append(actions, a)
		}
		want := []string{"item.created", "item.updated", "item.updated"}
		if !slices.Equal(actions, want) {
			t.Fatalf("events = %v, want %v", actions, want)
		}
		var title string
		_ = e.pool.QueryRow(context.Background(), `SELECT payload->'changes'->'title'->>'from' FROM events
			WHERE action = 'item.updated' ORDER BY id LIMIT 1`).Scan(&title)
		if title != "Fix login page" {
			t.Fatalf("diff payload from = %q", title)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if r := e.call("bob", "DELETE", items+"/1", nil); r.Status != 403 {
			t.Fatalf("guest delete: %d", r.Status)
		}
		e.must(204, "carol", "DELETE", items+"/1", nil)
		e.must(404, "carol", "GET", items+"/1", nil)
		e.must(404, "carol", "DELETE", items+"/1", nil)
		e.must(404, "carol", "PATCH", items+"/1", map[string]any{"version": 4, "title": "zombie"})
	})
}

func TestListPaginationAndFilters(t *testing.T) {
	e := newEnv(t)
	e.setup()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)

	titles := []string{"Login broken", "Invoice export", "Dark mode", "Login timeout", "Search slow", "Payroll report", "Onboarding docs"}
	for i, title := range titles {
		body := map[string]any{"title": title}
		if i%2 == 0 {
			body["assignee_id"] = carolID
		}
		e.must(201, "carol", "POST", items, body)
	}

	// Walk all pages of size 3: newest first, no duplicates, nothing missing.
	var seen []float64
	cursor := ""
	for page := 0; page < 10; page++ {
		path := items + "?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		r := e.must(200, "bob", "GET", path, nil)
		for _, it := range r.Body["items"].([]any) {
			seen = append(seen, it.(map[string]any)["number"].(float64))
		}
		next, _ := r.Body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if !slices.Equal(seen, []float64{7, 6, 5, 4, 3, 2, 1}) {
		t.Fatalf("pagination order = %v", seen)
	}

	count := func(query string) int {
		t.Helper()
		return len(e.must(200, "carol", "GET", items+"?"+query, nil).Body["items"].([]any))
	}
	if n := count("q=login"); n != 2 {
		t.Errorf("q=login: %d", n)
	}
	if n := count("assignee=me"); n != 4 {
		t.Errorf("assignee=me: %d", n)
	}
	if n := count("assignee=none"); n != 3 {
		t.Errorf("assignee=none: %d", n)
	}
	if n := count("q=" + "login%20OR%20payroll"); n != 3 {
		t.Errorf("websearch OR: %d", n)
	}

	for _, bad := range []string{"limit=0", "limit=201", "limit=x", "cursor=%%%", "cursor=djE6LTE", "status=nope", "assignee=xyz"} {
		if r := e.call("carol", "GET", items+"?"+bad, nil); r.Status != 400 {
			t.Errorf("%s: status %d", bad, r.Status)
		}
	}
	// SQL injection attempts are just search text.
	if r := e.call("carol", "GET", items+"?q='%3B%20DROP%20TABLE%20items%3B--", nil); r.Status != 200 {
		t.Fatalf("injection-looking query: %d %s", r.Status, r.Raw)
	}
	e.must(200, "carol", "GET", items, nil)
}

func TestItemCountsByStatus(t *testing.T) {
	e := newEnv(t)
	e.setup()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	for i, title := range []string{"Login broken", "Invoice export", "Login timeout", "Dark mode"} {
		body := map[string]any{"title": title}
		if i%2 == 0 {
			body["assignee_id"] = carolID
		}
		e.must(201, "carol", "POST", items, body)
	}
	// Move #1 to another status of its workflow.
	allowed := e.must(200, "carol", "GET", items+"/1/transitions", nil).Body["statuses"].([]any)
	if len(allowed) == 0 {
		t.Fatal("no transitions from the initial status")
	}
	target := allowed[0].(map[string]any)["id"].(string)
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": target})
	initial := e.must(200, "carol", "GET", items+"/2", nil).Body["status"].(map[string]any)["id"].(string)

	counts := func(query string) (map[string]any, float64) {
		t.Helper()
		b := e.must(200, "bob", "GET", items+"/counts?"+query, nil).Body
		return b["by_status"].(map[string]any), b["total"].(float64)
	}
	by, total := counts("")
	if total != 4 || by[initial] != 3.0 || by[target] != 1.0 {
		t.Fatalf("counts = %v total %v", by, total)
	}
	// Same filters as the list; a status filter does not hide other columns.
	if by, total = counts("q=login&status=" + initial); total != 2 || by[target] != 1.0 {
		t.Fatalf("q=login counts = %v total %v", by, total)
	}
	if _, total = counts("assignee=none"); total != 2 {
		t.Fatalf("assignee=none total %v", total)
	}
	// Deleted items are not counted.
	e.must(204, "carol", "DELETE", items+"/4", nil)
	if _, total = counts(""); total != 3 {
		t.Fatalf("after delete total %v", total)
	}
	if r := e.call("carol", "GET", items+"/counts?assignee=xyz", nil); r.Status != 400 {
		t.Fatalf("bad filter: %d", r.Status)
	}
	// Tenant isolation: an outsider cannot count another workspace's items.
	if r := e.call("dave", "GET", items+"/counts", nil); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}
}

func TestConcurrentCreatesGetUniqueSequentialNumbers(t *testing.T) {
	e := newEnv(t)
	e.setup()
	const n = 25
	var wg sync.WaitGroup
	numbers := make(chan float64, n)
	for i := range n {
		wg.Go(func() {
			r := e.call("carol", "POST", items, map[string]any{"title": fmt.Sprintf("parallel %d", i)})
			if r.Status != 201 {
				t.Errorf("create %d: %d %s", i, r.Status, r.Raw)
				return
			}
			numbers <- r.Body["number"].(float64)
		})
	}
	wg.Wait()
	close(numbers)
	var got []float64
	for num := range numbers {
		got = append(got, num)
	}
	slices.Sort(got)
	for i, num := range got {
		if num != float64(i+1) {
			t.Fatalf("numbers not 1..%d without gaps: %v", n, got)
		}
	}
}

func TestConcurrentUpdatesOnlyOneWins(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "carol", "POST", items, map[string]any{"title": "contended"})
	const n = 10
	var wg sync.WaitGroup
	statuses := make(chan int, n)
	for i := range n {
		wg.Go(func() {
			statuses <- e.call("carol", "PATCH", items+"/1", map[string]any{"version": 1, "title": fmt.Sprintf("writer %d", i)}).Status
		})
	}
	wg.Wait()
	close(statuses)
	ok, conflict := 0, 0
	for s := range statuses {
		switch s {
		case 200:
			ok++
		case 409:
			conflict++
		default:
			t.Errorf("unexpected status %d", s)
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("ok=%d conflict=%d, want exactly one winner", ok, conflict)
	}
}
