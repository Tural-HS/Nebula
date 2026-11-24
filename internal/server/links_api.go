package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Link and watcher writes fan out to two items' histories, webhooks and up
// to MaxWatchersPerItem notifications each, so they get their own per-user
// budget on top of the global one (ADR-0028): a steady 2/s, bursts of 20.
const (
	linkWriteRPS   = 2
	linkWriteBurst = 20
)

// linkRoutes registers item links and watchers (ADR-0028) under
// /projects/{project}. Reads need the project (loadProject); link writes need
// the member role; watching yourself is open to every role (it changes
// nothing about the item), changing someone else's watching needs admin.
func (a *api) linkRoutes(r chi.Router) {
	lim := ratelimit.New(linkWriteRPS, linkWriteBurst)
	limited := r.With(ratelimit.Middleware(lim, func(r *http.Request) string {
		return principal(r).UserID.String()
	}, writeError))
	r.Get("/items/{number}/links", a.listLinks)
	limited.With(requireRole(store.RoleMember)).Post("/items/{number}/links", a.addLink)
	limited.With(requireRole(store.RoleMember)).Delete("/items/{number}/links/{linkID}", a.removeLink)
	r.Get("/items/{number}/watchers", a.listWatchers)
	limited.Put("/items/{number}/watchers/me", a.watchSelf(true))
	limited.Delete("/items/{number}/watchers/me", a.watchSelf(false))
	limited.With(requireRole(store.RoleAdmin)).Put("/items/{number}/watchers/{userID}", a.watchOther(true))
	limited.With(requireRole(store.RoleAdmin)).Delete("/items/{number}/watchers/{userID}", a.watchOther(false))
}

func (a *api) listLinks(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	list, err := a.store.ListItemLinks(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": list})
}

func (a *api) addLink(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	var body struct {
		Type   string `json:"type"`
		Target string `json:"target"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	if len(body.Target) > 40 {
		a.fail(w, r, &store.ValidationError{Field: "target", Message: "must be an item key, e.g. OPS-12"})
		return
	}
	link, err := a.store.AddItemLink(r.Context(), store.AddLinkParams{
		WorkspaceID: workspaceFrom(r).ID, ProjectID: projectFrom(r).ID, Number: n,
		Viewer: viewer(r), Type: body.Type, Target: body.Target,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (a *api) removeLink(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "linkID"))
	if err != nil {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	if err := a.store.RemoveItemLink(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, id, viewer(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listWatchers(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	list, err := a.store.ListWatchers(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) setWatching(w http.ResponseWriter, r *http.Request, userID uuid.UUID, on bool) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	list, err := a.store.SetWatching(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, principal(r).UserID, userID, on)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *api) watchSelf(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.setWatching(w, r, principal(r).UserID, on)
	}
}

func (a *api) watchOther(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.uuidParam(w, r, "userID")
		if !ok {
			return
		}
		a.setWatching(w, r, id, on)
	}
}
