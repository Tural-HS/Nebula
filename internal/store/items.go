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

	"github.com/nebula/nebula/internal/opt"
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
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

const (
	maxTitleLen       = 500
	maxDescriptionLen = 100_000
)

const itemSelect = `SELECT i.id, i.number, p.key, i.project_id, i.title, i.description, i.assignee_id,
	i.reporter_id, i.fields, i.version, i.created_at, i.updated_at,
	t.id, t.key, t.name, s.id, s.name, s.category
	FROM items i
	JOIN projects   p ON p.id = i.project_id
	JOIN item_types t ON t.id = i.item_type_id
	JOIN statuses   s ON s.id = i.status_id`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var projectKey string
	err := row.Scan(&it.ID, &it.Number, &projectKey, &it.ProjectID, &it.Title, &it.Description, &it.AssigneeID,
		&it.ReporterID, &it.Fields, &it.Version, &it.CreatedAt, &it.UpdatedAt,
		&it.Type.ID, &it.Type.Key, &it.Type.Name, &it.Status.ID, &it.Status.Name, &it.Status.Category)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	it.Key = projectKey + "-" + strconv.FormatInt(it.Number, 10)
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
	Fields      map[string]json.RawMessage
}

// CreateItem allocates the next project number and inserts the item atomically.
func (s *Store) CreateItem(ctx context.Context, p CreateItemParams) (Item, error) {
	title, err := text("title", p.Title, 1, maxTitleLen)
	if err != nil {
		return Item{}, err
	}
	desc, err := optionalText("description", p.Description, maxDescriptionLen)
	if err != nil {
		return Item{}, err
	}
	var item Item
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var typeID, workflowID uuid.UUID
		var typeKey string
		err := tx.QueryRow(ctx, `SELECT id, key, workflow_id FROM item_types
			WHERE workspace_id = $1 AND archived_at IS NULL AND ($2 = '' OR key = $2)
			ORDER BY (key = 'task') DESC, created_at LIMIT 1`, p.WorkspaceID, p.TypeKey).Scan(&typeID, &typeKey, &workflowID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("type", "unknown item type")
		}
		if err != nil {
			return err
		}

		schema, err := loadFieldSchema(ctx, tx, p.WorkspaceID, typeID, fieldKeys(p.Fields))
		if err != nil {
			return err
		}
		fields, _, err := applyFields(ctx, tx, p.WorkspaceID, schema, typeKey, map[string]any{}, p.Fields, true)
		if err != nil {
			return err
		}

		statusID, err := resolveStatus(ctx, tx, workflowID, p.StatusID)
		if err != nil {
			return err
		}
		if p.AssigneeID != nil {
			if err := requireMember(ctx, tx, p.WorkspaceID, *p.AssigneeID); err != nil {
				return err
			}
		}

		// Row lock on the project serializes numbering: no gaps from races, no duplicates.
		var number int64
		err = tx.QueryRow(ctx, `UPDATE projects SET next_item_number = next_item_number + 1
			WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
			RETURNING next_item_number - 1`, p.ProjectID, p.WorkspaceID).Scan(&number)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		id := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO items
			(id, workspace_id, project_id, item_type_id, status_id, number, title, description, assignee_id, reporter_id, fields)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, p.WorkspaceID, p.ProjectID, typeID, statusID, number, title, desc, p.AssigneeID, p.ActorID, fields); err != nil {
			return err
		}
		item, err = scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, id))
		if err != nil {
			return err
		}
		payload := map[string]any{"key": item.Key, "title": title}
		if len(fields) > 0 {
			payload["fields"] = fields
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", id, "item.created", payload)
	})
	return item, err
}

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

