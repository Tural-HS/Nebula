package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// noteRoutes: the signed-in user's sticky notes on the home page
// (ADR-0052). Behind the per-user rate limit like the other /me endpoints.
func (a *api) noteRoutes(r chi.Router) {
	r.Get("/me/notes", a.listNotes)
	r.Post("/me/notes", a.createNote)
	r.Patch("/me/notes/{noteID}", a.updateNote)
	r.Delete("/me/notes/{noteID}", a.deleteNote)
}

func (a *api) listNotes(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListNotes(r.Context(), principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": list})
}

func (a *api) createNote(w http.ResponseWriter, r *http.Request) {
	var body store.NoteInput
	if !a.decode(w, r, &body) {
		return
	}
	n, err := a.store.CreateNote(r.Context(), principal(r).UserID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (a *api) updateNote(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "noteID")
	if !ok {
		return
	}
	var body store.NoteInput
	if !a.decode(w, r, &body) {
		return
	}
	n, err := a.store.UpdateNote(r.Context(), principal(r).UserID, id, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (a *api) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "noteID")
	if !ok {
		return
	}
	if err := a.store.DeleteNote(r.Context(), principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
