package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/engine"
	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
	"github.com/Tural-HS/Nebula/internal/worker"
)

type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) messages() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mail.Message(nil), f.sent...)
}

type p1env struct {
	*env
	st   *store.Store
	eng  *engine.Engine
	mail *fakeMail
}

func newP1Env(t *testing.T) *p1env {
	t.Helper()
	pool := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	st := store.New(pool).WithSecrets(box)
	opts := safehttp.Options{AllowPrivate: true, Timeout: 5 * time.Second} // receivers run on localhost
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewAPIHandler(Deps{
		Logger: logger, Metrics: observability.NewMetrics(), DB: pool, Store: st,
		Verifier: testUsers, RequestTimeout: 10 * time.Second, Webhooks: opts,
	})
	fm := &fakeMail{}
	eng := &engine.Engine{Store: st, HTTP: safehttp.NewClient(opts), HTTPOpts: opts, Mail: fm, Logger: logger, Version: "test"}
	return &p1env{env: &env{t: t, h: h, pool: pool}, st: st, eng: eng, mail: fm}
}

// drain runs queued jobs synchronously (like the worker) until none are runnable.
func (e *p1env) drain() {
	e.t.Helper()
	ctx := context.Background()
	handlers := e.eng.Handlers()
	for range 50 {
		jobs, err := e.st.ClaimJobs(ctx, 50, time.Minute)
		if err != nil {
			e.t.Fatal(err)
		}
		if len(jobs) == 0 {
			return
		}
		for _, j := range jobs {
			if err := handlers[j.Kind](ctx, j); err != nil {
				if _, ferr := e.st.FailJob(ctx, j, err, worker.IsPermanent(err), time.Hour); ferr != nil {
					e.t.Fatal(ferr)
				}
				continue
			}
			if err := e.st.CompleteJob(ctx, j.ID); err != nil {
				e.t.Fatal(err)
			}
		}
	}
	e.t.Fatal("job queue did not drain")
}

// retryNow makes scheduled retries runnable immediately.
func (e *p1env) retryNow() {
	if _, err := e.pool.Exec(context.Background(), `UPDATE jobs SET run_at = now() WHERE state = 'pending'`); err != nil {
		e.t.Fatal(err)
	}
}

func (e *p1env) defaultWorkflow() map[string]any {
	for _, w := range e.must(200, "alice", "GET", wfBase, nil).Body["workflows"].([]any) {
		if w.(map[string]any)["name"] == "Default" {
			return w.(map[string]any)
		}
	}
	e.t.Fatal("default workflow missing")
	return nil
}

func TestWorkflowTransitionsAreEnforced(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	wf := e.defaultWorkflow()
	path := wfBase + "/" + wf["id"].(string)
	todo, inProgress, done, canceled := statusID(t, wf, "To Do"), statusID(t, wf, "In Progress"), statusID(t, wf, "Done"), statusID(t, wf, "Canceled")

	if r := e.call("alice", "PATCH", path, map[string]any{"enforce_transitions": true}); r.Status != 409 || errCode(r) != "no_transitions" {
		t.Fatalf("enforcing without transitions: %d %s", r.Status, r.Raw)
	}
	e.must(201, "alice", "POST", path+"/transitions", map[string]any{"name": "Start", "from_status_id": todo, "to_status_id": inProgress})
	e.must(201, "alice", "POST", path+"/transitions", map[string]any{"name": "Finish", "from_status_id": inProgress, "to_status_id": done})
	wf = e.must(201, "alice", "POST", path+"/transitions", map[string]any{"name": "Cancel", "from_status_id": nil, "to_status_id": canceled}).Body
	if len(wf["transitions"].([]any)) != 3 {
		t.Fatalf("transitions: %v", wf["transitions"])
	}
	if r := e.call("alice", "POST", path+"/transitions", map[string]any{"name": "Again", "from_status_id": todo, "to_status_id": inProgress}); r.Status != 409 {
		t.Errorf("duplicate transition: %d", r.Status)
	}
	if r := e.call("carol", "POST", path+"/transitions", map[string]any{"name": "x", "from_status_id": done, "to_status_id": todo}); r.Status != 403 {
		t.Errorf("member adds transition: %d", r.Status)
	}
	e.must(200, "alice", "PATCH", path, map[string]any{"enforce_transitions": true})

	e.must(201, "carol", "POST", items, map[string]any{"title": "Ship it"})
	if r := e.call("carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": done}); r.Status != 422 {
		t.Fatalf("skipping a step must be rejected: %d %s", r.Status, r.Raw)
	}
	allowed := e.must(200, "bob", "GET", items+"/1/transitions", nil).Body["statuses"].([]any)
	names := []string{}
	for _, s := range allowed {
		names = append(names, s.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "In Progress,Canceled" {
		t.Fatalf("allowed from To Do: %v", names)
	}
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": inProgress})
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 2, "status_id": done})

	// Deleting the last transition while enforced would freeze all items.
	for _, tr := range e.defaultWorkflow()["transitions"].([]any)[:2] {
		e.must(200, "alice", "DELETE", path+"/transitions/"+tr.(map[string]any)["id"].(string), nil)
	}
	last := e.defaultWorkflow()["transitions"].([]any)[0].(map[string]any)["id"].(string)
	if r := e.call("alice", "DELETE", path+"/transitions/"+last, nil); r.Status != 409 || errCode(r) != "last_transition" {
		t.Fatalf("delete last transition: %d %s", r.Status, r.Raw)
	}
}

