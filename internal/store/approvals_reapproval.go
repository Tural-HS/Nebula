package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Re-approval when approved content changes (ADR-0034). An item that entered
// a guarded status through an approval whose snapshot says
// reapprove_on_change is "covered" by that approval. Changing watched content
// of a covered item does not leave a stale approval behind: in the same
// transaction the approval is marked invalidated, the item goes back to the
// status it was approved from, and a new request for the same move opens
// with the editor as requester, so the changed content needs fresh eyes
// (the editor can never approve it). The item's status therefore always
// means what it says: "Approved" is approved for the content it has now.

// reopening is a planned invalidation, applied by updateItemTx.
type reopening struct {
	old     approvalRow // the approved request being invalidated
	back    uuid.UUID   // the status the item returns to (old.fromStatus)
	guarded uuid.UUID   // the status that needs the new approval (old.toStatus)
	changed []string    // the watched changes that triggered it, sorted
	policy  ApprovalPolicy
}

// coveringApproval is the latest approved request that moved the item into
// status, if it has not been invalidated yet. Requests approved before
// migration 00027 have reapprove_on_change false and never invalidate.
func coveringApproval(ctx context.Context, q querier, itemID, status uuid.UUID) (*approvalRow, bool, []string, error) {
	var r approvalRow
	var reapprove bool
	var fields []string
	err := q.QueryRow(ctx, `SELECT id, item_id, from_status_id, to_status_id, requested_by, min_approvals,
			approver_roles, approver_user_ids, approvals, state, reapprove_on_change, reapprove_fields
		FROM approval_requests
		WHERE item_id = $1 AND state = 'approved' AND invalidated_at IS NULL AND to_status_id = $2
		ORDER BY closed_at DESC, id DESC LIMIT 1`, itemID, status).
		Scan(&r.id, &r.itemID, &r.fromStatus, &r.toStatus, &r.requestedBy, &r.minApprovals, &r.roles, &r.users,
			&r.approvals, &r.state, &reapprove, &fields)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil, nil
	}
	if err != nil {
		return nil, false, nil, err
	}
	return &r, reapprove, fields, nil
}

// reapprovalTriggers returns the change keys that touch watched content:
// the listed tokens, or (empty list) title, description and every custom
// field. Due date, assignee and parent are never content an approver signs.
func reapprovalTriggers(changes map[string]any, watched []string) []string {
	var out []string
	for k := range changes {
		if len(watched) == 0 {
			if k == "title" || k == "description" || strings.HasPrefix(k, "fields.") {
				out = append(out, k)
			}
		} else if slices.Contains(watched, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func errReapprovalRequired(msg string) error {
	return &ConflictError{Code: "reapproval_required", Message: msg}
}

// planReapproval decides whether an edit invalidates the item's covering
// approval. It returns nil when nothing needs re-approval: no covering
// approval, its snapshot does not ask for it, no watched change, or the
// status is no longer guarded at all (the admin removed the policy, so
// entering it needs no approval and neither does changing it). Edits that
// cannot open a request with a human requester are refused instead:
// automation and system changes, and bulk edits, which must not send a
// whole selection of approved items back in one click.
func planReapproval(ctx context.Context, tx pgx.Tx, p UpdateItemParams, itemID, status, workflowID uuid.UUID,
	changes map[string]any, extra map[string]any) (*reopening, error) {
	cov, reapprove, watched, err := coveringApproval(ctx, tx, itemID, status)
	if err != nil || cov == nil || !reapprove {
		return nil, err
	}
	changed := reapprovalTriggers(changes, watched)
	if len(changed) == 0 {
		return nil, nil
	}
	pol, guarded, err := policyOf(ctx, tx, workflowID, status)
	if err != nil || !guarded {
		return nil, err
	}
	if p.ActorID == uuid.Nil {
		return nil, errReapprovalRequired("this item's approved content can only be changed by a person: the change needs a new approval request")
	}
	if _, bulk := extra["bulk"]; bulk {
		return nil, errReapprovalRequired("changing approved content needs a new approval: edit this item on its own, not in a bulk edit")
	}
	if cov.fromStatus == nil {
		return nil, errReapprovalRequired("the status this item was approved from no longer exists: move it out of this status first")
	}
	var inWorkflow bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE id = $1 AND workflow_id = $2)`,
		*cov.fromStatus, workflowID).Scan(&inWorkflow); err != nil {
		return nil, err
	}
	if !inWorkflow {
		return nil, errReapprovalRequired("the status this item was approved from no longer exists: move it out of this status first")
	}
	return &reopening{old: *cov, back: *cov.fromStatus, guarded: status, changed: changed, policy: pol}, nil
}

// apply marks the old approval invalidated and opens the new request. It
// runs before the item row is updated, so the item read back afterwards
// already carries the new pending request. The returned function appends
// approval.invalidated and approval.requested after item.updated.
func (r *reopening) apply(ctx context.Context, tx pgx.Tx, wsID, actorID uuid.UUID) (uuid.UUID, func() error, error) {
	if _, err := tx.Exec(ctx, `UPDATE approval_requests SET invalidated_at = now() WHERE id = $1`, r.old.id); err != nil {
		return uuid.Nil, nil, err
	}
	// The new pending row must exist before the item is read back (its
	// approval summary); the events are appended after item.updated, so the
	// history reads "item.updated, approval.invalidated, approval.requested".
	reqID, old := newID(), r.old.id
	row, err := insertReopened(ctx, tx, openParams{id: reqID, ws: wsID, item: r.old.itemID, from: r.back, to: r.guarded,
		actor: actorID, reopenedFrom: &old}, r.policy)
	if err != nil {
		return uuid.Nil, nil, err
	}
	events := func() error {
		if err := appendEvent(ctx, tx, wsID, actorID, "item", r.old.itemID, EventApprovalInvalidated,
			approvalPayload(r.old, map[string]any{
				"changed": r.changed, "reverted_to_status_id": r.back, "reopened_request_id": reqID,
			})); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item", r.old.itemID, EventApprovalRequested,
			approvalPayload(row.row, row.extra))
	}
	return reqID, events, nil
}

// reopenedRow is an inserted request and the extra payload of its
// approval.requested event.
type reopenedRow struct {
	row   approvalRow
	extra map[string]any
}

// insertReopened is openRequest without the event: the caller orders it.
func insertReopened(ctx context.Context, tx pgx.Tx, o openParams, pol ApprovalPolicy) (reopenedRow, error) {
	due, err := requestDueOn(ctx, tx, o.ws, pol)
	if err != nil {
		return reopenedRow{}, err
	}
	row, err := insertRequest(ctx, tx, o, pol, due)
	if err != nil {
		return reopenedRow{}, err
	}
	return reopenedRow{row: row, extra: requestedExtra(o, due)}, nil
}
