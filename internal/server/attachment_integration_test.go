package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/blobstore"
	"github.com/Tural-HS/Nebula/internal/engine"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// A real (tiny) PNG: 1x1 transparent pixel.
var pngBytes, _ = hex.DecodeString("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000d4944415478da63f8cfc0f01f0005000201a5f6cb2b0000000049454e44ae426082")

type attachEnv struct {
	*p1env
	dir string
}

func newAttachEnv(t *testing.T, cfg Attachments) *attachEnv {
	t.Helper()
	pool := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	st := store.New(pool).WithSecrets(box)
	dir := t.TempDir()
	blobs, err := blobstore.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = blobs.Close() })
	cfg.Blobs = blobs
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := safehttp.Options{AllowPrivate: true, Timeout: 5 * time.Second}
	h := NewAPIHandler(Deps{
		Logger: logger, Metrics: observability.NewMetrics(), DB: pool, Store: st,
		Verifier: testUsers, RequestTimeout: 10 * time.Second, Webhooks: opts, Attachments: cfg,
	})
	fm := &fakeMail{}
	eng := &engine.Engine{Store: st, HTTP: safehttp.NewClient(opts), HTTPOpts: opts, Mail: fm, Logger: logger,
		Version: "test", Blobs: blobs}
	return &attachEnv{p1env: &p1env{env: &env{t: t, h: h, pool: pool}, st: st, eng: eng, mail: fm}, dir: dir}
}

type fileUpload struct {
	Filename    string
	ContentType string // what the client claims
	Data        []byte
	CommentID   string
}

// upload posts a multipart form like a browser would.
func (e *attachEnv) upload(token, path string, u fileUpload) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if u.CommentID != "" {
		_ = mw.WriteField("comment_id", u.CommentID)
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+u.Filename+`"`)
	hdr.Set("Content-Type", u.ContentType)
	pw, _ := mw.CreatePart(hdr)
	_, _ = pw.Write(u.Data)
	_ = mw.Close()
	req := httptest.NewRequestWithContext(e.t.Context(), "POST", path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	r := resp{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.Body)
	return r
}

