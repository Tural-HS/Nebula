package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/engine"
	"github.com/Tural-HS/Nebula/internal/mailroute"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

const (
	smtpSecret = "Sup3r-Secret-pa55word!" // must never leave the database encrypted
	mailpitAPI = "http://localhost:8025/api/v1"
)

// devPolicy mirrors docker-compose.yml: Mailpit on localhost:1025 without TLS.
var devPolicy = store.EmailPolicy{AllowPrivate: true, AllowPlaintext: true, Ports: []int{25, 465, 587, 1025, 2525}}

type emailEnv struct {
	*p1env
	key   []byte
	users tokenVerifier
}

// newEmailEnv is the phase-1 env with workspace SMTP routing: instance mail
// goes to the fake sender, workspace mail through the given policy.
func newEmailEnv(t *testing.T, policy store.EmailPolicy, extra tokenVerifier) *emailEnv {
	t.Helper()
	pool := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	st := store.New(pool).WithSecrets(box)
	users := maps.Clone(testUsers)
	maps.Copy(users, extra)
	opts := safehttp.Options{AllowPrivate: true, Timeout: 5 * time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fm := &fakeMail{}
	h := NewAPIHandler(Deps{
		Logger: logger, Metrics: observability.NewMetrics(), DB: pool, Store: st,
		Verifier: users, RequestTimeout: 30 * time.Second, Webhooks: opts, PublicURL: "https://nebula.test",
		Mail: mailroute.Router{Instance: fm, Policy: policy, Timeout: 5 * time.Second},
	})
	eng := &engine.Engine{Store: st, HTTP: safehttp.NewClient(opts), HTTPOpts: opts, Mail: fm,
		MailPolicy: policy, MailTimeout: 5 * time.Second, Logger: logger, Version: "test"}
	return &emailEnv{p1env: &p1env{env: &env{t: t, h: h, pool: pool}, st: st, eng: eng, mail: fm}, key: key, users: users}
}

func (e *emailEnv) wsID(slug string) uuid.UUID {
	e.t.Helper()
	return uuid.MustParse(e.must(200, "alice", "GET", "/api/v1/workspaces/"+slug, nil).Body["id"].(string))
}

func mailpitUp(t *testing.T) {
	t.Helper()
	c, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp", "localhost:1025")
	if err != nil {
		t.Skip("Mailpit is not running on localhost:1025 (make dev)")
	}
	_ = c.Close()
}

// mailpitMessages returns the messages Mailpit holds for one recipient.
func mailpitMessages(t *testing.T, to string) []map[string]any {
	t.Helper()
	var out []map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(t.Context(), "GET", mailpitAPI+"/search?query="+url.QueryEscape("to:"+to), nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("mailpit API: %v", err)
		}
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if out = body.Messages; len(out) > 0 {
			return out
		}
		time.Sleep(100 * time.Millisecond)
	}
	return out
}

func settingsBody(host string, port int, tls string, extra map[string]any) map[string]any {
	b := map[string]any{"host": host, "port": port, "tls_mode": tls, "username": "", "from_address": "Acme Finance <finance@example.com>"}
	maps.Copy(b, extra)
	return b
}

const emailBase = "/api/v1/workspaces/acme/email-settings"

