package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A token restricted to projects never widens to the workspace when its
// projects are deleted (ADR-0043): the purge revokes a token whose last
// project goes, and a restricted token left with an empty list (a deletion
// before migration 00036) reaches nothing.
func TestServiceTokenRestrictionSurvivesProjectDeletion(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Maliyyə", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli maaş cədvəli"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq iş"})
	a := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "AAA", "name": "Layihə A"}).Body
	ops := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "OPS", "name": "Ops"}).Body
	scopes := []string{"items:read", "reports:read"}

	onlyA, onlyAID := e.mintToken(map[string]any{"role": "admin", "scopes": scopes, "project_ids": []string{a["id"].(string)}})
	both, _ := e.mintToken(map[string]any{"role": "admin", "scopes": scopes, "project_ids": []string{a["id"].(string), ops["id"].(string)}})
	e.must(200, onlyA, "GET", projectsPath+"/AAA/items", nil)

	// Archive, then delete A (purged inline: it is empty).
	e.must(200, "alice", "POST", projectsPath+"/AAA/archive", nil)
	if r := e.must(200, "alice", "DELETE", projectsPath+"/AAA", map[string]any{"confirm": "AAA"}).Body; r["status"] != "deleted" {
		t.Fatalf("delete: %v", r)
	}

	// The token whose only project was A is revoked, audited, its principal inactive.
	for _, path := range []string{items, secItems, projectsPath, "/api/v1/workspaces/acme/reports"} {
		if r := e.call(onlyA, "GET", path, nil); r.Status != 401 {
			t.Errorf("token of a deleted project reads %s: %d %s", path, r.Status, r.Raw)
		}
	}
	if n := e.countEvents("service_token.revoked", onlyAID); n != 1 {
		t.Errorf("revocation events: %d", n)
	}
	var active bool
	if err := e.pool.QueryRow(context.Background(), `SELECT u.is_active FROM service_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.id = $1`, onlyAID).Scan(&active); err != nil || active {
		t.Errorf("principal still active: %v %v", active, err)
	}
	// A token with another project left keeps exactly that project.
	e.must(200, both, "GET", projectsPath+"/OPS/items", nil)
	e.must(404, both, "GET", items, nil)
	e.must(404, both, "GET", secItems, nil)

	// The empty-list rule on its own: a restricted token whose rows are gone
	// (as after a deletion before the purge revoked tokens) sees nothing.
	stale, staleID := e.mintToken(map[string]any{"role": "admin", "scopes": scopes, "project_ids": []string{ops["id"].(string)}})
	if _, err := e.pool.Exec(context.Background(), `DELETE FROM service_token_projects WHERE token_id = $1`, staleID); err != nil {
		t.Fatal(err)
	}
	e.must(404, stale, "GET", items, nil)
	e.must(404, stale, "GET", secItems, nil)
	e.must(404, stale, "GET", projectsPath+"/OPS/items", nil)
	if r := e.must(200, stale, "GET", projectsPath, nil); strings.Contains(r.Raw, `"key"`) {
		t.Errorf("projects of an empty restricted token: %s", r.Raw)
	}
	if r := e.call(stale, "GET", "/api/v1/workspaces/acme/reports", nil); r.Status != 403 || errCode(r) != "token_project_restricted" {
		t.Errorf("reports of an empty restricted token: %d %s", r.Status, r.Raw)
	}
	if r := e.must(200, "alice", "GET", tokensPath, nil); !strings.Contains(r.Raw, `"project_restricted":true`) {
		t.Errorf("restriction not shown to admins: %s", r.Raw)
	}
	// An unrestricted token is unaffected.
	admin, _ := e.mintToken(map[string]any{"role": "admin", "scopes": scopes})
	e.must(200, admin, "GET", secItems, nil)
}

// Feeds and link lists never carry the title of an item the viewer cannot
// see: not for a member outside a private project (ADR-0013), not for a
// token outside its allow-list, even with the admin role (ADR-0043).
func TestLinkLabelsRespectVisibility(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Maliyyə", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli maaş cədvəli"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq iş"})
	e.must(201, "alice", "POST", items+"/1/links", map[string]any{"type": "relates", "target": "SEC-1"})
	acme := e.must(200, "alice", "GET", projectsPath+"/ACME", nil).Body["id"].(string)

	restricted, _ := e.mintToken(map[string]any{"role": "admin", "scopes": []string{"items:read"}, "project_ids": []string{acme}})
	admin, _ := e.mintToken(map[string]any{"role": "admin", "scopes": []string{"items:read"}})
	// Tokens read activity, not the links route; people read both.
	hidden := map[string][]string{
		"carol":    {items + "/1/activity", items + "/1/links"},
		restricted: {items + "/1/activity"},
	}
	for who, paths := range hidden {
		for _, path := range paths {
			r := e.must(200, who, "GET", path, nil)
			if strings.Contains(r.Raw, "Gizli") || strings.Contains(r.Raw, "SEC-1") {
				t.Errorf("hidden item leaks in %s: %s", path, r.Raw)
			}
		}
	}
	// Those who can see SEC get the label.
	for _, who := range []string{"alice", admin} {
		if r := e.must(200, who, "GET", items+"/1/activity", nil); !strings.Contains(r.Raw, "Gizli maaş cədvəli") {
			t.Errorf("visible label missing: %s", r.Raw)
		}
	}
	if r := e.must(200, "alice", "GET", items+"/1/links", nil); !strings.Contains(r.Raw, "SEC-1") {
		t.Errorf("visible link missing: %s", r.Raw)
	}
}

// Purging a workspace deactivates its token principals: their users rows
// outlive the workspace for history but can never act again.
func TestWorkspacePurgeDeactivatesTokenPrincipals(t *testing.T) {
	e := newP1Env(t)
	e.archiveSetup()
	ctx := context.Background()
	_, tokenID := e.mintToken(nil)
	var userID string
	if err := e.pool.QueryRow(ctx, `SELECT user_id::text FROM service_tokens WHERE id = $1`, tokenID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	wsID := e.must(200, "alice", "GET", wsBase, nil).Body["id"].(string)
	e.must(200, "alice", "POST", wsBase+"/archive", nil)
	if _, err := e.st.RequestWorkspaceDeletion(ctx, mustUUID(t, wsID), mustUUID(t, e.userID("alice")), "acme", true); err != nil {
		t.Fatal(err)
	}
	if done, err := e.st.PurgeWorkspace(ctx, mustUUID(t, wsID), time.Now().Add(5*time.Second)); err != nil || !done {
		t.Fatalf("purge: %v %v", done, err)
	}
	if n := e.count(`SELECT count(*) FROM users WHERE id = $1 AND NOT is_active`, userID); n != 1 {
		t.Errorf("token principal still active after purge: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM users WHERE email = 'alice@example.com' AND is_active`); n != 1 {
		t.Errorf("people deactivated: %d", n)
	}
}