type receiver struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	status   []int // response codes to return, in order (last one repeats)
	srv      *httptest.Server
}

func newReceiver(t *testing.T, status ...int) *receiver {
	rc := &receiver{status: status}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.requests = append(rc.requests, r)
		rc.bodies = append(rc.bodies, b)
		code := rc.status[min(len(rc.requests)-1, len(rc.status)-1)]
		rc.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

func (rc *receiver) count() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.requests)
}

func TestWebhookDelivery(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	rc := newReceiver(t, 500, 200) // first attempt fails, retry succeeds
	base := "/api/v1/workspaces/acme/webhooks"

	if r := e.call("carol", "GET", base, nil); r.Status != 403 {
		t.Fatalf("member reads webhooks: %d", r.Status)
	}
	created := e.must(201, "alice", "POST", base, map[string]any{"url": rc.srv.URL + "/hook", "events": []string{"item.*"}}).Body
	secret := created["secret"].(string)
	hookID := created["webhook"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret: %q", secret)
	}
	if got := e.must(200, "alice", "GET", base+"/"+hookID, nil).Raw; strings.Contains(got, secret) {
		t.Fatal("secret must never be readable after creation")
	}
	var stored []byte
	_ = e.pool.QueryRow(context.Background(), `SELECT secret_encrypted FROM webhooks`).Scan(&stored)
	if bytes.Contains(stored, []byte(secret)) || bytes.Contains(stored, []byte(secret[6:])) {
		t.Fatal("secret stored in plain text")
	}

	e.must(201, "carol", "POST", items, map[string]any{"title": "Webhook me"})
	e.drain()
	if rc.count() != 1 {
		t.Fatalf("first attempt: %d requests", rc.count())
	}
	e.retryNow()
	e.drain()
	if rc.count() != 2 {
		t.Fatalf("retry: %d requests", rc.count())
	}

	req, body := rc.requests[1], rc.bodies[1]
	if req.Header.Get("Nebula-Event-Type") != "item.created" || req.Header.Get("Nebula-Delivery-Attempt") != "2" {
		t.Fatalf("headers: %v", req.Header)
	}
	sig := req.Header.Get("Nebula-Signature")
	var ts int64
	if _, err := fmtSscanf(sig, &ts); err != nil {
		t.Fatalf("signature header %q: %v", sig, err)
	}
	if want := engine.Sign(secret, time.Unix(ts, 0), body); sig != want {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", sig, want)
	}
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	if payload["type"] != "item.created" || payload["item"].(map[string]any)["key"] != "ACME-1" {
		t.Fatalf("payload: %s", body)
	}

	deliveries := e.must(200, "alice", "GET", base+"/"+hookID+"/deliveries", nil).Body["deliveries"].([]any)
	if len(deliveries) != 2 || deliveries[0].(map[string]any)["status_code"] != 200.0 || deliveries[1].(map[string]any)["status_code"] != 500.0 {
		t.Fatalf("deliveries: %v", deliveries)
	}

	t.Run("unsubscribed events are not sent", func(t *testing.T) {
		before := rc.count()
		e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"})
		e.drain()
		if rc.count() != before {
			t.Fatal("project.created delivered to an item.* webhook")
		}
	})

	t.Run("4xx is permanent, repeated failures disable", func(t *testing.T) {
		gone := newReceiver(t, 410)
		h := e.must(201, "alice", "POST", base, map[string]any{"url": gone.srv.URL, "events": []string{"*"}}).Body
		id := h["webhook"].(map[string]any)["id"].(string)
		_, _ = e.pool.Exec(context.Background(), `UPDATE webhooks SET failure_count = $1 WHERE id = $2`, store.WebhookDisableAfter-1, id)
		e.must(201, "carol", "POST", items, map[string]any{"title": "x"})
		e.drain()
		e.retryNow()
		e.drain()
		if gone.count() != 1 {
			t.Fatalf("410 must not be retried: %d attempts", gone.count())
		}
		got := e.must(200, "alice", "GET", base+"/"+id, nil).Body
		if got["active"] != false || got["disabled_reason"] == nil {
			t.Fatalf("webhook not disabled: %v", got)
		}
		e.must(200, "alice", "PATCH", base+"/"+id, map[string]any{"active": true})
		if e.must(200, "alice", "GET", base+"/"+id, nil).Body["failure_count"] != 0.0 {
			t.Fatal("re-activation must reset failure_count")
		}
	})

	t.Run("rotate secret", func(t *testing.T) {
		newSecret := e.must(200, "alice", "POST", base+"/"+hookID+"/rotate-secret", nil).Body["secret"].(string)
		if newSecret == secret {
			t.Fatal("secret not rotated")
		}
	})
}