func TestEmailSettingsPasswordAndAudit(t *testing.T) {
	e := newEmailEnv(t, store.EmailPolicy{}, nil)
	e.setup()
	ws := e.wsID("acme")

	got := e.must(200, "alice", "GET", emailBase, nil)
	if got.Body["settings"] != nil || got.Body["instance_configured"] != true {
		t.Fatalf("initial: %s", got.Raw)
	}

	for _, tok := range []string{"carol", "bob"} { // member, guest
		for _, m := range []string{"GET", "PUT", "DELETE"} {
			e.must(403, tok, m, emailBase, settingsBody("mail.example.com", 587, "starttls", nil))
		}
		e.must(403, tok, "POST", emailBase+"/test", map[string]any{})
	}

	put := e.must(200, "alice", "PUT", emailBase, settingsBody("Mail.Example.com", 587, "starttls",
		map[string]any{"username": "finance@example.com", "password": smtpSecret, "reply_to": "Help <help@example.com>"}))
	s := put.Body["settings"].(map[string]any)
	if s["host"] != "mail.example.com" || s["password_set"] != true || s["reply_to"] != `"Help" <help@example.com>` || s["enabled"] != true {
		t.Fatalf("saved: %s", put.Raw)
	}
	if _, ok := s["password"]; ok || strings.Contains(put.Raw, smtpSecret) {
		t.Fatalf("PUT returned the password: %s", put.Raw)
	}
	if r := e.must(200, "alice", "GET", emailBase, nil); strings.Contains(r.Raw, smtpSecret) || strings.Contains(r.Raw, "password\":") {
		t.Fatalf("GET returned the password: %s", r.Raw)
	}

	// Encrypted at rest: the raw column is neither the plaintext nor contains it.
	var raw []byte
	if err := e.pool.QueryRow(t.Context(), `SELECT password_encrypted FROM workspace_email_settings WHERE workspace_id = $1`, ws).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || bytes.Contains(raw, []byte(smtpSecret)) {
		t.Fatalf("password stored in clear: %q", raw)
	}
	if tr, err := e.st.EmailTransport(t.Context(), ws, true); err != nil || tr.Password != smtpSecret {
		t.Fatalf("decrypt: %v %q", err, tr.Password)
	}

	// A kept password is bound to its server and account: changing any of
	// them without re-entering it would let an admin send the stored secret
	// to a server they control (AUTH during a test send).
	for name, body := range map[string]map[string]any{
		"host":     settingsBody("mx.attacker.example", 587, "starttls", map[string]any{"username": "finance@example.com"}),
		"port":     settingsBody("mail.example.com", 465, "starttls", map[string]any{"username": "finance@example.com"}),
		"tls_mode": settingsBody("mail.example.com", 587, "tls", map[string]any{"username": "finance@example.com"}),
		"username": settingsBody("mail.example.com", 587, "starttls", map[string]any{"username": "other@example.com"}),
	} {
		r := e.must(422, "alice", "PUT", emailBase, body)
		if !strings.Contains(r.Raw, "password") {
			t.Fatalf("%s change without password: %s", name, r.Raw)
		}
	}
	if s := e.must(200, "alice", "GET", emailBase, nil).Body["settings"].(map[string]any); s["host"] != "mail.example.com" {
		t.Fatalf("rejected PUT changed the settings: %v", s)
	}
	// With the password entered again the move is allowed.
	e.must(200, "alice", "PUT", emailBase, settingsBody("mail.example.com", 465, "tls",
		map[string]any{"username": "finance@example.com", "password": smtpSecret}))

	// Omitted password: kept (only From changes). Explicit null: cleared.
	keep := e.must(200, "alice", "PUT", emailBase, settingsBody("mail.example.com", 465, "tls",
		map[string]any{"username": "finance@example.com", "from_address": "Acme Treasury <treasury@example.com>"}))
	if keep.Body["settings"].(map[string]any)["password_set"] != true {
		t.Fatalf("omitted password was not kept: %s", keep.Raw)
	}
	if tr, _ := e.st.EmailTransport(t.Context(), ws, true); tr.Password != smtpSecret {
		t.Fatal("password changed although omitted")
	}
	cleared := e.must(200, "alice", "PUT", emailBase, settingsBody("mail.example.com", 465, "tls", map[string]any{"password": nil}))
	if cleared.Body["settings"].(map[string]any)["password_set"] != false {
		t.Fatalf("null did not clear: %s", cleared.Raw)
	}

	e.must(204, "alice", "DELETE", emailBase, nil)
	e.must(404, "alice", "DELETE", emailBase, nil)
	if r := e.must(200, "alice", "GET", emailBase, nil); r.Body["settings"] != nil {
		t.Fatalf("after delete: %s", r.Raw)
	}

	// Audit: field names and host, never the secret.
	audit := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/audit-log?limit=100", nil).Raw
	for _, want := range []string{store.EmailSettingsUpdatedAction, store.EmailSettingsDeletedAction, `"password"`, "mail.example.com"} {
		if !strings.Contains(audit, want) {
			t.Errorf("audit log lacks %s", want)
		}
	}
	e.noSecretAnywhere(smtpSecret)
}

