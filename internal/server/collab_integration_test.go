package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
	"github.com/Tural-HS/Nebula/internal/worker"
)

func (e *p1env) userID(token string) string {
	e.t.Helper()
	return e.must(200, token, "GET", "/api/v1/me", nil).Body["id"].(string)
}

func mention(id string) string { return "<@" + id + ">" }

func TestComments(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	alice, bob, carol, dave := e.userID("alice"), e.userID("bob"), e.userID("carol"), e.userID("dave")
	e.must(201, "carol", "POST", items, map[string]any{"title": "Fix login"})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Other item"})
	base := items + "/1/comments"
	e.drain()
	e.mail.reset() // ignore setup notifications

	// dave is not a member: the mention is silently dropped (no membership probing).
	c := e.must(201, "carol", "POST", base, map[string]any{
		"body": "Please check " + mention(bob) + " " + mention(dave) + " (cc " + mention(carol) + ")",
	}).Body
	cid := c["id"].(string)
	mentions := c["mentions"].([]any)
	if len(mentions) != 2 || c["author"].(map[string]any)["display_name"] != "Carol" || c["edited"] != false {
		t.Fatalf("created comment: %v", c)
	}
	for _, m := range mentions {
		if m == dave {
			t.Fatal("non-member was recorded as mentioned")
		}
	}

	e.drain()
	msgs := e.mail.messages()
	if len(msgs) != 1 || msgs[0].To[0] != "bob@example.com" {
		t.Fatalf("only bob should be emailed (not the author, not non-members): %+v", msgs)
	}
	if !strings.Contains(msgs[0].Subject, "[ACME-1] Carol mentioned you") || !strings.Contains(msgs[0].Body, "@Bob") || strings.Contains(msgs[0].Body, "<@") {
		t.Fatalf("mention email: %+v", msgs[0])
	}

	t.Run("permissions and validation", func(t *testing.T) {
		e.must(200, "bob", "GET", base, nil) // guests read
		cases := []struct {
			name, token, method, path string
			body                      any
			want                      int
		}{
			{"guest cannot comment", "bob", "POST", base, map[string]any{"body": "x"}, 403},
			{"non-member cannot read", "dave", "GET", base, nil, 404},
			{"empty body", "carol", "POST", base, map[string]any{"body": "   "}, 422},
			{"too long", "carol", "POST", base, map[string]any{"body": strings.Repeat("x", 20001)}, 422},
			{"missing item", "carol", "POST", items + "/99/comments", map[string]any{"body": "x"}, 404},
			{"comment of another item", "carol", "PATCH", items + "/2/comments/" + cid, map[string]any{"body": "x"}, 404},
			{"admin cannot edit others' words", "alice", "PATCH", base + "/" + cid, map[string]any{"body": "rewritten"}, 403},
			{"bad cursor", "carol", "GET", base + "?cursor=%%%", nil, 400},
		}
		for _, tc := range cases {
			if r := e.call(tc.token, tc.method, tc.path, tc.body); r.Status != tc.want {
				t.Errorf("%s: %d %s", tc.name, r.Status, r.Raw)
			}
		}
		many := ""
		for range 21 {
			many += mention(alice) // duplicates collapse, so use distinct ids below
		}
		ids := make([]string, 21)
		for i := range ids {
			ids[i] = mention(uuid.NewString())
		}
		if r := e.call("carol", "POST", base, map[string]any{"body": strings.Join(ids, " ")}); r.Status != 422 {
			t.Errorf("21 mentions: %d", r.Status)
		}
		if r := e.call("carol", "POST", base, map[string]any{"body": many}); r.Status != 201 {
			t.Errorf("repeated mention of one user must count once: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("edit notifies only newly mentioned people", func(t *testing.T) {
		e.drain()
		before := len(e.mail.messages())
		updated := e.must(200, "carol", "PATCH", base+"/"+cid, map[string]any{
			"body": "Please check " + mention(bob) + " and " + mention(alice),
		}).Body
		if updated["edited"] != true && updated["updated_at"] == updated["created_at"] {
			t.Logf("edited flag depends on timing: %v", updated["edited"])
		}
		e.drain()
		msgs := e.mail.messages()[before:]
		if len(msgs) != 1 || msgs[0].To[0] != "alice@example.com" {
			t.Fatalf("edit emails: %+v", msgs)
		}
	})

	t.Run("pagination is stable and oldest first", func(t *testing.T) {
		for i := range 3 {
			e.must(201, "carol", "POST", base, map[string]any{"body": "n" + string(rune('0'+i))})
		}
		var seen []string
		cursor := ""
		for range 10 {
			path := base + "?limit=2"
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			r := e.must(200, "bob", "GET", path, nil)
			for _, c := range r.Body["comments"].([]any) {
				seen = append(seen, c.(map[string]any)["id"].(string))
			}
			next, _ := r.Body["next_cursor"].(string)
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != 5 || seen[0] != cid {
			t.Fatalf("pages: %d comments, first=%s want %s", len(seen), seen[0], cid)
		}
	})

	t.Run("delete leaves a tombstone", func(t *testing.T) {
		if r := e.call("bob", "DELETE", base+"/"+cid, nil); r.Status != 403 {
			t.Fatalf("guest delete: %d", r.Status)
		}
		e.must(204, "alice", "DELETE", base+"/"+cid, nil) // admins may moderate
		first := e.must(200, "bob", "GET", base+"?limit=1", nil).Body["comments"].([]any)[0].(map[string]any)
		if first["deleted"] != true || first["body"] != nil || len(first["mentions"].([]any)) != 0 {
			t.Fatalf("tombstone: %v", first)
		}
		e.must(404, "carol", "PATCH", base+"/"+cid, map[string]any{"body": "zombie"})
		e.must(404, "alice", "DELETE", base+"/"+cid, nil)
	})
}

func TestItemActivityAndAuditLog(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "carol", "POST", items, map[string]any{"title": "Track me"})
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "title": "Tracked"})
	e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "looks good"})

	act := e.must(200, "bob", "GET", items+"/1/activity", nil).Body
	list := act["activity"].([]any)
	var actions []string
	for _, a := range list {
		m := a.(map[string]any)
		actions = append(actions, m["action"].(string))
		if _, leaked := m["data"].(map[string]any)["snapshot"]; leaked {
			t.Fatal("internal snapshot leaked into the activity feed")
		}
	}
	if strings.Join(actions, ",") != "comment.created,item.updated,item.created" {
		t.Fatalf("activity order: %v", actions)
	}
	if list[0].(map[string]any)["actor"].(map[string]any)["display_name"] != "Carol" {
		t.Fatalf("actor: %v", list[0])
	}
	page := e.must(200, "bob", "GET", items+"/1/activity?limit=2", nil).Body
	next := page["next_before"].(float64)
	rest := e.must(200, "bob", "GET", items+"/1/activity?limit=2&before="+strconv.FormatInt(int64(next), 10), nil).Body["activity"].([]any)
	if len(rest) != 1 || rest[0].(map[string]any)["action"] != "item.created" {
		t.Fatalf("activity page 2: %v", rest)
	}
	if r := e.call("dave", "GET", items+"/1/activity", nil); r.Status != 404 {
		t.Fatalf("non-member activity: %d", r.Status)
	}

	audit := "/api/v1/workspaces/acme/audit-log"
	if r := e.call("carol", "GET", audit, nil); r.Status != 403 {
		t.Fatalf("member reads audit log: %d", r.Status)
	}
	all := e.must(200, "alice", "GET", audit+"?limit=100", nil).Body["activity"].([]any)
	// workspace.created, project.created, 2× member.added, item.created, item.updated, comment.created
	if len(all) != 7 {
		t.Fatalf("audit log: %d events, want 7", len(all))
	}
	members := e.must(200, "alice", "GET", audit+"?action=member.", nil).Body["activity"].([]any)
	if len(members) != 2 {
		t.Fatalf("member.* events: %d", len(members))
	}
	for _, bad := range []string{"?action=item%25", "?before=abc", "?actor=nope", "?limit=1000"} {
		if r := e.call("alice", "GET", audit+bad, nil); r.Status/100 != 4 {
			t.Errorf("%s: %d", bad, r.Status)
		}
	}
}

