package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// ItemTypeRef is the embedded item type summary.
type ItemTypeRef struct {
	ID   uuid.UUID `json:"id"`
	Key  string    `json:"key"`
	Name string    `json:"name"`
}

// Item is a unit of work.
type Item struct {
	ID          uuid.UUID      `json:"id"`
	Key         string         `json:"key"`
	Number      int64          `json:"number"`
	ProjectID   uuid.UUID      `json:"project_id"`
	Type        ItemTypeRef    `json:"type"`
	Status      Status         `json:"status"`
	Title       string         `json:"title"`
	Description *string        `json:"description"`
	AssigneeID  *uuid.UUID     `json:"assignee_id"`
	ReporterID  *uuid.UUID     `json:"reporter_id"`
	Fields      map[string]any `json:"fields"`
	DueDate     *string        `json:"due_date"` // YYYY-MM-DD, in the workspace's time zone
	Overdue     bool           `json:"overdue"`  // past due and not done/canceled (ADR-0009)
	Parent      *ItemParent    `json:"parent"`   // nil for a top-level item (ADR-0021)
	Subtasks    SubtaskSummary `json:"subtasks"`
	// Approval is the pending approval request (ADR-0026); nil when none.
	Approval *PendingApproval `json:"approval"`
	// Blocked: an open item the viewer can see blocks this one (ADR-0028).
	// Only list responses fill it in; elsewhere it is omitted.
	Blocked   *bool     `json:"blocked,omitempty"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	maxTitleLen       = 500
	maxDescriptionLen = 100_000
)

const itemSelect = `SELECT i.id, i.number, p.key, i.project_id, i.title, i.description, i.assignee_id,
	i.reporter_id, i.fields, i.version, i.created_at, i.updated_at,
	t.id, t.key, t.name, s.id, s.name, s.category, i.due_date::text,
	COALESCE(i.due_date < (now() AT TIME ZONE w.timezone)::date AND s.category IN ('todo', 'in_progress'), false),
	par.number, par.title, sub.total, sub.closed,
	ar.id, ar.to_status_id, ars.name, ars.category, ar.requested_by, ar.approvals, ar.min_approvals,
	ar.due_on::text, COALESCE(ar.due_on < (now() AT TIME ZONE w.timezone)::date, false)
	FROM items i
	JOIN workspaces w ON w.id = i.workspace_id
	JOIN projects   p ON p.id = i.project_id
	JOIN item_types t ON t.id = i.item_type_id
	JOIN statuses   s ON s.id = i.status_id
	LEFT JOIN items par ON par.id = i.parent_id AND par.deleted_at IS NULL
	LEFT JOIN LATERAL (SELECT count(*) AS total, count(*) FILTER (WHERE cs.category IN ('done', 'canceled')) AS closed
		FROM items c JOIN statuses cs ON cs.id = c.status_id
		WHERE c.parent_id = i.id AND c.deleted_at IS NULL) sub ON true
	LEFT JOIN approval_requests ar ON ar.item_id = i.id AND ar.state = 'pending'
	LEFT JOIN statuses ars ON ars.id = ar.to_status_id`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var projectKey string
	var parentNumber *int64
	var parentTitle *string
	var apID, apTo, apBy *uuid.UUID
	var apToName, apToCat *string
	var apCount, apMin *int
	var apDue *string
	var apOverdue bool
	err := row.Scan(&it.ID, &it.Number, &projectKey, &it.ProjectID, &it.Title, &it.Description, &it.AssigneeID,
		&it.ReporterID, &it.Fields, &it.Version, &it.CreatedAt, &it.UpdatedAt,
		&it.Type.ID, &it.Type.Key, &it.Type.Name, &it.Status.ID, &it.Status.Name, &it.Status.Category, &it.DueDate, &it.Overdue,
		&parentNumber, &parentTitle, &it.Subtasks.Count, &it.Subtasks.Done,
		&apID, &apTo, &apToName, &apToCat, &apBy, &apCount, &apMin, &apDue, &apOverdue)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if apID != nil && apBy != nil && apCount != nil && apMin != nil {
		it.Approval = &PendingApproval{ID: *apID, RequestedBy: *apBy, Approvals: *apCount, MinApprovals: *apMin,
			DueOn: apDue, Overdue: apOverdue}
		if apTo != nil && apToName != nil && apToCat != nil {
			it.Approval.ToStatus = Status{ID: *apTo, Name: *apToName, Category: *apToCat}
		}
	}
	it.Key = itemKey(projectKey, it.Number)
	if parentNumber != nil && parentTitle != nil {
		it.Parent = &ItemParent{Key: itemKey(projectKey, *parentNumber), Number: *parentNumber, Title: *parentTitle}
	}
	return it, err
}

// CreateItemParams are the inputs for CreateItem.
type CreateItemParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	ActorID     uuid.UUID
	Title       string
	Description *string
	TypeKey     string // empty: "task", else the first type
	StatusID    *uuid.UUID
	AssigneeID  *uuid.UUID
	DueDate     *string // YYYY-MM-DD
	Fields      map[string]json.RawMessage
	// ParentNumber makes the new item a subtask of that item in the same project.
	ParentNumber *int64
}

// CreateItem allocates the next project number and inserts the item atomically.
func (s *Store) CreateItem(ctx context.Context, p CreateItemParams) (Item, error) {
	var item Item
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		item, err = createItemTx(ctx, tx, p, nil)
		return err
	})
	return item, err
}

// createItemTx validates and inserts an item and records item.created, inside
// the caller's transaction. A zero ActorID creates it as the system (no
// reporter). extra is merged into the event payload.
func createItemTx(ctx context.Context, tx pgx.Tx, p CreateItemParams, extra map[string]any) (Item, error) {
	cache := newItemCache(p.WorkspaceID, nil)
	prep, err := prepareItem(ctx, tx, cache, p)
	if err != nil {
		return Item{}, err
	}
	if prep.fields, err = cache.withFormulas(ctx, tx, prep.fields, prep.due); err != nil {
		return Item{}, err
	}

	// Row lock on the project serializes numbering: no gaps from races, no duplicates.
	var parentID *uuid.UUID
	if p.ParentNumber != nil {
		ref, err := lockParent(ctx, tx, p.WorkspaceID, p.ProjectID, *p.ParentNumber)
		if err != nil {
			return Item{}, err
		}
		parentID = &ref.id
	}

	var number int64
	err = tx.QueryRow(ctx, `UPDATE projects SET next_item_number = next_item_number + 1
		WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
		RETURNING next_item_number - 1`, p.ProjectID, p.WorkspaceID).Scan(&number)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}

	var reporter *uuid.UUID
	if p.ActorID != uuid.Nil {
		reporter = &p.ActorID
	}
	id := newID()
	if _, err := tx.Exec(ctx, insertItemSQL,
		id, p.WorkspaceID, p.ProjectID, prep.typ.ID, prep.status.ID, number, prep.title, prep.desc,
		prep.assignee, reporter, prep.fields, prep.due); err != nil {
		return Item{}, err
	}
	if parentID != nil {
		// A separate statement keeps insertItemSQL shared with imports, which
		// never set a parent.
		if _, err := tx.Exec(ctx, `UPDATE items SET parent_id = $1 WHERE id = $2`, *parentID, id); err != nil {
			return Item{}, parentError(err)
		}
	}
	item, err := scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, id))
	if err != nil {
		return Item{}, err
	}
	// The reporter and the assignee watch the item (ADR-0028).
	if err := autoWatch(ctx, tx, p.WorkspaceID, id, reporter, item.AssigneeID); err != nil {
		return Item{}, err
	}
	payload := createdPayload(item)
	for k, v := range extra {
		payload[k] = v
	}
	return item, appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", id, "item.created", payload)
}

const insertItemSQL = `INSERT INTO items
	(id, workspace_id, project_id, item_type_id, status_id, number, title, description, assignee_id, reporter_id, fields, due_date)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::date)`

func resolveStatus(ctx context.Context, q querier, workflowID uuid.UUID, want *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	var err error
	if want != nil {
		err = q.QueryRow(ctx, `SELECT id FROM statuses WHERE id = $1 AND workflow_id = $2`, *want, workflowID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, invalid("status_id", "status does not belong to this item type's workflow")
		}
		return id, err
	}
	err = q.QueryRow(ctx, `SELECT id FROM statuses WHERE workflow_id = $1
		ORDER BY is_initial DESC, position LIMIT 1`, workflowID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, invalid("status_id", "workflow has no statuses")
	}
	return id, err
}

// requireProjectAccess: an assignee must be able to see the item's project
// (ADR-0013), otherwise notifications would leak a private project's items.
func requireProjectAccess(ctx context.Context, q querier, wsID, projectID, userID uuid.UUID) error {
	ok, err := canSeeProject(ctx, q, wsID, projectID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return invalid("assignee_id", "user is not a member of this workspace or cannot see this project")
	}
	return nil
}

// GetItem returns a live item by its project number.
func (s *Store) GetItem(ctx context.Context, wsID, projectID uuid.UUID, number int64) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, itemSelect+`
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL`,
		wsID, projectID, number))
}

// ListItemsParams filter and paginate ListItems.
type ListItemsParams struct {
	WorkspaceID  uuid.UUID
	ProjectID    uuid.UUID
	StatusIDs    []uuid.UUID
	AssigneeID   *uuid.UUID
	Unassigned   bool
	Query        string
	FieldFilters map[string]string // ?field.<key>=<value>, exact match
	Due          string            // "", "overdue", "today", "week" (next 7 days) or "none"
	DueFrom      *string           // due_date >= this YYYY-MM-DD (calendar range, inclusive)
	DueTo        *string           // due_date <= this YYYY-MM-DD (inclusive)
	Sort         string            // SortNumber (default) or SortDue, see itemsort.go
	ParentNumber *int64            // only subtasks of this item
	TopLevel     bool              // only items without a parent
	Approval     string            // "", "pending" (any open request) or "mine" (Viewer may decide), ADR-0026
	// Viewer, when set, fills Item.Blocked for the page (one batched query)
	// and is required by the Blocked and Approval == "mine" filters:
	// blockers in projects the viewer cannot see never count (ADR-0013).
	Viewer  *Viewer
	Blocked bool // only items with an open, visible blocker
	Limit   int
	Cursor  *ItemCursor // continue after this position (nil = from the start)
}

// ListItems returns one page of items in the requested order using keyset
// pagination, which stays fast at any depth (OFFSET would scan every skipped
// row). next is nil on the last page.
func (s *Store) ListItems(ctx context.Context, p ListItemsParams) (items []Item, next *ItemCursor, err error) {
	where, args, err := s.itemFilter(ctx, p, true)
	if err != nil {
		return nil, nil, err
	}
	fsort, err := resolveFieldSort(ctx, s.pool, p.WorkspaceID, p.Sort, "sort")
	if err != nil {
		return nil, nil, err
	}
	if fsort != nil && p.Cursor != nil && p.Cursor.Value != nil {
		if err := checkCursorValue(fsort, *p.Cursor.Value); err != nil {
			return nil, nil, err
		}
	}
	order, where, args, err := sortAndSeek(p, fsort, where, args)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, p.Limit+1)
	sql := fmt.Sprintf("%s WHERE %s ORDER BY %s LIMIT $%d", itemSelect, strings.Join(where, " AND "), order, len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	items = []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(items) > p.Limit {
		items = items[:p.Limit]
		next = cursorAfter(p.Sort, items[len(items)-1])
		if fsort != nil {
			// The exact sort value as SQL sees it (a float64 copy could
			// differ in the last digit and skip or repeat rows).
			if err := s.pool.QueryRow(ctx, "SELECT ("+fsort.expr("$2")+")::text FROM items i WHERE i.id = $1",
				items[len(items)-1].ID, fsort.key).Scan(&next.Value); err != nil {
				return nil, nil, err
			}
		}
	}
	if p.Viewer != nil {
		ids := make([]uuid.UUID, len(items))
		for i, it := range items {
			ids[i] = it.ID
		}
		blocked, err := blockedItems(ctx, s.pool, p.WorkspaceID, ids, *p.Viewer)
		if err != nil {
			return nil, nil, err
		}
		for i := range items {
			b := blocked[items[i].ID]
			items[i].Blocked = &b
		}
	}
	return items, next, nil
}

// CountItemsByStatus counts the items matching the same filters as ListItems,
// per status. Status filters, cursor and limit are ignored: a board needs the
// size of every column, whatever is currently loaded.
func (s *Store) CountItemsByStatus(ctx context.Context, p ListItemsParams) (map[uuid.UUID]int64, error) {
	where, args, err := s.itemFilter(ctx, p, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT i.status_id, count(*) FROM items i WHERE "+strings.Join(where, " AND ")+" GROUP BY i.status_id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]int64{}
	for rows.Next() {
		var id uuid.UUID
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// CountItemsUpTo counts the items ListItems would return over all pages
// (every filter applies, cursor and limit do not), stopping at limit+1 so a
// huge project costs no more than the cap (table export, ADR-0040).
func (s *Store) CountItemsUpTo(ctx context.Context, p ListItemsParams, limit int) (int, error) {
	where, args, err := s.itemFilter(ctx, p, true)
	if err != nil {
		return 0, err
	}
	args = append(args, limit+1)
	var n int
	err = s.pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM (SELECT 1 FROM items i WHERE %s LIMIT $%d) c",
		strings.Join(where, " AND "), len(args)), args...).Scan(&n)
	return n, err
}

// RecordItemsExport appends items.exported on the project: who exported how
// many rows, in which format, with which filters. Never cell data.
func (s *Store) RecordItemsExport(ctx context.Context, wsID, actorID, projectID uuid.UUID, payload map[string]any) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "items.exported", payload)
	})
}

// itemFilter builds the WHERE conditions shared by listing and counting.
// Values are always bound as parameters, never interpolated.
func (s *Store) itemFilter(ctx context.Context, p ListItemsParams, withStatus bool) (where []string, args []any, err error) {
	where = []string{"i.workspace_id = $1", "i.project_id = $2", "i.deleted_at IS NULL"}
	args = []any{p.WorkspaceID, p.ProjectID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if withStatus && len(p.StatusIDs) > 0 {
		ids := make([]string, len(p.StatusIDs))
		for i, id := range p.StatusIDs {
			ids[i] = id.String()
		}
		add("i.status_id = ANY(?::uuid[])", ids)
	}
	if p.AssigneeID != nil {
		add("i.assignee_id = ?", *p.AssigneeID)
	}
	if p.Unassigned {
		where = append(where, "i.assignee_id IS NULL")
	}
	if p.ParentNumber != nil {
		add(`i.parent_id = (SELECT id FROM items
			WHERE workspace_id = $1 AND project_id = $2 AND number = ? AND deleted_at IS NULL)`, *p.ParentNumber)
	}
	if p.TopLevel {
		where = append(where, "i.parent_id IS NULL")
	}
	switch p.Approval {
	case "":
	case "pending":
		where = append(where, "EXISTS (SELECT 1 FROM approval_requests far WHERE far.item_id = i.id AND far.state = 'pending')")
	case "mine":
		if p.Viewer == nil {
			return nil, nil, errors.New("approval filter mine needs a viewer")
		}
		// Same eligibility as eligibleApproversSQL, minus project access: the
		// list is already limited to a project the viewer can see.
		args = append(args, p.Viewer.UserID, p.Viewer.Role)
		u, r := "$"+strconv.Itoa(len(args)-1), "$"+strconv.Itoa(len(args))
		where = append(where, `EXISTS (SELECT 1 FROM approval_requests far WHERE far.item_id = i.id AND far.state = 'pending'
			AND far.requested_by <> `+u+` AND `+r+`::text <> 'guest'
			AND (`+r+`::text = ANY(far.approver_roles) OR `+u+`::uuid = ANY(far.approver_user_ids))
			AND NOT EXISTS (SELECT 1 FROM approval_decisions fd WHERE fd.request_id = far.id
			    AND (fd.user_id = `+u+` OR fd.on_behalf_of = `+u+`)))`)
	default:
		return nil, nil, invalid("approval", "must be pending or mine")
	}
	if p.Blocked {
		if p.Viewer == nil {
			return nil, nil, errors.New("blocked filter needs a viewer")
		}
		args = append(args, p.Viewer.UserID, RoleAtLeast(p.Viewer.Role, RoleAdmin))
		where = append(where, blockedFilter("$"+strconv.Itoa(len(args)-1), "$"+strconv.Itoa(len(args))+"::boolean"))
	}
	if len(p.FieldFilters) > 0 {
		conds, err := fieldFilterConditions(ctx, s.pool, p.WorkspaceID, p.FieldFilters)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range conds {
			add("i.fields @> ?::jsonb", c)
		}
	}
	if q := strings.TrimSpace(p.Query); q != "" {
		add("i.search @@ websearch_to_tsquery('simple', ?)", q)
	}
	// "Today" is the workspace's local date ($1 is the workspace id).
	const today = "(now() AT TIME ZONE (SELECT timezone FROM workspaces WHERE id = $1))::date"
	switch p.Due {
	case "":
	case "overdue":
		where = append(where, "i.due_date < "+today,
			"i.status_id IN (SELECT id FROM statuses WHERE workspace_id = $1 AND category IN ('todo', 'in_progress'))")
	case "today":
		where = append(where, "i.due_date = "+today)
	case "week":
		where = append(where, "i.due_date BETWEEN "+today+" AND "+today+" + 6")
	case "none":
		where = append(where, "i.due_date IS NULL")
	default:
		return nil, nil, invalid("due", "must be overdue, today, week or none")
	}
	from, to, err := DueRange(p.DueFrom, p.DueTo)
	if err != nil {
		return nil, nil, err
	}
	if from != nil {
		add("i.due_date >= ?::date", *from)
	}
	if to != nil {
		add("i.due_date <= ?::date", *to)
	}
	return where, args, nil
}

// UpdateItemParams is a partial update; unset fields are left unchanged.
type UpdateItemParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	Number      int64
	ActorID     uuid.UUID
	Version     int // must equal the current version (optimistic locking)
	Title       opt.Field[string]
	Description opt.Field[string]
	StatusID    opt.Field[uuid.UUID]
	AssigneeID  opt.Field[uuid.UUID]
	DueDate     opt.Field[string] // YYYY-MM-DD; null clears it
	Parent      opt.Field[int64]  // parent item number; null makes it top-level
	// Fields is merged: listed keys are set (or cleared with null), others kept.
	Fields opt.Field[map[string]json.RawMessage]
	// approvalApply is the approval request whose final approval applies
	// this change (ADR-0026). Only DecideApproval sets it; it is the one way
	// past the approval guard and lock.
	approvalApply uuid.UUID
}

// UpdateItem applies a partial update if Version is current, recording a diff event.
func (s *Store) UpdateItem(ctx context.Context, p UpdateItemParams) (Item, error) {
	in, err := checkUpdate(p)
	if err != nil {
		return Item{}, err
	}
	var item Item
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		item, _, err = updateItemTx(ctx, tx, p, in, nil)
		return err
	})
	return item, err
}

// checkedUpdate holds the normalized scalar inputs of an update.
type checkedUpdate struct {
	title string
	desc  *string
	due   *string
}

// checkUpdate validates the parts of an update that need no database.
func checkUpdate(p UpdateItemParams) (checkedUpdate, error) {
	var title string
	var err error
	if p.Title.Set {
		if p.Title.Null {
			return checkedUpdate{}, invalid("title", "cannot be null")
		}
		if title, err = text("title", p.Title.Value, 1, maxTitleLen); err != nil {
			return checkedUpdate{}, err
		}
	}
	var desc *string
	if p.Description.Set {
		if desc, err = optionalText("description", p.Description.Ptr(), maxDescriptionLen); err != nil {
			return checkedUpdate{}, err
		}
	}
	var due *string
	if p.DueDate.Set {
		if due, err = dueDate(p.DueDate.Ptr()); err != nil {
			return checkedUpdate{}, err
		}
	}
	if p.StatusID.Set && p.StatusID.Null {
		return checkedUpdate{}, invalid("status_id", "cannot be null")
	}
	if p.Fields.Set && p.Fields.Null {
		return checkedUpdate{}, invalid("fields", "cannot be null (send {\"key\": null} to clear one field)")
	}
	return checkedUpdate{title: title, desc: desc, due: due}, nil
}

// updateItemTx applies a checked update inside the caller's transaction and
// records item.updated (extra is merged into its payload). changed is false
// when the update was a no-op: then no event is written.
func updateItemTx(ctx context.Context, tx pgx.Tx, p UpdateItemParams, in checkedUpdate, extra map[string]any) (item Item, changed bool, err error) {
	title, desc, due := in.title, in.desc, in.due
	err = func() error {
		var (
			id                    uuid.UUID
			version               int
			curTitle              string
			curDesc               *string
			curStatus, workflowID uuid.UUID
			curAssignee           *uuid.UUID
			curFields             map[string]any
			curDue                *string
			typeID                uuid.UUID
			typeKey               string
			curParent             *uuid.UUID
			curParentNumber       *int64
			projectKey            string
		)
		err := tx.QueryRow(ctx, `SELECT i.id, i.version, i.title, i.description, i.status_id, i.assignee_id, i.fields,
				i.due_date::text, t.workflow_id, t.id, t.key, i.parent_id, par.number, p.key
			FROM items i JOIN item_types t ON t.id = i.item_type_id
			JOIN projects p ON p.id = i.project_id
			LEFT JOIN items par ON par.id = i.parent_id
			WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL
			FOR UPDATE OF i`, p.WorkspaceID, p.ProjectID, p.Number).
			Scan(&id, &version, &curTitle, &curDesc, &curStatus, &curAssignee, &curFields, &curDue, &workflowID, &typeID, &typeKey,
				&curParent, &curParentNumber, &projectKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if version != p.Version {
			return &ConflictError{Code: "version_conflict",
				Message: fmt.Sprintf("item was modified by someone else (current version %d); reload and retry", version)}
		}
		pending, err := pendingApprovalID(ctx, tx, id)
		if err != nil {
			return err
		}
		applying := pending != nil && *pending == p.approvalApply

		changes := map[string]any{}
		newTitle, newDesc, newStatus, newAssignee, newDue := curTitle, curDesc, curStatus, curAssignee, curDue
		newParent := curParent
		if p.Parent.Set {
			var to *int64
			newParent = nil
			if !p.Parent.Null {
				if p.Parent.Value == p.Number {
					return invalid("parent", "an item cannot be its own parent")
				}
				ref, err := lockParent(ctx, tx, p.WorkspaceID, p.ProjectID, p.Parent.Value)
				if err != nil {
					return err
				}
				if !equalPtr(curParent, &ref.id) {
					if err := requireNoSubtasks(ctx, tx, id); err != nil {
						return err
					}
				}
				newParent, to = &ref.id, &p.Parent.Value
			}
			if !equalPtr(curParent, newParent) {
				changes["parent"] = diff(keyOrNil(projectKey, curParentNumber), keyOrNil(projectKey, to))
			}
		}
		if p.DueDate.Set && !equalPtr(curDue, due) {
			changes["due_date"] = diff(curDue, due)
			newDue = due
		}
		if p.Title.Set && title != curTitle {
			changes["title"] = diff(curTitle, title)
			newTitle = title
		}
		if p.Description.Set && !equalPtr(curDesc, desc) {
			changes["description"] = diff(curDesc, desc)
			newDesc = desc
		}
		if p.StatusID.Set && p.StatusID.Value != curStatus {
			if _, err := resolveStatus(ctx, tx, workflowID, &p.StatusID.Value); err != nil {
				return err
			}
			if err := checkTransition(ctx, tx, workflowID, curStatus, p.StatusID.Value); err != nil {
				return err
			}
			if pending == nil {
				guarded, err := statusGuarded(ctx, tx, p.StatusID.Value)
				if err != nil {
					return err
				}
				if guarded {
					return errApprovalRequired()
				}
			}
			changes["status_id"] = diff(curStatus, p.StatusID.Value)
			newStatus = p.StatusID.Value
		}
		if p.AssigneeID.Set {
			a := p.AssigneeID.Ptr()
			if a != nil {
				if err := requireProjectAccess(ctx, tx, p.WorkspaceID, p.ProjectID, *a); err != nil {
					return err
				}
			}
			if !equalPtr(curAssignee, a) {
				changes["assignee_id"] = diff(curAssignee, a)
				newAssignee = a
			}
		}

		newFields := curFields
		if newFields == nil {
			newFields = map[string]any{} // column is NOT NULL
		}
		if p.Fields.Set && len(p.Fields.Value) > 0 {
			schema, err := loadFieldSchema(ctx, tx, p.WorkspaceID, typeID, fieldKeys(p.Fields.Value))
			if err != nil {
				return err
			}
			next, fieldChanges, err := applyFields(ctx, tx, p.WorkspaceID, schema, typeKey, newFields, p.Fields.Value, false)
			if err != nil {
				return err
			}
			newFields = next
			maps.Copy(changes, fieldChanges)
		}
		if pending != nil && !applying && lockedChange(changes) {
			return errApprovalPending()
		}
		// Changing approved content sends the item back for a new approval
		// (ADR-0034). A change that also moves the status leaves the
		// approved status anyway, so its approval no longer covers anything.
		var reopen *reopening
		if _, moving := changes["status_id"]; pending == nil && !moving && len(changes) > 0 {
			if reopen, err = planReapproval(ctx, tx, p, id, curStatus, workflowID, changes, extra); err != nil {
				return err
			}
			if reopen != nil {
				changes["status_id"] = diff(curStatus, reopen.back)
				newStatus = reopen.back
			}
		}
		// Formula values follow every input change, the due date included
		// (ADR-0030). They are not listed in changes: the inputs are.
		formulas, err := loadFormulaSet(ctx, tx, p.WorkspaceID, true)
		if err != nil {
			return err
		}
		computed, err := formulas.apply(ctx, tx, newFields, newDue)
		if err != nil {
			return err
		}
		stale := formulas.formulaValuesDiffer(newFields, computed)
		newFields = computed

		if len(changes) == 0 && stale {
			// Nothing the caller asked for changed, but a stored value was
			// out of date (e.g. a recompute pass skipped this locked row).
			if err := writeComputedFields(ctx, tx, id, newFields); err != nil {
				return err
			}
		}
		var reopenEvents func() error
		if reopen != nil {
			var reqID uuid.UUID
			if reqID, reopenEvents, err = reopen.apply(ctx, tx, p.WorkspaceID, p.ActorID); err != nil {
				return err
			}
			extra = maps.Clone(extra)
			if extra == nil {
				extra = map[string]any{}
			}
			extra["approval"] = map[string]any{"invalidated_request_id": reopen.old.id, "request_id": reqID}
		}
		if len(changes) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE items SET title = $1, description = $2, status_id = $3,
				assignee_id = $4, fields = $5, due_date = $6::date, parent_id = $8, version = version + 1 WHERE id = $7`,
				newTitle, newDesc, newStatus, newAssignee, newFields, newDue, id, newParent); err != nil {
				return parentError(err)
			}
		}
		item, err = scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, id))
		if err != nil || len(changes) == 0 {
			return err
		}
		changed = true
		if _, ok := changes["assignee_id"]; ok {
			if err := autoWatch(ctx, tx, p.WorkspaceID, id, newAssignee); err != nil {
				return err
			}
		}
		payload := map[string]any{"changes": changes, "snapshot": Snapshot(item)}
		maps.Copy(payload, extra)
		if err := appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", id, "item.updated", payload); err != nil {
			return err
		}
		if reopenEvents != nil {
			return reopenEvents()
		}
		return nil
	}()
	return item, changed, err
}

