package server

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/blobstore"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Attachments configures file attachments (ADR-0022). A nil Blobs disables them.
type Attachments struct {
	Blobs               blobstore.Store
	MaxFileBytes        int64
	WorkspaceQuotaBytes int64 // 0: unlimited
	MaxPerItem          int   // 0: unlimited
	UploadsPerMinute    int   // per user; 0: unlimited
	TransferTimeout     time.Duration
}

const (
	defaultMaxAttachmentBytes = 20 << 20
	multipartOverhead         = 64 << 10 // boundaries, part headers, comment_id
	sniffLen                  = 512      // what http.DetectContentType looks at
)

// inlineImageTypes may be shown in the browser (as <img> from a blob: URL).
// Everything else, SVG included, is served as an opaque download.
var inlineImageTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

func (a *api) attachmentRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	r.Get("/items/{number}/attachments", a.listAttachments)
	member.Post("/items/{number}/attachments", a.uploadAttachment)
	r.Get("/items/{number}/attachments/{attachmentID}/content", a.downloadAttachment)
	member.Delete("/items/{number}/attachments/{attachmentID}", a.deleteAttachment)
}

// isTransfer marks requests that move file bytes: they get their own, longer
// deadline instead of the API-wide request timeout (see timeoutUnless).
func isTransfer(r *http.Request) bool {
	p := r.URL.Path
	switch r.Method {
	case http.MethodPost:
		return strings.HasSuffix(p, "/attachments") || isFormSubmission(r)
	case http.MethodGet, http.MethodHead:
		return strings.HasSuffix(p, "/content") && strings.Contains(p, "/attachments/")
	}
	return false
}

// timeoutUnless is middleware.Timeout for every request except those skip
// selects; those handlers bound their own work.
func timeoutUnless(d time.Duration, skip func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := middleware.Timeout(d)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

func (a *api) maxAttachmentBytes() int64 {
	if a.attach.MaxFileBytes > 0 {
		return a.attach.MaxFileBytes
	}
	return defaultMaxAttachmentBytes
}

func (a *api) transferTimeout() time.Duration {
	if a.attach.TransferTimeout > 0 {
		return a.attach.TransferTimeout
	}
	return 5 * time.Minute
}

// transferContext extends the connection deadlines (the server's read/write
// timeouts are sized for JSON calls) and bounds the transfer instead.
func (a *api) transferContext(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc) {
	d := a.transferTimeout()
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(d)) // unsupported in tests (recorder): fine
	_ = rc.SetWriteDeadline(time.Now().Add(d + 5*time.Second))
	return context.WithTimeout(r.Context(), d)
}

func (a *api) attachmentsEnabled(w http.ResponseWriter) bool {
	if a.attach.Blobs == nil {
		writeError(w, http.StatusServiceUnavailable, "attachments_disabled", "file attachments are not configured on this server")
		return false
	}
	return true
}

func (a *api) listAttachments(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	list, err := a.store.ListAttachments(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attachments": list,
		"enabled":     a.attach.Blobs != nil,
		"limits":      map[string]any{"max_file_bytes": a.maxAttachmentBytes(), "max_per_item": a.attach.MaxPerItem},
	})
}

// attachmentFileName is the display name: base name only, no control or
// invisible formatting characters (bidi overrides could disguise "exe.txt").
func attachmentFileName(raw string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, cleanFileName(raw))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "upload"
	}
	return name
}

// staged is an upload written to a staging file, with what we learned on the way.
type staged struct {
	file  *os.File
	name  string
	size  int64
	sum   []byte
	sniff []byte
}

// sniffWriter keeps the first sniffLen bytes written through it.
type sniffWriter struct{ buf []byte }

