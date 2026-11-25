package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/mailroute"
	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/store"
)

const maxBodyBytes = 1 << 20

type api struct {
	store       *store.Store
	logger      *slog.Logger
	webhookOpts safehttp.Options
	publicURL   string
	// attach and uploadLimiter: file attachments (attachment_api.go).
	attach        Attachments
	uploadLimiter *ratelimit.Limiter
	// evidence and version: audit evidence export (evidence_api.go).
	evidence *evidenceExports
	version  string
	// mail and emailTestLimiter: workspace SMTP settings (email_settings_api.go).
	mail             mailroute.Router
	emailTestLimiter *ratelimit.Limiter
	// exports: item table export (export_api.go).
	exports *itemExports
	// reportsLimiter: workspace reports (reports_api.go).
	reportsLimiter *ratelimit.Limiter
	// absenceLimiter: absence changes (absences_api.go).
	absenceLimiter *ratelimit.Limiter
	// tokenDenials audits refused service token requests (servicetokens_api.go).
	tokenDenials *tokenDenials
}

type ctxKey int

const (
	workspaceKey ctxKey = iota
	projectKey
)

func (a *api) routes(r chi.Router) {
	r.Get("/me", a.me)
	a.invitationPublicRoutes(r)
	a.notificationRoutes(r)
	a.homeRoutes(r)
	a.formPublicRoutes(r)
	r.Get("/workspaces", a.listWorkspaces)
	r.Post("/workspaces", a.createWorkspace)

	r.Route("/workspaces/{workspace}", func(r chi.Router) {
		r.Use(a.loadWorkspace)
		a.invitationAdminRoutes(r)
		r.Get("/", a.getWorkspace)
		r.With(requireRole(store.RoleAdmin)).Patch("/", a.updateWorkspace)
		a.workspaceLifecycleRoutes(r)
		r.Get("/item-types", a.listItemTypes)
		a.configRoutes(r)
		a.fieldRoutes(r)
		a.automationRoutes(r)
		a.emailSettingsRoutes(r)
		a.workspaceCollabRoutes(r)
		a.searchRoutes(r)
		a.calendarRoutes(r)
		a.evidenceRoutes(r)
		a.approvalInboxRoutes(r)
		a.absenceRoutes(r)
		a.reportsRoutes(r)
		a.serviceTokenRoutes(r)
		r.Get("/members", a.listMembers)
		r.With(requireRole(store.RoleAdmin)).Post("/members", a.addMember)
		r.With(requireRole(store.RoleAdmin)).Patch("/members/{userID}", a.updateMemberRole)
		r.Delete("/members/{userID}", a.removeMember) // admins remove others; anyone may leave
		r.Get("/projects", a.listProjects)
		r.With(requireRole(store.RoleAdmin)).Post("/projects", a.createProject)

		r.Route("/projects/{project}", func(r chi.Router) {
			// Restore and delete work on archived projects (ADR-0039);
			// everything else needs an active one. Two groups, because a
			// mux's Use would wrap every route of it.
			r.Group(func(r chi.Router) {
				r.Use(a.loadProjectAnyState)
				a.projectLifecycleRoutes(r)
			})
			r.Group(func(r chi.Router) { a.activeProjectRoutes(r) })
		})
	})
}

// activeProjectRoutes are the routes of /projects/{project} that need an
// active (not archived) project.
func (a *api) activeProjectRoutes(r chi.Router) {
	r.Use(a.loadProject)
	a.projectArchiveRoute(r)
	r.Get("/", a.getProject)
	r.Get("/items", a.listItems)
	r.Get("/items/counts", a.countItems)
	r.Get("/items/queue-counts", a.queueCounts)
	r.Get("/items/export", a.exportItems)
	r.With(requireRole(store.RoleMember)).Post("/items", a.createItem)
	r.Get("/items/{number}", a.getItem)
	r.Get("/items/{number}/transitions", a.allowedStatuses)
	r.With(requireRole(store.RoleMember)).Patch("/items/{number}", a.updateItem)
	r.With(requireRole(store.RoleMember)).Delete("/items/{number}", a.deleteItem)
	a.itemCollabRoutes(r)
	a.linkRoutes(r)
	a.attachmentRoutes(r)
	a.importRoutes(r)
	a.recurrenceRoutes(r)
	a.projectSettingsRoutes(r)
	a.viewRoutes(r)
	a.bulkRoutes(r)
	a.approvalRoutes(r)
	a.itemEvidenceRoutes(r)
	a.formProjectRoutes(r)
}

