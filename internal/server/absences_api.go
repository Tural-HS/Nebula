package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Absences and approval delegation (ADR-0042). Every route needs the
// member role: guests never approve, so they have nothing to hand over.
// Members manage their own absences; admins may manage anyone's (the store
// decides). Writes are rate limited per user: each one is an audit event.

// newAbsenceLimiter allows a burst of 60 absence changes, then one every 10 s.
func newAbsenceLimiter() *ratelimit.Limiter { return ratelimit.New(0.1, 60) }

func (a *api) absenceRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	member.Get("/absences", a.listAbsences)
	member.Post("/absences", a.createAbsence)
	member.Patch("/absences/{absenceID}", a.updateAbsence)
	member.Post("/absences/{absenceID}/end", a.endAbsence)
}

func (a *api) absenceWriteAllowed(w http.ResponseWriter, r *http.Request) bool {
	if ok, wait := a.absenceLimiter.Allow(principal(r).UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many absence changes; try again in a few seconds")
		return false
	}
	return true
}

// listAbsences: current and upcoming absences of the workspace (?user_id=
// for one member). Notes are only shown to the member, their delegate and
// admins.
func (a *api) listAbsences(w http.ResponseWriter, r *http.Request) {
	var user *uuid.UUID
	if s := r.URL.Query().Get("user_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "user_id must be a UUID")
			return
		}
		user = &id
	}
	list, err := a.store.ListAbsences(r.Context(), workspaceFrom(r).ID, viewer(r), user)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absences": list})
}

func (a *api) createAbsence(w http.ResponseWriter, r *http.Request) {
	var body store.AbsenceInput
	if !a.decode(w, r, &body) {
		return
	}
	if !a.absenceWriteAllowed(w, r) {
		return
	}
	out, err := a.store.CreateAbsence(r.Context(), workspaceFrom(r).ID, viewer(r), body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (a *api) updateAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "absenceID")
	if !ok {
		return
	}
	var body store.AbsencePatch
	if !a.decode(w, r, &body) {
		return
	}
	if !a.absenceWriteAllowed(w, r) {
		return
	}
	out, err := a.store.UpdateAbsence(r.Context(), workspaceFrom(r).ID, id, viewer(r), body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) endAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "absenceID")
	if !ok {
		return
	}
	if !a.absenceWriteAllowed(w, r) {
		return
	}
	out, err := a.store.EndAbsence(r.Context(), workspaceFrom(r).ID, id, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
