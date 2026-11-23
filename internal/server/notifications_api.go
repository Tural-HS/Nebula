package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// notificationRoutes are the signed-in user's in-app notifications across
// all workspaces (ADR-0017).
func (a *api) notificationRoutes(r chi.Router) {
	r.Get("/me/notifications", a.listNotifications)
	r.Get("/me/notifications/unread-count", a.unreadNotifications)
	r.Post("/me/notifications/read-all", a.markAllNotificationsRead)
	r.Post("/me/notifications/{id}/read", a.markNotificationRead)
}

func (a *api) listNotifications(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 100)
	if !ok {
		return
	}
	f := store.NotificationFilter{Limit: limit}
	switch q.Get("unread") {
	case "", "false":
	case "true":
		f.UnreadOnly = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "unread must be true or false")
		return
	}
	if c := q.Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		f.Before = n
	}
	list, next, err := a.store.ListNotifications(r.Context(), principal(r).UserID, f)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"notifications": list, "next_cursor": nil}
	if next != "" {
		resp["next_cursor"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

// unreadNotifications is polled by every open tab, so it is a single capped
// query; "capped" tells the UI the real number may be higher.
func (a *api) unreadNotifications(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.UnreadNotifications(r.Context(), principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"unread": n, "capped": n >= store.UnreadCountCap})
}

func (a *api) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	if err := a.store.MarkNotificationRead(r.Context(), principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markAllNotificationsRead marks everything read, or only one workspace's
// notifications with ?workspace=<slug>.
func (a *api) markAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	n, err := a.store.MarkAllNotificationsRead(r.Context(), principal(r).UserID, q.Get("workspace"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"marked": n})
}
