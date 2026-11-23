package server

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/engine"
)

func TestSlackWebhook(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.eng.PublicURL = "https://nebula.example.com"
	rc := newReceiver(t, 200)
	base := "/api/v1/workspaces/acme/webhooks"
	// The token in a Mattermost/Slack incoming-webhook URL is the credential.
	const token = "xoxTOKENabcdefghijklmnop1234"
	hookURL := rc.srv.URL + "/hooks/" + token

	if r := e.call("alice", "POST", base, map[string]any{"url": hookURL, "format": "teams", "events": []string{"*"}}); r.Status != 422 {
		t.Fatalf("unknown format: %d %s", r.Status, r.Raw)
	}
	created := e.must(201, "alice", "POST", base, map[string]any{"url": hookURL, "format": "slack", "events": []string{"item.*", "comment.*"}})
	if strings.Contains(created.Raw, token) {
		t.Fatalf("create response leaks the URL: %s", created.Raw)
	}
	if created.Body["secret"] != nil {
		t.Fatalf("slack webhooks have no signing secret: %v", created.Body["secret"])
	}
	hook := created.Body["webhook"].(map[string]any)
	id := hook["id"].(string)
	wantMasked := rc.srv.URL + "/…1234"
	if hook["format"] != "slack" || hook["url"] != wantMasked {
		t.Fatalf("webhook: %v (want url %q)", hook, wantMasked)
	}
	for _, path := range []string{base, base + "/" + id} {
		if r := e.must(200, "alice", "GET", path, nil); strings.Contains(r.Raw, token) {
			t.Fatalf("GET %s leaks the URL: %s", path, r.Raw)
		}
	}
	if r := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/audit-log?limit=50", nil); strings.Contains(r.Raw, token) {
		t.Fatalf("audit log leaks the URL: %s", r.Raw)
	}
	var row, events []byte
	_ = e.pool.QueryRow(context.Background(), `SELECT url || encode(url_encrypted, 'escape') FROM webhooks WHERE id = $1`, id).Scan(&row)
	_ = e.pool.QueryRow(context.Background(), `SELECT string_agg(payload::text, ' ')::bytea FROM events`).Scan(&events)
	if bytes.Contains(row, []byte(token)) || bytes.Contains(events, []byte(token)) {
		t.Fatal("URL stored in plain text")
	}
	if r := e.call("alice", "POST", base+"/"+id+"/rotate-secret", nil); r.Status != 409 {
		t.Fatalf("rotate-secret on an unsigned webhook: %d", r.Status)
	}
	if r := e.call("alice", "PATCH", base+"/"+id, map[string]any{"format": "nebula"}); r.Status != 400 {
		t.Fatalf("format is fixed after creation: %d", r.Status)
	}

	// Delivery: a Slack-compatible body, no Nebula signature headers.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Login <!channel> [x](http://evil)"})
	e.drain()
	if rc.count() != 1 {
		t.Fatalf("deliveries: %d", rc.count())
	}
	req, body := rc.requests[0], rc.bodies[0]
	if req.URL.Path != "/hooks/"+token {
		t.Fatalf("delivered to %s", req.URL.Path)
	}
	if req.Header.Get("Nebula-Signature") != "" || req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers: %v", req.Header)
	}
	var msg engine.ChatMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	wantLink := "<https://nebula.example.com/w/acme/p/ACME?item=1|ACME-1>"
	if msg.Username != "Nebula" || !strings.HasPrefix(msg.Text, "🆕 "+wantLink) || !strings.HasSuffix(msg.Text, "yaradıldı — Carol") {
		t.Fatalf("message: %+v", msg)
	}
	if strings.Contains(msg.Text, "<!channel>") || strings.Contains(msg.Text, "](") {
		t.Fatalf("user text not escaped: %q", msg.Text)
	}

	// Status change names both statuses and the actor.
	wf := e.defaultWorkflow()
	inProgress := statusID(t, wf, "In Progress")
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": inProgress, "title": "Login"})
	e.drain()
	_ = json.Unmarshal(rc.bodies[1], &msg)
	if !strings.Contains(msg.Text, "🔄 "+wantLink+" “Login”: To Do → In Progress — Carol") {
		t.Fatalf("status message: %q", msg.Text)
	}

	t.Run("ping", func(t *testing.T) {
		e.must(202, "alice", "POST", base+"/"+id+"/ping", nil)
		e.drain()
		_ = json.Unmarshal(rc.bodies[len(rc.bodies)-1], &msg)
		if !strings.HasPrefix(msg.Text, "✅ Nebula test mesajı") {
			t.Fatalf("ping: %q", msg.Text)
		}
	})

	t.Run("URL change re-encrypts and stays masked", func(t *testing.T) {
		other := newReceiver(t, 200)
		newURL := other.srv.URL + "/hooks/" + token + "NEW9"
		r := e.must(200, "alice", "PATCH", base+"/"+id, map[string]any{"url": newURL})
		if strings.Contains(r.Raw, token) || r.Body["url"] != other.srv.URL+"/…NEW9" {
			t.Fatalf("patch: %s", r.Raw)
		}
		e.must(202, "alice", "POST", base+"/"+id+"/ping", nil)
		e.drain()
		if other.count() != 1 || other.requests[0].URL.Path != "/hooks/"+token+"NEW9" {
			t.Fatalf("delivery after URL change: %d", other.count())
		}
	})

	t.Run("delivery errors never contain the URL", func(t *testing.T) {
		dead := newReceiver(t, 200)
		deadURL := dead.srv.URL + "/hooks/" + token
		dead.srv.Close() // connection refused
		h := e.must(201, "alice", "POST", base, map[string]any{"url": deadURL, "format": "slack", "events": []string{"*"}}).Body
		hid := h["webhook"].(map[string]any)["id"].(string)
		e.must(202, "alice", "POST", base+"/"+hid+"/ping", nil)
		e.drain()
		r := e.must(200, "alice", "GET", base+"/"+hid+"/deliveries", nil)
		ds := r.Body["deliveries"].([]any)
		// The ping plus the webhook.created event of the "*" subscription.
		if len(ds) == 0 || ds[0].(map[string]any)["error"] == nil || strings.Contains(r.Raw, token) {
			t.Fatalf("deliveries: %s", r.Raw)
		}
		var jobErr string
		_ = e.pool.QueryRow(context.Background(), `SELECT coalesce(string_agg(last_error, ' '), '') FROM jobs`).Scan(&jobErr)
		if strings.Contains(jobErr, token) {
			t.Fatalf("job error leaks the URL: %s", jobErr)
		}
	})
}
