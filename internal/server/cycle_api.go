package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// cycleRoutes: the monthly period close of a project (ADR-0051). Members
// can read the configuration and the closed months; changing it, the
// preview and closing a month by hand are admin+, like recurrences.
func (a *api) cycleRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	admin := r.With(requireRole(store.RoleAdmin))
	member.Get("/cycle", a.getCycle)
	member.Get("/cycle/closes", a.listCloses)
	member.Get("/cycle/closes/{period}", a.getClose)
	admin.Put("/cycle", a.saveCycle)
	admin.Post("/cycle/preview", a.previewCycle)
	admin.Post("/cycle/close", a.closeCycle)
}

func (a *api) getCycle(w http.ResponseWriter, r *http.Request) {
	c, err := a.store.GetCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *api) saveCycle(w http.ResponseWriter, r *http.Request) {
	var body store.CycleInput
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.SaveCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type periodBody struct {
	Period string `json:"period"` // YYYY-MM; empty = last month
}

func (a *api) previewCycle(w http.ResponseWriter, r *http.Request) {
	var body periodBody
	if !a.decode(w, r, &body) {
		return
	}
	p, err := a.store.PreviewCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, body.Period)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) closeCycle(w http.ResponseWriter, r *http.Request) {
	var body periodBody
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.CloseCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body.Period)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (a *api) listCloses(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListCloses(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closes": list})
}

func (a *api) getClose(w http.ResponseWriter, r *http.Request) {
	c, items, err := a.store.GetCloseSnapshot(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, chi.URLParam(r, "period"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"close": c, "items": items})
}
