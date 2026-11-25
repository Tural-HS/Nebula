package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Archive, restore and permanent deletion (ADR-0039). Synthetic data only:
// alice owns "acme", dave is an admin, carol a member, bob a guest.

func (e *p1env) archiveSetup() {
	e.t.Helper()
	e.setup()
	e.must(201, "alice", "POST", wsBase+"/members", map[string]any{"email": "dave@example.com", "role": "admin"})
}

func TestProjectArchiveRestoreDelete(t *testing.T) {
	e := newP1Env(t)
	e.archiveSetup()
	ctx := context.Background()
	acme := projectsPath + "/ACME"
	projectID := e.must(200, "alice", "GET", acme, nil).Body["id"].(string)
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "OPS", "name": "Ops"})
	e.must(201, "alice", "POST", projectsPath+"/OPS/items", map[string]any{"title": "Bank reconciliation"})

	// Everything that hangs off a project: items, a subtask, a comment, a
	// watcher, a cross-project link, an attachment row, a recurrence, a
	// default view, a form and a webhook filtered to this project only.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Invoice 1001"})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Invoice 1001 lines", "parent": "ACME-1"})
	e.must(200, "bob", "PUT", items+"/1/watchers/me", nil)
	e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "Checked"})
	e.must(201, "carol", "POST", projectsPath+"/OPS/items/1/links", map[string]any{"type": "relates", "target": "ACME-1"})
	itemID := e.must(200, "alice", "GET", items+"/1", nil).Body["id"].(string)
	if _, err := e.pool.Exec(ctx, `INSERT INTO attachments (id, workspace_id, item_id, filename, content_type, size_bytes, sha256, storage_key)
		SELECT gen_random_uuid(), workspace_id, id, 'invoice.pdf', 'application/pdf', 3, sha256('x'), 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' FROM items WHERE id = $1`, itemID); err != nil {
		t.Fatal(err)
	}
	e.must(201, "alice", "POST", acme+"/recurrences", map[string]any{"title": "Daily check", "freq": "daily", "every": 1,
		"start_date": time.Now().AddDate(0, 0, -3).Format(time.DateOnly)})
	view := e.must(201, "alice", "POST", acme+"/views", map[string]any{"name": "All", "kind": "table", "shared": true,
		"config": map[string]any{"columns": []string{"key", "title"}}}).Body["id"].(string)
	e.must(200, "alice", "PATCH", acme, map[string]any{"default_view_id": view})
	e.must(201, "alice", "POST", acme+"/forms", map[string]any{"slug": "it-help", "title": "IT help", "item_type": "task", "title_template": "IT help request",
		"fields": []map[string]any{{"key": "description", "label": "Details"}}, "audience": "members"})
	rc := newReceiver(t, 200)
	hook := e.must(201, "alice", "POST", wsBase+"/webhooks", map[string]any{"url": rc.srv.URL + "/h", "events": []string{"*"},
		"project_ids": []string{projectID}}).Body["webhook"].(map[string]any)["id"].(string)
	e.drain()
	before := rc.count()
	notes := func() int {
		t.Helper()
		return len(e.must(200, "bob", "GET", "/api/v1/me/notifications", nil).Body["notifications"].([]any))
	}
	if notes() == 0 {
		t.Fatal("setup: bob has no notification to hide")
	}

	// Archive: admins only.
	for _, u := range []string{"bob", "carol"} {
		if r := e.call(u, "POST", acme+"/archive", nil); r.Status != 403 {
			t.Fatalf("%s archives: %d", u, r.Status)
		}
	}
	p := e.must(200, "dave", "POST", acme+"/archive", nil).Body
	if p["archived_at"] == nil || p["deleting"] != false {
		t.Fatalf("archived project: %v", p)
	}
	if r := e.call("dave", "POST", acme+"/archive", nil); r.Status != 410 {
		t.Fatalf("archive twice: %d", r.Status)
	}

	// Hidden from lists, search and home; deep links say "archived".
	for _, p := range e.must(200, "carol", "GET", projectsPath, nil).Body["projects"].([]any) {
		if p.(map[string]any)["key"] == "ACME" {
			t.Fatal("archived project listed")
		}
	}
	for _, u := range []string{"bob", "carol", "alice"} {
		if r := e.call(u, "GET", acme, nil); r.Status != 410 || errCode(r) != "project_archived" {
			t.Fatalf("%s opens archived project: %d %s", u, r.Status, r.Raw)
		}
	}
	if r := e.call("carol", "POST", items, map[string]any{"title": "x"}); r.Status != 410 {
		t.Fatalf("write to archived project: %d", r.Status)
	}
	if r := e.call("carol", "GET", items+"/1", nil); r.Status != 410 {
		t.Fatalf("item of archived project: %d", r.Status)
	}
	if hits := e.search("carol", "Invoice"); len(hits.Hits) != 0 {
		t.Fatalf("search finds archived items: %v", hits.Hits)
	}
	if r := e.call("carol", "POST", projectsPath+"/OPS/items/1/links", map[string]any{"type": "blocks", "target": "ACME-2"}); r.Status != 422 {
		t.Fatalf("link to an archived item: %d", r.Status)
	}
	form := e.must(200, "bob", "GET", "/api/v1/forms/it-help", nil).Body["form"].(map[string]any)
	if form["open"] != false || form["closed_reason"] != "archived" {
		t.Fatalf("form of an archived project: %v", form)
	}
	if r := e.call("bob", "POST", "/api/v1/forms/it-help/submissions", submit(map[string]any{"description": "help"})); r.Status != 409 || errCode(r) != "form_closed" {
		t.Fatalf("submit to archived project's form: %d %s", r.Status, r.Raw)
	}
	if n := notes(); n != 0 {
		t.Fatalf("notifications of an archived project listed: %d", n)
	}
	if r := e.call("carol", "GET", wsBase+"/archived-projects", nil); r.Status != 403 {
		t.Fatalf("member lists archived projects: %d", r.Status)
	}
	arch := e.must(200, "dave", "GET", wsBase+"/archived-projects", nil).Body["projects"].([]any)
	if len(arch) != 1 || arch[0].(map[string]any)["key"] != "ACME" {
		t.Fatalf("archived projects: %v", arch)
	}

	// Nothing fires: no recurring items, no webhook deliveries.
	e.eng.Tick(ctx)
	e.drain()
	if n := e.count(`SELECT count(*) FROM items WHERE project_id = $1`, projectID); n != 2 {
		t.Fatalf("recurrence of an archived project created items: %d", n)
	}
	if rc.count() != before {
		t.Fatalf("webhook fired for an archived project: %d deliveries", rc.count()-before)
	}

	// Restore (admin), then delete: only an archived project, only the owner, typed key.
	if r := e.call("carol", "POST", acme+"/restore", nil); r.Status != 403 {
		t.Fatalf("member restores: %d", r.Status)
	}
	if p := e.must(200, "dave", "POST", acme+"/restore", nil).Body; p["archived_at"] != nil {
		t.Fatalf("restored project: %v", p)
	}
	e.must(200, "carol", "GET", acme, nil)
	if r := e.call("dave", "POST", acme+"/restore", nil); r.Status != 409 || errCode(r) != "project_not_archived" {
		t.Fatalf("restore an active project: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "DELETE", acme, map[string]any{"confirm": "ACME"}); r.Status != 409 || errCode(r) != "project_not_archived" {
		t.Fatalf("delete an active project: %d %s", r.Status, r.Raw)
	}
	e.must(200, "alice", "POST", acme+"/archive", nil)
	if r := e.call("dave", "DELETE", acme, map[string]any{"confirm": "ACME"}); r.Status != 403 {
		t.Fatalf("admin deletes: %d", r.Status)
	}
	if r := e.call("alice", "DELETE", acme, map[string]any{"confirm": "acme-1"}); r.Status != 422 {
		t.Fatalf("wrong confirmation: %d", r.Status)
	}
	if r := e.must(200, "alice", "DELETE", acme, map[string]any{"confirm": "ACME"}).Body; r["status"] != "deleted" {
		t.Fatalf("delete: %v", r)
	}

	// Gone, with everything that hung off it; the other project survives.
	if r := e.call("alice", "GET", acme, nil); r.Status != 404 {
		t.Fatalf("deleted project: %d", r.Status)
	}
	if r := e.call("alice", "POST", acme+"/restore", nil); r.Status != 404 {
		t.Fatalf("restore deleted: %d", r.Status)
	}
	for _, q := range []string{
		`SELECT count(*) FROM projects WHERE id = $1`,
		`SELECT count(*) FROM items WHERE project_id = $1`,
		`SELECT count(*) FROM recurrences WHERE project_id = $1`,
		`SELECT count(*) FROM saved_views WHERE project_id = $1`,
		`SELECT count(*) FROM forms WHERE project_id = $1`,
		`SELECT count(*) FROM notifications WHERE project_id = $1`,
		`SELECT count(*) FROM search_docs WHERE project_id = $1`,
		`SELECT count(*) FROM project_members WHERE project_id = $1`,
		`SELECT count(*) FROM attachments a WHERE a.item_id NOT IN (SELECT id FROM items)`,
		`SELECT count(*) FROM comments c WHERE c.item_id NOT IN (SELECT id FROM items)`,
		`SELECT count(*) FROM item_links l WHERE l.source_id NOT IN (SELECT id FROM items) OR l.target_id NOT IN (SELECT id FROM items)`,
		`SELECT count(*) FROM item_watchers w WHERE w.item_id NOT IN (SELECT id FROM items)`,
	} {
		var args []any
		if strings.Contains(q, "$1") {
			args = append(args, projectID)
		}
		if n := e.count(q, args...); n != 0 {
			t.Errorf("%s: %d left", q, n)
		}
	}
	if n := e.count(`SELECT count(*) FROM attachment_blob_purges WHERE storage_key = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'`); n != 1 {
		t.Errorf("blob purge not scheduled: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM items WHERE project_id <> $1 AND deleted_at IS NULL`, projectID); n != 1 {
		t.Errorf("other project's items: %d", n)
	}
	var active bool
	var reason *string
	if err := e.pool.QueryRow(ctx, `SELECT is_active, disabled_reason FROM webhooks WHERE id = $1`, hook).Scan(&active, &reason); err != nil {
		t.Fatal(err)
	}
	if active || reason == nil {
		t.Errorf("webhook filtered to the deleted project stays active (would widen to every project): %v %v", active, reason)
	}
	if n := e.countEvents(`action = 'project.deleted' AND entity_id = $1 AND (payload->>'items')::int = 2`, projectID); n != 1 {
		t.Errorf("project.deleted events: %d", n)
	}
}

func TestPrivateArchivedProjectStaysHidden(t *testing.T) {
	e := newP1Env(t)
	e.archiveSetup()
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Legal", "visibility": "private"})
	e.must(200, "alice", "POST", projectsPath+"/SEC/archive", nil)
	// A member who cannot see the private project gets 404, not "archived".
	if r := e.call("carol", "GET", projectsPath+"/SEC", nil); r.Status != 404 {
		t.Fatalf("private archived project for a non-member: %d", r.Status)
	}
	if r := e.call("carol", "POST", projectsPath+"/SEC/restore", nil); r.Status != 404 {
		t.Fatalf("restore a hidden project: %d", r.Status)
	}
}

func TestWorkspaceArchiveRestoreDelete(t *testing.T) {
	e := newP1Env(t)
	e.archiveSetup()
	ctx := context.Background()
	wsID := e.must(200, "alice", "GET", wsBase, nil).Body["id"].(string)
	e.must(201, "carol", "POST", items, map[string]any{"title": "Payroll"})
	e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "Done"})
	e.must(201, "alice", "POST", projectsPath+"/ACME/recurrences", map[string]any{"title": "Daily", "freq": "daily", "every": 1,
		"start_date": time.Now().AddDate(0, 0, -2).Format(time.DateOnly)})
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "other", "name": "Other"})
	e.must(201, "alice", "POST", wsBase+"/invitations", map[string]any{"email": "erin@example.com", "role": "member"})
	e.drain()
	token := e.lastInviteToken("erin@example.com")

	// Archive: the owner only.
	for _, u := range []string{"carol", "dave"} {
		if r := e.call(u, "POST", wsBase+"/archive", nil); r.Status != 403 {
			t.Fatalf("%s archives the workspace: %d", u, r.Status)
		}
	}
	if w := e.must(200, "alice", "POST", wsBase+"/archive", nil).Body; w["archived_at"] == nil {
		t.Fatalf("archived workspace: %v", w)
	}

	// Hidden from the list and the home page, listed as archived for admins.
	for _, w := range e.must(200, "alice", "GET", "/api/v1/workspaces", nil).Body["workspaces"].([]any) {
		if w.(map[string]any)["slug"] == "acme" {
			t.Fatal("archived workspace listed")
		}
	}
	for _, c := range e.must(200, "carol", "GET", "/api/v1/me/home", nil).Body["workspaces"].([]any) {
		if c.(map[string]any)["slug"] == "acme" {
			t.Fatal("archived workspace on the home page")
		}
	}
	for user, want := range map[string]int{"alice": 1, "dave": 1, "carol": 0, "bob": 0} {
		if got := len(e.must(200, user, "GET", "/api/v1/workspaces?state=archived", nil).Body["workspaces"].([]any)); got != want {
			t.Errorf("%s sees %d archived workspaces, want %d", user, got, want)
		}
	}
	if r := e.call("alice", "GET", "/api/v1/workspaces?state=gone", nil); r.Status != 400 {
		t.Fatalf("bad state: %d", r.Status)
	}

	// Read-only: reads work, writes are refused, evidence exports are not.
	e.must(200, "carol", "GET", items+"/1", nil)
	for _, c := range []struct{ user, method, path string }{
		{"carol", "POST", items},
		{"alice", "PATCH", wsBase},
		{"alice", "POST", projectsPath},
		{"alice", "POST", wsBase + "/invitations"},
		{"carol", "POST", items + "/1/comments"},
		{"alice", "POST", projectsPath + "/ACME/archive"},
	} {
		if r := e.call(c.user, c.method, c.path, map[string]any{"title": "x", "name": "x", "body": "x"}); r.Status != 409 || errCode(r) != "workspace_archived" {
			t.Errorf("%s %s in an archived workspace: %d %s", c.method, c.path, r.Status, r.Raw)
		}
	}
	if r := e.call("alice", "POST", wsBase+"/evidence/exports", map[string]any{}); errCode(r) == "workspace_archived" {
		t.Errorf("evidence export refused in an archived workspace")
	}
	if r := e.call("erin", "POST", "/api/v1/invitations/accept", map[string]any{"token": token}); r.Status != 409 || errCode(r) != "workspace_archived" {
		t.Errorf("accept an invitation to an archived workspace: %d %s", r.Status, r.Raw)
	}
	e.eng.Tick(ctx)
	if n := e.count(`SELECT count(*) FROM items WHERE workspace_id = $1`, wsID); n != 1 {
		t.Errorf("recurrence of an archived workspace created items: %d", n)
	}

	// Restore (admin+), archive again, then delete: owner, typed slug, evidence acknowledged.
	e.must(200, "dave", "POST", wsBase+"/restore", nil)
	e.must(201, "carol", "POST", items, map[string]any{"title": "After restore"})
	if r := e.call("alice", "DELETE", wsBase, map[string]any{"confirm": "acme", "evidence_acknowledged": true}); r.Status != 409 || errCode(r) != "workspace_not_archived" {
		t.Fatalf("delete an active workspace: %d %s", r.Status, r.Raw)
	}
	e.must(200, "alice", "POST", wsBase+"/archive", nil)
	if r := e.call("dave", "DELETE", wsBase, map[string]any{"confirm": "acme", "evidence_acknowledged": true}); r.Status != 403 {
		t.Fatalf("admin deletes the workspace: %d", r.Status)
	}
	for _, body := range []map[string]any{
		{"confirm": "acme"},
		{"confirm": "acme-inc", "evidence_acknowledged": true},
	} {
		if r := e.call("alice", "DELETE", wsBase, body); r.Status != 422 {
			t.Fatalf("delete with %v: %d", body, r.Status)
		}
	}
	if r := e.must(200, "alice", "DELETE", wsBase, map[string]any{"confirm": "ACME", "evidence_acknowledged": true}).Body; r["status"] != "deleted" {
		t.Fatalf("delete: %v", r)
	}

	if r := e.call("alice", "GET", wsBase, nil); r.Status != 404 {
		t.Fatalf("deleted workspace: %d", r.Status)
	}
	for _, table := range []string{"projects", "items", "comments", "events", "notifications", "recurrences", "item_types",
		"workflows", "statuses", "workspace_members", "workspace_invitations", "search_docs", "field_definitions"} {
		if n := e.count(`SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, wsID); n != 0 {
			t.Errorf("%s: %d rows left", table, n)
		}
	}
	var slug string
	var completed *time.Time
	var items, members int
	if err := e.pool.QueryRow(ctx, `SELECT slug, completed_at, (summary->>'items')::int, (summary->>'members')::int
		FROM workspace_deletions WHERE workspace_id = $1`, wsID).Scan(&slug, &completed, &items, &members); err != nil {
		t.Fatal(err)
	}
	if slug != "acme" || completed == nil || items != 2 || members != 4 {
		t.Errorf("deletion record: %s %v items=%d members=%d", slug, completed, items, members)
	}
	e.must(200, "alice", "GET", "/api/v1/workspaces/other", nil) // other workspaces are untouched
	// The slug is free again.
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "acme", "name": "Acme again"})
}

// A large deletion is finished by the worker: the request only marks it
// when the inline budget is spent; the purge is safe to repeat.
func TestWorkspaceDeletionResumesInWorker(t *testing.T) {
	e := newP1Env(t)
	e.archiveSetup()
	ctx := context.Background()
	wsID := e.must(200, "alice", "GET", wsBase, nil).Body["id"].(string)
	e.must(201, "carol", "POST", items, map[string]any{"title": "Ledger"})
	e.must(200, "alice", "POST", wsBase+"/archive", nil)
	sum, err := e.st.RequestWorkspaceDeletion(ctx, mustUUID(t, wsID), mustUUID(t, e.userID("alice")), "acme", true)
	if err != nil || sum.Items != 1 || sum.EvidenceExported == nil || *sum.EvidenceExported {
		t.Fatalf("request: %+v %v", sum, err)
	}
	// While deleting, the workspace is gone for everyone.
	if r := e.call("carol", "GET", wsBase, nil); r.Status != 410 || errCode(r) != "workspace_deleting" {
		t.Fatalf("deleting workspace: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", wsBase+"/restore", nil); r.Status != 410 {
		t.Fatalf("restore while deleting: %d", r.Status)
	}
	// An expired budget does nothing; asking twice is harmless.
	if done, err := e.st.PurgeWorkspace(ctx, mustUUID(t, wsID), time.Now().Add(-time.Second)); err != nil || done {
		t.Fatalf("purge without budget: %v %v", done, err)
	}
	if _, err := e.st.RequestWorkspaceDeletion(ctx, mustUUID(t, wsID), mustUUID(t, e.userID("alice")), "acme", true); err != nil {
		t.Fatal(err)
	}
	e.eng.Tick(ctx)
	if n := e.count(`SELECT count(*) FROM workspaces WHERE id = $1`, wsID); n != 0 {
		t.Fatal("tick did not finish the deletion")
	}
	if done, err := e.st.PurgeWorkspace(ctx, mustUUID(t, wsID), time.Now().Add(time.Second)); err != nil || !done {
		t.Fatalf("purge again: %v %v", done, err)
	}
}
