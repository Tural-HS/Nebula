package server

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// approvalInboxRoutes: the workspace-wide approval inbox (ADR-0034). Any
// member can read it; it only ever lists requests of projects the viewer
// can see, and mine=true only those the viewer may decide on now.
func (a *api) approvalInboxRoutes(r chi.Router) {
	r.Get("/approvals", a.listApprovalInbox)
	r.Get("/approvals/count", a.countApprovalInbox)
}

// inboxMine parses ?mine (true|false, default false).
func inboxMine(w http.ResponseWriter, q url.Values) (bool, bool) {
	switch q.Get("mine") {
	case "", "false":
		return false, true
	case "true":
		return true, true
	}
	writeError(w, http.StatusBadRequest, "bad_request", "mine must be true or false")
	return false, false
}

func (a *api) listApprovalInbox(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Only pending requests have an inbox; closed ones are in each item's
	// history and in the evidence export (ADR-0029).
	if st := q.Get("state"); st != "" && st != store.ApprovalPending {
		writeError(w, http.StatusBadRequest, "bad_request", "state must be pending")
		return
	}
	mine, ok := inboxMine(w, q)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 100)
	if !ok {
		return
	}
	f := store.InboxFilter{Mine: mine, Limit: limit}
	if c := q.Get("cursor"); c != "" {
		cur, err := store.DecodeInboxCursor(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		f.After = &cur
	}
	list, next, err := a.store.ListApprovalInbox(r.Context(), workspaceFrom(r).ID, viewer(r), f)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"approvals": list, "next_cursor": nil}
	if next != "" {
		resp["next_cursor"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *api) countApprovalInbox(w http.ResponseWriter, r *http.Request) {
	mine, ok := inboxMine(w, r.URL.Query())
	if !ok {
		return
	}
	c, err := a.store.CountApprovalInbox(r.Context(), workspaceFrom(r).ID, viewer(r), mine)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": c.Count, "overdue": c.Overdue, "cap": store.InboxCountCap})
}
