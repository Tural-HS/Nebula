package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Four-eyes approvals (ADR-0026). A policy guards entering one status of a
// workflow. Moving an item there is refused (approval_required) unless it
// happens through an approval request that reached min_approvals. While a
// request is pending, the content approvers sign off on is locked
// (approval_pending): status, title, description, custom fields, item-level
// attachments, and the item cannot be deleted. The requester never counts
// as an approver.

const (
	maxMinApprovals      = 5
	maxPolicyUsers       = 50
	maxApprovalText      = 2000
	maxApprovalHistory   = 20
	maxApproverNotifyees = 100
)

// Approval request states.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalCanceled = "canceled"
)

// Approval event actions (entity type "item").
const (
	EventApprovalRequested = "approval.requested"
	EventApprovalApproved  = "approval.approved"
	EventApprovalRejected  = "approval.rejected"
	EventApprovalCanceled  = "approval.canceled"
	EventApprovalApplied   = "approval.applied"
)

var approverRoles = []string{RoleMember, RoleAdmin, RoleOwner}

// ForbiddenError is a permission refusal with a specific code, so clients
// can tell "you are the requester" from "you are not an approver".
type ForbiddenError struct {
	Code    string
	Message string
}

func (e *ForbiddenError) Error() string { return e.Code + ": " + e.Message }

