package server

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// itemCollabRoutes: comments and activity, mounted under /projects/{project}.
func (a *api) itemCollabRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	r.Get("/items/{number}/comments", a.listComments)
	member.Post("/items/{number}/comments", a.createComment)
	member.Patch("/items/{number}/comments/{commentID}", a.updateComment)
	member.Delete("/items/{number}/comments/{commentID}", a.deleteComment)
	r.Get("/items/{number}/activity", a.itemActivity)
}

// workspaceCollabRoutes: audit log and webhook test pings (admin only).
func (a *api) workspaceCollabRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Get("/audit-log", a.auditLog)
	admin.Post("/webhooks/{webhookID}/ping", a.pingWebhook)
}

// strictQuery parses the query string, rejecting malformed input instead of
// silently dropping it (url.Query() ignores bad pairs).
func strictQuery(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "malformed query string")
		return nil, false
	}
	return q, true
}

const defaultPageLimit = 50

// pageLimit parses ?limit within [1, maxLimit] (default 50).
func pageLimit(w http.ResponseWriter, q url.Values, maxLimit int) (int, bool) {
	v := q.Get("limit")
	if v == "" {
		return defaultPageLimit, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxLimit {
		writeError(w, http.StatusBadRequest, "bad_request", "limit must be between 1 and "+strconv.Itoa(maxLimit))
		return 0, false
	}
	return n, true
}

func (a *api) listComments(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 100)
	if !ok {
		return
	}
	var after *store.CommentCursor
	if c := q.Get("cursor"); c != "" {
		cur, err := store.DecodeCommentCursor(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		after = &cur
	}
	list, next, err := a.store.ListComments(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, after, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"comments": list, "next_cursor": nil}
	if next != "" {
		resp["next_cursor"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

type commentBody struct {
	Body string `json:"body"`
}

func (a *api) createComment(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	var body commentBody
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.CreateComment(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, principal(r).UserID, body.Body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (a *api) commentParams(w http.ResponseWriter, r *http.Request) (int64, uuid.UUID, bool) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return 0, uuid.Nil, false
	}
	id, ok := a.uuidParam(w, r, "commentID")
	return n, id, ok
}

func (a *api) updateComment(w http.ResponseWriter, r *http.Request) {
	n, id, ok := a.commentParams(w, r)
	if !ok {
		return
	}
	var body commentBody
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.UpdateComment(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, id, principal(r).UserID, body.Body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *api) deleteComment(w http.ResponseWriter, r *http.Request) {
	n, id, ok := a.commentParams(w, r)
	if !ok {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.DeleteComment(r.Context(), ws.ID, projectFrom(r).ID, n, id, principal(r).UserID, ws.Role); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseBefore(w http.ResponseWriter, q url.Values) (int64, bool) {
	v := q.Get("before")
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		writeError(w, http.StatusBadRequest, "bad_request", "before must be an event id")
		return 0, false
	}
	return n, true
}

func activityResponse(list []store.Activity, next int64) map[string]any {
	resp := map[string]any{"activity": list, "next_before": nil}
	if next > 0 {
		resp["next_before"] = next
	}
	return resp
}

func (a *api) itemActivity(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	before, ok := parseBefore(w, q)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 100)
	if !ok {
		return
	}
	list, next, err := a.store.ItemActivity(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, before, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, activityResponse(list, next))
}

func (a *api) auditLog(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	before, ok := parseBefore(w, q)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 100)
	if !ok {
		return
	}
	p := store.AuditLogParams{WorkspaceID: workspaceFrom(r).ID, Before: before, Limit: limit, ActionPrefix: q.Get("action")}
	if v := q.Get("actor"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "actor must be a user id")
			return
		}
		p.ActorID = &id
	}
	list, next, err := a.store.AuditLog(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, activityResponse(list, next))
}

func (a *api) pingWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	eventID, err := a.store.PingWebhook(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// Delivery is asynchronous; the result appears in /webhooks/{id}/deliveries.
	writeJSON(w, http.StatusAccepted, map[string]any{"event_id": eventID})
}
