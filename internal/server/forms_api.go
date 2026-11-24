package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/blobstore"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Intake forms (ADR-0032). Each submission creates an item, may store files
// and notifies people, so submissions get their own per-user budget on top
// of the global limit: bursts of 10, then one every 5 seconds.
const (
	formSubmitRPS   = 0.2
	formSubmitBurst = 10
	// JSON submissions are small; they keep a fixed deadline even though the
	// route is exempt from the API-wide timeout (for multipart uploads).
	formJSONTimeout = 30 * time.Second
	myRequestsMax   = 100
)

// formPublicRoutes are open to every signed-in user: whether a form exists
// for them is decided by its audience (store.loadFormFor). "My requests"
// only ever reads the caller's own submissions.
func (a *api) formPublicRoutes(r chi.Router) {
	lim := ratelimit.New(formSubmitRPS, formSubmitBurst)
	r.Get("/forms/{form}", a.getPublicForm)
	r.With(ratelimit.Middleware(lim, func(r *http.Request) string {
		return principal(r).UserID.String()
	}, writeError)).Post("/forms/{form}/submissions", a.submitForm)
	r.Get("/me/requests", a.listMyRequests)
	r.Get("/me/requests/{requestID}", a.getMyRequest)
	r.Post("/me/requests/{requestID}/seen", a.markRequestSeen)
}

// formProjectRoutes are under /projects/{project}: the builder (admins) and
// the submission behind an item (members).
func (a *api) formProjectRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Get("/forms", a.listForms)
	admin.Post("/forms", a.createForm)
	admin.Get("/forms/{formID}", a.getForm)
	admin.Put("/forms/{formID}", a.updateForm)
	admin.Delete("/forms/{formID}", a.deleteForm)
	member := r.With(requireRole(store.RoleMember))
	member.Get("/items/{number}/request", a.getItemRequest)
	member.Put("/items/{number}/request/message", a.setRequestMessage)
}

// isFormSubmission marks POST /api/v1/forms/{form}/submissions (multipart
// uploads bound their own transfer time, see timeoutUnless).
func isFormSubmission(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/forms/") &&
		strings.HasSuffix(r.URL.Path, "/submissions")
}

// ---------------------------------------------------------------------------
// Builder
// ---------------------------------------------------------------------------

type formBody struct {
	Slug              string            `json:"slug"`
	Title             string            `json:"title"`
	Intro             *string           `json:"intro"`
	ItemType          string            `json:"item_type"`
	TitleTemplate     string            `json:"title_template"`
	Fields            []store.FormField `json:"fields"`
	DefaultAssigneeID *uuid.UUID        `json:"default_assignee_id"`
	InitialStatusID   *uuid.UUID        `json:"initial_status_id"`
	Audience          string            `json:"audience"`
	Enabled           *bool             `json:"enabled"` // default true
	MaxFiles          int               `json:"max_files"`
}

func (b formBody) input() store.FormInput {
	return store.FormInput{Slug: b.Slug, Title: b.Title, Intro: b.Intro, ItemType: b.ItemType, TitleTemplate: b.TitleTemplate,
		Fields: b.Fields, DefaultAssigneeID: b.DefaultAssigneeID, InitialStatusID: b.InitialStatusID, Audience: b.Audience,
		Enabled: b.Enabled == nil || *b.Enabled, MaxFiles: b.MaxFiles}
}

func (a *api) listForms(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListForms(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"forms": nonNil(list)})
}

func (a *api) getForm(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "formID")
	if !ok {
		return
	}
	f, err := a.store.GetForm(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *api) createForm(w http.ResponseWriter, r *http.Request) {
	var body formBody
	if !a.decode(w, r, &body) {
		return
	}
	f, err := a.store.CreateForm(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body.input())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ws, p := workspaceFrom(r), projectFrom(r)
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/forms/%s", ws.Slug, p.Key, f.ID))
	writeJSON(w, http.StatusCreated, f)
}