func fmtSscanf(sig string, ts *int64) (int, error) {
	t, _, _ := strings.Cut(strings.TrimPrefix(sig, "t="), ",")
	return 1, json.Unmarshal([]byte(t), ts)
}

func TestWebhookSSRFProtection(t *testing.T) {
	pool := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	h := NewAPIHandler(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(), DB: pool,
		Store: store.New(pool).WithSecrets(box), Verifier: testUsers, RequestTimeout: 10 * time.Second,
		Webhooks: safehttp.Options{RequireHTTPS: true}, // production policy
	})
	e := &env{t: t, h: h, pool: pool}
	e.setup()
	for _, u := range []string{
		"http://hooks.example.com/x", "https://127.0.0.1/x", "https://169.254.169.254/latest/meta-data/",
		"https://localhost/x", "https://10.0.0.5/x", "https://[::1]/x", "https://user:pw@example.com/x",
		"https://0x7f000001/", "https://example.com:6379/",
	} {
		if r := e.call("alice", "POST", "/api/v1/workspaces/acme/webhooks", map[string]any{"url": u, "events": []string{"*"}}); r.Status != 422 {
			t.Errorf("%s: %d %s", u, r.Status, r.Raw)
		}
	}
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/webhooks", map[string]any{"url": "https://hooks.example.com/nebula", "events": []string{"item.*"}})
	if r := e.call("alice", "POST", "/api/v1/workspaces/acme/webhooks", map[string]any{"url": "https://hooks.example.com/x", "events": []string{"bad pattern"}}); r.Status != 422 {
		t.Errorf("bad event pattern: %d", r.Status)
	}
}

