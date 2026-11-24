package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// homeRoutes: the home page "Mənim işlərim" (ADR-0037). Like the other /me
// endpoints it is behind the per-user rate limit of the API router.
func (a *api) homeRoutes(r chi.Router) {
	r.Get("/me/home", a.home)
}

const defaultHomeLimit = 15

// home returns the counters, one list and the workspace cards in one
// response. ?show= picks the list (attention by default), ?limit= its size.
func (a *api) home(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	limit := defaultHomeLimit
	if q.Has("limit") {
		if limit, ok = pageLimit(w, q, store.MaxHomeList); !ok {
			return
		}
	}
	show := q.Get("show")
	if show == "" {
		show = store.HomeAttention
	}
	if !store.IsHomeShow(show) {
		writeError(w, http.StatusBadRequest, "bad_request", "show must be attention, overdue, approval, week or assigned")
		return
	}
	home, err := a.store.HomeFor(r.Context(), principal(r).UserID, show, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, home)
}