// ApprovalPolicy guards entering StatusID of a workflow.
type ApprovalPolicy struct {
	ID              uuid.UUID   `json:"id"`
	StatusID        uuid.UUID   `json:"status_id"`
	MinApprovals    int         `json:"min_approvals"`
	ApproverRoles   []string    `json:"approver_roles"`
	ApproverUserIDs []uuid.UUID `json:"approver_user_ids"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// ApprovalPolicyInput is the body of a policy PUT.
type ApprovalPolicyInput struct {
	MinApprovals    int         `json:"min_approvals"`
	ApproverRoles   []string    `json:"approver_roles"`
	ApproverUserIDs []uuid.UUID `json:"approver_user_ids"`
}

// PendingApproval is the summary of an item's open request, embedded in
// every item so lists and boards can show a badge without another query.
type PendingApproval struct {
	ID           uuid.UUID `json:"id"`
	ToStatus     Status    `json:"to_status"`
	RequestedBy  uuid.UUID `json:"requested_by"`
	Approvals    int       `json:"approvals"`
	MinApprovals int       `json:"min_approvals"`
}

// ApprovalDecision is one approver's vote.
type ApprovalDecision struct {
	User      CommentAuthor `json:"user"`
	Decision  string        `json:"decision"` // approve | reject
	Comment   *string       `json:"comment"`
	CreatedAt time.Time     `json:"created_at"`
}

// ApprovalRequest is a request with its decisions, as seen by one viewer.
type ApprovalRequest struct {
	ID            uuid.UUID          `json:"id"`
	State         string             `json:"state"`
	FromStatus    *Status            `json:"from_status"` // nil if the status was deleted since
	ToStatus      *Status            `json:"to_status"`
	RequestedBy   *CommentAuthor     `json:"requested_by"`
	Comment       *string            `json:"comment"`
	MinApprovals  int                `json:"min_approvals"`
	Approvals     int                `json:"approvals"`
	ApproverRoles []string           `json:"approver_roles"`
	Decisions     []ApprovalDecision `json:"decisions"`
	// Approvers who may still decide (pending requests only): eligible
	// people who can see the project and have not voted. Never the requester.
	Approvers  []CommentAuthor `json:"approvers"`
	CanApprove bool            `json:"can_approve"` // the viewer may approve or reject now
	CanCancel  bool            `json:"can_cancel"`
	CreatedAt  time.Time       `json:"created_at"`
	ClosedAt   *time.Time      `json:"closed_at"`
	ClosedBy   *CommentAuthor  `json:"closed_by"`
}

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

func listApprovalPolicies(ctx context.Context, q querier, wsID uuid.UUID, only *uuid.UUID) (map[uuid.UUID][]ApprovalPolicy, error) {
	rows, err := q.Query(ctx, `SELECT workflow_id, id, status_id, min_approvals, approver_roles, approver_user_ids, updated_at
		FROM approval_policies WHERE workspace_id = $1 AND ($2::uuid IS NULL OR workflow_id = $2)
		ORDER BY created_at, id`, wsID, only)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]ApprovalPolicy{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var wf uuid.UUID
		var p ApprovalPolicy
		err := r.Scan(&wf, &p.ID, &p.StatusID, &p.MinApprovals, &p.ApproverRoles, &p.ApproverUserIDs, &p.UpdatedAt)
		out[wf] = append(out[wf], p)
		return struct{}{}, err
	})
	return out, err
}

// SetApprovalPolicy creates or replaces the policy guarding a status.
func (s *Store) SetApprovalPolicy(ctx context.Context, wsID, actorID, workflowID, statusID uuid.UUID, in ApprovalPolicyInput) (Workflow, error) {
	var out Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = setApprovalPolicy(ctx, tx, wsID, actorID, workflowID, statusID, in)
		return err
	})
	return out, err
}

// setApprovalPolicy is SetApprovalPolicy inside the caller's transaction (workspace kinds, ADR-0033).
func setApprovalPolicy(ctx context.Context, tx pgx.Tx, wsID, actorID, workflowID, statusID uuid.UUID, in ApprovalPolicyInput) (Workflow, error) {
	if in.MinApprovals < 1 || in.MinApprovals > maxMinApprovals {
		return Workflow{}, invalid("min_approvals", "must be between 1 and %d", maxMinApprovals)
	}
	roles := []string{}
	for _, r := range in.ApproverRoles {
		if !slices.Contains(approverRoles, r) {
			return Workflow{}, invalid("approver_roles", "must be member, admin or owner (guests never approve)")
		}
		if !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	users := []uuid.UUID{}
	for _, u := range in.ApproverUserIDs {
		if !slices.Contains(users, u) {
			users = append(users, u)
		}
	}
	if len(users) > maxPolicyUsers {
		return Workflow{}, invalid("approver_user_ids", "at most %d users", maxPolicyUsers)
	}
	if len(roles) == 0 && len(users) == 0 {
		return Workflow{}, invalid("approver_roles", "name at least one approver role or user")
	}
	if len(roles) == 0 && len(users) < in.MinApprovals {
		return Workflow{}, invalid("min_approvals", "cannot exceed the number of approvers")
	}
	var wf Workflow
	err := func() error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE id = $1 AND workflow_id = $2)`,
			statusID, workflowID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if len(users) > 0 {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM workspace_members
				WHERE workspace_id = $1 AND user_id = ANY($2::uuid[]) AND role <> 'guest'`, wsID, users).Scan(&n); err != nil {
				return err
			}
			if n != len(users) {
				return invalid("approver_user_ids", "every approver must be a workspace member with at least the member role")
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO approval_policies
			(id, workspace_id, workflow_id, status_id, min_approvals, approver_roles, approver_user_ids)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (status_id) DO UPDATE SET min_approvals = EXCLUDED.min_approvals,
				approver_roles = EXCLUDED.approver_roles, approver_user_ids = EXCLUDED.approver_user_ids, updated_at = now()`,
			newID(), wsID, workflowID, statusID, in.MinApprovals, roles, users)
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "workflow", workflowID, "approval_policy.set", map[string]any{
			"status_id": statusID, "min_approvals": in.MinApprovals, "approver_roles": roles, "approver_user_ids": users,
		}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	}()
	return wf, err
}

// DeleteApprovalPolicy removes the guard from a status. Pending requests
// keep their snapshot and can still be decided.
func (s *Store) DeleteApprovalPolicy(ctx context.Context, wsID, actorID, workflowID, statusID uuid.UUID) (Workflow, error) {
	var wf Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM approval_policies WHERE workflow_id = $1 AND status_id = $2`, workflowID, statusID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "workflow", workflowID, "approval_policy.deleted",
			map[string]any{"status_id": statusID}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	})
	return wf, err
}

// statusGuarded reports whether entering a status needs approval.
func statusGuarded(ctx context.Context, q querier, statusID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approval_policies WHERE status_id = $1)`, statusID).Scan(&ok)
	return ok, err
}

// ---------------------------------------------------------------------------
// Guards used by item edits, deletes and attachments
// ---------------------------------------------------------------------------

