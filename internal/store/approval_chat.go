package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PendingApprovers returns the display names (never emails) of the people who
// may still decide on a pending approval request, at most limit of them, and
// how many there are in all. A closed or unknown request has none. Used by the
// chat card of approval.requested (ADR-0031); the set is evaluated at delivery
// time with the same predicate as the decision itself (ADR-0026), so a
// private project's request only ever names people who can see it.
func (s *Store) PendingApprovers(ctx context.Context, wsID, requestID uuid.UUID, limit int) ([]string, int, error) {
	var r approvalRow
	var projectID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT r.id, r.item_id, r.requested_by, r.approver_roles, r.approver_user_ids, r.state, i.project_id
		FROM approval_requests r JOIN items i ON i.id = r.item_id
		WHERE r.workspace_id = $1 AND r.id = $2`, wsID, requestID).
		Scan(&r.id, &r.itemID, &r.requestedBy, &r.roles, &r.users, &r.state, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if r.state != ApprovalPending {
		return nil, 0, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT display_name, count(*) OVER () FROM (`+eligibleApproversSQL+`
		AND NOT EXISTS (SELECT 1 FROM approval_decisions d WHERE d.request_id = $7 AND (d.user_id = m.user_id OR d.on_behalf_of = m.user_id))) a
		ORDER BY display_name, id LIMIT $8`,
		wsID, projectID, r.requestedBy, r.roles, r.users, nil, r.id, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	names := []string{}
	total := 0
	for rows.Next() {
		var n string
		if err := rows.Scan(&n, &total); err != nil {
			return nil, 0, err
		}
		names = append(names, n)
	}
	return names, total, rows.Err()
}

// PendingDelegate is someone who may decide on a pending request as the
// delegate of absent approvers (ADR-0042), by display name.
type PendingDelegate struct {
	Name string
	For  []string
}

// PendingDelegates lists who may decide on a pending request on an absent
// approver's behalf now, at most limit of them, for the chat card: the same
// predicate as the delegated decision itself.
func (s *Store) PendingDelegates(ctx context.Context, wsID, requestID uuid.UUID, limit int) ([]PendingDelegate, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approval_requests WHERE workspace_id = $1 AND id = $2 AND state = 'pending')`,
		wsID, requestID).Scan(&ok); err != nil || !ok {
		return nil, err
	}
	list, err := requestDelegates(ctx, s.pool, requestID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]PendingDelegate, 0, len(list))
	for _, d := range list {
		pd := PendingDelegate{Name: d.DisplayName}
		for _, p := range d.For {
			pd.For = append(pd.For, p.DisplayName)
		}
		out = append(out, pd)
	}
	return out, nil
}