func TestAutomationRules(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	base := "/api/v1/workspaces/acme/automations"
	wf := e.defaultWorkflow()
	inProgress := statusID(t, wf, "In Progress")
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	bobID := e.must(200, "bob", "GET", "/api/v1/me", nil).Body["id"].(string)

	defaults := e.must(200, "alice", "GET", base, nil).Body["automations"].([]any)
	if len(defaults) != 2 {
		t.Fatalf("default rules: %v", defaults)
	}
	if r := e.call("carol", "GET", base, nil); r.Status != 403 {
		t.Fatalf("member reads rules: %d", r.Status)
	}

	t.Run("validation", func(t *testing.T) {
		cases := map[string]map[string]any{
			"external email": {"name": "a", "trigger": "item.created", "actions": []any{map[string]any{"type": "send_email", "to": []string{"attacker@evil.example"}, "subject": "x", "body": "y"}}},
			"changed on create": {"name": "b", "trigger": "item.created", "conditions": []any{map[string]any{"field": "changed", "op": "includes", "value": "title"}},
				"actions": []any{map[string]any{"type": "set_assignee", "user_id": carolID}}},
			"unknown status":    {"name": "c", "trigger": "item.created", "actions": []any{map[string]any{"type": "set_status", "status_id": "01a20727-0000-7000-8000-000000000000"}}},
			"non-member user":   {"name": "d", "trigger": "item.created", "actions": []any{map[string]any{"type": "set_assignee", "user_id": "01a20727-0000-7000-8000-000000000000"}}},
			"bad trigger":       {"name": "e", "trigger": "item.deleted", "actions": []any{map[string]any{"type": "set_assignee"}}},
			"no actions":        {"name": "f", "trigger": "item.created", "actions": []any{}},
			"bad op":            {"name": "g", "trigger": "item.created", "conditions": []any{map[string]any{"field": "type", "op": "regex", "value": ".*"}}, "actions": []any{map[string]any{"type": "set_assignee"}}},
			"multiline subject": {"name": "h", "trigger": "item.created", "actions": []any{map[string]any{"type": "send_email", "to": []string{"assignee"}, "subject": "a\nBcc: x@y.z", "body": "y"}}},
		}
		for name, body := range cases {
			if r := e.call("alice", "POST", base, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	triage := e.must(201, "alice", "POST", base, map[string]any{
		"name": "Triage bugs", "trigger": "item.created",
		"conditions": []any{map[string]any{"field": "type", "op": "eq", "value": "bug"}},
		"actions": []any{
			map[string]any{"type": "set_status", "status_id": inProgress},
			map[string]any{"type": "set_assignee", "user_id": carolID},
			map[string]any{"type": "send_email", "to": []string{"assignee", "carol@example.com"}, "subject": "New bug {{item.key}}: {{item.title}}", "body": "{{item.key}} is {{item.status}}."},
		},
	}).Body
	// Fires on every human update; must NOT fire on the triage rule's own update.
	e.must(201, "alice", "POST", base, map[string]any{
		"name": "Escalate on edit", "trigger": "item.updated",
		"conditions": []any{map[string]any{"field": "changed", "op": "includes", "value": "title"}},
		"actions":    []any{map[string]any{"type": "set_assignee", "user_id": bobID}},
	})

	e.must(201, "alice", "POST", items, map[string]any{"title": "Login crashes", "type": "bug"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Write docs", "type": "task"})
	e.drain()

	bug := e.must(200, "alice", "GET", items+"/1", nil).Body
	if bug["status"].(map[string]any)["name"] != "In Progress" || bug["assignee_id"] != carolID {
		t.Fatalf("triage not applied: status=%v assignee=%v", bug["status"], bug["assignee_id"])
	}
	if task := e.must(200, "alice", "GET", items+"/2", nil).Body; task["assignee_id"] != nil {
		t.Fatalf("rule matched a task: %v", task)
	}

	var source, ruleID string
	_ = e.pool.QueryRow(context.Background(), `SELECT source, automation_rule_id::text FROM events
		WHERE action = 'item.updated' ORDER BY id LIMIT 1`).Scan(&source, &ruleID)
	if source != "automation" || ruleID != triage["id"] {
		t.Fatalf("automation provenance: source=%q rule=%q", source, ruleID)
	}

	// Emails: triage (to carol, deduplicated) + default "notify on reassignment"
	// does not fire because the reassignment came from automation.
	msgs := e.mail.messages()
	if len(msgs) != 1 || len(msgs[0].To) != 1 || msgs[0].To[0] != "carol@example.com" || msgs[0].Subject != "New bug ACME-1: Login crashes" {
		t.Fatalf("emails: %+v", msgs)
	}
	if msgs[0].Body != "ACME-1 is In Progress." {
		t.Fatalf("rendered body: %q", msgs[0].Body)
	}

	// A human edit triggers the second rule (and the default reassignment email to bob).
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 2, "title": "Login crashes on Safari"})
	e.drain()
	bug = e.must(200, "alice", "GET", items+"/1", nil).Body
	if bug["assignee_id"] != bobID {
		t.Fatalf("escalation not applied: %v", bug["assignee_id"])
	}
	e.drain()
	if n := len(e.mail.messages()); n != 1 {
		t.Fatalf("automation-caused reassignment must not trigger more rules; emails=%d", n)
	}

	t.Run("disable and delete", func(t *testing.T) {
		id := triage["id"].(string)
		r := e.must(200, "alice", "PATCH", base+"/"+id, map[string]any{"enabled": false}).Body
		if r["enabled"] != false {
			t.Fatal("not disabled")
		}
		e.must(201, "alice", "POST", items, map[string]any{"title": "Another bug", "type": "bug"})
		e.drain()
		if e.must(200, "alice", "GET", items+"/3", nil).Body["assignee_id"] != nil {
			t.Fatal("disabled rule ran")
		}
		e.must(204, "alice", "DELETE", base+"/"+id, nil)
		e.must(404, "alice", "GET", base+"/"+id, nil)
	})
}

func TestDefaultAssignmentNotification(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	aliceID := e.must(200, "alice", "GET", "/api/v1/me", nil).Body["id"].(string)

	e.must(201, "alice", "POST", items, map[string]any{"title": "Self assigned", "assignee_id": aliceID})
	e.drain()
	if n := len(e.mail.messages()); n != 0 {
		t.Fatalf("no email for assigning yourself; got %d", n)
	}
	e.must(201, "alice", "POST", items, map[string]any{"title": "For Carol", "assignee_id": carolID})
	e.drain()
	msgs := e.mail.messages()
	if len(msgs) != 1 || msgs[0].To[0] != "carol@example.com" || !strings.Contains(msgs[0].Subject, "ACME-2") {
		t.Fatalf("assignment email: %+v", msgs)
	}
}

// Regression: conditions must see the item as of the event. Here an
// "assign on create" rule and the default "notify assignee on create" rule run
// on the same event; the item was unassigned when created, so no email.
func TestRulesSeeEventTimeState(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/automations", map[string]any{
		"name": "Auto-assign", "trigger": "item.created",
		"actions": []any{map[string]any{"type": "set_assignee", "user_id": carolID}},
	})
	// Run jobs newest-first so the auto-assign rule finishes before the notify rule evaluates.
	if _, err := e.pool.Exec(context.Background(), `UPDATE automation_rules SET created_at = now() - interval '1 day'
		WHERE name = 'Auto-assign'`); err != nil {
		t.Fatal(err)
	}
	e.must(201, "alice", "POST", items, map[string]any{"title": "Unassigned at creation"})
	e.drain()
	if got := e.must(200, "alice", "GET", items+"/1", nil).Body["assignee_id"]; got != carolID {
		t.Fatalf("auto-assign: %v", got)
	}
	if n := len(e.mail.messages()); n != 0 {
		t.Fatalf("notify-on-create fired on a later state: %d emails", n)
	}
}

func TestOutboxIsTransactional(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	count := func(sql string) int {
		var n int
		if err := e.pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	events, jobs := count(`SELECT count(*) FROM events`), count(`SELECT count(*) FROM jobs WHERE kind = 'event.dispatch'`)
	if events != jobs {
		t.Fatalf("every event needs exactly one dispatch job: events=%d jobs=%d", events, jobs)
	}
	// A rejected write leaves neither an event nor a job behind.
	e.call("carol", "POST", items, map[string]any{"title": "", "type": "task"})
	if count(`SELECT count(*) FROM events`) != events || count(`SELECT count(*) FROM jobs`) != jobs {
		t.Fatal("failed request leaked events or jobs")
	}
	e.drain()
	if n := count(`SELECT count(*) FROM jobs WHERE state = 'pending'`); n != 0 {
		t.Fatalf("pending after drain: %d", n)
	}
}