// noSecretAnywhere checks events (audit, webhooks) and jobs never carry s.
func (e *emailEnv) noSecretAnywhere(s string) {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.t.Context(), `SELECT (SELECT count(*) FROM events WHERE payload::text LIKE '%' || $1 || '%')
		+ (SELECT count(*) FROM jobs WHERE payload::text LIKE '%' || $1 || '%')`, s).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	if n != 0 {
		e.t.Fatalf("%d events/jobs contain the SMTP password", n)
	}
}

func TestEmailSettingsValidationAndSSRF(t *testing.T) {
	e := newEmailEnv(t, store.EmailPolicy{}, nil) // production policy
	e.setup()
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"loopback IP", settingsBody("127.0.0.1", 587, "starttls", nil), "host"},
		{"private IP", settingsBody("10.0.0.5", 587, "starttls", nil), "host"},
		{"metadata IP", settingsBody("169.254.169.254", 587, "starttls", nil), "host"},
		{"IPv6 loopback", settingsBody("[::1]", 587, "starttls", nil), "host"},
		{"localhost", settingsBody("localhost", 587, "starttls", nil), "host"},
		{"internal name", settingsBody("metadata.google.internal", 587, "starttls", nil), "host"},
		{"obfuscated IP", settingsBody("2130706433", 587, "starttls", nil), "host"},
		{"URL instead of host", settingsBody("smtp://mail.example.com", 587, "starttls", nil), "host"},
		{"port outside the list", settingsBody("mail.example.com", 1025, "starttls", nil), "port"},
		{"port out of range", settingsBody("mail.example.com", 70000, "starttls", nil), "port"},
		{"plaintext refused", settingsBody("mail.example.com", 25, "none", nil), "tls_mode"},
		{"unknown TLS mode", settingsBody("mail.example.com", 25, "ssl", nil), "tls_mode"},
		{"bad from", settingsBody("mail.example.com", 587, "starttls", map[string]any{"from_address": "not an address"}), "from_address"},
		{"from header injection", settingsBody("mail.example.com", 587, "starttls", map[string]any{"from_address": "a@example.com\r\nBcc: x@example.com"}), "from_address"},
		{"bad reply-to", settingsBody("mail.example.com", 587, "starttls", map[string]any{"reply_to": "nope"}), "reply_to"},
		// 130 Cyrillic letters fit the 400-character input limit but grow past
		// the column limit once RFC 2047 encoded: 422, not a CHECK violation.
		{"encoded from too long", settingsBody("mail.example.com", 587, "starttls", map[string]any{"from_address": strings.Repeat("Ж", 130) + " <a@example.com>"}), "from_address"},
		{"encoded reply-to too long", settingsBody("mail.example.com", 587, "starttls", map[string]any{"reply_to": strings.Repeat("Ж", 130) + " <a@example.com>"}), "reply_to"},
		{"password with newline", settingsBody("mail.example.com", 587, "starttls", map[string]any{"password": "a\r\nb"}), "password"},
	}
	for _, c := range cases {
		r := e.must(422, "alice", "PUT", emailBase, c.body)
		if f := r.Body["error"].(map[string]any)["field"]; f != c.field {
			t.Errorf("%s: field %v, want %s (%s)", c.name, f, c.field, r.Raw)
		}
	}

	// Credentials are never sent in clear, even where plaintext is allowed.
	d := newEmailEnv(t, devPolicy, nil)
	d.setup()
	d.must(422, "alice", "PUT", emailBase, settingsBody("mailpit", 1025, "none", map[string]any{"username": "u", "password": "p"}))
	d.must(200, "alice", "PUT", emailBase, settingsBody("mailpit", 1025, "none", nil))
}