func (e *attachEnv) download(token, path string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequestWithContext(e.t.Context(), "GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// blobCount counts committed blobs and staging files under the store directory.
func (e *attachEnv) blobCount() (blobs, staging int) {
	e.t.Helper()
	_ = filepath.WalkDir(e.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Base(filepath.Dir(p)) == ".staging" {
			staging++
		} else {
			blobs++
		}
		return nil
	})
	return blobs, staging
}

func (e *attachEnv) purge() {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE attachment_blob_purges SET purge_after = now()`); err != nil {
		e.t.Fatal(err)
	}
	e.eng.Tick(context.Background())
}

const acmeItem = items + "/1/attachments"

func TestAttachments(t *testing.T) {
	e := newAttachEnv(t, Attachments{MaxFileBytes: 1 << 20, MaxPerItem: 50})
	e.setup() // alice owner, bob guest, carol member, dave outsider
	e.must(201, "alice", "POST", items, map[string]any{"title": "Faktura"})

	// The client lies about the type; the server sniffs the bytes. The name is
	// reduced to its base, without bidi overrides or control characters.
	png := e.upload("carol", acmeItem, fileUpload{Filename: "../../etc/cat\u202egnp.exe", ContentType: "text/plain", Data: pngBytes})
	if png.Status != 201 {
		t.Fatalf("upload: %d %s", png.Status, png.Raw)
	}
	if png.Body["content_type"] != "image/png" || png.Body["filename"] != "catgnp.exe" || png.Body["size"] != float64(len(pngBytes)) {
		t.Fatalf("stored: %v", png.Body)
	}
	if _, ok := png.Body["storage_key"]; ok {
		t.Fatal("storage key leaked")
	}
	sum := sha256.Sum256(pngBytes)
	if png.Body["sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256: %v", png.Body["sha256"])
	}
	pngID := png.Body["id"].(string)

	// An HTML page claiming to be a PNG stays an opaque download.
	html := e.upload("carol", acmeItem, fileUpload{Filename: "photo.png", ContentType: "image/png",
		Data: []byte("<!DOCTYPE html><html><script>alert(document.domain)</script></html>")})
	if html.Status != 201 || !strings.HasPrefix(html.Body["content_type"].(string), "text/html") {
		t.Fatalf("html upload: %d %v", html.Status, html.Body)
	}
	htmlID := html.Body["id"].(string)
	for _, q := range []string{"", "?inline=1"} {
		rec := e.download("bob", acmeItem+"/"+htmlID+"/content"+q) // guests may read
		h := rec.Header()
		if rec.Code != 200 || h.Get("Content-Type") != "application/octet-stream" ||
			!strings.HasPrefix(h.Get("Content-Disposition"), "attachment") ||
			h.Get("X-Content-Type-Options") != "nosniff" ||
			!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
			t.Fatalf("html download%s: %d %v", q, rec.Code, h)
		}
	}

	// Safe raster images may be shown inline, on request only.
	rec := e.download("bob", acmeItem+"/"+pngID+"/content?inline=1")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline") || !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("inline png: %d %v", rec.Code, rec.Header())
	}
	if cd := e.download("bob", acmeItem+"/"+pngID+"/content").Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Fatalf("default disposition: %s", cd)
	}
	// Non-ASCII names are RFC 2231 encoded, never raw in the header.
	az := e.upload("carol", acmeItem, fileUpload{Filename: "Müqavilə son.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7\n%âãÏÓ\n")})
	if az.Status != 201 || az.Body["content_type"] != "application/pdf" {
		t.Fatalf("pdf: %d %v", az.Status, az.Body)
	}
	cd := e.download("carol", acmeItem+"/"+az.Body["id"].(string)+"/content").Header().Get("Content-Disposition")
	if !strings.Contains(cd, "filename*=utf-8''M%C3%BCqavil%C9%99") {
		t.Fatalf("encoded disposition: %s", cd)
	}

	list := e.must(200, "bob", "GET", acmeItem, nil).Body
	if n := len(list["attachments"].([]any)); n != 3 {
		t.Fatalf("listed %d", n)
	}
	if list["limits"].(map[string]any)["max_file_bytes"] != float64(1<<20) {
		t.Fatalf("limits: %v", list["limits"])
	}

	// Roles: guests cannot upload or delete; outsiders see nothing.
	if r := e.upload("bob", acmeItem, fileUpload{Filename: "x.txt", ContentType: "text/plain", Data: []byte("x")}); r.Status != 403 {
		t.Fatalf("guest upload: %d", r.Status)
	}
	if r := e.call("bob", "DELETE", acmeItem+"/"+pngID, nil); r.Status != 403 {
		t.Fatalf("guest delete: %d", r.Status)
	}
	for _, p := range []string{acmeItem, acmeItem + "/" + pngID + "/content"} {
		if r := e.call("dave", "GET", p, nil); r.Status != 404 {
			t.Fatalf("outsider GET %s: %d", p, r.Status)
		}
	}

	// Limits and malformed requests store nothing.
	before, _ := e.blobCount()
	big := bytes.Repeat([]byte("a"), 1<<20+1)
	if r := e.upload("carol", acmeItem, fileUpload{Filename: "big.bin", ContentType: "application/octet-stream", Data: big}); r.Status != 413 || errCode(r) != "file_too_large" {
		t.Fatalf("oversize: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("carol", acmeItem, fileUpload{Filename: "empty.txt", ContentType: "text/plain"}); r.Status != 422 {
		t.Fatalf("empty: %d", r.Status)
	}
	if r := e.call("carol", "POST", acmeItem, map[string]any{"file": "x"}); r.Status != 415 {
		t.Fatalf("json upload: %d", r.Status)
	}
	if r := e.upload("carol", items+"/99/attachments", fileUpload{Filename: "x.txt", ContentType: "text/plain", Data: []byte("x")}); r.Status != 404 {
		t.Fatalf("missing item: %d", r.Status)
	}
	after, staging := e.blobCount()
	if after != before || staging != 0 {
		t.Fatalf("rejected uploads left files: blobs %d->%d, staging %d", before, after, staging)
	}
	var reserved int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM attachment_blob_purges`).Scan(&reserved)
	if reserved != 0 {
		t.Fatalf("rejected uploads left %d key reservations", reserved)
	}

	// Delete: uploader or admin. The blob goes once the worker purges.
	if r := e.must(201, "alice", "POST", acmeItem[:len(acmeItem)-len("/1/attachments")], map[string]any{"title": "x"}); r.Status != 201 {
		t.Fatal(r.Raw)
	}
	mine := e.upload("alice", acmeItem, fileUpload{Filename: "a.txt", ContentType: "text/plain", Data: []byte("alice")})
	if r := e.call("carol", "DELETE", acmeItem+"/"+mine.Body["id"].(string), nil); r.Status != 403 {
		t.Fatalf("member deletes another's file: %d", r.Status)
	}
	e.must(204, "carol", "DELETE", acmeItem+"/"+htmlID, nil)                                      // own file
	e.must(204, "alice", "DELETE", acmeItem+"/"+pngID, nil)                                       // admin, someone else's file
	e.must(404, "alice", "DELETE", acmeItem+"/"+pngID, nil)                                       // gone
	e.must(404, "carol", "GET", acmeItem+"/"+pngID+"/content", nil)                               // gone
	e.must(404, "carol", "GET", items+"/2/attachments/"+mine.Body["id"].(string)+"/content", nil) // wrong item
	blobs, _ := e.blobCount()
	e.purge()
	if left, _ := e.blobCount(); left != blobs-2 {
		t.Fatalf("purge: %d blobs before, %d after", blobs, left)
	}

	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?action=attachment.", nil).Raw
	if !strings.Contains(audit, "attachment.added") || !strings.Contains(audit, "attachment.removed") {
		t.Fatalf("audit: %s", audit)
	}

	// A deleted item's files are gone from the API.
	e.must(204, "alice", "DELETE", items+"/1", nil)
	e.must(404, "carol", "GET", acmeItem, nil)
	e.must(404, "carol", "GET", acmeItem+"/"+az.Body["id"].(string)+"/content", nil)
}

