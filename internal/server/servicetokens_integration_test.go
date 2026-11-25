package server

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

const tokensPath = "/api/v1/workspaces/acme/service-tokens"

// newTokenEnv is newEnv with a per-token rate limit.
func newTokenEnv(t *testing.T, rl config.RateLimit) *env {
	t.Helper()
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(),
		DB: pool, Store: store.New(pool), Verifier: testUsers, Version: "test",
		RequestTimeout: 10 * time.Second, RateLimit: rl,
	})
	return &env{t: t, h: h, pool: pool}
}

// mintToken creates a token as alice and returns the secret string and id.
func (e *env) mintToken(body map[string]any) (string, string) {
	e.t.Helper()
	b := map[string]any{"name": "1C inteqrasiyası", "role": "member", "scopes": []string{"items:read", "items:write"}, "expires_in_days": 30}
	for k, v := range body {
		b[k] = v
	}
	r := e.must(201, "alice", "POST", tokensPath, b)
	tok, _ := r.Body["token"].(string)
	id, _ := r.Body["id"].(string)
	if !strings.HasPrefix(tok, "nbl_") {
		e.t.Fatalf("token: %s", r.Raw)
	}
	return tok, id
}

func (e *env) countEvents(action, tokenID string) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE action = $1 AND entity_id = $2`, action, tokenID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestServiceTokenManagement(t *testing.T) {
	e := newEnv(t)
	e.setup()
	body := map[string]any{"name": "x", "role": "member", "scopes": []string{"items:read"}, "expires_in_days": 30}
	for _, u := range []string{"bob", "carol"} {
		e.must(403, u, "POST", tokensPath, body)
		e.must(403, u, "GET", tokensPath, nil)
	}
	for name, b := range map[string]map[string]any{
		"owner role":    {"role": "owner"},
		"unknown scope": {"scopes": []string{"admin:all"}},
		"no scope":      {"scopes": []string{}},
		"guest write":   {"role": "guest", "scopes": []string{"items:write"}},
		"no expiry":     {"expires_in_days": 0},
		"too long":      {"expires_in_days": 366},
		"no name":       {"name": "  "},
		"bad project":   {"project_ids": []string{"00000000-0000-0000-0000-000000000001"}},
	} {
		full := map[string]any{}
		for k, v := range body {
			full[k] = v
		}
		for k, v := range b {
			full[k] = v
		}
		if r := e.call("alice", "POST", tokensPath, full); r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}

	tok, id := e.mintToken(nil)
	list := e.must(200, "alice", "GET", tokensPath, nil)
	_, secret, _ := strings.Cut(strings.TrimPrefix(tok, "nbl_"), "_")
	if strings.Contains(list.Raw, secret) || strings.Contains(list.Raw, "secret_hash") || !strings.Contains(list.Raw, "🔑 1C inteqrasiyası") {
		t.Fatalf("list leaks the secret or lacks the token: %s", list.Raw)
	}
	var stored []byte
	if err := e.pool.QueryRow(t.Context(), `SELECT secret_hash FROM service_tokens WHERE id = $1`, id).Scan(&stored); err != nil || len(stored) != 32 {
		t.Fatalf("hash: %v %d", err, len(stored))
	}
	var payload string
	if err := e.pool.QueryRow(t.Context(), `SELECT payload::text FROM events WHERE action = 'service_token.created' AND entity_id = $1`, id).Scan(&payload); err != nil || strings.Contains(payload, secret) {
		t.Fatalf("created event: %v %s", err, payload)
	}

	// The principal is never a workspace member: it cannot be added by
	// its placeholder email and does not show in the member list.
	var email string
	if err := e.pool.QueryRow(t.Context(), `SELECT u.email FROM users u JOIN service_tokens t ON t.user_id = u.id WHERE t.id = $1`, id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	e.must(422, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": email, "role": "member"})
	if r := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/members", nil); strings.Contains(r.Raw, "🔑") {
		t.Fatalf("token in member list: %s", r.Raw)
	}

	// Use: create an item; the token is the actor and "last used" is set.
	item := e.must(201, tok, "POST", items, map[string]any{"title": "Bank çıxarışı 2026-11"}).Body
	num := int(item["number"].(float64))
	act := e.must(200, "alice", "GET", items+"/"+strconv.Itoa(num)+"/activity", nil)
	if !strings.Contains(act.Raw, "🔑 1C inteqrasiyası") {
		t.Fatalf("actor not attributed: %s", act.Raw)
	}
	list = e.must(200, "alice", "GET", tokensPath, nil)
	tk := list.Body["tokens"].([]any)[0].(map[string]any)
	if tk["last_used_at"] == nil || tk["last_used_ip"] == nil {
		t.Fatalf("last used not recorded: %s", list.Raw)
	}

	// Revocation is immediate; revoking again is harmless.
	e.must(204, "alice", "DELETE", tokensPath+"/"+id, nil)
	e.must(204, "alice", "DELETE", tokensPath+"/"+id, nil)
	if r := e.call(tok, "GET", items, nil); r.Status != 401 || errCode(r) != "token_revoked" {
		t.Fatalf("revoked: %d %s", r.Status, r.Raw)
	}
	if e.countEvents("service_token.revoked", id) != 1 || e.countEvents("service_token.rejected", id) != 1 {
		t.Fatal("revoke audit events")
	}
	e.must(404, "alice", "DELETE", tokensPath+"/00000000-0000-0000-0000-000000000001", nil)
}

func TestServiceTokenScopesAndRoutes(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "Seed"})
	read, readID := e.mintToken(map[string]any{"name": "BI", "role": "guest", "scopes": []string{"items:read"}})
	full, _ := e.mintToken(map[string]any{"name": "ERP", "role": "admin",
		"scopes": []string{"items:read", "items:write", "comments:write", "reports:read", "export:read"}})

	e.must(200, read, "GET", "/api/v1/me", nil)
	e.must(200, read, "GET", "/api/v1/workspaces/acme", nil)
	e.must(200, read, "GET", items, nil)
	e.must(200, read, "GET", items+"/1", nil)
	e.must(200, read, "GET", "/api/v1/workspaces/acme/fields", nil)
	for _, c := range []struct{ method, path, code string }{
		{"POST", items, "token_scope_missing"},
		{"POST", items + "/1/comments", "token_scope_missing"},
		{"GET", "/api/v1/workspaces/acme/reports", "token_scope_missing"},
		{"GET", items + "/export?format=csv", "token_scope_missing"},
	} {
		if r := e.call(read, c.method, c.path, map[string]any{"title": "x", "body": "x"}); r.Status != 403 || errCode(r) != c.code {
			t.Errorf("read token %s %s: %d %s", c.method, c.path, r.Status, r.Raw)
		}
	}

	e.must(201, full, "POST", items, map[string]any{"title": "From ERP"})
	e.must(200, full, "PATCH", items+"/1", map[string]any{"version": 1, "title": "Renamed by ERP"})
	e.must(201, full, "POST", items+"/1/comments", map[string]any{"body": "Ödəniş qəbul edildi"})
	e.must(200, full, "GET", "/api/v1/workspaces/acme/reports", nil)
	e.must(200, full, "GET", items+"/export?format=csv", nil)

	// Even an admin token never reaches management, lifecycle, members,
	// settings or other workspaces' lists.
	for _, c := range []struct{ method, path string }{
		{"GET", tokensPath},
		{"POST", tokensPath},
		{"GET", "/api/v1/workspaces"},
		{"POST", "/api/v1/workspaces"},
		{"PATCH", "/api/v1/workspaces/acme"},
		{"POST", "/api/v1/workspaces/acme/archive"},
		{"DELETE", "/api/v1/workspaces/acme"},
		{"POST", "/api/v1/workspaces/acme/projects/ACME/archive"},
		{"GET", "/api/v1/workspaces/acme/members"},
		{"POST", "/api/v1/workspaces/acme/members"},
		{"GET", "/api/v1/workspaces/acme/invitations"},
		{"GET", "/api/v1/workspaces/acme/email-settings"},
		{"GET", "/api/v1/workspaces/acme/webhooks"},
		{"DELETE", items + "/1"},
		{"POST", "/api/v1/workspaces/acme/projects"},
		{"GET", "/api/v1/me/notifications"},
		{"GET", "/api/v1/me/home"},
	} {
		if r := e.call(full, c.method, c.path, map[string]any{}); r.Status != 403 || errCode(r) != "token_route_forbidden" {
			t.Errorf("admin token %s %s: %d %s", c.method, c.path, r.Status, r.Raw)
		}
	}

	// Denials of a valid token are audited, throttled per reason.
	if n := e.countEvents("service_token.denied", readID); n != 1 {
		t.Fatalf("denied events: %d", n)
	}
	var actor, display string
	if err := e.pool.QueryRow(t.Context(), `SELECT e.actor_id::text, u.display_name FROM events e JOIN users u ON u.id = e.actor_id
		WHERE e.action = 'service_token.denied' AND e.entity_id = $1`, readID).Scan(&actor, &display); err != nil || display != "🔑 BI" {
		t.Fatalf("denied actor: %v %q", err, display)
	}

	// Malformed or forged tokens never reach the OIDC verifier and say nothing.
	for _, bad := range []string{"nbl_garbage", read[:len(read)-1] + "0", read[:len(read)-1] + "1"} {
		if bad == read {
			continue
		}
		if r := e.call(bad, "GET", items, nil); r.Status != 401 || errCode(r) != "invalid_token" {
			t.Errorf("forged %q: %d %s", bad, r.Status, r.Raw)
		}
	}
}

func TestServiceTokenProjectsAndWorkspaces(t *testing.T) {
	e := newEnv(t)
	e.setup()
	priv := e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "FIN", "name": "Maliyyə", "visibility": "private"}).Body
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"})
	finItems := "/api/v1/workspaces/acme/projects/FIN/items"

	// No allow-list: the role decides. A member token does not see private projects.
	member, _ := e.mintToken(nil)
	e.must(404, member, "GET", finItems, nil)
	if r := e.must(200, member, "GET", "/api/v1/workspaces/acme/projects", nil); strings.Contains(r.Raw, "FIN") || !strings.Contains(r.Raw, "OPS") {
		t.Fatalf("member token projects: %s", r.Raw)
	}
	admin, _ := e.mintToken(map[string]any{"role": "admin"})
	e.must(200, admin, "GET", finItems, nil)

	// Allow-list: exactly the listed projects, private ones included.
	listed, listedID := e.mintToken(map[string]any{"project_ids": []string{priv["id"].(string)}, "scopes": []string{"items:read", "items:write", "reports:read"}})
	e.must(201, listed, "POST", finItems, map[string]any{"title": "Faktura"})
	e.must(404, listed, "GET", items, nil)
	e.must(404, listed, "GET", "/api/v1/workspaces/acme/projects/OPS", nil)
	if r := e.must(200, listed, "GET", "/api/v1/workspaces/acme/projects", nil); !strings.Contains(r.Raw, "FIN") || strings.Contains(r.Raw, "OPS") || strings.Contains(r.Raw, "ACME") {
		t.Fatalf("listed token projects: %s", r.Raw)
	}
	if r := e.call(listed, "GET", "/api/v1/workspaces/acme/reports", nil); r.Status != 403 || errCode(r) != "token_project_restricted" {
		t.Fatalf("restricted reports: %d %s", r.Status, r.Raw)
	}
	if e.countEvents("service_token.denied", listedID) != 2 { // project_not_allowed + project_restricted
		t.Fatalf("project denials: %d", e.countEvents("service_token.denied", listedID))
	}
	tokens := e.must(200, "alice", "GET", tokensPath, nil)
	if !strings.Contains(tokens.Raw, `"key":"FIN"`) {
		t.Fatalf("allow-list in list: %s", tokens.Raw)
	}

	// Another workspace is invisible even to its own members' tokens.
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "beta", "name": "Beta"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/beta/projects", map[string]any{"key": "BETA", "name": "Beta"})
	if r := e.call(admin, "GET", "/api/v1/workspaces/beta/projects/BETA/items", nil); r.Status != 404 {
		t.Fatalf("cross-workspace: %d %s", r.Status, r.Raw)
	}
	e.must(404, admin, "GET", "/api/v1/workspaces/beta", nil)
}

func TestServiceTokenExpiryArchiveAndRateLimit(t *testing.T) {
	e := newTokenEnv(t, config.RateLimit{TokenRPS: 0.001, TokenBurst: 3})
	e.setup()
	tok, id := e.mintToken(nil)
	other, _ := e.mintToken(map[string]any{"name": "Bank"})
	for range 3 {
		e.must(200, tok, "GET", items, nil)
	}
	if r := e.call(tok, "GET", items, nil); r.Status != 429 || r.Raw == "" {
		t.Fatalf("rate limit: %d %s", r.Status, r.Raw)
	}
	e.must(200, other, "GET", items, nil) // limits are per token

	if _, err := e.pool.Exec(t.Context(), `UPDATE service_tokens SET created_at = now() - interval '40 days',
		expires_at = now() - interval '1 day' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if r := e.call(tok, "GET", items, nil); r.Status != 401 || errCode(r) != "token_expired" {
		t.Fatalf("expired: %d %s", r.Status, r.Raw)
	}
	if e.countEvents("service_token.rejected", id) != 1 {
		t.Fatal("expired use not audited")
	}

	e.must(200, "alice", "POST", "/api/v1/workspaces/acme/archive", nil)
	if r := e.call(other, "GET", items, nil); r.Status != 401 || errCode(r) != "token_workspace_archived" {
		t.Fatalf("archived: %d %s", r.Status, r.Raw)
	}
}