func (a *api) updateForm(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "formID")
	if !ok {
		return
	}
	var body formBody
	if !a.decode(w, r, &body) {
		return
	}
	f, err := a.store.UpdateForm(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, id, principal(r).UserID, body.input())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *api) deleteForm(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "formID")
	if !ok {
		return
	}
	if err := a.store.DeleteForm(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, id, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Filling in
// ---------------------------------------------------------------------------

func (a *api) getPublicForm(w http.ResponseWriter, r *http.Request) {
	f, err := a.store.FormForSubmitter(r.Context(), chi.URLParam(r, "form"), principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if f.MaxFiles > 0 && a.attach.Blobs == nil {
		f.MaxFiles = 0 // the server cannot take files
	}
	resp := map[string]any{"form": f}
	if f.MaxFiles > 0 {
		resp["limits"] = map[string]any{"max_file_bytes": a.maxAttachmentBytes()}
	}
	writeJSON(w, http.StatusOK, resp)
}

// submitForm takes {"values": {...}} as JSON, or as multipart/form-data a
// "values" part with the same JSON plus up to the form's max_files "file"
// parts. Files are stored first (outside the transaction, like single
// uploads); the item, its files and the submission then commit together, and
// on any failure the stored blobs are removed again.
func (a *api) submitForm(w http.ResponseWriter, r *http.Request) {
	ref, user := chi.URLParam(r, "form"), principal(r)
	params := store.SubmitFormParams{Ref: ref, SubmitterID: user.UserID, SubmitterName: user.DisplayName,
		Limits: store.AttachmentLimits{WorkspaceQuotaBytes: a.attach.WorkspaceQuotaBytes, MaxPerItem: a.attach.MaxPerItem}}

	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "multipart/form-data" {
		ctx, cancel := context.WithTimeout(r.Context(), formJSONTimeout)
		defer cancel()
		var body struct {
			Values map[string]json.RawMessage `json:"values"`
		}
		if !a.decode(w, r, &body) {
			return
		}
		params.Values = body.Values
		a.finishSubmission(w, r.WithContext(ctx), params)
		return
	}

	ctx, cancel := a.transferContext(w, r)
	defer cancel()
	r = r.WithContext(ctx)
	// Fail fast, before reading the body: no such form, closed, no files.
	wsID, maxFiles, err := a.store.FormAcceptsFiles(ctx, ref, user.UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if maxFiles > 0 && a.attach.Blobs == nil {
		maxFiles = 0
	}
	if q := a.attach.WorkspaceQuotaBytes; q > 0 && maxFiles > 0 {
		used, err := a.store.AttachmentUsage(ctx, wsID)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if used >= q {
			writeError(w, http.StatusRequestEntityTooLarge, "quota_exceeded", fmt.Sprintf("the workspace storage quota (%d MiB) is full", q>>20))
			return
		}
	}
	maxBytes := a.maxAttachmentBytes()
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes+int64(maxFiles)*(maxBytes+multipartOverhead)+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body")
		return
	}

	var files []*staged
	var keys []string // reserved storage keys, parallel to files once staged
	committed := 0    // files[:committed] were handed to the blob store
	stored := false   // the submission owns the blobs
	defer func() {
		if stored {
			return
		}
		cleanup := context.WithoutCancel(ctx)
		for i, key := range keys {
			switch {
			case i < committed:
				_ = a.attach.Blobs.Delete(cleanup, key)
				_ = a.store.ReleaseBlobKey(cleanup, key)
			default:
				if i < len(files) {
					blobstore.Discard(files[i].file)
				}
				_ = a.store.CancelBlobKey(cleanup, key)
			}
		}
	}()
	gotValues := false
	for parts := 0; ; parts++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || parts > maxFiles+1 {
			a.uploadError(w, err, maxBytes)
			return
		}
		switch part.FormName() {
		case "values":
			if gotValues {
				writeError(w, http.StatusBadRequest, "bad_request", "send the values part once")
				return
			}
			gotValues = true
			dec := json.NewDecoder(io.LimitReader(part, maxBodyBytes+1))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&params.Values); err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "values must be a JSON object of at most 1 MiB")
				return
			}
		case "file":
			if len(files) >= maxFiles {
				if maxFiles == 0 {
					a.fail(w, r, &store.ValidationError{Field: "files", Message: "this form does not take files"})
				} else {
					a.fail(w, r, &store.ValidationError{Field: "files", Message: fmt.Sprintf("at most %d files", maxFiles)})
				}
				return
			}
			key := blobstore.NewKey()
			if err := a.store.ReserveBlobKey(ctx, key); err != nil {
				a.fail(w, r, err)
				return
			}
			keys = append(keys, key)
			up, err := a.stageUpload(part, maxBytes)
			if err != nil {
				if errors.Is(err, errTooLarge) {
					writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("the file exceeds %d MiB", maxBytes>>20))
				} else {
					a.uploadError(w, err, maxBytes)
				}
				return
			}
			files = append(files, up)
			if up.size == 0 {
				a.fail(w, r, &store.ValidationError{Field: fmt.Sprintf("files[%d]", len(files)-1), Message: "is empty"})
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("unexpected form field %q", part.FormName()))
			return
		}
	}
	if !gotValues {
		writeError(w, http.StatusBadRequest, "bad_request", "the values part is required")
		return
	}
	for i, up := range files {
		committed = i + 1 // Commit consumes the staging file, success or not
		if err := a.attach.Blobs.Commit(ctx, keys[i], up.file, up.size, up.sum); err != nil {
			a.fail(w, r, fmt.Errorf("store blob (%s): %w", a.attach.Blobs.Name(), err))
			return
		}
		params.Files = append(params.Files, store.SubmittedFile{Filename: up.name, ContentType: http.DetectContentType(up.sniff),
			Size: up.size, SHA256: up.sum, StorageKey: keys[i]})
	}
	stored = a.finishSubmission(w, r, params)
}