func requireMember(ctx context.Context, q querier, wsID, userID uuid.UUID) error {
	ok, err := isMember(ctx, q, wsID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return invalid("assignee_id", "user is not a member of this workspace")
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
	Limit        int
	BeforeNumber int64 // cursor: return items with number < BeforeNumber (0 = from newest)
}

// ListItems returns items newest-first using keyset pagination, which stays
// fast at any depth (OFFSET would scan every skipped row).
func (s *Store) ListItems(ctx context.Context, p ListItemsParams) (items []Item, nextBefore int64, err error) {
	where := []string{"i.workspace_id = $1", "i.project_id = $2", "i.deleted_at IS NULL"}
	args := []any{p.WorkspaceID, p.ProjectID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if len(p.StatusIDs) > 0 {
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
	if len(p.FieldFilters) > 0 {
		conds, err := fieldFilterConditions(ctx, s.pool, p.WorkspaceID, p.FieldFilters)
		if err != nil {
			return nil, 0, err
		}
		for _, c := range conds {
			add("i.fields @> ?::jsonb", c)
		}
	}
	if q := strings.TrimSpace(p.Query); q != "" {
		add("i.search @@ websearch_to_tsquery('simple', ?)", q)
	}
	if p.BeforeNumber > 0 {
		add("i.number < ?", p.BeforeNumber)
	}
	args = append(args, p.Limit+1)
	sql := fmt.Sprintf("%s WHERE %s ORDER BY i.number DESC LIMIT $%d", itemSelect, strings.Join(where, " AND "), len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items = []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(items) > p.Limit {
		items = items[:p.Limit]
		nextBefore = items[len(items)-1].Number
	}
	return items, nextBefore, nil
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
	// Fields is merged: listed keys are set (or cleared with null), others kept.
	Fields opt.Field[map[string]json.RawMessage]
}

// UpdateItem applies a partial update if Version is current, recording a diff event.
func (s *Store) UpdateItem(ctx context.Context, p UpdateItemParams) (Item, error) {
	var title string
	var err error
	if p.Title.Set {
		if p.Title.Null {
			return Item{}, invalid("title", "cannot be null")
		}
		if title, err = text("title", p.Title.Value, 1, maxTitleLen); err != nil {
			return Item{}, err
		}
	}
	var desc *string
	if p.Description.Set {
		if desc, err = optionalText("description", p.Description.Ptr(), maxDescriptionLen); err != nil {
			return Item{}, err
		}
	}
	if p.StatusID.Set && p.StatusID.Null {
		return Item{}, invalid("status_id", "cannot be null")
	}
	if p.Fields.Set && p.Fields.Null {
		return Item{}, invalid("fields", "cannot be null (send {\"key\": null} to clear one field)")
	}

	var item Item
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var (
			id                    uuid.UUID
			version               int
			curTitle              string
			curDesc               *string
			curStatus, workflowID uuid.UUID
			curAssignee           *uuid.UUID
			curFields             map[string]any
			typeID                uuid.UUID
			typeKey               string
		)
		err := tx.QueryRow(ctx, `SELECT i.id, i.version, i.title, i.description, i.status_id, i.assignee_id, i.fields,
				t.workflow_id, t.id, t.key
			FROM items i JOIN item_types t ON t.id = i.item_type_id
			WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL
			FOR UPDATE OF i`, p.WorkspaceID, p.ProjectID, p.Number).
			Scan(&id, &version, &curTitle, &curDesc, &curStatus, &curAssignee, &curFields, &workflowID, &typeID, &typeKey)
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

		changes := map[string]any{}
		newTitle, newDesc, newStatus, newAssignee := curTitle, curDesc, curStatus, curAssignee
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
			changes["status_id"] = diff(curStatus, p.StatusID.Value)
			newStatus = p.StatusID.Value
		}
		if p.AssigneeID.Set {
			a := p.AssigneeID.Ptr()
			if a != nil {
				if err := requireMember(ctx, tx, p.WorkspaceID, *a); err != nil {
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

		if len(changes) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE items SET title = $1, description = $2, status_id = $3,
				assignee_id = $4, fields = $5, version = version + 1 WHERE id = $6`,
				newTitle, newDesc, newStatus, newAssignee, newFields, id); err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "item", id, "item.updated",
				map[string]any{"changes": changes}); err != nil {
				return err
			}
		}
		item, err = scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, id))
		return err
	})
	return item, err
}

// DeleteItem soft-deletes an item; history and events are preserved.
func (s *Store) DeleteItem(ctx context.Context, wsID, projectID uuid.UUID, number int64, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `UPDATE items SET deleted_at = now(), version = version + 1
			WHERE workspace_id = $1 AND project_id = $2 AND number = $3 AND deleted_at IS NULL
			RETURNING id`, wsID, projectID, number).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item", id, "item.deleted", nil)
	})
}

func diff(from, to any) map[string]any { return map[string]any{"from": from, "to": to} }

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