func TestEmailTestSendToMailpit(t *testing.T) {
	mailpitUp(t)
	addr := fmt.Sprintf("smtp-test-%s@example.com", uuid.NewString()[:8])
	e := newEmailEnv(t, devPolicy, tokenVerifier{
		"zoe": {Issuer: "https://idp.test", Subject: "sub-zoe-" + addr, Email: addr, Name: "Zoe", EmailVerified: true},
	})
	e.setup()
	e.must(200, "zoe", "GET", "/api/v1/me", nil)
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": addr, "role": "admin"})

	// No workspace settings: the test goes through the instance sender.
	r := e.must(200, "zoe", "POST", emailBase+"/test", nil)
	if r.Body["ok"] != true || r.Body["source"] != "instance" || len(e.mail.messages()) != 1 {
		t.Fatalf("instance test: %s", r.Raw)
	}

	e.must(200, "zoe", "PUT", emailBase, settingsBody("localhost", 1025, "none", map[string]any{"from_address": "Acme Finance <finance@example.com>"}))
	r = e.must(200, "zoe", "POST", emailBase+"/test", map[string]any{})
	if r.Body["ok"] != true || r.Body["source"] != "workspace" || r.Body["to"] != addr {
		t.Fatalf("workspace test: %s", r.Raw)
	}
	msgs := mailpitMessages(t, addr)
	if len(msgs) != 1 {
		t.Fatalf("mailpit has %d messages for %s", len(msgs), addr)
	}
	if from := msgs[0]["From"].(map[string]any)["Address"]; from != "finance@example.com" {
		t.Fatalf("from %v", from)
	}
	if len(e.mail.messages()) != 1 {
		t.Fatal("workspace test went through the instance sender")
	}

	// Recipients: members only; never an arbitrary address.
	if r := e.must(422, "zoe", "POST", emailBase+"/test", map[string]any{"to": "stranger@example.org"}); r.Body["error"].(map[string]any)["field"] != "to" {
		t.Fatalf("non-member: %s", r.Raw)
	}
	if r := e.must(200, "zoe", "POST", emailBase+"/test", map[string]any{"to": "Carol@Example.com"}); r.Body["to"] != "carol@example.com" {
		t.Fatalf("member: %s", r.Raw)
	}

	// Real workspace mail (an automation email job) is routed the same way.
	e.mail.reset()
	other := "smtp-job-" + uuid.NewString()[:8] + "@example.com"
	if err := e.st.EnqueueEmail(t.Context(), e.wsID("acme"), []string{other}, "Overdue", "body", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.st.EnqueueEmail(t.Context(), uuid.Nil, []string{"ops@example.com"}, "No workspace", "body", ""); err != nil {
		t.Fatal(err)
	}
	e.drain()
	if got := mailpitMessages(t, other); len(got) != 1 {
		t.Fatalf("workspace job: %d messages in mailpit", len(got))
	}
	if m := e.mail.messages(); len(m) != 1 || m[0].Subject != "No workspace" {
		t.Fatalf("instance got %v", m)
	}

	// Disabled settings: workspace mail falls back to the instance again.
	e.mail.reset()
	e.must(200, "zoe", "PUT", emailBase, settingsBody("localhost", 1025, "none", map[string]any{"enabled": false}))
	if err := e.st.EnqueueEmail(t.Context(), e.wsID("acme"), []string{other}, "Fallback", "body", ""); err != nil {
		t.Fatal(err)
	}
	e.drain()
	if m := e.mail.messages(); len(m) != 1 || m[0].Subject != "Fallback" {
		t.Fatalf("disabled settings did not fall back: %v", m)
	}

	audit := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/audit-log?limit=100", nil).Raw
	if !strings.Contains(audit, store.EmailSettingsTestedAction) {
		t.Error("test sends are not audited")
	}
}

func TestEmailTestErrorsAndRateLimit(t *testing.T) {
	// A closed port on loopback: the connection is refused at once.
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	policy := devPolicy
	policy.Ports = append([]int{port}, devPolicy.Ports...)

	e := newEmailEnv(t, policy, nil)
	e.setup()
	e.must(200, "alice", "PUT", emailBase, settingsBody("127.0.0.1", port, "none", nil))
	for i := range emailTestBurst {
		r := e.must(200, "alice", "POST", emailBase+"/test", nil)
		msg, _ := r.Body["error"].(string)
		if r.Body["ok"] != false || !strings.Contains(msg, "refused") || strings.Contains(msg, "127.0.0.1") {
			t.Fatalf("send %d: %s", i, r.Raw)
		}
	}
	r := e.must(429, "alice", "POST", emailBase+"/test", nil)
	if errCode(r) != "rate_limited" {
		t.Fatalf("rate limit: %s", r.Raw)
	}

	// Unverified requester without "to": refused, nothing is sent.
	f := newEmailEnv(t, devPolicy, nil)
	f.setup()
	f.must(200, "frank", "GET", "/api/v1/me", nil)
	f.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "frank@example.com", "role": "admin"})
	if r := f.must(422, "frank", "POST", emailBase+"/test", nil); errCode(r) != "email_unverified" {
		t.Fatalf("unverified: %s", r.Raw)
	}
}