func TestAttachmentsFollowProjectPrivacy(t *testing.T) {
	e := newAttachEnv(t, Attachments{})
	e.setup()
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Gizli", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Müqavilə"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq"})
	secret := e.upload("alice", secItems+"/1/attachments", fileUpload{Filename: "s.txt", ContentType: "text/plain", Data: []byte("secret")})
	if secret.Status != 201 {
		t.Fatal(secret.Raw)
	}
	id := secret.Body["id"].(string)

	// carol is a workspace member but not in the private project: 404 everywhere.
	for _, p := range []string{secItems + "/1/attachments", secItems + "/1/attachments/" + id + "/content"} {
		if r := e.call("carol", "GET", p, nil); r.Status != 404 {
			t.Errorf("GET %s: %d", p, r.Status)
		}
	}
	if r := e.call("carol", "DELETE", secItems+"/1/attachments/"+id, nil); r.Status != 404 {
		t.Errorf("delete: %d", r.Status)
	}
	if r := e.upload("carol", secItems+"/1/attachments", fileUpload{Filename: "x", ContentType: "text/plain", Data: []byte("x")}); r.Status != 404 {
		t.Errorf("upload: %d", r.Status)
	}
	// The id does not work through an item she can see either.
	if r := e.call("carol", "GET", acmeItem+"/"+id+"/content", nil); r.Status != 404 {
		t.Errorf("cross-project download: %d", r.Status)
	}
	if r := e.call("alice", "GET", acmeItem+"/"+id+"/content", nil); r.Status != 404 {
		t.Errorf("cross-project download even for the owner: %d", r.Status)
	}
	// Given access, she can read it.
	e.must(204, "alice", "PUT", secPath+"/members/"+e.userID("carol"), nil)
	if rec := e.download("carol", secItems+"/1/attachments/"+id+"/content"); rec.Code != 200 || rec.Body.String() != "secret" {
		t.Fatalf("after access: %d", rec.Code)
	}
}