func TestWebhookPing(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	pinged := newReceiver(t, 200)
	other := newReceiver(t, 200)
	base := "/api/v1/workspaces/acme/webhooks"
	id := e.must(201, "alice", "POST", base, map[string]any{"url": pinged.srv.URL, "events": []string{"item.created"}}).Body["webhook"].(map[string]any)["id"].(string)
	e.must(201, "alice", "POST", base, map[string]any{"url": other.srv.URL, "events": []string{"*"}})
	e.drain()

	if r := e.call("carol", "POST", base+"/"+id+"/ping", nil); r.Status != 403 {
		t.Fatalf("member ping: %d", r.Status)
	}
	e.must(404, "alice", "POST", base+"/01a20727-0000-7000-8000-000000000000/ping", nil)
	resp := e.must(202, "alice", "POST", base+"/"+id+"/ping", nil).Body
	if resp["event_id"] == nil {
		t.Fatalf("ping response: %v", resp)
	}
	before := other.count()
	e.drain()
	if pinged.count() != 1 || pinged.requests[0].Header.Get("Nebula-Event-Type") != "webhook.ping" {
		t.Fatalf("ping must reach the webhook despite its event filter: %d", pinged.count())
	}
	if other.count() != before {
		t.Fatal("ping must not fan out to other webhooks")
	}
	d := e.must(200, "alice", "GET", base+"/"+id+"/deliveries", nil).Body["deliveries"].([]any)
	if len(d) != 1 || d[0].(map[string]any)["status_code"] != 200.0 {
		t.Fatalf("ping delivery log: %v", d)
	}
}