// pendingApprovalID returns the item's open request, if any. Callers hold a
// lock on the item row, so the answer cannot change under them: a request
// is created only under FOR UPDATE on the same row.
func pendingApprovalID(ctx context.Context, q querier, itemID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM approval_requests WHERE item_id = $1 AND state = 'pending'`, itemID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func errApprovalPending() error {
	return &ConflictError{Code: "approval_pending",
		Message: "this item is waiting for approval: status, title, description, custom fields and attachments are locked until the request is decided or canceled"}
}

func errApprovalRequired() error {
	return &ConflictError{Code: "approval_required",
		Message: "moving an item into this status requires approval; request it instead (POST .../approvals)"}
}

// lockedChange reports whether an item change touches content an approver
// signs off on.
func lockedChange(changes map[string]any) bool {
	for k := range changes {
		if k == "title" || k == "description" || k == "status_id" || strings.HasPrefix(k, "fields.") {
			return true
		}
	}
	return false
}

// requireNoPendingApproval locks the item row (FOR SHARE, so it orders
// against a request being created) and refuses if a request is open.
func requireNoPendingApproval(ctx context.Context, q querier, itemID uuid.UUID) error {
	if _, err := q.Exec(ctx, `SELECT 1 FROM items WHERE id = $1 FOR SHARE`, itemID); err != nil {
		return err
	}
	id, err := pendingApprovalID(ctx, q, itemID)
	if err != nil {
		return err
	}
	if id != nil {
		return errApprovalPending()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Requests and decisions
// ---------------------------------------------------------------------------

// RequestApprovalParams ask for an item to move into a guarded status.
type RequestApprovalParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	Number      int64
	ActorID     uuid.UUID
	Version     int
	ToStatusID  uuid.UUID
	Comment     *string
}

// approvalRow is a request as stored, with its policy snapshot.
type approvalRow struct {
	id, itemID   uuid.UUID
	fromStatus   *uuid.UUID
	toStatus     *uuid.UUID
	requestedBy  uuid.UUID
	minApprovals int
	roles        []string
	users        []uuid.UUID
	approvals    int
	state        string
}

func lockApproval(ctx context.Context, tx pgx.Tx, itemID, requestID uuid.UUID) (approvalRow, error) {
	var r approvalRow
	err := tx.QueryRow(ctx, `SELECT id, item_id, from_status_id, to_status_id, requested_by, min_approvals,
			approver_roles, approver_user_ids, approvals, state
		FROM approval_requests WHERE id = $1 AND item_id = $2 FOR UPDATE`, requestID, itemID).
		Scan(&r.id, &r.itemID, &r.fromStatus, &r.toStatus, &r.requestedBy, &r.minApprovals, &r.roles, &r.users, &r.approvals, &r.state)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// lockedItem is the part of an item an approval transaction needs, read
// under FOR UPDATE: every approval transaction locks the item row first and
// the request row second, so they serialize in one order.
type lockedItem struct {
	id, status, workflow uuid.UUID
	version              int
}

func lockItemRow(ctx context.Context, tx pgx.Tx, wsID, projectID uuid.UUID, number int64) (lockedItem, error) {
	var it lockedItem
	err := tx.QueryRow(ctx, `SELECT i.id, i.status_id, t.workflow_id, i.version
		FROM items i JOIN item_types t ON t.id = i.item_type_id
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL
		FOR UPDATE OF i`, wsID, projectID, number).Scan(&it.id, &it.status, &it.workflow, &it.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

func approvalText(field string, v *string, required bool) (*string, error) {
	out, err := optionalText(field, v, maxApprovalText)
	if err != nil {
		return nil, err
	}
	if required && out == nil {
		return nil, invalid(field, "is required")
	}
	return out, nil
}

// approvalPayload is the common part of every approval.* event.
func approvalPayload(r approvalRow, extra map[string]any) map[string]any {
	p := map[string]any{
		"request_id": r.id, "requested_by": r.requestedBy,
		"from_status_id": r.fromStatus, "to_status_id": r.toStatus,
		"approvals": r.approvals, "min_approvals": r.minApprovals,
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// RequestApproval opens an approval request for moving an item into a
// guarded status. The item does not move; its guarded content locks.
func (s *Store) RequestApproval(ctx context.Context, p RequestApprovalParams) (uuid.UUID, error) {
	comment, err := approvalText("comment", p.Comment, false)
	if err != nil {
		return uuid.Nil, err
	}
	id := newID()
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		it, err := lockItemRow(ctx, tx, p.WorkspaceID, p.ProjectID, p.Number)
		if err != nil {
			return err
		}
		if it.version != p.Version {
			return &ConflictError{Code: "version_conflict",
				Message: fmt.Sprintf("item was modified by someone else (current version %d); reload and retry", it.version)}
		}
		if pending, err := pendingApprovalID(ctx, tx, it.id); err != nil {
			return err
		} else if pending != nil {
			return &ConflictError{Code: "approval_pending", Message: "this item already has a pending approval request"}
		}
		if p.ToStatusID == it.status {
			return invalid("to_status_id", "the item is already in this status")
		}
		var inWorkflow bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE id = $1 AND workflow_id = $2)`,
			p.ToStatusID, it.workflow).Scan(&inWorkflow); err != nil {
			return err
		}
		if !inWorkflow {
			return invalid("to_status_id", "status does not belong to this item type's workflow")
		}
		if err := checkTransition(ctx, tx, it.workflow, it.status, p.ToStatusID); err != nil {
			return asField(err, "to_status_id")
		}
		var pol ApprovalPolicy
		err = tx.QueryRow(ctx, `SELECT id, min_approvals, approver_roles, approver_user_ids FROM approval_policies
			WHERE workflow_id = $1 AND status_id = $2`, it.workflow, p.ToStatusID).
			Scan(&pol.ID, &pol.MinApprovals, &pol.ApproverRoles, &pol.ApproverUserIDs)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("to_status_id", "this status needs no approval; change the status directly")
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO approval_requests (id, workspace_id, item_id, policy_id, from_status_id, to_status_id,
				requested_by, comment, min_approvals, approver_roles, approver_user_ids)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, p.WorkspaceID, it.id, pol.ID, it.status, p.ToStatusID, p.ActorID, comment,
			pol.MinApprovals, pol.ApproverRoles, pol.ApproverUserIDs)
		if isUniqueViolation(err, "approval_requests_one_pending_idx") {
			return &ConflictError{Code: "approval_pending", Message: "this item already has a pending approval request"}
		}
		if err != nil {
			return err
		}
		from, to := it.status, p.ToStatusID
		row := approvalRow{id: id, itemID: it.id, fromStatus: &from, toStatus: &to, requestedBy: p.ActorID,
			minApprovals: pol.MinApprovals, roles: pol.ApproverRoles, users: pol.ApproverUserIDs}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", it.id, EventApprovalRequested,
			approvalPayload(row, map[string]any{"comment": comment}))
	})
	return id, err
}

