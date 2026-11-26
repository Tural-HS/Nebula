package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// reminderRoutes: the project's yearly date reminder (ADR-0054). Members can
// read it; choosing or clearing the field is admin+.
func (a *api) reminderRoutes(r chi.Router) {
	r.With(requireRole(store.RoleMember)).Get("/date-reminder", a.getDateReminder)
	r.With(requireRole(store.RoleAdmin)).Put("/date-reminder", a.saveDateReminder)
}

func (a *api) getDateReminder(w http.ResponseWriter, r *http.Request) {
	d, err := a.store.GetDateReminder(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) saveDateReminder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Field opt.Field[string] `json:"field"` // null = off
	}
	if !a.decode(w, r, &body) {
		return
	}
	if !body.Field.Set {
		a.fail(w, r, &store.ValidationError{Field: "field", Message: "is required (a date field key or null)"})
		return
	}
	d, err := a.store.SaveDateReminder(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body.Field.Ptr())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