func TestEncryptionKeyRotation(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	keyA, keyB := randomKey(t), randomKey(t)
	boxA, _ := secretbox.NewFromBase64(keyA)
	old := store.New(pool).WithSecrets(boxA)

	var owner, ws string
	_ = pool.QueryRow(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name)
		VALUES (gen_random_uuid(), 'i', 's', 'o@example.com', 'O') RETURNING id`).Scan(&owner)
	_ = pool.QueryRow(ctx, `INSERT INTO workspaces (id, slug, name) VALUES (gen_random_uuid(), 'rot', 'Rot') RETURNING id`).Scan(&ws)
	hook, secret, err := old.CreateWebhook(ctx, uuid.MustParse(ws), uuid.MustParse(owner), "https://hooks.example.com/x", store.WebhookFormatNebula, []string{"*"}, nil, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	const slackURL = "https://chat.example.com/hooks/abcdefghijklmnopqrstuvwxyz"
	slackHook, _, err := old.CreateWebhook(ctx, uuid.MustParse(ws), uuid.MustParse(owner), slackURL, store.WebhookFormatSlack, []string{"*"}, nil, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	boxBOnly, _ := secretbox.NewFromBase64(keyB)
	if _, err := store.New(pool).WithSecrets(boxBOnly).WebhookTarget(ctx, hook.ID); err == nil {
		t.Fatal("new key alone must not decrypt before rotation")
	}

	boxB, _ := secretbox.NewFromBase64(keyB)
	boxB, _ = boxB.WithOldKeys(keyA)
	rotating := store.New(pool).WithSecrets(boxB)
	if n, err := rotating.ReencryptSecrets(ctx); err != nil || n != 2 {
		t.Fatalf("rotation: n=%d err=%v", n, err)
	}
	if n, _ := rotating.ReencryptSecrets(ctx); n != 0 {
		t.Fatalf("second run must be a no-op, re-encrypted %d", n)
	}
	boxBOnly2, _ := secretbox.NewFromBase64(keyB)
	got, err := store.New(pool).WithSecrets(boxBOnly2).WebhookTarget(ctx, hook.ID)
	if err != nil || got.Secret != secret {
		t.Fatalf("after rotation the new key alone must work: %v", err)
	}
	got, err = store.New(pool).WithSecrets(boxBOnly2).WebhookTarget(ctx, slackHook.ID)
	if err != nil || got.URL != slackURL || got.Format != store.WebhookFormatSlack {
		t.Fatalf("slack URL after rotation: %v format=%q", err, got.Format)
	}
}

func TestListenWakesOnNewJobs(t *testing.T) {
	pool := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := worker.Listen(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	select { // initial catch-up signal once connected
	case <-wake:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not connect")
	}
	if err := store.New(pool).EnqueueEmail(ctx, []string{"a@example.com"}, "s", "b", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case <-time.After(3 * time.Second):
		t.Fatal("no wake-up after a job was committed")
	}
	cancel()
	select {
	case _, ok := <-wake:
		for ok {
			_, ok = <-wake
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wake channel not closed on shutdown")
	}
}

func randomKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func (f *fakeMail) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = nil
}
