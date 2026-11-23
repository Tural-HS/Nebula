package server

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

const invPath = "/api/v1/workspaces/acme/invitations"

var inviteLinkRe = regexp.MustCompile(`https://nebula\.test/invite/([A-Za-z0-9_-]{43})`)

// lastInviteToken returns the token from the newest invitation email to addr.
func (e *p1env) lastInviteToken(addr string) string {
	e.t.Helper()
	e.drain()
	msgs := e.mail.messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(msgs[i].To[0], addr) {
			if m := inviteLinkRe.FindStringSubmatch(msgs[i].Body); m != nil {
				return m[1]
			}
		}
	}
	e.t.Fatalf("no invitation email to %s", addr)
	return ""
}

func TestInvitations(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider
	ctx := context.Background()

	// Who may invite, and what.
	if r := e.call("carol", "POST", invPath, map[string]any{"email": "erin@example.com", "role": "member"}); r.Status != 403 {
		t.Fatalf("member invites: %d", r.Status)
	}
	for _, bad := range []map[string]any{
		{"email": "not an email", "role": "member"}, {"email": "Erin <erin@example.com>", "role": "member"},
		{"email": "erin@example.com", "role": "superuser"},
	} {
		if r := e.call("alice", "POST", invPath, bad); r.Status != 422 {
			t.Errorf("%v: %d %s", bad, r.Status, r.Raw)
		}
	}
	if r := e.call("alice", "POST", invPath, map[string]any{"email": "BOB@example.com", "role": "member"}); r.Status != 409 {
		t.Fatalf("inviting an existing member: %d", r.Status)
	}

	inv := e.must(201, "alice", "POST", invPath, map[string]any{"email": "Erin@Example.com", "role": "member"}).Body
	if inv["email"] != "erin@example.com" || inv["role"] != "member" || inv["invited_by"] != "Alice" {
		t.Fatalf("created: %v", inv)
	}
	if r := e.call("alice", "POST", invPath, map[string]any{"email": "erin@example.com", "role": "admin"}); r.Status != 409 {
		t.Fatalf("second pending invitation: %d", r.Status)
	}
	token := e.lastInviteToken("erin@example.com")
	msgs := e.mail.messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Subject, "Alice sizi Nebula-da") || !strings.Contains(last.Body, "7 gün") {
		t.Fatalf("email: %+v", last)
	}
	list := e.must(200, "alice", "GET", invPath, nil)
	if strings.Contains(list.Raw, token) || len(list.Body["invitations"].([]any)) != 1 {
		t.Fatalf("list must show the invitation but never the token: %s", list.Raw)
	}
	var stored []byte
	_ = e.pool.QueryRow(ctx, `SELECT token_hash FROM workspace_invitations`).Scan(&stored)
	if strings.Contains(string(stored), token) {
		t.Fatal("token stored in plain text")
	}

	// A forwarded link does not let someone else in.
	pv := e.must(200, "eve", "POST", "/api/v1/invitations/preview", map[string]any{"token": token}).Body
	if pv["email_matches"] != false || pv["workspace_name"] != "" || pv["email"] != "e***@example.com" {
		t.Fatalf("preview for the wrong account: %v", pv)
	}
	if r := e.call("eve", "POST", "/api/v1/invitations/accept", map[string]any{"token": token}); r.Status != 403 {
		t.Fatalf("wrong account accepts: %d", r.Status)
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 43), "../../etc"} {
		if r := e.call("erin", "POST", "/api/v1/invitations/accept", map[string]any{"token": bad}); r.Status != 404 {
			t.Errorf("token %q: %d", bad, r.Status)
		}
	}

	// The invited person: preview, accept once.
	pv = e.must(200, "erin", "POST", "/api/v1/invitations/preview", map[string]any{"token": token}).Body
	if pv["email_matches"] != true || pv["workspace_name"] != "Acme Inc" || pv["role"] != "member" || pv["invited_by"] != "Alice" {
		t.Fatalf("preview: %v", pv)
	}
	if ws := e.must(200, "erin", "POST", "/api/v1/invitations/accept", map[string]any{"token": token}).Body["workspace"]; ws != "acme" {
		t.Fatalf("accept: %v", ws)
	}
	if role := e.must(200, "erin", "GET", wsBase, nil).Body["role"]; role != "member" {
		t.Fatalf("erin's role: %v", role)
	}
	if r := e.call("erin", "POST", "/api/v1/invitations/accept", map[string]any{"token": token}); r.Status != 409 {
		t.Fatalf("token reused: %d", r.Status)
	}
	if n := len(e.must(200, "alice", "GET", invPath, nil).Body["invitations"].([]any)); n != 0 {
		t.Fatalf("accepted invitation still pending: %d", n)
	}

	// An address the IdP has not verified cannot accept.
	e.must(201, "alice", "POST", invPath, map[string]any{"email": "frank@example.com", "role": "guest"})
	frankToken := e.lastInviteToken("frank@example.com")
	if r := e.call("frank", "POST", "/api/v1/invitations/accept", map[string]any{"token": frankToken}); r.Status != 403 || !strings.Contains(r.Raw, "not verified") {
		t.Fatalf("unverified email: %d %s", r.Status, r.Raw)
	}

	// Resend replaces the token; revoke kills it; expiry is enforced.
	daveInv := e.must(201, "alice", "POST", invPath, map[string]any{"email": "dave@example.com", "role": "guest"}).Body["id"].(string)
	oldToken := e.lastInviteToken("dave@example.com")
	e.must(204, "alice", "POST", invPath+"/"+daveInv+"/resend", nil)
	newToken := e.lastInviteToken("dave@example.com")
	if newToken == oldToken {
		t.Fatal("resend kept the token")
	}
	if r := e.call("dave", "POST", "/api/v1/invitations/preview", map[string]any{"token": oldToken}); r.Status != 404 {
		t.Fatalf("old token after resend: %d", r.Status)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE workspace_invitations SET expires_at = now() - interval '1 minute' WHERE id = $1`, daveInv); err != nil {
		t.Fatal(err)
	}
	if r := e.call("dave", "POST", "/api/v1/invitations/accept", map[string]any{"token": newToken}); r.Status != 409 || !strings.Contains(r.Raw, "expired") {
		t.Fatalf("expired: %d %s", r.Status, r.Raw)
	}
	// After expiry the address can be invited again (the old row is retired).
	again := e.must(201, "alice", "POST", invPath, map[string]any{"email": "dave@example.com", "role": "guest"}).Body["id"].(string)
	e.must(204, "alice", "DELETE", invPath+"/"+again, nil)
	if r := e.call("dave", "POST", "/api/v1/invitations/accept", map[string]any{"token": e.lastInviteToken("dave@example.com")}); r.Status != 404 {
		t.Fatalf("revoked: %d", r.Status)
	}

	audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw
	for _, a := range []string{"invitation.created", "invitation.accepted", "invitation.resent", "invitation.revoked"} {
		if !strings.Contains(audit, a) {
			t.Errorf("audit lacks %s", a)
		}
	}
	if strings.Contains(audit, token) || strings.Contains(audit, newToken) {
		t.Fatal("a token leaked into the audit log")
	}
}

func TestInvitationsNeedPublicURL(t *testing.T) {
	e := newEnv(t) // no PublicURL configured
	e.setup()
	if r := e.call("alice", "POST", invPath, map[string]any{"email": "erin@example.com", "role": "member"}); r.Status != 503 {
		t.Fatalf("without a public URL: %d %s", r.Status, r.Raw)
	}
}
