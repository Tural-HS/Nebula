package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// approvalRoutes: four-eyes approvals on status changes (ADR-0026). Reading
// is open to everyone who can see the project; acting needs the member role.
// Who may approve, reject or cancel is decided per request by the store.
func (a *api) approvalRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	r.Get("/items/{number}/approvals", a.listApprovals)
	member.Post("/items/{number}/approvals", a.requestApproval)
	member.Post("/items/{number}/approvals/{requestID}/approve", a.decideApproval(true))
	member.Post("/items/{number}/approvals/{requestID}/reject", a.decideApproval(false))
	member.Post("/items/{number}/approvals/{requestID}/cancel", a.cancelApproval)
}

// approvalPolicyRoutes are mounted with the workflow configuration (admin).
func (a *api) approvalPolicyRoutes(admin chi.Router) {
	admin.Put("/workflows/{workflowID}/statuses/{statusID}/approval-policy", a.setApprovalPolicy)
	admin.Delete("/workflows/{workflowID}/statuses/{statusID}/approval-policy", a.deleteApprovalPolicy)
}

func (a *api) setApprovalPolicy(w http.ResponseWriter, r *http.Request) {
	wfID, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	stID, ok := a.uuidParam(w, r, "statusID")
	if !ok {
		return
	}
	var body store.ApprovalPolicyInput
	if !a.decode(w, r, &body) {
		return
	}
	wf, err := a.store.SetApprovalPolicy(r.Context(), workspaceFrom(r).ID, principal(r).UserID, wfID, stID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) deleteApprovalPolicy(w http.ResponseWriter, r *http.Request) {
	wfID, ok := a.uuidParam(w, r, "workflowID")
	if !ok {
		return
	}
	stID, ok := a.uuidParam(w, r, "statusID")
	if !ok {
		return
	}
	wf, err := a.store.DeleteApprovalPolicy(r.Context(), workspaceFrom(r).ID, principal(r).UserID, wfID, stID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (a *api) listApprovals(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	list, err := a.store.ListApprovals(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": list})
}

// writeApprovals answers a change with the item's approval list as it now
// stands, plus the item itself (its status, approval badge and version).
func (a *api) writeApprovals(w http.ResponseWriter, r *http.Request, status int, n int64) {
	ws, proj := workspaceFrom(r), projectFrom(r)
	list, err := a.store.ListApprovals(r.Context(), ws.ID, proj.ID, n, viewer(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	item, err := a.store.GetItem(r.Context(), ws.ID, proj.ID, n)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, status, map[string]any{"approvals": list, "item": item})
}

func (a *api) requestApproval(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	var body struct {
		Version    *int       `json:"version"`
		ToStatusID *uuid.UUID `json:"to_status_id"`
		Comment    *string    `json:"comment"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	if body.Version == nil || *body.Version < 1 {
		a.fail(w, r, &store.ValidationError{Field: "version", Message: "is required (the version you last read)"})
		return
	}
	if body.ToStatusID == nil {
		a.fail(w, r, &store.ValidationError{Field: "to_status_id", Message: "is required"})
		return
	}
	ws, proj := workspaceFrom(r), projectFrom(r)
	id, err := a.store.RequestApproval(r.Context(), store.RequestApprovalParams{
		WorkspaceID: ws.ID, ProjectID: proj.ID, Number: n, ActorID: principal(r).UserID,
		Version: *body.Version, ToStatusID: *body.ToStatusID, Comment: body.Comment,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/items/%d/approvals#%s", ws.Slug, proj.Key, n, id))
	a.writeApprovals(w, r, http.StatusCreated, n)
}

func (a *api) decideApproval(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, ok := itemNumber(r)
		if !ok {
			a.fail(w, r, store.ErrNotFound)
			return
		}
		reqID, ok := a.uuidParam(w, r, "requestID")
		if !ok {
			return
		}
		var body struct {
			Comment *string `json:"comment"` // approve: optional note
			Reason  *string `json:"reason"`  // reject: required
			// Decide as the delegate of this absent approver (ADR-0042).
			OnBehalfOf *uuid.UUID `json:"on_behalf_of"`
		}
		if !a.decode(w, r, &body) {
			return
		}
		text := body.Comment
		if !approve {
			text = body.Reason
		}
		err := a.store.DecideApproval(r.Context(), store.DecideApprovalParams{
			WorkspaceID: workspaceFrom(r).ID, ProjectID: projectFrom(r).ID, Number: n, RequestID: reqID,
			ActorID: principal(r).UserID, Approve: approve, Comment: text, OnBehalfOf: body.OnBehalfOf,
		})
		if err != nil {
			a.fail(w, r, err)
			return
		}
		a.writeApprovals(w, r, http.StatusOK, n)
	}
}

func (a *api) cancelApproval(w http.ResponseWriter, r *http.Request) {
	n, ok := itemNumber(r)
	if !ok {
		a.fail(w, r, store.ErrNotFound)
		return
	}
	reqID, ok := a.uuidParam(w, r, "requestID")
	if !ok {
		return
	}
	if err := a.store.CancelApproval(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, n, reqID, viewer(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeApprovals(w, r, http.StatusOK, n)
}
