package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/engine"
)

// Approval cards in chat channels (ADR-0031): the whole flow reaches a fake
// Mattermost/Slack receiver as attachments, names no emails, links to the
// approval panel, and a private project's request reaches only the webhook
// that lists that project.

func (rc *receiver) chat(t *testing.T, from int) []engine.ChatMessage {
	t.Helper()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var out []engine.ChatMessage
	for _, b := range rc.bodies[from:] {
		var m engine.ChatMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("body %s: %v", b, err)
		}
		out = append(out, m)
	}
	return out
}

func cards(msgs []engine.ChatMessage) []engine.ChatAttachment {
	var out []engine.ChatAttachment
	for _, m := range msgs {
		out = append(out, m.Attachments...)
	}
	return out
}

func TestChatApprovalCards(t *testing.T) {
	e := newP1Env(t)
	wf, _, inProgress, done := e.approvalSetup()
	e.eng.PublicURL = "https://nebula.example.com"
	carol, eve := e.userID("carol"), e.userID("eve")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_roles": []string{"member"}})
	sec := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Xəzinə", "visibility": "private"}).Body["id"].(string)
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	e.must(204, "alice", "PUT", secPath+"/members/"+eve, nil)

	hooks := wsBase + "/webhooks"
	team := newReceiver(t, 200)   // every workspace-visible project
	vault := newReceiver(t, 200)  // lists the private project
	noAppr := newReceiver(t, 200) // opted out of approval events
	for rc, body := range map[*receiver]map[string]any{
		team:   {"events": []string{"item.*", "comment.created", "approval.*"}},
		vault:  {"events": []string{"approval.*"}, "project_ids": []string{sec}},
		noAppr: {"events": []string{"item.created"}},
	} {
		body["url"], body["format"] = rc.srv.URL+"/hooks/tok"+strings.Repeat("x", 20), "slack"
		e.must(201, "alice", "POST", hooks, body)
	}
	e.drain()
	start := map[*receiver]int{team: team.count(), vault: vault.count(), noAppr: noAppr.count()}

	// --- workspace project: request, approve, approve (applies) ------------
	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş <!channel> [Giriş](https://evil.example)"})
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": inProgress})
	r := e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 2, "to_status_id": done, "comment": "@all qəbz əlavədədir"})
	id := lastApproval(r)["id"].(string)
	e.drain()
	got := cards(team.chat(t, start[team]))
	if len(got) != 1 {
		t.Fatalf("cards after the request: %+v", got)
	}
	req := got[0]
	link := "https://nebula.example.com/w/acme/p/ACME?item=1&focus=approval"
	if req.Color != "#F59E0B" || req.Title != "ACME-1 · Təsdiq gözləyir" || req.TitleLink != link ||
		!strings.Contains(req.Text, "<"+link+"|🔗 Nebula-da aç>") {
		t.Fatalf("request card: %+v", req)
	}
	fields := map[string]string{}
	for _, f := range req.Fields {
		fields[f.Title] = f.Value
	}
	if fields["Təsdiq istəyən"] != "Carol" || fields["Status"] != "In Progress → Done" || fields["Təsdiqlər"] != "0/2" ||
		fields["Təsdiq edə bilənlər"] != "Erin, Eve" {
		t.Fatalf("request fields: %v", fields)
	}
	raw := strings.Join(rawBodies(team, start[team]), "\n")
	for _, bad := range []string{"@example.com", "<!channel>", "](https://evil", "@all"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("chat body contains %q: %s", bad, raw)
		}
	}

	e.must(200, "eve", "POST", items+"/1/approvals/"+id+"/approve", map[string]any{"comment": "Yoxladım"})
	e.must(200, "erin", "POST", items+"/1/approvals/"+id+"/approve", map[string]any{})
	e.drain()
	var titles, colors []string
	for _, c := range cards(team.chat(t, start[team])) {
		titles, colors = append(titles, c.Title), append(colors, c.Color)
	}
	wantTitles := "ACME-1 · Təsdiq gözləyir|ACME-1 · Təsdiq verildi (1/2)|ACME-1 · Təsdiq verildi (2/2)|ACME-1 · Təsdiqləndi, status dəyişdi"
	if strings.Join(titles, "|") != wantTitles || strings.Join(colors, ",") != "#F59E0B,#F59E0B,#059669,#059669" {
		t.Fatalf("flow cards: %v %v", titles, colors)
	}
	// The applied status change is still the plain item.updated line.
	if !strings.Contains(strings.Join(rawBodies(team, start[team]), "\n"), "In Progress → Done") {
		t.Fatal("no status line")
	}
	if n := noAppr.count() - start[noAppr]; n != 1 {
		t.Fatalf("opted-out webhook got %d messages (want the item.created only)", n)
	}

	// --- private project: reject with reason --------------------------------
	for rc := range start {
		start[rc] = rc.count()
	}
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli ödəniş"})
	r = e.must(201, "carol", "POST", secItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})
	sid := lastApproval(r)["id"].(string)
	e.must(200, "eve", "POST", secItems+"/1/approvals/"+sid+"/reject", map[string]any{"reason": "Sənəd çatışmır @here"})
	e.drain()
	if n := team.count() - start[team]; n != 0 {
		t.Fatalf("the unfiltered webhook got %d private messages: %s", n, strings.Join(rawBodies(team, start[team]), "\n"))
	}
	vc := cards(vault.chat(t, start[vault]))
	if len(vc) != 2 || vc[0].Title != "SEC-1 · Təsdiq gözləyir" || vc[1].Color != "#DC2626" {
		t.Fatalf("private project cards: %+v", vc)
	}
	// Only people who can see the project are named: eve is a project
	// member, erin is not.
	for _, f := range vc[0].Fields {
		if f.Title == "Təsdiq edə bilənlər" && f.Value != "Eve" {
			t.Fatalf("approvers of the private request: %q", f.Value)
		}
	}
	var reason string
	for _, f := range vc[1].Fields {
		if f.Title == "Səbəb" {
			reason = f.Value
		}
	}
	if !strings.HasPrefix(reason, "“Sənəd çatışmır @") || strings.Contains(reason, "@here") {
		t.Fatalf("reason: %q", reason)
	}
}

func rawBodies(rc *receiver, from int) []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]string, 0, len(rc.bodies)-from)
	for _, b := range rc.bodies[from:] {
		out = append(out, string(b))
	}
	return out
}
