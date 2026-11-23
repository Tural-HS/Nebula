package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// bulkRoutes: change or delete many items of a project at once (ADR-0023).
// Members may bulk-edit; bulk delete is for admins, since one click can
// remove hundreds of items.
func (a *api) bulkRoutes(r chi.Router) {
	r.With(requireRole(store.RoleMember)).Post("/items/bulk", a.bulkItems)
}

func (a *api) bulkItems(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items      []store.BulkTarget                    `json:"items"`
		Delete     bool                                  `json:"delete"`
		StatusID   opt.Field[uuid.UUID]                  `json:"status_id"`
		AssigneeID opt.Field[uuid.UUID]                  `json:"assignee_id"`
		DueDate    opt.Field[string]                     `json:"due_date"`
		Fields     opt.Field[map[string]json.RawMessage] `json:"fields"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	if body.Delete && !store.RoleAtLeast(ws.Role, store.RoleAdmin) {
		writeError(w, http.StatusForbidden, "forbidden", "bulk delete requires the admin role or higher")
		return
	}
	actor := principal(r).UserID
	res, err := a.store.BulkItems(r.Context(), store.BulkItemsParams{
		WorkspaceID: ws.ID, ProjectID: projectFrom(r).ID, ActorID: actor, Items: body.Items, Delete: body.Delete,
		StatusID: body.StatusID, AssigneeID: body.AssigneeID, DueDate: body.DueDate, Fields: body.Fields,
	})
	var be *store.BulkError
	if errors.As(err, &be) {
		writeBulkError(w, be)
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// The events carry the bulk id too; this line ties them to the request.
	a.logger.InfoContext(r.Context(), "bulk item change",
		"bulk_id", res.BulkID, "workspace_id", ws.ID, "project_id", projectFrom(r).ID, "actor_id", actor,
		"delete", body.Delete, "items", len(body.Items), "changed", res.Changed,
		"request_id", middleware.GetReqID(r.Context()))
	writeJSON(w, http.StatusOK, res)
}

// writeBulkError reports a rolled-back bulk change with every failing item.
// 409 when some item changed meanwhile (reload and retry), 422 otherwise.
func writeBulkError(w http.ResponseWriter, be *store.BulkError) {
	status := http.StatusUnprocessableEntity
	for _, it := range be.Items {
		if it.Code == "version_conflict" {
			status = http.StatusConflict
			break
		}
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code":    "bulk_failed",
		"message": be.Error() + "; nothing was changed",
		"items":   be.Items,
	}})
}
