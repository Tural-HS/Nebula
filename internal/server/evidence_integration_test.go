package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/engine"
	"github.com/Tural-HS/Nebula/internal/evidence"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// Audit evidence export (ADR-0029). Synthetic people and data only.

type evEnv struct {
	*p1env
	signer *evidence.Signer
}

func newEvidenceEnv(t *testing.T, withKey bool) *evEnv {
	t.Helper()
	pool := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	st := store.New(pool).WithSecrets(box)
	var signer *evidence.Signer
	if withKey {
		seed := make([]byte, 32)
		_, _ = rand.Read(seed)
		signer, _ = evidence.ParseSigningKey(base64.StdEncoding.EncodeToString(seed))
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := safehttp.Options{AllowPrivate: true, Timeout: 5 * time.Second}
	h := NewAPIHandler(Deps{
		Logger: logger, Metrics: observability.NewMetrics(), DB: pool, Store: st, Verifier: testUsers,
		RequestTimeout: 10 * time.Second, Webhooks: opts, Version: "v-test", EvidenceSigner: signer,
	})
	eng := &engine.Engine{Store: st, HTTP: safehttp.NewClient(opts), HTTPOpts: opts, Mail: &fakeMail{}, Logger: logger, Version: "test"}
	return &evEnv{p1env: &p1env{env: &env{t: t, h: h, pool: pool}, st: st, eng: eng, mail: &fakeMail{}}, signer: signer}
}

// download posts and returns the raw response.
func (e *evEnv) download(token, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequestWithContext(e.t.Context(), "POST", path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *evEnv) pkg(rec *httptest.ResponseRecorder) *evidence.Package {
	e.t.Helper()
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		e.t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	b := rec.Body.Bytes()
	p, err := evidence.Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		e.t.Fatal(err)
	}
	if rep := p.Verify(e.signer.Fingerprint()); !rep.OK {
		e.t.Fatalf("package does not verify: %v", rep.Problems)
	}
	return p
}

func jsonFile[T any](t *testing.T, p *evidence.Package, name string) T {
	t.Helper()
	var v T
	b, ok := p.File(name)
	if !ok {
		t.Fatalf("%s missing", name)
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func actions(t *testing.T, p *evidence.Package) []string {
	var out []string
	for _, e := range jsonFile[[]store.EvidenceEvent](t, p, evidence.EventsFile) {
		out = append(out, e.Action)
	}
	return out
}

// approvedItem: carol prepares ACME-1 (comment, attachment), eve approves the move to Done.
func (e *evEnv) approvedItem() {
	e.t.Helper()
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş: təchizatçı 42"})
	e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "Qəbz əlavə olundu"})
	var itemID uuid.UUID
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM items WHERE number = 1`).Scan(&itemID); err != nil {
		e.t.Fatal(err)
	}
	// Attachment metadata only: the export never reads blobs.
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO attachments (id, workspace_id, item_id, filename, content_type, size_bytes, sha256, storage_key)
		SELECT gen_random_uuid(), workspace_id, id, 'qebz.pdf', 'application/pdf', 1234, sha256('qebz'), encode(sha256('qebz'), 'hex') FROM items WHERE id = $1`, itemID); err != nil {
		e.t.Fatal(err)
	}
	req := e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"to_status_id": done, "version": 1, "comment": "Hazırdır"}).Body
	id := req["approvals"].([]any)[0].(map[string]any)["id"].(string)
	e.must(200, "eve", "POST", items+"/1/approvals/"+id+"/approve", map[string]any{"comment": "Yoxlandı"})
}

