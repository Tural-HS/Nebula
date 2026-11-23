package server

import "testing"

func TestMemberManagement(t *testing.T) {
	e := newEnv(t)
	e.setup() // alice owner, bob guest, carol member; dave signed in, not a member
	id := func(tok string) string { return e.must(200, tok, "GET", "/api/v1/me", nil).Body["id"].(string) }
	alice, bob, carol, dave := id("alice"), id("bob"), id("carol"), id("dave")
	base := "/api/v1/workspaces/acme/members/"

	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "dave@example.com", "role": "admin"})

	cases := []struct {
		name, token, method, user string
		body                      any
		want                      int
		code                      string
	}{
		{"member cannot change roles", "carol", "PATCH", bob, map[string]any{"role": "member"}, 403, ""},
		{"admin cannot grant owner", "dave", "PATCH", carol, map[string]any{"role": "owner"}, 403, ""},
		{"admin cannot demote the owner", "dave", "PATCH", alice, map[string]any{"role": "member"}, 403, ""},
		{"admin cannot remove the owner", "dave", "DELETE", alice, nil, 403, ""},
		{"last owner cannot be demoted", "alice", "PATCH", alice, map[string]any{"role": "admin"}, 409, "last_owner"},
		{"last owner cannot leave", "alice", "DELETE", alice, nil, 409, "last_owner"},
		{"invalid role", "alice", "PATCH", carol, map[string]any{"role": "superuser"}, 422, ""},
		{"unknown member", "alice", "PATCH", "01a20727-0000-7000-8000-000000000000", map[string]any{"role": "member"}, 404, ""},
		{"member cannot remove others", "carol", "DELETE", bob, nil, 403, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := e.call(tc.token, tc.method, base+tc.user, tc.body)
			if r.Status != tc.want || (tc.code != "" && errCode(r) != tc.code) {
				t.Fatalf("got %d %s", r.Status, r.Raw)
			}
		})
	}

	// Admin promotes a guest; the change is audited.
	if m := e.must(200, "dave", "PATCH", base+bob, map[string]any{"role": "member"}).Body; m["role"] != "member" {
		t.Fatalf("promote: %v", m)
	}
	// With a second owner, the first one may step down.
	e.must(200, "alice", "PATCH", base+carol, map[string]any{"role": "owner"})
	e.must(200, "alice", "PATCH", base+alice, map[string]any{"role": "admin"})
	// Anyone may leave; afterwards the workspace is invisible to them.
	e.must(204, "bob", "DELETE", base+bob, nil)
	e.must(404, "bob", "GET", "/api/v1/workspaces/acme", nil)
	// Admin removes a member.
	e.must(204, "alice", "DELETE", base+dave, nil)
	if n := len(e.must(200, "carol", "GET", "/api/v1/workspaces/acme/members", nil).Body["members"].([]any)); n != 2 {
		t.Fatalf("members left: %d", n)
	}
	var events int
	_ = e.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE action IN ('member.role_changed', 'member.removed')`).Scan(&events)
	if events != 5 {
		t.Fatalf("membership changes audited: %d, want 5", events)
	}
}
