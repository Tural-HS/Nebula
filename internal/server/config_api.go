package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// configRoutes manages workflows, statuses and item types. Reads are open to
// every member (clients need them to render items); writes require admin.
func (a *api) configRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))

	r.Get("/workflows", a.listWorkflows)
	admin.Post("/workflows", a.createWorkflow)
	admin.Patch("/workflows/{workflowID}", a.updateWorkflow)
	admin.Post("/workflows/{workflowID}/transitions", a.addTransition)
	admin.Delete("/workflows/{workflowID}/transitions/{transitionID}", a.deleteTransition)
	admin.Post("/workflows/{workflowID}/statuses", a.addStatus)
	admin.Patch("/workflows/{workflowID}/statuses/{statusID}", a.updateStatus)
	admin.Delete("/workflows/{workflowID}/statuses/{statusID}", a.deleteStatus)
	a.approvalPolicyRoutes(admin)

	admin.Post("/item-types", a.createItemType)
	r.Route("/item-types/{type}", func(r chi.Router) {
		admin := r.With(requireRole(store.RoleAdmin))
		admin.Patch("/", a.updateItemType)
		admin.Delete("/", a.archiveItemType)
		a.itemTypeFieldRoutes(r) // PUT/DELETE /fields/{field}: field bindings
	})
}

// uuidParam parses a path UUID; malformed IDs are reported as not found.
func (a *api) uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		a.fail(w, r, store.ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (a *api) listWorkflows(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListWorkflows(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": nonNil(list)})
}

func (a *api) createWorkflow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string              `json:"name"`
		Statuses []store.StatusInput `json:"statuses"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.CreateWorkflow(r.Context(), workspaceFrom(r).ID, principal(r).UserID, body.Name, body.Statuses)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (a *api) updateWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	var body struct {
		Name               opt.Field[string] `json:"name"`
		EnforceTransitions opt.Field[bool]   `json:"enforce_transitions"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.UpdateWorkflow(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id, body.Name, body.EnforceTransitions)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) addTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	var body struct {
		Name string     `json:"name"`
		From *uuid.UUID `json:"from_status_id"`
		To   uuid.UUID  `json:"to_status_id"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.AddTransition(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id, body.Name, body.From, body.To)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (a *api) deleteTransition(w http.ResponseWriter, r *http.Request) {
	wfID, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	trID, ok := a.uuidParam(w, r, "transitionID")
	if !ok {
		return
	}
	wf, err := a.store.DeleteTransition(r.Context(), workspaceFrom(r).ID, principal(r).UserID, wfID, trID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) allowedStatuses(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	sts, err := a.store.AllowedStatuses(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"statuses": sts})
}

func (a *api) addStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	var body store.StatusInput
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.AddStatus(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (a *api) updateStatus(w http.ResponseWriter, r *http.Request) {
	wfID, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	stID, ok := a.uuidParam(w, r, "statusID")
	if !ok {
		return
	}
	var body struct {
		Name      opt.Field[string] `json:"name"`
		Category  opt.Field[string] `json:"category"`
		Position  opt.Field[int]    `json:"position"`
		IsInitial opt.Field[bool]   `json:"is_initial"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.UpdateStatus(r.Context(), store.UpdateStatusParams{
		WorkspaceID: workspaceFrom(r).ID, ActorID: principal(r).UserID, WorkflowID: wfID, StatusID: stID,
		Name: body.Name, Category: body.Category, Position: body.Position, IsInitial: body.IsInitial,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) deleteStatus(w http.ResponseWriter, r *http.Request) {
	wfID, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	stID, ok := a.uuidParam(w, r, "statusID")
	if !ok {
		return
	}
	wf, err := a.store.DeleteStatus(r.Context(), workspaceFrom(r).ID, principal(r).UserID, wfID, stID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) createItemType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key        string    `json:"key"`
		Name       string    `json:"name"`
		Icon       *string   `json:"icon"`
		WorkflowID uuid.UUID `json:"workflow_id"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	it, err := a.store.CreateItemType(r.Context(), workspaceFrom(r).ID, principal(r).UserID, body.Key, body.Name, body.Icon, body.WorkflowID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (a *api) updateItemType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name opt.Field[string] `json:"name"`
		Icon opt.Field[string] `json:"icon"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	it, err := a.store.UpdateItemType(r.Context(), workspaceFrom(r).ID, principal(r).UserID, chi.URLParam(r, "type"), body.Name, body.Icon)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (a *api) archiveItemType(w http.ResponseWriter, r *http.Request) {
	if err := a.store.ArchiveItemType(r.Context(), workspaceFrom(r).ID, principal(r).UserID, chi.URLParam(r, "type")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