// ---------------------------------------------------------------------------
// Context loaders and authorization
// ---------------------------------------------------------------------------

func principal(r *http.Request) auth.Principal {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		panic("principal missing: route registered outside the auth middleware")
	}
	return p
}

func workspaceFrom(r *http.Request) store.Workspace {
	return r.Context().Value(workspaceKey).(store.Workspace)
}

func projectFrom(r *http.Request) store.Project {
	return r.Context().Value(projectKey).(store.Project)
}

// loadWorkspace resolves the workspace for a member. An archived workspace
// is read-only (ADR-0039): writes other than restore, delete and evidence
// exports get 409 workspace_archived; a workspace being deleted is gone (410).
func (a *api) loadWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ws store.Workspace
		var err error
		if t, ok := serviceToken(r); ok {
			ws, err = a.store.WorkspaceForServiceToken(r.Context(), chi.URLParam(r, "workspace"), t)
		} else {
			ws, err = a.store.WorkspaceForMember(r.Context(), chi.URLParam(r, "workspace"), principal(r).UserID)
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if ws.Deleting {
			writeError(w, http.StatusGone, "workspace_deleting", "this workspace is being deleted")
			return
		}
		if ws.ArchivedAt != nil && !writeAllowedWhileArchived(r.Method, remainingPath(r)) {
			writeError(w, http.StatusConflict, "workspace_archived", "this workspace is archived and read-only; an admin can restore it")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), workspaceKey, ws)))
	})
}

// loadProject resolves an active project. An archived one the viewer can see
// is 410 project_archived (project_deleting while it is purged), so a deep
// link can say so and offer a restore; others stay 404 (ADR-0013).
func (a *api) loadProject(next http.Handler) http.Handler {
	return a.projectLoader(false, next)
}

// loadProjectAnyState resolves archived projects too (restore, delete).
func (a *api) loadProjectAnyState(next http.Handler) http.Handler {
	return a.projectLoader(true, next)
}

func (a *api) projectLoader(anyState bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := viewer(r)
		t, isToken := serviceToken(r)
		if isToken && t.ProjectIDs != nil {
			// An allow-listed project is granted by an admin, who sees every
			// project: the token reaches it even when it is private (ADR-0043).
			v.Role = store.RoleAdmin
		}
		p, err := a.store.GetProjectAnyState(r.Context(), workspaceFrom(r).ID, chi.URLParam(r, "project"), v)
		if err == nil && isToken && !t.AllowsProject(p.ID) {
			a.tokenDenials.record(r.Context(), t, "project_not_allowed", r)
			err = store.ErrNotFound
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if !anyState && p.Deleting {
			writeError(w, http.StatusGone, "project_deleting", "this project is being deleted")
			return
		}
		if !anyState && p.ArchivedAt != nil {
			writeError(w, http.StatusGone, "project_archived", "this project is archived; an admin can restore it")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), projectKey, p)))
	})
}

// viewer is the signed-in user with their workspace role, for access checks.
func viewer(r *http.Request) store.Viewer {
	return store.Viewer{UserID: principal(r).UserID, Role: workspaceFrom(r).Role}
}

func requireRole(minRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !store.RoleAtLeast(workspaceFrom(r).Role, minRole) {
				writeError(w, http.StatusForbidden, "forbidden", "requires the "+minRole+" role or higher")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, principal(r))
}

// listWorkspaces: the active workspaces, or with ?state=archived the
// archived ones the caller administers (ADR-0039).
func (a *api) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	var list []store.Workspace
	var err error
	switch r.URL.Query().Get("state") {
	case "", "active":
		list, err = a.store.ListWorkspaces(r.Context(), principal(r).UserID)
	case "archived":
		list, err = a.store.ListArchivedWorkspaces(r.Context(), principal(r).UserID)
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "state must be active or archived")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": nonNil(list)})
}