// Snapshot is the item's state as of an event, in the flat shape automation
// conditions read ("type", "status_category", "fields.<key>", ...). Storing it
// in the event makes rule evaluation independent of later changes, including
// other rules running on the same event.
func Snapshot(it Item) map[string]any {
	v := map[string]any{
		"type":            it.Type.Key,
		"project":         it.Key[:strings.LastIndexByte(it.Key, '-')],
		"status":          it.Status.ID.String(),
		"status_category": it.Status.Category,
		"title":           it.Title,
		"assignee":        nil,
		"reporter":        nil,
		"due_date":        nil,
		"parent":          nil,
	}
	if it.Parent != nil {
		v["parent"] = it.Parent.Key
	}
	if it.DueDate != nil {
		v["due_date"] = *it.DueDate
	}
	if it.AssigneeID != nil {
		v["assignee"] = it.AssigneeID.String()
	}
	if it.ReporterID != nil {
		v["reporter"] = it.ReporterID.String()
	}
	for k, fv := range it.Fields {
		v["fields."+k] = fv
	}
	return v
}

// DeleteItem soft-deletes an item; history and events are preserved.
func (s *Store) DeleteItem(ctx context.Context, wsID, projectID uuid.UUID, number int64, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return deleteItemTx(ctx, tx, wsID, projectID, number, nil, actorID, nil)
	})
}

