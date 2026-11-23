package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// recurrenceRoutes: recurring items of a project (ADR-0012). Members can see
// the rules and preview schedules; changing them is admin+, like automations.
func (a *api) recurrenceRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	admin := r.With(requireRole(store.RoleAdmin))
	member.Get("/recurrences", a.listRecurrences)
	member.Post("/recurrences/preview", a.previewRecurrence)
	member.Get("/recurrences/{recurrenceID}", a.getRecurrence)
	admin.Post("/recurrences", a.createRecurrence)
	admin.Put("/recurrences/{recurrenceID}", a.replaceRecurrence)
	admin.Delete("/recurrences/{recurrenceID}", a.deleteRecurrence)
}

func (a *api) listRecurrences(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListRecurrences(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recurrences": list})
}

func (a *api) previewRecurrence(w http.ResponseWriter, r *http.Request) {
	var body store.ScheduleInput
	if !a.decode(w, r, &body) {
		return
	}
	dates, err := a.store.PreviewSchedule(r.Context(), workspaceFrom(r).ID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upcoming": dates})
}

func (a *api) getRecurrence(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "recurrenceID")
	if !ok {
		return
	}
	rec, err := a.store.GetRecurrence(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (a *api) createRecurrence(w http.ResponseWriter, r *http.Request) {
	a.saveRecurrence(w, r, uuid.Nil, http.StatusCreated)
}

func (a *api) replaceRecurrence(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "recurrenceID")
	if !ok {
		return
	}
	a.saveRecurrence(w, r, id, http.StatusOK)
}

func (a *api) saveRecurrence(w http.ResponseWriter, r *http.Request, id uuid.UUID, status int) {
	var body store.RecurrenceInput
	if !a.decode(w, r, &body) {
		return
	}
	rec, err := a.store.SaveRecurrence(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, id, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, status, rec)
}

func (a *api) deleteRecurrence(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "recurrenceID")
	if !ok {
		return
	}
	if err := a.store.DeleteRecurrence(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