func (a *api) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug     string `json:"slug"`
		Name     string `json:"name"`
		Template string `json:"template"` // "en" (default), "az", "tr" or "ru"; ADR-0020, ADR-0025
		Kind     string `json:"kind"`     // "general" (default), "finance", "it" or "hr"; ADR-0033
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws, err := a.store.CreateWorkspaceOfKind(r.Context(), principal(r).UserID, body.Slug, body.Name, body.Template, body.Kind)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/workspaces/"+ws.Slug)
	writeJSON(w, http.StatusCreated, ws)
}

func (a *api) getWorkspace(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, workspaceFrom(r))
}

// updateWorkspace changes workspace settings: name, time zone, digest hour, accent colour.
func (a *api) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name              opt.Field[string] `json:"name"`
		Timezone          opt.Field[string] `json:"timezone"`
		OverdueDigestHour opt.Field[int]    `json:"overdue_digest_hour"`
		Accent            opt.Field[string] `json:"accent"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.UpdateWorkspace(r.Context(), ws.ID, principal(r).UserID, store.UpdateWorkspaceParams{
		Name: body.Name, Timezone: body.Timezone, OverdueDigestHour: body.OverdueDigestHour, Accent: body.Accent,
	}); err != nil {
		a.fail(w, r, err)
		return
	}
	updated, err := a.store.WorkspaceForMember(r.Context(), ws.Slug, principal(r).UserID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *api) listItemTypes(w http.ResponseWriter, r *http.Request) {
	types, err := a.store.ListItemTypes(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item_types": nonNil(types)})
}

func (a *api) listMembers(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListMembers(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": nonNil(list)})
}

func (a *api) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "userID")
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	m, err := a.store.UpdateMemberRole(r.Context(), ws.ID, principal(r).UserID, ws.Role, id, body.Role)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *api) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "userID")
	if !ok {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.RemoveMember(r.Context(), ws.ID, principal(r).UserID, ws.Role, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) addMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	m, err := a.store.AddMember(r.Context(), ws.ID, principal(r).UserID, ws.Role, body.Email, body.Role)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (a *api) listProjects(w http.ResponseWriter, r *http.Request) {
	v := viewer(r)
	t, isToken := serviceToken(r)
	if isToken && t.ProjectIDs != nil {
		v.Role = store.RoleAdmin // filtered to the allow-list below
	}
	list, err := a.store.ListProjects(r.Context(), workspaceFrom(r).ID, v)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if isToken {
		list = slices.DeleteFunc(list, func(p store.Project) bool { return !t.AllowsProject(p.ID) })
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": nonNil(list)})
}

func (a *api) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key         string  `json:"key"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Visibility  string  `json:"visibility"`
		// Optional template (ADR-0020): copy setup from an existing project.
		Template *struct {
			From        string `json:"from"`
			Members     bool   `json:"members"`
			Recurrences bool   `json:"recurrences"`
			Views       bool   `json:"views"`
		} `json:"template"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	var tpl store.ProjectTemplate
	if body.Template != nil {
		if strings.TrimSpace(body.Template.From) == "" {
			a.fail(w, r, &store.ValidationError{Field: "template.from", Message: "is required"})
			return
		}
		tpl = store.ProjectTemplate{From: body.Template.From, Members: body.Template.Members,
			Recurrences: body.Template.Recurrences, Views: body.Template.Views}
	}
	ws := workspaceFrom(r)
	p, err := a.store.CreateProject(r.Context(), ws.ID, principal(r).UserID, body.Key, body.Name, body.Description, body.Visibility, tpl)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/projects/%s", ws.Slug, p.Key))
	writeJSON(w, http.StatusCreated, p)
}

func (a *api) getProject(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, projectFrom(r))
}

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

func (a *api) listItems(w http.ResponseWriter, r *http.Request) {
	p, ok := parseItemFilters(w, r)
	if !ok {
		return
	}
	items, next, err := a.store.ListItems(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"items": items, "next_cursor": nil}
	if next != nil {
		resp["next_cursor"] = encodeItemCursor(*next)
	}
	writeJSON(w, http.StatusOK, resp)
}

// countItems returns how many items match the list filters, per status, so a
// board can show true column sizes while loading each column page by page.
func (a *api) countItems(w http.ResponseWriter, r *http.Request) {
	p, ok := parseItemFilters(w, r)
	if !ok {
		return
	}
	counts, err := a.store.CountItemsByStatus(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	byStatus := make(map[string]int64, len(counts))
	var total int64
	for id, n := range counts {
		byStatus[id.String()] = n
		total += n
	}
	writeJSON(w, http.StatusOK, map[string]any{"by_status": byStatus, "total": total})
}

// queueCounts returns the quick-filter counts of the item list (ADR-0035) in
// one request: the list's other filters apply, the presets replace assignee,
// due, approval and blocked.
func (a *api) queueCounts(w http.ResponseWriter, r *http.Request) {
	p, ok := parseItemFilters(w, r)
	if !ok {
		return
	}
	counts, err := a.store.CountQueues(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, counts)
}

// parseItemFilters reads the item list query string, writing a 400 and
// returning false when it is invalid.
func parseItemFilters(w http.ResponseWriter, r *http.Request) (store.ListItemsParams, bool) {
	// url.Query() silently drops malformed pairs; reject them instead.
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "malformed query string")
		return store.ListItemsParams{}, false
	}
	p := store.ListItemsParams{
		WorkspaceID: workspaceFrom(r).ID,
		ProjectID:   projectFrom(r).ID,
		Query:       q.Get("q"),
		Limit:       defaultPageSize,
	}
	if len(p.Query) > 500 {
		writeError(w, http.StatusBadRequest, "bad_request", "q must be at most 500 characters")
		return store.ListItemsParams{}, false
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("limit must be between 1 and %d", maxPageSize))
			return store.ListItemsParams{}, false
		}
		p.Limit = n
	}
	p.Sort = q.Get("sort")
	if !store.ValidSort(p.Sort) {
		writeError(w, http.StatusBadRequest, "bad_request", "sort "+store.SortError)
		return store.ListItemsParams{}, false
	}
	if v := q.Get("cursor"); v != "" {
		c, err := decodeItemCursor(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid cursor")
			return store.ListItemsParams{}, false
		}
		if c.Sort != cmp.Or(p.Sort, store.SortNumber) {
			writeError(w, http.StatusBadRequest, "bad_request", "cursor belongs to another sort")
			return store.ListItemsParams{}, false
		}
		p.Cursor = &c
	}
	for _, raw := range q["status"] {
		for part := range strings.SplitSeq(raw, ",") {
			id, err := uuid.Parse(strings.TrimSpace(part))
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "status must be a list of status ids")
				return store.ListItemsParams{}, false
			}
			p.StatusIDs = append(p.StatusIDs, id)
		}
	}
	if len(p.StatusIDs) > 50 {
		writeError(w, http.StatusBadRequest, "bad_request", "at most 50 status filters")
		return store.ListItemsParams{}, false
	}
	for key, vals := range q {
		name, ok := strings.CutPrefix(key, "field.")
		if !ok {
			continue
		}
		if len(vals) != 1 {
			writeError(w, http.StatusBadRequest, "bad_request", key+" may be given only once")
			return store.ListItemsParams{}, false
		}
		if p.FieldFilters == nil {
			p.FieldFilters = map[string]string{}
		}
		p.FieldFilters[name] = vals[0]
	}
	p.Due = q.Get("due") // validated by the store
	for _, k := range []string{"due_from", "due_to"} {
		if len(q[k]) > 1 {
			writeError(w, http.StatusBadRequest, "bad_request", k+" may be given only once")
			return store.ListItemsParams{}, false
		}
	}
	if v := q.Get("due_from"); v != "" {
		p.DueFrom = &v
	}
	if v := q.Get("due_to"); v != "" {
		p.DueTo = &v
	}
	// Range rules (dates, order, at most store.MaxDueRangeDays) are the store's.
	if !parseParentFilters(w, q, projectFrom(r).Key, &p) {
		return store.ListItemsParams{}, false
	}
	v := viewer(r)
	p.Viewer = &v
	switch a := q.Get("approval"); a {
	case "", "pending", "mine":
		p.Approval = a
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "approval must be pending or mine")
		return store.ListItemsParams{}, false
	}
	switch q.Get("blocked") {
	case "", "false":
	case "true":
		p.Blocked = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "blocked must be true or false")
		return store.ListItemsParams{}, false
	}
	switch v := q.Get("assignee"); v {
	case "":
	case "me":
		id := principal(r).UserID
		p.AssigneeID = &id
	case "none":
		p.Unassigned = true
	default:
		id, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "assignee must be a user id, 'me' or 'none'")
			return store.ListItemsParams{}, false
		}
		p.AssigneeID = &id
	}

	return p, true
}

func (a *api) createItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string                     `json:"title"`
		Description *string                    `json:"description"`
		Type        string                     `json:"type"`
		StatusID    *uuid.UUID                 `json:"status_id"`
		AssigneeID  *uuid.UUID                 `json:"assignee_id"`
		DueDate     *string                    `json:"due_date"`
		Fields      map[string]json.RawMessage `json:"fields"`
		Parent      *itemRef                   `json:"parent"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws, proj := workspaceFrom(r), projectFrom(r)
	parent, err := body.Parent.number(proj.Key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	item, err := a.store.CreateItem(r.Context(), store.CreateItemParams{
		WorkspaceID: ws.ID, ProjectID: proj.ID, ActorID: principal(r).UserID,
		Title: body.Title, Description: body.Description, TypeKey: body.Type,
		StatusID: body.StatusID, AssigneeID: body.AssigneeID, DueDate: body.DueDate, Fields: body.Fields,
		ParentNumber: parent,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/items/%d", ws.Slug, proj.Key, item.Number))
	writeJSON(w, http.StatusCreated, item)
}

func itemNumber(r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(chi.URLParam(r, "number"), 10, 64)
	return n, err == nil && n > 0
}

func (a *api) getItem(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	item, err := a.store.GetItem(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *api) updateItem(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	var body struct {
		Version     *int                                  `json:"version"`
		Title       opt.Field[string]                     `json:"title"`
		Description opt.Field[string]                     `json:"description"`
		StatusID    opt.Field[uuid.UUID]                  `json:"status_id"`
		AssigneeID  opt.Field[uuid.UUID]                  `json:"assignee_id"`
		DueDate     opt.Field[string]                     `json:"due_date"`
		Fields      opt.Field[map[string]json.RawMessage] `json:"fields"`
		Parent      opt.Field[itemRef]                    `json:"parent"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	if body.Version == nil || *body.Version < 1 {
		a.fail(w, r, &store.ValidationError{Field: "version", Message: "is required (the version you last read)"})
		return
	}
	parent, err := parentField(body.Parent, projectFrom(r).Key)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	item, err := a.store.UpdateItem(r.Context(), store.UpdateItemParams{
		WorkspaceID: workspaceFrom(r).ID, ProjectID: projectFrom(r).ID, Number: n,
		ActorID: principal(r).UserID, Version: *body.Version,
		Title: body.Title, Description: body.Description, StatusID: body.StatusID, AssigneeID: body.AssigneeID, DueDate: body.DueDate,
		Fields: body.Fields, Parent: parent,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *api) deleteItem(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	if err := a.store.DeleteItem(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// decode parses a single JSON object, rejecting unknown fields and oversized bodies.
func (a *api) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("body must contain a single JSON object")
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds 1 MiB")
			return false
		}
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// fail maps store errors to HTTP; unexpected errors are logged, never echoed.
func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *store.ValidationError
	var ce *store.ConflictError
	var fe *store.ForbiddenError
	switch {
	case errors.As(err, &fe):
		writeError(w, http.StatusForbidden, fe.Code, fe.Message)
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]string{
			"code": "validation_failed", "message": ve.Message, "field": ve.Field}})
	case errors.As(err, &ce):
		writeError(w, http.StatusConflict, ce.Code, ce.Message)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, store.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, context.Canceled):
		// Client went away; nothing useful to send.
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, "timeout", "request timed out")
	default:
		a.logger.ErrorContext(r.Context(), "request failed", "error", err,
			"request_id", middleware.GetReqID(r.Context()))
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