func TestEmailPolicyIsCheckedAtSendTime(t *testing.T) {
	// Saved under the dev policy, sent under the production one: blocked.
	e := newEmailEnv(t, devPolicy, nil)
	e.setup()
	e.must(200, "alice", "PUT", emailBase, settingsBody("127.0.0.1", 1025, "none", nil))
	strict := &engine.Engine{Store: e.st, Mail: e.mail, MailPolicy: store.EmailPolicy{}, Logger: e.eng.Logger}
	jobs := e.claimEmail(e.wsID("acme"))
	if err := strict.Handlers()[store.JobSendEmail](t.Context(), jobs); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("got %v, want a policy rejection", err)
	}
	if len(e.mail.messages()) != 0 {
		t.Fatal("blocked workspace mail fell back to the instance sender")
	}
}

func (e *emailEnv) claimEmail(ws uuid.UUID) store.Job {
	e.t.Helper()
	if err := e.st.EnqueueEmail(e.t.Context(), ws, []string{"ops@example.com"}, "x", "y", ""); err != nil {
		e.t.Fatal(err)
	}
	jobs, err := e.st.ClaimJobs(context.Background(), 50, time.Minute)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, j := range jobs {
		if j.Kind == store.JobSendEmail {
			return j
		}
	}
	e.t.Fatal("no email job")
	return store.Job{}
}

func TestEmailPasswordKeyRotation(t *testing.T) {
	e := newEmailEnv(t, store.EmailPolicy{}, nil)
	e.setup()
	ws := e.wsID("acme")
	e.must(200, "alice", "PUT", emailBase, settingsBody("mail.example.com", 587, "starttls",
		map[string]any{"username": "u@example.com", "password": smtpSecret}))
	var before []byte
	_ = e.pool.QueryRow(t.Context(), `SELECT password_encrypted FROM workspace_email_settings WHERE workspace_id = $1`, ws).Scan(&before)

	newKey := make([]byte, 32)
	_, _ = rand.Read(newKey)
	box, _ := secretbox.New(newKey)
	box, err := box.WithOldKeys(base64.StdEncoding.EncodeToString(e.key))
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.New(e.pool).WithSecrets(box).ReencryptSecrets(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("re-encrypt: %d %v", n, err)
	}
	onlyNew, _ := secretbox.New(newKey)
	if tr, err := store.New(e.pool).WithSecrets(onlyNew).EmailTransport(t.Context(), ws, true); err != nil || tr.Password != smtpSecret {
		t.Fatalf("new key cannot read the password: %v", err)
	}
	if _, err := e.st.EmailTransport(t.Context(), ws, true); err == nil {
		t.Fatal("old key still decrypts after rotation")
	}
	if n, err := store.New(e.pool).WithSecrets(box).ReencryptSecrets(t.Context()); err != nil || n != 0 {
		t.Fatalf("second run: %d %v", n, err)
	}
	var after []byte
	_ = e.pool.QueryRow(t.Context(), `SELECT password_encrypted FROM workspace_email_settings WHERE workspace_id = $1`, ws).Scan(&after)
	if bytes.Equal(before, after) {
		t.Fatal("ciphertext unchanged")
	}

	// The old key can no longer open the password. While the row is enabled
	// mail must not silently leave through the instance; once the admin
	// disables it, it does (ADR-0038: disabled = instance), without trying
	// to decrypt.
	r := mailroute.Router{Instance: e.mail, Store: e.st, Policy: store.EmailPolicy{}}
	if _, err := r.For(t.Context(), ws); err == nil {
		t.Fatal("undecryptable enabled row fell back to the instance")
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE workspace_email_settings SET is_enabled = false WHERE workspace_id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	if route, err := r.For(t.Context(), ws); err != nil || route.Source != mailroute.SourceInstance {
		t.Fatalf("disabled undecryptable row: source %q, err %v", route.Source, err)
	}
	if _, err := r.ForTest(t.Context(), ws); err == nil {
		t.Fatal("test send of an undecryptable row did not report the error")
	}
}
