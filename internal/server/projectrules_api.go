package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// projectRulesRoutes: the project's item rules (ADR-0061). Members read them
// (the table colours late cells, a refused move is explained); admin+ change.
func (a *api) projectRulesRoutes(r chi.Router) {
	r.With(requireRole(store.RoleMember)).Get("/rules", a.getProjectRules)
	r.With(requireRole(store.RoleAdmin)).Put("/rules", a.saveProjectRules)
}

func (a *api) getProjectRules(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.GetProjectRules(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) saveProjectRules(w http.ResponseWriter, r *http.Request) {
	var body store.ProjectRules
	if !a.decode(w, r, &body) {
		return
	}
	out, err := a.store.SaveProjectRules(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