func TestEvidenceItemExport(t *testing.T) {
	e := newEvidenceEnv(t, true)
	e.approvedItem()

	for _, u := range []string{"carol", "bob"} {
		if r := e.download(u, items+"/1/evidence", map[string]any{}); r.Code != 403 {
			t.Fatalf("%s exports: %d", u, r.Code)
		}
	}
	if r := e.download("alice", items+"/99/evidence", map[string]any{}); r.Code != 404 {
		t.Fatalf("unknown item: %d", r.Code)
	}
	rec := e.download("alice", items+"/1/evidence", map[string]any{"language": "en"})
	p := e.pkg(rec)
	m := p.Manifest
	if m.Kind != evidence.KindItem || m.Parameters.ItemKey != "ACME-1" || m.ServerVersion != "v-test" ||
		m.ExportedBy.Email != "alice@example.com" || m.ExportedBy.Role != "owner" || m.Parameters.Language != "en" {
		t.Fatalf("manifest: %+v", m)
	}
	acts := strings.Join(actions(t, p), ",")
	for _, want := range []string{"item.created", "comment.created", "approval.requested", "approval.approved", "approval.applied", "item.updated"} {
		if !strings.Contains(acts, want) {
			t.Errorf("events miss %s: %s", want, acts)
		}
	}
	approvals := jsonFile[[]store.EvidenceApproval](t, p, "data/approvals.json")
	if len(approvals) != 1 || approvals[0].State != "approved" || len(approvals[0].Decisions) != 1 ||
		approvals[0].Decisions[0].UserID.String() != e.userID("eve") || approvals[0].RequestedBy.String() != e.userID("carol") {
		t.Fatalf("approvals: %+v", approvals)
	}
	att := jsonFile[[]store.EvidenceAttachment](t, p, "data/attachments.json")
	if len(att) != 1 || att[0].Filename != "qebz.pdf" || len(att[0].SHA256) != 64 {
		t.Fatalf("attachments: %+v", att)
	}
	report, _ := p.File(evidence.ReportFile)
	for _, want := range []string{"Audit evidence package", "Ödəniş: təchizatçı 42", "Qəbz əlavə olundu", "Yoxlandı", att[0].SHA256, "Eve"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report misses %q", want)
		}
	}

	// The export is itself an event, on the item, with the manifest hash.
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?action=evidence.", nil).Body["activity"].([]any)
	if len(audit) != 1 {
		t.Fatalf("evidence events: %v", audit)
	}
	ev := audit[0].(map[string]any)
	data := ev["data"].(map[string]any)
	if ev["entity_type"] != "item" || data["manifest_sha256"] != rec.Header().Get("X-Evidence-Manifest-Sha256") ||
		data["export_id"] != m.ExportID.String() || data["key_fingerprint"] != e.signer.Fingerprint() || data["item_key"] != "ACME-1" {
		t.Fatalf("evidence.exported: %v", ev)
	}
	if !strings.Contains(e.must(200, "carol", "GET", items+"/1/activity", nil).Raw, "evidence.exported") {
		t.Fatal("the item's history shows the export")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="nebula-evidence-acme-ACME-1.zip"` {
		t.Fatalf("content-disposition: %s", cd)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("evidence must not be cached")
	}
}

func TestEvidenceScopeExportAndPrivacy(t *testing.T) {
	e := newEvidenceEnv(t, true)
	e.approvedItem()                                                             // ACME-1, approved
	e.must(201, "carol", "POST", items, map[string]any{"title": "Adi tapşırıq"}) // ACME-2, no approval
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq", "visibility": "private"})
	sec := wsBase + "/projects/SEC/items"
	e.must(201, "alice", "POST", sec, map[string]any{"title": "Məxfi müqavilə"}) // SEC-1
	e.must(201, "alice", "POST", items+"/2/links", map[string]any{"type": "relates", "target": "SEC-1"})

	today := time.Now().UTC().Format(time.DateOnly)
	from := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	to := time.Now().UTC().AddDate(0, 0, 1).Format(time.DateOnly)
	scope := map[string]any{"from": from, "to": to}

	// Admin: everything, private project included (ADR-0013: admins see all).
	all := e.pkg(e.download("alice", wsBase+"/evidence/exports", scope))
	if all.Manifest.Counts.Items != 3 || all.Manifest.Counts.Links != 1 || all.Manifest.Counts.Projects != 2 {
		t.Fatalf("admin scope counts: %+v", all.Manifest.Counts)
	}
	appr := e.pkg(e.download("alice", wsBase+"/evidence/exports", map[string]any{"from": from, "to": to, "approvals_only": true}))
	if appr.Manifest.Counts.Items != 1 || !appr.Manifest.Parameters.ApprovalsOnly {
		t.Fatalf("approvals only: %+v", appr.Manifest.Counts)
	}
	one := e.pkg(e.download("alice", wsBase+"/evidence/exports", map[string]any{"from": today, "to": today, "project_keys": []string{"ACME"}}))
	for _, it := range jsonFile[[]store.EvidenceItem](t, one, "data/items.json") {
		if it.ProjectKey != "ACME" {
			t.Fatalf("project filter: %s", it.Key)
		}
	}

	// The workspace-level event does not name the private project.
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?action=evidence.", nil).Raw
	if strings.Contains(audit, `"SEC"`) || !strings.Contains(audit, "project_ids") {
		t.Fatalf("scope export event: %s", audit)
	}

	// A member's view (the store applies the same predicate everywhere).
	ws, err := e.st.WorkspaceForMember(context.Background(), "acme", uuid.MustParse(e.userID("carol")))
	if err != nil {
		t.Fatal(err)
	}
	member := store.Viewer{UserID: uuid.MustParse(e.userID("carol")), Role: ws.Role}
	set, err := e.st.CollectEvidence(context.Background(), store.EvidenceQuery{WorkspaceID: ws.ID, Viewer: member, From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range set.Items {
		if strings.HasPrefix(it.Key, "SEC") {
			t.Fatalf("hidden item exported: %s", it.Key)
		}
	}
	if len(set.Items) != 2 || len(set.Links) != 0 || len(set.Projects) != 1 {
		t.Fatalf("member scope: items %d links %d projects %d", len(set.Items), len(set.Links), len(set.Projects))
	}
	for _, ev := range set.Events {
		if ev.Action == "link.added" {
			t.Fatal("a link event to a hidden item leaks it")
		}
	}
	// A hidden project and a missing one fail the same way.
	var msgs []string
	for _, key := range []string{"SEC", "NOPE"} {
		_, err := e.st.CollectEvidence(context.Background(), store.EvidenceQuery{WorkspaceID: ws.ID, Viewer: member, From: from, To: to,
			ProjectKeys: []string{key}})
		var ve *store.ValidationError
		if !errors.As(err, &ve) || ve.Field != "project_keys" {
			t.Fatalf("%s: %v", key, err)
		}
		msgs = append(msgs, strings.ReplaceAll(ve.Message, key, "?"))
	}
	if msgs[0] != msgs[1] {
		t.Fatalf("hidden vs missing project: %v", msgs)
	}
	secItem, err := e.st.ListProjects(context.Background(), ws.ID, member)
	if err != nil || len(secItem) != 1 {
		t.Fatalf("member projects: %v %v", secItem, err)
	}
}

func TestEvidenceLimitsAndValidation(t *testing.T) {
	e := newEvidenceEnv(t, true)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "Bir"})
	e.must(201, "alice", "POST", items+"/1/comments", map[string]any{"body": "x"})
	path := wsBase + "/evidence/exports"
	for name, body := range map[string]map[string]any{
		"no range":     {},
		"no end":       {"from": "2026-01-01"},
		"bad date":     {"from": "2026-13-01", "to": "2026-12-31"},
		"reversed":     {"from": "2026-02-01", "to": "2026-01-01"},
		"over a year":  {"from": "2025-01-01", "to": "2026-01-02"},
		"too many":     {"from": "2026-01-01", "to": "2026-01-02", "project_keys": make([]string, 51)},
		"unknown proj": {"from": "2026-01-01", "to": "2026-01-02", "project_keys": []string{"NOPE"}},
	} {
		if r := e.download("alice", path, body); r.Code != 422 {
			t.Errorf("%s: %d %s", name, r.Code, r.Body.String())
		}
	}
	if r := e.download("carol", path, map[string]any{"from": "2026-01-01", "to": "2026-01-02"}); r.Code != 403 {
		t.Fatalf("member: %d", r.Code)
	}

	// Limits fail the export instead of truncating it.
	ws, _ := e.st.WorkspaceForMember(context.Background(), "acme", uuid.MustParse(e.userID("alice")))
	var projectID uuid.UUID
	_ = e.pool.QueryRow(context.Background(), `SELECT id FROM projects WHERE key = 'ACME'`).Scan(&projectID)
	q := store.EvidenceQuery{WorkspaceID: ws.ID, Viewer: store.Viewer{UserID: uuid.MustParse(e.userID("alice")), Role: "owner"},
		ProjectID: projectID, Number: 1, Limits: store.EvidenceLimits{MaxDays: 1, MaxItems: 1, MaxEvents: 1, MaxComments: 1,
			MaxApprovals: 1, MaxAttachments: 1, MaxLinks: 1}}
	var tl *store.EvidenceTooLargeError
	if _, err := e.st.CollectEvidence(context.Background(), q); !errors.As(err, &tl) || tl.What != "events" {
		t.Fatalf("event limit: %v", err)
	}
	q.Limits.MaxEvents = 100
	if _, err := e.st.CollectEvidence(context.Background(), q); err != nil {
		t.Fatalf("within limits: %v", err)
	}

	// Rate limit: 5 back to back, then 429 with Retry-After. Input errors found
	// without the database cost nothing; "unknown proj" above used one token.
	for i := range 4 {
		if r := e.download("alice", items+"/1/evidence", map[string]any{}); r.Code != 200 {
			t.Fatalf("export %d: %d %s", i, r.Code, r.Body.String())
		}
	}
	r := e.download("alice", items+"/1/evidence", map[string]any{})
	if r.Code != 429 || r.Header().Get("Retry-After") == "" {
		t.Fatalf("over the export limit: %d", r.Code)
	}
}

func TestEvidenceKeyAndDisabled(t *testing.T) {
	e := newEvidenceEnv(t, true)
	e.setup()
	k := e.must(200, "alice", "GET", wsBase+"/evidence/key", nil).Body
	if k["enabled"] != true || k["fingerprint"] != e.signer.Fingerprint() || k["public_key"] != e.signer.PublicKey() {
		t.Fatalf("key: %v", k)
	}
	if r := e.call("carol", "GET", wsBase+"/evidence/key", nil); r.Status != 403 {
		t.Fatalf("member reads key: %d", r.Status)
	}

	off := newEvidenceEnv(t, false)
	off.setup()
	if k := off.must(200, "alice", "GET", wsBase+"/evidence/key", nil).Body; k["enabled"] != false {
		t.Fatalf("disabled key: %v", k)
	}
	off.must(201, "alice", "POST", items, map[string]any{"title": "Bir"})
	if r := off.download("alice", items+"/1/evidence", map[string]any{}); r.Code != 503 || !strings.Contains(r.Body.String(), "evidence_disabled") {
		t.Fatalf("disabled export: %d %s", r.Code, r.Body.String())
	}
	if strings.Contains(off.must(200, "alice", "GET", wsBase+"/audit-log?action=evidence.", nil).Raw, "evidence.exported") {
		t.Fatal("a refused export is not recorded as one")
	}
}