func (s *sniffWriter) Write(p []byte) (int, error) {
	if room := sniffLen - len(s.buf); room > 0 {
		s.buf = append(s.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (a *api) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	if !a.attachmentsEnabled(w) {
		return
	}
	if ok, wait := a.uploadLimiter.Allow(principal(r).UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads; try again shortly")
		return
	}
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the file as multipart/form-data")
		return
	}
	ctx, cancel := a.transferContext(w, r)
	defer cancel()
	ws, proj, actor := workspaceFrom(r), projectFrom(r), principal(r).UserID
	maxBytes := a.maxAttachmentBytes()

	// Fail fast, before reading the body: a gone item or a full workspace.
	if err := a.store.CheckAttachmentTarget(ctx, ws.ID, proj.ID, n, nil, actor); err != nil {
		a.fail(w, r, err)
		return
	}
	if q := a.attach.WorkspaceQuotaBytes; q > 0 {
		used, err := a.store.AttachmentUsage(ctx, ws.ID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if used >= q {
			writeError(w, http.StatusRequestEntityTooLarge, "quota_exceeded", fmt.Sprintf("the workspace storage quota (%d MiB) is full", q>>20))
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body")
		return
	}
	var up *staged
	key := ""
	var commentID *uuid.UUID
	committed := false
	defer func() {
		if committed {
			return
		}
		if up != nil {
			blobstore.Discard(up.file)
		}
		if key != "" { // reserved, but no blob was ever stored under it
			_ = a.store.CancelBlobKey(context.WithoutCancel(ctx), key)
		}
	}()
	for parts := 0; ; parts++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || parts >= 3 {
			a.uploadError(w, err, maxBytes)
			return
		}
		switch part.FormName() {
		case "comment_id":
			b, err := io.ReadAll(io.LimitReader(part, 64))
			id, perr := uuid.Parse(strings.TrimSpace(string(b)))
			if err != nil || perr != nil || commentID != nil {
				a.fail(w, r, &store.ValidationError{Field: "comment_id", Message: "must be a comment id"})
				return
			}
			commentID = &id
		case "file":
			if up != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "send exactly one file")
				return
			}
			key = blobstore.NewKey()
			// Registered before the blob exists: a crash after writing it still gets it cleaned up.
			if err := a.store.ReserveBlobKey(ctx, key); err != nil {
				a.fail(w, r, err)
				return
			}
			up, err = a.stageUpload(part, maxBytes)
			if err != nil {
				if errors.Is(err, errTooLarge) {
					writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("the file exceeds %d MiB", maxBytes>>20))
				} else {
					a.uploadError(w, err, maxBytes)
				}
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("unexpected form field %q", part.FormName()))
			return
		}
	}
	if up == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "the file field is required")
		return
	}
	if up.size == 0 {
		a.fail(w, r, &store.ValidationError{Field: "file", Message: "is empty"})
		return
	}
	// The client's Content-Type is ignored: the type comes from the bytes.
	contentType := http.DetectContentType(up.sniff)

	committed = true // Commit consumes the staging file, success or not
	if err := a.attach.Blobs.Commit(ctx, key, up.file, up.size, up.sum); err != nil {
		_ = a.store.ReleaseBlobKey(context.WithoutCancel(ctx), key)
		a.fail(w, r, fmt.Errorf("store blob (%s): %w", a.attach.Blobs.Name(), err))
		return
	}
	att, err := a.store.CreateAttachment(ctx, store.CreateAttachmentParams{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Number: n, CommentID: commentID, ActorID: actor,
		Filename: up.name, ContentType: contentType, Size: up.size, SHA256: up.sum, StorageKey: key,
		Limits: store.AttachmentLimits{WorkspaceQuotaBytes: a.attach.WorkspaceQuotaBytes, MaxPerItem: a.attach.MaxPerItem},
	})
	if err != nil {
		// No row: remove the blob now; if that fails the worker purges it.
		cleanup := context.WithoutCancel(ctx)
		_ = a.attach.Blobs.Delete(cleanup, key)
		_ = a.store.ReleaseBlobKey(cleanup, key)
		var qe *store.QuotaError
		if errors.As(err, &qe) {
			writeError(w, http.StatusRequestEntityTooLarge, "quota_exceeded", qe.Message)
			return
		}
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, att)
}

var errTooLarge = errors.New("file too large")

// stageUpload streams one file part into a staging file, hashing and sizing
// it on the way and stopping one byte past the limit.
func (a *api) stageUpload(part io.Reader, maxBytes int64) (*staged, error) {
	f, err := a.attach.Blobs.CreateTemp()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	sn := &sniffWriter{}
	written, err := io.Copy(io.MultiWriter(f, h, sn), io.LimitReader(part, maxBytes+1))
	if err == nil && written > maxBytes {
		err = errTooLarge
	}
	if err != nil {
		blobstore.Discard(f)
		return nil, err
	}
	name := "upload"
	if p, ok := part.(interface{ FileName() string }); ok {
		name = attachmentFileName(p.FileName())
	}
	return &staged{file: f, name: name, size: written, sum: h.Sum(nil), sniff: sn.buf}, nil
}

func (a *api) uploadError(w http.ResponseWriter, err error, maxBytes int64) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("the file exceeds %d MiB", maxBytes>>20))
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body")
}

func (a *api) attachmentParams(w http.ResponseWriter, r *http.Request) (int64, uuid.UUID, bool) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return 0, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "attachmentID"))
	if err != nil {
		a.fail(w, r, store.ErrNotFound)
		return 0, uuid.Nil, false
	}
	return n, id, true
}

func (a *api) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	if !a.attachmentsEnabled(w) {
		return
	}
	n, id, ok := a.attachmentParams(w, r)
	if !ok {
		return
	}
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	ctx, cancel := a.transferContext(w, r)
	defer cancel()
	att, err := a.store.GetAttachment(ctx, workspaceFrom(r).ID, projectFrom(r).ID, n, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	body, err := a.attach.Blobs.Open(ctx, att.StorageKey)
	if errors.Is(err, blobstore.ErrNotFound) {
		a.logger.ErrorContext(ctx, "attachment blob missing", "attachment_id", att.ID,
			"request_id", middleware.GetReqID(ctx))
		a.fail(w, r, store.ErrNotFound)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	disposition, ctype := "attachment", "application/octet-stream"
	if q.Get("inline") == "1" && inlineImageTypes[att.ContentType] {
		disposition, ctype = "inline", att.ContentType
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Disposition", contentDisposition(disposition, att.Filename))
	h.Set("Content-Length", strconv.FormatInt(att.Size, 10))
	// Even if a browser rendered the response as a document, it could do nothing.
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Cache-Control", "private, no-store")
	h.Set("ETag", `"`+att.SHA256+`"`)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, body); err != nil && ctx.Err() == nil {
		a.logger.WarnContext(ctx, "attachment download interrupted", "attachment_id", att.ID, "error", err,
			"request_id", middleware.GetReqID(ctx))
	}
}

// contentDisposition encodes the file name safely (RFC 6266 / RFC 2231 for
// non-ASCII); a name that cannot be encoded is left out.
func contentDisposition(kind, name string) string {
	if v := mime.FormatMediaType(kind, map[string]string{"filename": name}); v != "" {
		return v
	}
	return kind
}

func (a *api) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	n, id, ok := a.attachmentParams(w, r)
	if !ok {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.DeleteAttachment(r.Context(), ws.ID, projectFrom(r).ID, n, id, principal(r).UserID, ws.Role); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newUploadLimiter allows perMinute uploads per user (burst up to 10).
func newUploadLimiter(perMinute int) *ratelimit.Limiter {
	if perMinute <= 0 {
		return nil
	}
	return ratelimit.New(float64(perMinute)/60, min(perMinute, 10))
}
