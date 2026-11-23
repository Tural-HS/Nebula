package server

import (
	"context"
	"testing"
)

const (
	projectsPath = "/api/v1/workspaces/acme/projects"
	secPath      = projectsPath + "/SEC"
	secItems     = secPath + "/items"
)

// visibleKeys lists the project keys a user sees in the workspace.
func (e *env) visibleKeys(user string) map[string]bool {
	e.t.Helper()
	out := map[string]bool{}
	for _, p := range e.must(200, user, "GET", projectsPath, nil).Body["projects"].([]any) {
		out[p.(map[string]any)["key"].(string)] = true
	}
	return out
}

func TestPrivateProjects(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider; project ACME is workspace-visible
	bob, carol := e.userID("bob"), e.userID("carol")

	if r := e.call("carol", "POST", projectsPath, map[string]any{"key": "SEC", "name": "x", "visibility": "secret"}); r.Status != 403 && r.Status != 422 {
		t.Fatalf("member creates project / bad visibility: %d", r.Status)
	}
	if r := e.call("alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "x", "visibility": "secret"}); r.Status != 422 {
		t.Fatalf("bad visibility: %d", r.Status)
	}
	p := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq şöbəsi", "visibility": "private"}).Body
	if p["visibility"] != "private" {
		t.Fatalf("created: %v", p)
	}
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Məhkəmə işi"})

	// Not a project member: the project does not exist for them, everywhere.
	for _, user := range []string{"bob", "carol"} {
		if e.visibleKeys(user)["SEC"] {
			t.Fatalf("%s lists the private project", user)
		}
		for _, path := range []string{secPath, secItems, secItems + "/1", secItems + "/counts", secItems + "/1/comments",
			secItems + "/1/activity", secPath + "/recurrences", secPath + "/members"} {
			if r := e.call(user, "GET", path, nil); r.Status != 404 {
				t.Errorf("%s GET %s: %d", user, path, r.Status)
			}
		}
		if r := e.call(user, "POST", secItems, map[string]any{"title": "x"}); r.Status != 404 {
			t.Errorf("%s creates in private project: %d", user, r.Status)
		}
	}
	if !e.visibleKeys("alice")["SEC"] || !e.visibleKeys("carol")["ACME"] {
		t.Fatal("owner must see the private project; members keep seeing workspace projects")
	}

	// Only people who can see the project can be assigned or mentioned.
	if r := e.call("alice", "PATCH", secItems+"/1", map[string]any{"version": 1, "assignee_id": carol}); r.Status != 422 {
		t.Fatalf("assigning a non-member of the project: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", secPath+"/recurrences", map[string]any{
		"title": "x", "freq": "daily", "every": 1, "start_date": "2026-11-23", "assignee_id": carol,
	}); r.Status != 422 {
		t.Fatalf("recurrence assignee without access: %d", r.Status)
	}
	e.drain()
	e.mail.reset()
	c := e.must(201, "alice", "POST", secItems+"/1/comments", map[string]any{"body": "Baxın " + mention(carol)}).Body
	if len(c["mentions"].([]any)) != 0 {
		t.Fatalf("outsider was mentioned: %v", c["mentions"])
	}
	e.drain()
	if msgs := e.mail.messages(); len(msgs) != 0 {
		t.Fatalf("private content emailed to an outsider: %+v", msgs)
	}

	// Managing access is admin+.
	if r := e.call("carol", "PUT", secPath+"/members/"+carol, nil); r.Status != 404 { // cannot even see it
		t.Fatalf("outsider adds themself: %d", r.Status)
	}
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil) // idempotent
	if r := e.call("carol", "PUT", secPath+"/members/"+bob, nil); r.Status != 403 {
		t.Fatalf("member manages project members: %d", r.Status)
	}
	if r := e.call("alice", "PUT", secPath+"/members/"+e.userID("dave"), nil); r.Status != 422 {
		t.Fatalf("adding a non-workspace-member: %d", r.Status)
	}

	// carol is in: she sees it, works in it, can be assigned and mentioned.
	if !e.visibleKeys("carol")["SEC"] {
		t.Fatal("project member does not see the project")
	}
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Müqavilə", "assignee_id": carol})
	members := e.must(200, "carol", "GET", secPath+"/members", nil).Body["members"].([]any)
	if len(members) != 1 || members[0].(map[string]any)["user_id"] != carol {
		t.Fatalf("members: %v", members)
	}
	if e.visibleKeys("bob")["SEC"] {
		t.Fatal("bob sees it although only carol was added")
	}

	// Leaving the workspace removes project memberships (no forgotten access).
	e.must(204, "alice", "PUT", secPath+"/members/"+bob, nil)
	e.must(204, "alice", "DELETE", "/api/v1/workspaces/acme/members/"+bob, nil)
	var n int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM project_members WHERE user_id = $1`, bob).Scan(&n)
	if n != 0 {
		t.Fatalf("project membership survived leaving the workspace: %d", n)
	}

	// Removing access and switching visibility.
	e.must(204, "alice", "DELETE", secPath+"/members/"+carol, nil)
	if r := e.call("carol", "GET", secPath, nil); r.Status != 404 {
		t.Fatalf("removed member still sees it: %d", r.Status)
	}
	if r := e.call("carol", "PATCH", projectsPath+"/ACME", map[string]any{"visibility": "private"}); r.Status != 403 {
		t.Fatalf("member changes visibility: %d", r.Status)
	}
	if v := e.must(200, "alice", "PATCH", secPath, map[string]any{"visibility": "workspace"}).Body["visibility"]; v != "workspace" {
		t.Fatalf("visibility: %v", v)
	}
	e.must(200, "carol", "GET", secPath, nil)

	audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw
	for _, action := range []string{"project.member_added", "project.member_removed", "project.updated"} {
		if !contains(audit, action) {
			t.Errorf("audit log lacks %s", action)
		}
	}
}
