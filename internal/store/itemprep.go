package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// itemTypeRow is an item type as needed to create items of it.
type itemTypeRow struct {
	ItemTypeRef
	workflowID uuid.UUID
}

// preparedItem is a validated CreateItemParams, ready to insert.
type preparedItem struct {
	typ      itemTypeRow
	status   Status
	title    string
	desc     *string
	assignee *uuid.UUID
	fields   map[string]any
	due      *string // YYYY-MM-DD
}

// itemCache memoizes the workspace configuration that item creation reads, so
// an import of thousands of rows validates each row exactly like CreateItem
// without repeating the same lookups. Within one transaction the cached data
// cannot go stale in a way that matters: config changes made concurrently are
// not visible to the transaction anyway.
type itemCache struct {
	wsID      uuid.UUID
	projectID uuid.UUID // set by prepareItem; assignees must be able to see it
	fieldKeys []string  // nil: load the field schema per item for its own keys
	types     map[string]itemTypeRow
	schemas   map[uuid.UUID]map[string]schemaField
	statuses  map[uuid.UUID][]Status // per workflow; first = initial status
	guarded   map[uuid.UUID]bool     // statuses that need approval to enter (ADR-0026)
	members   map[uuid.UUID]bool
}

func newItemCache(wsID uuid.UUID, fieldKeys []string) *itemCache {
	if fieldKeys != nil {
		fieldKeys = slices.Sorted(slices.Values(fieldKeys))
	}
	return &itemCache{wsID: wsID, fieldKeys: fieldKeys, types: map[string]itemTypeRow{},
		schemas: map[uuid.UUID]map[string]schemaField{}, statuses: map[uuid.UUID][]Status{}, guarded: map[uuid.UUID]bool{},
		members: map[uuid.UUID]bool{}}
}

// prepareItem validates a new item: title and description, item type, custom
// fields (types, choices, required fields), status within the type's
// workflow and assignee membership. It only reads.
func prepareItem(ctx context.Context, q querier, c *itemCache, p CreateItemParams) (preparedItem, error) {
	if c.projectID != p.ProjectID {
		c.projectID, c.members = p.ProjectID, map[uuid.UUID]bool{} // access is per project
	}
	title, err := text("title", p.Title, 1, maxTitleLen)
	if err != nil {
		return preparedItem{}, err
	}
	desc, err := optionalText("description", p.Description, maxDescriptionLen)
	if err != nil {
		return preparedItem{}, err
	}
	due, err := dueDate(p.DueDate)
	if err != nil {
		return preparedItem{}, err
	}
	typ, err := c.itemType(ctx, q, p.TypeKey)
	if err != nil {
		return preparedItem{}, err
	}
	schema, err := c.schema(ctx, q, typ.ID, p.Fields)
	if err != nil {
		return preparedItem{}, err
	}
	fields, _, err := applyFields(ctx, q, c.wsID, schema, typ.Key, map[string]any{}, p.Fields, true)
	if err != nil {
		return preparedItem{}, err
	}
	status, err := c.status(ctx, q, typ.workflowID, p.StatusID)
	if err != nil {
		return preparedItem{}, err
	}
	if p.AssigneeID != nil {
		if err := c.requireMember(ctx, q, *p.AssigneeID); err != nil {
			return preparedItem{}, err
		}
	}
	return preparedItem{typ: typ, status: status, title: title, desc: desc, assignee: p.AssigneeID, fields: fields, due: due}, nil
}

func (c *itemCache) itemType(ctx context.Context, q querier, key string) (itemTypeRow, error) {
	if t, ok := c.types[key]; ok {
		return t, nil
	}
	var t itemTypeRow
	err := q.QueryRow(ctx, `SELECT id, key, name, workflow_id FROM item_types
		WHERE workspace_id = $1 AND archived_at IS NULL AND ($2 = '' OR key = $2)
		ORDER BY (key = 'task') DESC, created_at LIMIT 1`, c.wsID, key).Scan(&t.ID, &t.Key, &t.Name, &t.workflowID)
	if errors.Is(err, pgx.ErrNoRows) {
		return itemTypeRow{}, invalid("type", "unknown item type")
	}
	if err != nil {
		return itemTypeRow{}, err
	}
	c.types[key] = t
	return t, nil
}

func (c *itemCache) schema(ctx context.Context, q querier, typeID uuid.UUID, raw map[string]json.RawMessage) (map[string]schemaField, error) {
	keys := fieldKeys(raw)
	covered := c.fieldKeys != nil
	for _, k := range keys {
		if _, ok := slices.BinarySearch(c.fieldKeys, k); !ok {
			covered = false
			break
		}
	}
	if !covered {
		return loadFieldSchema(ctx, q, c.wsID, typeID, keys)
	}
	if s, ok := c.schemas[typeID]; ok {
		return s, nil
	}
	s, err := loadFieldSchema(ctx, q, c.wsID, typeID, c.fieldKeys)
	if err != nil {
		return nil, err
	}
	c.schemas[typeID] = s
	return s, nil
}

// status resolves the requested status (nil: the workflow's initial one) with
// the same rules as resolveStatus.
func (c *itemCache) status(ctx context.Context, q querier, workflowID uuid.UUID, want *uuid.UUID) (Status, error) {
	list, ok := c.statuses[workflowID]
	if !ok {
		rows, err := q.Query(ctx, `SELECT id, name, category, EXISTS (SELECT 1 FROM approval_policies ap WHERE ap.status_id = statuses.id)
			FROM statuses WHERE workflow_id = $1 ORDER BY is_initial DESC, position`, workflowID)
		if err != nil {
			return Status{}, err
		}
		list, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Status, error) {
			var s Status
			var guarded bool
			err := r.Scan(&s.ID, &s.Name, &s.Category, &guarded)
			if guarded {
				c.guarded[s.ID] = true
			}
			return s, err
		})
		if err != nil {
			return Status{}, err
		}
		c.statuses[workflowID] = list
	}
	if want == nil {
		if len(list) == 0 {
			return Status{}, invalid("status_id", "workflow has no statuses")
		}
		return list[0], nil
	}
	for i, s := range list {
		if s.ID != *want {
			continue
		}
		// Creating (or importing) an item straight into a guarded status
		// would skip its approval. The initial status is exempt: a new item
		// starts there anyway, and its policy guards moving back into it.
		if i > 0 && c.guarded[s.ID] {
			return Status{}, invalid("status_id", "this status requires approval (approval_required): create the item in another status and request approval")
		}
		return s, nil
	}
	return Status{}, invalid("status_id", "status does not belong to this item type's workflow")
}

func (c *itemCache) requireMember(ctx context.Context, q querier, userID uuid.UUID) error {
	ok, cached := c.members[userID]
	if !cached {
		var err error
		if ok, err = canSeeProject(ctx, q, c.wsID, c.projectID, userID); err != nil {
			return err
		}
		c.members[userID] = ok
	}
	if !ok {
		return invalid("assignee_id", "user is not a member of this workspace or cannot see this project")
	}
	return nil
}

// createdPayload is the item.created event payload, shared by single creates and imports.
func createdPayload(item Item) map[string]any {
	payload := map[string]any{"key": item.Key, "title": item.Title, "snapshot": Snapshot(item)}
	if len(item.Fields) > 0 {
		payload["fields"] = item.Fields
	}
	return payload
}
