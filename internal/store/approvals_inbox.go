package store

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The workspace-wide approval inbox and deadline reminders (ADR-0034).

const (
	maxInboxPage = 100
	// InboxCountCap bounds the badge count query; the UI shows "99+".
	InboxCountCap = 100
)

// InboxItem names the item of an inbox entry.
type InboxItem struct {
	Key        string `json:"key"`
	Number     int64  `json:"number"`
	ProjectKey string `json:"project_key"`
	Title      string `json:"title"`
}

// InboxEntry is one pending request in the workspace inbox.
type InboxEntry struct {
	ID           uuid.UUID     `json:"id"`
	Item         InboxItem     `json:"item"`
	RequestedBy  CommentAuthor `json:"requested_by"`
	FromStatus   *Status       `json:"from_status"`
	ToStatus     *Status       `json:"to_status"`
	Approvals    int           `json:"approvals"`
	MinApprovals int           `json:"min_approvals"`
	CreatedAt    time.Time     `json:"created_at"`
	DueOn        *string       `json:"due_on"`
	Overdue      bool          `json:"overdue"`
	// CanDecide: the viewer may approve or reject it now, on their own
	// account or as a delegate (ADR-0042).
	CanDecide bool `json:"can_decide"`
	// OnBehalfOf: the absent approvers the viewer may decide for ("X adına").
	OnBehalfOf []CommentAuthor `json:"on_behalf_of"`
}

// InboxCursor is the keyset position (created_at, id) after the last entry.
type InboxCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// EncodeInboxCursor makes an opaque cursor string.
func EncodeInboxCursor(e InboxEntry) string {
	return base64.RawURLEncoding.EncodeToString([]byte(e.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + e.ID.String()))
}

// DecodeInboxCursor parses a cursor made by EncodeInboxCursor (same format
// as the comment cursor).
func DecodeInboxCursor(s string) (InboxCursor, error) {
	c, err := DecodeCommentCursor(s)
	return InboxCursor(c), err
}

// InboxFilter selects a page of the inbox.
type InboxFilter struct {
	Mine  bool // only requests the viewer may decide on now
	After *InboxCursor
	Limit int
}

// inboxFrom joins a pending request with what the inbox shows, as seen by
// the viewer $2: only projects the viewer can see (ADR-0013), live items.
const inboxFrom = ` FROM approval_requests r
	JOIN workspaces w ON w.id = r.workspace_id
	JOIN items i ON i.id = r.item_id AND i.deleted_at IS NULL
	JOIN projects p ON p.id = i.project_id
	JOIN workspace_members vm ON vm.workspace_id = r.workspace_id AND vm.user_id = $2
	JOIN users vu ON vu.id = vm.user_id`

// inboxWhere restricts to the workspace's pending requests the viewer may
// see; canDecide is the viewer's eligibility (approverEligible, the same
// predicate as the decision itself) and their approval slot still open
// (no decision by them or on their behalf), or (ADR-0042) standing in for
// an absent approver now (canActForSomeone, the same predicate as a
// delegated decision).
var (
	inboxWhere = ` WHERE r.workspace_id = $1 AND r.state = 'pending' AND p.archived_at IS NULL AND ` +
		visibleProjectFor("p", "vm.user_id", "vm.role IN ('admin', 'owner')")
	inboxCanDecide = `((` + approverEligible("vm", "vu", "p", "r.requested_by", "r.approver_roles", "r.approver_user_ids") + `
	  AND NOT EXISTS (SELECT 1 FROM approval_decisions d WHERE d.request_id = r.id AND (d.user_id = vm.user_id OR d.on_behalf_of = vm.user_id)))
	  OR ` + canActForSomeone("vm", "vu", "r", "p") + `)`
	inboxPrincipals = principalsJSON("vm", "vu", "r", "p")
	inboxOverdue    = `COALESCE(r.due_on < ` + localToday + `, false)`
)