// deleteItemTx soft-deletes one item inside the caller's transaction. A
// non-nil version must match the current one (bulk deletes are checked like
// edits, so a stale selection never deletes something changed meanwhile).
func deleteItemTx(ctx context.Context, tx pgx.Tx, wsID, projectID uuid.UUID, number int64, version *int, actorID uuid.UUID, extra map[string]any) error {
	var id uuid.UUID
	var cur int
	err := tx.QueryRow(ctx, `SELECT id, version FROM items
		WHERE workspace_id = $1 AND project_id = $2 AND number = $3 AND deleted_at IS NULL
		FOR UPDATE`, wsID, projectID, number).Scan(&id, &cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if version != nil && *version != cur {
		return &ConflictError{Code: "version_conflict",
			Message: fmt.Sprintf("item was modified by someone else (current version %d); reload and retry", cur)}
	}
	if pending, err := pendingApprovalID(ctx, tx, id); err != nil {
		return err
	} else if pending != nil {
		return &ConflictError{Code: "approval_pending", Message: "this item is waiting for approval; cancel the request before deleting it"}
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET deleted_at = now(), version = version + 1 WHERE id = $1`, id); err != nil {
		return err
	}
	if err := appendEvent(ctx, tx, wsID, actorID, "item", id, "item.deleted", extra); err != nil {
		return err
	}
	// A deleted item's links go with it (ADR-0028); the other side's history
	// records link.removed.
	if err := removeItemLinks(ctx, tx, wsID, actorID, id); err != nil {
		return err
	}
	// Subtasks of a deleted item become top-level (ADR-0021), for single and
	// bulk deletes alike. The FOR UPDATE above orders this against a
	// concurrent "add subtask" (items_check_parent locks the parent FOR SHARE).
	return detachSubtasks(ctx, tx, wsID, actorID, id)
}

func diff(from, to any) map[string]any { return map[string]any{"from": from, "to": to} }

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// dueDate validates an optional YYYY-MM-DD date (a calendar date, no time).
func dueDate(v *string) (*string, error) {
	if v == nil {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(*v))
	if err != nil || d.Year() < 1900 || d.Year() > 2200 {
		return nil, invalid("due_date", "must be a date YYYY-MM-DD between 1900 and 2200")
	}
	out := d.Format(time.DateOnly)
	return &out, nil
}

// MaxDueRangeDays bounds a due_from..due_to window: a calendar shows at most
// six weeks, an agenda a few months; a year is plenty and keeps the range
// scan on items_project_due_number_idx small.
const MaxDueRangeDays = 366

// DueRange validates an optional inclusive date window (calendar view).
func DueRange(from, to *string) (*string, *string, error) {
	f, err := dueDateField("due_from", from)
	if err != nil {
		return nil, nil, err
	}
	t, err := dueDateField("due_to", to)
	if err != nil {
		return nil, nil, err
	}
	if f != nil && t != nil {
		df, _ := time.Parse(time.DateOnly, *f)
		dt, _ := time.Parse(time.DateOnly, *t)
		if dt.Before(df) {
			return nil, nil, invalid("due_to", "must not be before due_from")
		}
		if dt.Sub(df) > (MaxDueRangeDays-1)*24*time.Hour {
			return nil, nil, invalid("due_to", "range must be at most %d days", MaxDueRangeDays)
		}
	}
	return f, t, nil
}

func dueDateField(field string, v *string) (*string, error) {
	out, err := dueDate(v)
	var ve *ValidationError
	if errors.As(err, &ve) {
		ve.Field = field
	}
	return out, err
}
