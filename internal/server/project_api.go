package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// projectSettingsRoutes: visibility and members of a project (ADR-0013).
// Mounted inside /projects/{project}, so loadProject has already checked
// that the caller can see the project.
func (a *api) projectSettingsRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Patch("/", a.updateProject)
	r.Get("/members", a.listProjectMembers)
	admin.Put("/members/{userID}", a.addProjectMember)
	admin.Delete("/members/{userID}", a.removeProjectMember)
}

func (a *api) updateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        opt.Field[string] `json:"name"`
		Description opt.Field[string] `json:"description"`
		Visibility  opt.Field[string] `json:"visibility"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	p := projectFrom(r)
	if err := a.store.UpdateProject(r.Context(), workspaceFrom(r).ID, p.ID, principal(r).UserID, store.UpdateProjectParams{
		Name: body.Name, Description: body.Description, Visibility: body.Visibility,
	}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.store.GetProject(r.Context(), workspaceFrom(r).ID, p.Key, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *api) listProjectMembers(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListProjectMembers(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": nonNil(list)})
}

func (a *api) addProjectMember(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "userID")
	if !ok {
		return
	}
	if err := a.store.AddProjectMember(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) removeProjectMember(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "userID")
	if !ok {
		return
	}
	if err := a.store.RemoveProjectMember(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