// finishSubmission runs the transaction and writes the response; true when
// the submission committed (its files now belong to the item).
func (a *api) finishSubmission(w http.ResponseWriter, r *http.Request, p store.SubmitFormParams) bool {
	sub, err := a.store.SubmitForm(r.Context(), p)
	if err != nil {
		var qe *store.QuotaError
		if errors.As(err, &qe) {
			writeError(w, http.StatusRequestEntityTooLarge, "quota_exceeded", qe.Message)
			return false
		}
		a.fail(w, r, err)
		return false
	}
	w.Header().Set("Location", "/api/v1/me/requests/"+sub.ID.String())
	writeJSON(w, http.StatusCreated, sub)
	return true
}

// ---------------------------------------------------------------------------
// My requests
// ---------------------------------------------------------------------------

func encodeRequestCursor(c store.MyRequestCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte("r1:" + strconv.FormatInt(c.CreatedAt.UnixMicro(), 10) + ":" + c.ID.String()))
}

func decodeRequestCursor(s string) (store.MyRequestCursor, error) {
	if len(s) > 120 {
		return store.MyRequestCursor{}, errBadCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return store.MyRequestCursor{}, errBadCursor
	}
	parts := strings.Split(string(b), ":")
	if len(parts) != 3 || parts[0] != "r1" {
		return store.MyRequestCursor{}, errBadCursor
	}
	us, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return store.MyRequestCursor{}, errBadCursor
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		return store.MyRequestCursor{}, errBadCursor
	}
	return store.MyRequestCursor{CreatedAt: time.UnixMicro(us), ID: id}, nil
}

func (a *api) listMyRequests(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, myRequestsMax)
	if !ok {
		return
	}
	var after *store.MyRequestCursor
	if v := q.Get("cursor"); v != "" {
		c, err := decodeRequestCursor(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		after = &c
	}
	list, next, unseen, err := a.store.ListMyRequests(r.Context(), principal(r).UserID, after, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"requests": nonNil(list), "next_cursor": nil, "unseen": unseen}
	if next != nil {
		resp["next_cursor"] = encodeRequestCursor(*next)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) getMyRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "requestID")
	if !ok {
		return
	}
	d, err := a.store.GetMyRequest(r.Context(), principal(r).UserID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) markRequestSeen(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "requestID")
	if !ok {
		return
	}
	if err := a.store.MarkRequestSeen(r.Context(), principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// The team's side
// ---------------------------------------------------------------------------

func (a *api) getItemRequest(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	req, err := a.store.GetItemRequest(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

func (a *api) setRequestMessage(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	var body struct {
		Message *string `json:"message"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	req, err := a.store.SetRequestMessage(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, principal(r).UserID, body.Message)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}
