package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Search is costlier than a normal read (GIN scans, ranking, snippets) and
// is typed as you go, so it has its own per-user budget on top of the global
// one: a steady 3/s (the UI debounces keystrokes) with bursts of 15.
const (
	searchRPS   = 3
	searchBurst = 15
)

// searchRoutes registers workspace-wide search (ADR-0024). Each call gets its
// own limiter, so every handler instance (and every test) starts fresh.
func (a *api) searchRoutes(r chi.Router) {
	lim := ratelimit.New(searchRPS, searchBurst)
	r.With(ratelimit.Middleware(lim, func(r *http.Request) string {
		return principal(r).UserID.String()
	}, writeError)).Get("/search", a.search)
}

func (a *api) search(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	if len(q["q"]) > 1 {
		writeError(w, http.StatusBadRequest, "bad_request", "q may be given only once")
		return
	}
	limit, ok := pageLimit(w, q, store.SearchMaxLimit)
	if !ok {
		return
	}
	if q.Get("limit") == "" {
		limit = 20
	}
	p := store.SearchParams{WorkspaceID: workspaceFrom(r).ID, Viewer: viewer(r), Query: q.Get("q"), Limit: limit}
	if c := q.Get("cursor"); c != "" {
		cur, err := store.DecodeSearchCursor(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return
		}
		p.Cursor = &cur
	}
	res, err := a.store.Search(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"hits": res.Hits, "next_cursor": nil, "counts": res.Counts}
	if res.Next != nil {
		resp["next_cursor"] = res.Next.Encode()
	}
	writeJSON(w, http.StatusOK, resp)
}