func asField(err error, field string) error {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return &ValidationError{Field: field, Message: ve.Message}
	}
	return err
}

// DecideApprovalParams approve or reject a pending request.
type DecideApprovalParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	Number      int64
	RequestID   uuid.UUID
	ActorID     uuid.UUID
	Approve     bool
	Comment     *string // optional for approve, the mandatory reason for reject
}

// DecideApproval records one approver's decision. The approval that reaches
// min_approvals applies the status change in the same transaction, through
// the normal item update path (workflow transition rules, item.updated
// event). Concurrent approvals serialize on the item row, so the change is
// applied exactly once; a late approval finds the request closed.
func (s *Store) DecideApproval(ctx context.Context, p DecideApprovalParams) error {
	field := "reason" // a rejection must say why
	if p.Approve {
		field = "comment"
	}
	comment, err := approvalText(field, p.Comment, !p.Approve)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		it, err := lockItemRow(ctx, tx, p.WorkspaceID, p.ProjectID, p.Number)
		if err != nil {
			return err
		}
		r, err := lockApproval(ctx, tx, it.id, p.RequestID)
		if err != nil {
			return err
		}
		if r.state != ApprovalPending {
			return &ConflictError{Code: "approval_closed", Message: "this approval request is already " + r.state}
		}
		if p.ActorID == r.requestedBy {
			return &ForbiddenError{Code: "self_approval", Message: "the requester cannot approve or reject their own request"}
		}
		ok, err := isEligibleApprover(ctx, tx, p.WorkspaceID, p.ProjectID, r, p.ActorID)
		if err != nil {
			return err
		}
		if !ok {
			return &ForbiddenError{Code: "not_an_approver", Message: "you are not an approver of this request"}
		}
		decision := "reject"
		if p.Approve {
			decision = "approve"
		}
		_, err = tx.Exec(ctx, `INSERT INTO approval_decisions (request_id, user_id, decision, comment) VALUES ($1, $2, $3, $4)`,
			r.id, p.ActorID, decision, comment)
		if isUniqueViolation(err, "approval_decisions_pkey") {
			return &ConflictError{Code: "already_decided", Message: "you have already decided on this request"}
		}
		if err != nil {
			return err
		}

		if !p.Approve {
			if _, err := tx.Exec(ctx, `UPDATE approval_requests SET state = 'rejected', closed_at = now(), closed_by = $2 WHERE id = $1`,
				r.id, p.ActorID); err != nil {
				return err
			}
			return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", it.id, EventApprovalRejected,
				approvalPayload(r, map[string]any{"reason": comment}))
		}

		r.approvals++
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET approvals = $2 WHERE id = $1`, r.id, r.approvals); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", it.id, EventApprovalApproved,
			approvalPayload(r, map[string]any{"comment": comment})); err != nil {
			return err
		}
		if r.approvals < r.minApprovals {
			return nil
		}
		if r.toStatus == nil {
			return &ConflictError{Code: "approval_closed", Message: "the target status no longer exists; cancel this request"}
		}
		// Apply: the version is the one read under this transaction's lock,
		// so nothing can slip in between the check and the change.
		upd := UpdateItemParams{WorkspaceID: p.WorkspaceID, ProjectID: p.ProjectID, Number: p.Number,
			ActorID: p.ActorID, Version: it.version, StatusID: opt.Of(*r.toStatus), approvalApply: r.id}
		if _, _, err := updateItemTx(ctx, tx, upd, checkedUpdate{}, map[string]any{"approval": map[string]any{"request_id": r.id}}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET state = 'approved', closed_at = now(), closed_by = $2 WHERE id = $1`,
			r.id, p.ActorID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", it.id, EventApprovalApplied, approvalPayload(r, nil))
	})
}

