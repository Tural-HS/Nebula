package server

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Archive, restore and permanent deletion of projects and workspaces
// (ADR-0039). Archiving is reversible and hides the object; deleting is only
// possible once archived, needs the typed key or slug, and runs as a purge
// in batches: the request does what fits in inlinePurgeBudget, the worker's
// tick finishes the rest.

const inlinePurgeBudget = 3 * time.Second

// allowedWhileArchived lists the non-GET routes an archived workspace still
// serves: restoring and deleting it, and the evidence exports (POST, but
// read-only) its owners should download before deleting it. Paths are
// relative to /workspaces/{workspace}.
var allowedWhileArchived = []struct {
	method string
	path   *regexp.Regexp
}{
	{http.MethodPost, regexp.MustCompile(`^/restore$`)},
	{http.MethodDelete, regexp.MustCompile(`^/?$`)},
	{http.MethodPost, regexp.MustCompile(`^/evidence/exports$`)},
	{http.MethodPost, regexp.MustCompile(`^/projects/[^/]+/items/[0-9]+/evidence$`)},
}

// writeAllowedWhileArchived reports whether a request may run in an archived
// workspace: reads, and the routes above.
func writeAllowedWhileArchived(method, routePath string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	for _, a := range allowedWhileArchived {
		if a.method == method && a.path.MatchString(routePath) {
			return true
		}
	}
	return false
}

// remainingPath is the request path below the current sub-router mount.
func remainingPath(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePath != "" {
		return rc.RoutePath
	}
	return r.URL.Path
}

func (a *api) workspaceLifecycleRoutes(r chi.Router) {
	r.With(requireRole(store.RoleOwner)).Post("/archive", a.archiveWorkspace)
	r.With(requireRole(store.RoleAdmin)).Post("/restore", a.restoreWorkspace)
	r.With(requireRole(store.RoleOwner)).Delete("/", a.deleteWorkspace)
	r.With(requireRole(store.RoleAdmin)).Get("/archived-projects", a.listArchivedProjects)
}

func (a *api) archiveWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := workspaceFrom(r)
	if err := a.store.ArchiveWorkspace(r.Context(), ws.ID, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.logger.InfoContext(r.Context(), "workspace archived", "workspace_id", ws.ID, "slug", ws.Slug, "actor_id", principal(r).UserID)
	a.writeWorkspace(w, r, ws.Slug)
}

func (a *api) restoreWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := workspaceFrom(r)
	if err := a.store.RestoreWorkspace(r.Context(), ws.ID, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.logger.InfoContext(r.Context(), "workspace restored", "workspace_id", ws.ID, "slug", ws.Slug, "actor_id", principal(r).UserID)
	a.writeWorkspace(w, r, ws.Slug)
}

func (a *api) writeWorkspace(w http.ResponseWriter, r *http.Request, slug string) {
	updated, err := a.store.WorkspaceForMember(r.Context(), slug, principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *api) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm              string `json:"confirm"`
		EvidenceAcknowledged bool   `json:"evidence_acknowledged"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	actor := principal(r).UserID
	sum, err := a.store.RequestWorkspaceDeletion(r.Context(), ws.ID, actor, body.Confirm, body.EvidenceAcknowledged)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// The workspace's own audit log is deleted with it: the server log and
	// workspace_deletions keep the record (ADR-0039).
	a.logger.InfoContext(r.Context(), "workspace deletion requested", "workspace_id", ws.ID, "slug", ws.Slug,
		"actor_id", actor, "projects", sum.Projects, "items", sum.Items, "attachments", sum.Attachments,
		"members", sum.Members, "events", sum.Events)
	done, err := a.store.PurgeWorkspace(r.Context(), ws.ID, time.Now().Add(inlinePurgeBudget))
	a.purgeResponse(w, r, done, err, "workspace", "workspace_id", ws.ID.String(), "slug", ws.Slug)
}

// purgeResponse answers a deletion request: 200 "deleted" when the inline
// purge finished, else 202 "deleting" (the worker continues). A purge error
// is logged, not returned: the deletion is recorded and will be retried.
func (a *api) purgeResponse(w http.ResponseWriter, r *http.Request, done bool, err error, kind string, attrs ...any) {
	switch {
	case err != nil && !isCanceled(r.Context()):
		a.logger.ErrorContext(r.Context(), kind+" purge failed; the worker will retry", append(attrs, "error", err)...)
	case done:
		a.logger.InfoContext(r.Context(), "deletion completed", append([]any{"kind", kind}, attrs...)...)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "deleting"})
}

func isCanceled(ctx context.Context) bool { return ctx.Err() != nil }

func (a *api) listArchivedProjects(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListArchivedProjects(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": nonNil(list)})
}

// projectArchiveRoute is served with the normal (active-only) project loader.
func (a *api) projectArchiveRoute(r chi.Router) {
	r.With(requireRole(store.RoleAdmin)).Post("/archive", a.archiveProject)
}

// projectLifecycleRoutes are served for archived projects too.
func (a *api) projectLifecycleRoutes(r chi.Router) {
	r.With(requireRole(store.RoleAdmin)).Post("/restore", a.restoreProject)
	r.With(requireRole(store.RoleOwner)).Delete("/", a.deleteProject)
}

func (a *api) archiveProject(w http.ResponseWriter, r *http.Request) {
	ws, p := workspaceFrom(r), projectFrom(r)
	if err := a.store.ArchiveProject(r.Context(), ws.ID, p.ID, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeProject(w, r, p.Key)
}

func (a *api) restoreProject(w http.ResponseWriter, r *http.Request) {
	ws, p := workspaceFrom(r), projectFrom(r)
	if err := a.store.RestoreProject(r.Context(), ws.ID, p.ID, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeProject(w, r, p.Key)
}

func (a *api) writeProject(w http.ResponseWriter, r *http.Request, key string) {
	p, err := a.store.GetProjectAnyState(r.Context(), workspaceFrom(r).ID, key, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) deleteProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws, p := workspaceFrom(r), projectFrom(r)
	actor := principal(r).UserID
	sum, err := a.store.RequestProjectDeletion(r.Context(), ws.ID, p.ID, actor, body.Confirm)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.logger.InfoContext(r.Context(), "project deletion requested", "workspace_id", ws.ID, "project_id", p.ID,
		"key", p.Key, "actor_id", actor, "items", sum.Items, "attachments", sum.Attachments)
	done, err := a.store.PurgeProject(r.Context(), ws.ID, p.ID, time.Now().Add(inlinePurgeBudget))
	a.purgeResponse(w, r, done, err, "project", "workspace_id", ws.ID.String(), "project_id", p.ID.String(), "key", p.Key)
}