func TestAttachmentsOnComments(t *testing.T) {
	e := newAttachEnv(t, Attachments{})
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "x"})
	c := e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "Ekran görüntüsü"}).Body["id"].(string)

	if r := e.upload("alice", acmeItem, fileUpload{Filename: "a.png", ContentType: "image/png", Data: pngBytes, CommentID: c}); r.Status != 403 {
		t.Fatalf("file on someone else's comment: %d", r.Status)
	}
	if r := e.upload("carol", acmeItem, fileUpload{Filename: "a.png", ContentType: "image/png", Data: pngBytes, CommentID: "not-a-uuid"}); r.Status != 422 {
		t.Fatalf("bad comment id: %d", r.Status)
	}
	if r := e.upload("carol", acmeItem, fileUpload{Filename: "a.png", ContentType: "image/png", Data: pngBytes, CommentID: "00000000-0000-7000-8000-000000000000"}); r.Status != 422 {
		t.Fatalf("unknown comment: %d", r.Status)
	}
	r := e.upload("carol", acmeItem, fileUpload{Filename: "a.png", ContentType: "image/png", Data: pngBytes, CommentID: c})
	if r.Status != 201 || r.Body["comment_id"] != c {
		t.Fatalf("comment attachment: %d %v", r.Status, r.Body)
	}
	e.must(204, "carol", "DELETE", items+"/1/comments/"+c, nil)
	if n := len(e.must(200, "carol", "GET", acmeItem, nil).Body["attachments"].([]any)); n != 0 {
		t.Fatalf("a deleted comment's file is still listed (%d)", n)
	}
	e.purge()
	if blobs, _ := e.blobCount(); blobs != 0 {
		t.Fatalf("blob of a deleted comment kept: %d", blobs)
	}
}