// CancelApproval closes a pending request without a decision. The requester
// may cancel their own request; admins may cancel any.
func (s *Store) CancelApproval(ctx context.Context, wsID, projectID uuid.UUID, number int64, requestID uuid.UUID, actor Viewer) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		it, err := lockItemRow(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		r, err := lockApproval(ctx, tx, it.id, requestID)
		if err != nil {
			return err
		}
		if r.state != ApprovalPending {
			return &ConflictError{Code: "approval_closed", Message: "this approval request is already " + r.state}
		}
		if actor.UserID != r.requestedBy && !RoleAtLeast(actor.Role, RoleAdmin) {
			return &ForbiddenError{Code: "forbidden", Message: "only the requester or an admin can cancel this request"}
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET state = 'canceled', closed_at = now(), closed_by = $2 WHERE id = $1`,
			r.id, actor.UserID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actor.UserID, "item", it.id, EventApprovalCanceled, approvalPayload(r, nil))
	})
}

// eligibleApproversSQL lists the people who may decide on a request: active
// members with at least the member role, matching the snapshot's roles or
// users, who can see the project (ADR-0013; keep in sync with
// canSeeProject), excluding the requester. $1 workspace, $2 project,
// $3 requester, $4 roles, $5 users, $6 one user to check (NULL = all).
const eligibleApproversSQL = `SELECT u.id, u.display_name FROM workspace_members m
	JOIN users u ON u.id = m.user_id
	JOIN projects p ON p.workspace_id = m.workspace_id AND p.id = $2
	WHERE m.workspace_id = $1 AND u.is_active AND m.role <> 'guest' AND m.user_id <> $3
	  AND (m.role = ANY($4::text[]) OR m.user_id = ANY($5::uuid[]))
	  AND ($6::uuid IS NULL OR m.user_id = $6)
	  AND (p.visibility = 'workspace' OR m.role IN ('admin', 'owner')
	       OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = m.user_id))`

func isEligibleApprover(ctx context.Context, q querier, wsID, projectID uuid.UUID, r approvalRow, userID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (`+eligibleApproversSQL+`)`,
		wsID, projectID, r.requestedBy, r.roles, r.users, userID).Scan(&ok)
	return ok, err
}

// eligibleApprovers lists who may still decide (not yet voted), by name.
func eligibleApprovers(ctx context.Context, q querier, wsID, projectID uuid.UUID, r approvalRow, limit int) ([]CommentAuthor, error) {
	rows, err := q.Query(ctx, eligibleApproversSQL+`
		AND NOT EXISTS (SELECT 1 FROM approval_decisions d WHERE d.request_id = $7 AND d.user_id = m.user_id)
		ORDER BY u.display_name, u.id LIMIT $8`,
		wsID, projectID, r.requestedBy, r.roles, r.users, nil, r.id, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (CommentAuthor, error) {
		var a CommentAuthor
		return a, row.Scan(&a.ID, &a.DisplayName)
	})
}