// ListApprovalInbox returns the workspace's pending requests the viewer can
// see, oldest first (the longest-waiting request is the most urgent), with
// keyset pagination. With Mine, only those the viewer may decide on now.
func (s *Store) ListApprovalInbox(ctx context.Context, wsID uuid.UUID, v Viewer, f InboxFilter) ([]InboxEntry, string, error) {
	limit := min(max(f.Limit, 1), maxInboxPage)
	args := []any{wsID, v.UserID, limit + 1}
	where := inboxWhere
	if f.Mine {
		where += ` AND ` + inboxCanDecide
	}
	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		where += ` AND (r.created_at, r.id) > ($` + strconv.Itoa(len(args)-1) + `, $` + strconv.Itoa(len(args)) + `)`
	}
	rows, err := s.pool.Query(ctx, `SELECT r.id, i.number, p.key, i.title, r.requested_by, ru.display_name,
			r.from_status_id, fs.name, fs.category, r.to_status_id, ts.name, ts.category,
			r.approvals, r.min_approvals, r.created_at, r.due_on::text, `+inboxOverdue+`, `+inboxCanDecide+`, `+inboxPrincipals+
		inboxFrom+`
		JOIN users ru ON ru.id = r.requested_by
		LEFT JOIN statuses fs ON fs.id = r.from_status_id
		LEFT JOIN statuses ts ON ts.id = r.to_status_id`+where+`
		ORDER BY r.created_at, r.id LIMIT $3`, args...)
	if err != nil {
		return nil, "", err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (InboxEntry, error) {
		var e InboxEntry
		var fromID, toID *uuid.UUID
		var fromName, fromCat, toName, toCat *string
		err := row.Scan(&e.ID, &e.Item.Number, &e.Item.ProjectKey, &e.Item.Title, &e.RequestedBy.ID, &e.RequestedBy.DisplayName,
			&fromID, &fromName, &fromCat, &toID, &toName, &toCat,
			&e.Approvals, &e.MinApprovals, &e.CreatedAt, &e.DueOn, &e.Overdue, &e.CanDecide, &e.OnBehalfOf)
		e.OnBehalfOf = nonNilSlice(e.OnBehalfOf)
		e.Item.Key = itemKey(e.Item.ProjectKey, e.Item.Number)
		if fromID != nil && fromName != nil && fromCat != nil {
			e.FromStatus = &Status{ID: *fromID, Name: *fromName, Category: *fromCat}
		}
		if toID != nil && toName != nil && toCat != nil {
			e.ToStatus = &Status{ID: *toID, Name: *toName, Category: *toCat}
		}
		return e, err
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = EncodeInboxCursor(out[len(out)-1])
	}
	return nonNilSlice(out), next, nil
}

// InboxCount is the badge: how many requests wait (capped at InboxCountCap)
// and how many of those are past their deadline.
type InboxCount struct {
	Count   int `json:"count"`
	Overdue int `json:"overdue"`
}

// CountApprovalInbox counts what ListApprovalInbox would list, up to the cap.
func (s *Store) CountApprovalInbox(ctx context.Context, wsID uuid.UUID, v Viewer, mine bool) (InboxCount, error) {
	where := inboxWhere
	if mine {
		where += ` AND ` + inboxCanDecide
	}
	var c InboxCount
	// Overdue rows first, so the overdue count is exact whenever it is
	// below the cap.
	err := s.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE overdue) FROM (
			SELECT `+inboxOverdue+` AS overdue`+inboxFrom+where+`
			ORDER BY 1 DESC LIMIT $3) x`, wsID, v.UserID, InboxCountCap).Scan(&c.Count, &c.Overdue)
	return c, err
}

// EmitApprovalOverdue emits approval.overdue once for up to limit pending
// requests whose deadline day has passed (workspace local date), and returns
// how many. The UPDATE ... WHERE overdue_notified_at IS NULL is the
// idempotency marker: a second worker replica, or a re-run, finds nothing.
// Rows an approval transaction holds are skipped (SKIP LOCKED) and caught
// on the next tick; the scan never waits on an item lock, so it cannot
// deadlock with decisions (item row first, then request).
func (s *Store) EmitApprovalOverdue(ctx context.Context, limit int) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE approval_requests r SET overdue_notified_at = now()
			WHERE r.id IN (
				SELECT c.id FROM approval_requests c
				JOIN workspaces w ON w.id = c.workspace_id AND w.archived_at IS NULL
				JOIN items ci ON ci.id = c.item_id
				JOIN projects cp ON cp.id = ci.project_id AND cp.archived_at IS NULL
				WHERE c.state = 'pending' AND c.due_on IS NOT NULL AND c.overdue_notified_at IS NULL
				  AND c.due_on < `+localToday+`
				ORDER BY c.due_on, c.id LIMIT $1
				FOR UPDATE OF c SKIP LOCKED)
			  AND r.state = 'pending' AND r.overdue_notified_at IS NULL
			RETURNING r.id, r.workspace_id, r.item_id, r.from_status_id, r.to_status_id, r.requested_by,
				r.min_approvals, r.approvals, r.due_on::text`, limit)
		if err != nil {
			return err
		}
		type due struct {
			ws  uuid.UUID
			row approvalRow
			on  string
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (due, error) {
			var d due
			return d, r.Scan(&d.row.id, &d.ws, &d.row.itemID, &d.row.fromStatus, &d.row.toStatus, &d.row.requestedBy,
				&d.row.minApprovals, &d.row.approvals, &d.on)
		})
		if err != nil {
			return err
		}
		for _, d := range list {
			if err := appendEvent(ctx, tx, d.ws, uuid.Nil, "item", d.row.itemID, EventApprovalOverdue,
				approvalPayload(d.row, map[string]any{"due_on": d.on})); err != nil {
				return err
			}
		}
		n = len(list)
		return nil
	})
	return n, err
}