func TestAttachmentLimits(t *testing.T) {
	e := newAttachEnv(t, Attachments{MaxFileBytes: 1 << 20, WorkspaceQuotaBytes: 2 << 20, MaxPerItem: 3})
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "x"})
	chunk := bytes.Repeat([]byte("z"), 900<<10)
	e.must(201, "alice", "POST", items, map[string]any{"title": "y"})
	for i, want := range []int{201, 201, 413} {
		r := e.upload("alice", acmeItem, fileUpload{Filename: "f.bin", ContentType: "application/octet-stream", Data: chunk})
		if r.Status != want {
			t.Fatalf("upload %d: %d %s", i, r.Status, r.Raw)
		}
		if want == 413 && errCode(r) != "quota_exceeded" {
			t.Fatalf("quota error: %s", r.Raw)
		}
	}
	// Per-item count limit (small files fit the quota).
	for i, want := range []int{201, 413} {
		r := e.upload("alice", acmeItem, fileUpload{Filename: "s.txt", ContentType: "text/plain", Data: []byte("s")})
		if r.Status != want {
			t.Fatalf("per-item %d: %d %s", i, r.Status, r.Raw)
		}
	}
	if _, staging := e.blobCount(); staging != 0 {
		t.Fatalf("staging files left: %d", staging)
	}
	e.purge() // the over-quota upload's blob was removed or is purged now
	var rows int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM attachments`).Scan(&rows)
	if blobs, _ := e.blobCount(); blobs != rows {
		t.Fatalf("%d blobs for %d rows", blobs, rows)
	}
}

func TestAttachmentUploadRateLimit(t *testing.T) {
	e := newAttachEnv(t, Attachments{UploadsPerMinute: 2})
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "x"})
	for i, want := range []int{201, 201, 429} {
		if r := e.upload("alice", acmeItem, fileUpload{Filename: "f.txt", ContentType: "text/plain", Data: []byte("f")}); r.Status != want {
			t.Fatalf("upload %d: %d", i, r.Status)
		}
	}
	// Other users have their own budget; reads are not limited.
	e.must(201, "carol", "POST", acmeItem[:len(acmeItem)-len("/1/attachments")], map[string]any{"title": "y"})
	if r := e.upload("carol", acmeItem, fileUpload{Filename: "f.txt", ContentType: "text/plain", Data: []byte("f")}); r.Status != 201 {
		t.Fatalf("other user: %d", r.Status)
	}
	e.must(200, "alice", "GET", acmeItem, nil)
}

// A blob whose upload crashed before its row was written is removed once its
// reservation expires; a blob with a row is never touched.
func TestOrphanedBlobsArePurged(t *testing.T) {
	e := newAttachEnv(t, Attachments{})
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "x"})
	kept := e.upload("alice", acmeItem, fileUpload{Filename: "k.txt", ContentType: "text/plain", Data: []byte("keep")})
	if kept.Status != 201 {
		t.Fatal(kept.Raw)
	}
	ctx := context.Background()
	orphan := blobstore.NewKey()
	if err := e.st.ReserveBlobKey(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	f, err := e.eng.Blobs.CreateTemp()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("orphan")
	if err := e.eng.Blobs.Commit(ctx, orphan, f, 6, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	e.eng.Tick(ctx) // still within the grace period: kept
	if blobs, _ := e.blobCount(); blobs != 2 {
		t.Fatalf("blobs during grace: %d", blobs)
	}
	// A reservation pointing at a live row must never delete it.
	var keptKey string
	_ = e.pool.QueryRow(ctx, `SELECT storage_key FROM attachments`).Scan(&keptKey)
	if _, err := e.pool.Exec(ctx, `INSERT INTO attachment_blob_purges (storage_key) VALUES ($1)`, keptKey); err != nil {
		t.Fatal(err)
	}
	e.purge()
	if blobs, _ := e.blobCount(); blobs != 1 {
		t.Fatalf("blobs after purge: %d", blobs)
	}
	if rec := e.download("alice", acmeItem+"/"+kept.Body["id"].(string)+"/content"); rec.Code != 200 {
		t.Fatalf("kept file: %d", rec.Code)
	}

	// Deleted files outlive their row by the purge delay (point-in-time restores).
	e.eng.BlobPurgeDelay = time.Hour
	e.must(204, "alice", "DELETE", acmeItem+"/"+kept.Body["id"].(string), nil)
	e.eng.Tick(ctx)
	if blobs, _ := e.blobCount(); blobs != 1 {
		t.Fatalf("deleted blob purged before the delay: %d", blobs)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE attachment_blob_purges SET purge_after = now() - interval '2 hours'`); err != nil {
		t.Fatal(err)
	}
	e.eng.Tick(ctx)
	if blobs, _ := e.blobCount(); blobs != 0 {
		t.Fatalf("deleted blob kept after the delay: %d", blobs)
	}
	if _, err := os.Stat(e.dir); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentsDisabled(t *testing.T) {
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(), DB: pool,
		Store: store.New(pool), Verifier: testUsers, RequestTimeout: 10 * time.Second,
	})
	e := &env{t: t, h: h, pool: pool}
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "x"})
	if l := e.must(200, "alice", "GET", acmeItem, nil).Body; l["enabled"] != false {
		t.Fatalf("list: %v", l)
	}
	ae := &attachEnv{p1env: &p1env{env: e}}
	if r := ae.upload("alice", acmeItem, fileUpload{Filename: "f", ContentType: "text/plain", Data: []byte("f")}); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("upload: %d", r.Status)
	}
}

func TestIsTransfer(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/api/v1/workspaces/a/projects/B/items/1/attachments", true},
		{"GET", "/api/v1/workspaces/a/projects/B/items/1/attachments/x/content", true},
		{"GET", "/api/v1/workspaces/a/projects/B/items/1/attachments", false},
		{"DELETE", "/api/v1/workspaces/a/projects/B/items/1/attachments/x", false},
		{"POST", "/api/v1/workspaces/a/projects/B/items", false},
		{"GET", "/api/v1/content", false},
	} {
		r := httptest.NewRequestWithContext(t.Context(), c.method, c.path, nil)
		if got := isTransfer(r); got != c.want {
			t.Errorf("%s %s: %v", c.method, c.path, got)
		}
	}
}

func TestAttachmentFileName(t *testing.T) {
	for in, want := range map[string]string{
		"report.pdf":            "report.pdf",
		`C:\Users\x\secret.doc`: "secret.doc",
		"../../../etc/passwd":   "passwd",
		"a\u202etxt.exe":        "atxt.exe",
		"\u200b":                "upload",
		"":                      "upload",
		"..":                    "upload",
		"line\nbreak.txt":       "linebreak.txt",
	} {
		if got := attachmentFileName(in); got != want {
			t.Errorf("attachmentFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