// ListApprovals returns an item's recent approval requests, newest first,
// with the viewer's possible actions.
func (s *Store) ListApprovals(ctx context.Context, wsID, projectID uuid.UUID, number int64, v Viewer) ([]ApprovalRequest, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.state, r.from_status_id, fs.name, fs.category, r.to_status_id, ts.name, ts.category,
			r.requested_by, ru.display_name, r.comment, r.min_approvals, r.approvals, r.approver_roles, r.approver_user_ids,
			r.created_at, r.closed_at, r.closed_by, cu.display_name
		FROM approval_requests r
		LEFT JOIN statuses fs ON fs.id = r.from_status_id
		LEFT JOIN statuses ts ON ts.id = r.to_status_id
		JOIN users ru ON ru.id = r.requested_by
		LEFT JOIN users cu ON cu.id = r.closed_by
		WHERE r.workspace_id = $1 AND r.item_id = $2
		ORDER BY r.created_at DESC, r.id DESC LIMIT $3`, wsID, item.id, maxApprovalHistory)
	if err != nil {
		return nil, err
	}
	var raws []approvalRow
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApprovalRequest, error) {
		var a ApprovalRequest
		var raw approvalRow
		var fromName, fromCat, toName, toCat *string
		var reqName string
		var closedBy *uuid.UUID
		var closedName *string
		err := row.Scan(&a.ID, &a.State, &raw.fromStatus, &fromName, &fromCat, &raw.toStatus, &toName, &toCat,
			&raw.requestedBy, &reqName, &a.Comment, &a.MinApprovals, &a.Approvals, &a.ApproverRoles, &raw.users,
			&a.CreatedAt, &a.ClosedAt, &closedBy, &closedName)
		if err != nil {
			return a, err
		}
		if raw.fromStatus != nil && fromName != nil {
			a.FromStatus = &Status{ID: *raw.fromStatus, Name: *fromName, Category: *fromCat}
		}
		if raw.toStatus != nil && toName != nil {
			a.ToStatus = &Status{ID: *raw.toStatus, Name: *toName, Category: *toCat}
		}
		a.RequestedBy = &CommentAuthor{ID: raw.requestedBy, DisplayName: reqName}
		if closedBy != nil && closedName != nil {
			a.ClosedBy = &CommentAuthor{ID: *closedBy, DisplayName: *closedName}
		}
		raw.id, raw.roles, raw.state, raw.itemID = a.ID, a.ApproverRoles, a.State, item.id
		raws = append(raws, raw)
		a.Decisions, a.Approvers = []ApprovalDecision{}, []CommentAuthor{}
		return a, nil
	})
	if err != nil {
		return nil, err
	}
	for i := range list {
		a := &list[i]
		rows, err := s.pool.Query(ctx, `SELECT d.user_id, u.display_name, d.decision, d.comment, d.created_at
			FROM approval_decisions d JOIN users u ON u.id = d.user_id
			WHERE d.request_id = $1 ORDER BY d.created_at, d.user_id`, a.ID)
		if err != nil {
			return nil, err
		}
		a.Decisions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApprovalDecision, error) {
			var d ApprovalDecision
			return d, row.Scan(&d.User.ID, &d.User.DisplayName, &d.Decision, &d.Comment, &d.CreatedAt)
		})
		if err != nil {
			return nil, err
		}
		if a.State != ApprovalPending {
			continue
		}
		if a.Approvers, err = eligibleApprovers(ctx, s.pool, wsID, projectID, raws[i], 50); err != nil {
			return nil, err
		}
		a.Approvers = nonNilSlice(a.Approvers)
		// Checked directly: the names above are capped for display.
		decided := slices.ContainsFunc(a.Decisions, func(d ApprovalDecision) bool { return d.User.ID == v.UserID })
		if !decided {
			if a.CanApprove, err = isEligibleApprover(ctx, s.pool, wsID, projectID, raws[i], v.UserID); err != nil {
				return nil, err
			}
		}
		a.CanCancel = v.UserID == raws[i].requestedBy || RoleAtLeast(v.Role, RoleAdmin)
	}
	return nonNilSlice(list), nil
}
