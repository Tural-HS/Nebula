package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// slaRoutes: a project's SLA policy (ADR-0045). Everyone who sees the
// project may read it (the table shows SLA badges); admins change it.
func (a *api) slaRoutes(r chi.Router) {
	r.Get("/sla", a.getSLAPolicy)
	r.With(requireRole(store.RoleAdmin)).Put("/sla", a.putSLAPolicy)
}

func (a *api) getSLAPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := a.store.GetSLAPolicy(r.Context(), projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) putSLAPolicy(w http.ResponseWriter, r *http.Request) {
	var body store.SLAPolicyInput
	if !a.decode(w, r, &body) {
		return
	}
	p, err := a.store.SetSLAPolicy(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
