package store

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

const maxItemTypesPerWorkspace = 100

var itemTypeKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func getItemType(ctx context.Context, q querier, wsID uuid.UUID, key string) (ItemType, error) {
	types, err := listItemTypes(ctx, q, wsID, key)
	if err != nil {
		return ItemType{}, err
	}
	if len(types) == 0 {
		return ItemType{}, ErrNotFound
	}
	return types[0], nil
}

func normalizeIcon(icon *string) (*string, error) {
	return optionalText("icon", icon, 50)
}

// CreateItemType adds an item type using an existing workflow.
func (s *Store) CreateItemType(ctx context.Context, wsID, actorID uuid.UUID, key, name string, icon *string, workflowID uuid.UUID) (ItemType, error) {
	key = strings.TrimSpace(key)
	if !itemTypeKeyRe.MatchString(key) {
		return ItemType{}, invalid("key", "must be 1-63 characters: lowercase letters, digits and underscores, starting with a letter")
	}
	name, err := text("name", name, 1, 100)
	if err != nil {
		return ItemType{}, err
	}
	if icon, err = normalizeIcon(icon); err != nil {
		return ItemType{}, err
	}

	var it ItemType
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); errors.Is(err, ErrNotFound) {
			return invalid("workflow_id", "unknown workflow")
		} else if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM item_types WHERE workspace_id = $1 AND archived_at IS NULL`,
			wsID).Scan(&n); err != nil {
			return err
		}
		if n >= maxItemTypesPerWorkspace {
			return &ConflictError{Code: "item_type_limit_reached", Message: "a workspace can have at most 100 active item types"}
		}
		id := newID()
		_, err := tx.Exec(ctx, `INSERT INTO item_types (id, workspace_id, workflow_id, key, name, icon)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, wsID, workflowID, key, name, icon)
		if isUniqueViolation(err, "item_types_workspace_id_key_key") {
			return &ConflictError{Code: "item_type_key_taken", Message: "an item type with this key already exists (keys of archived types stay reserved)"}
		}
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item_type", id, "item_type.created",
			map[string]any{"key": key, "workflow_id": workflowID}); err != nil {
			return err
		}
		it, err = getItemType(ctx, tx, wsID, key)
		return err
	})
	return it, err
}

// UpdateItemType renames an item type or changes its icon. The workflow is
// fixed: moving items to another workflow needs a status mapping (future work).
func (s *Store) UpdateItemType(ctx context.Context, wsID, actorID uuid.UUID, key string, name, icon opt.Field[string]) (ItemType, error) {
	if name.Set && name.Null {
		return ItemType{}, invalid("name", "cannot be null")
	}
	var it ItemType
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var id uuid.UUID
		var curName string
		var curIcon *string
		err := tx.QueryRow(ctx, `SELECT id, name, icon FROM item_types
			WHERE workspace_id = $1 AND key = $2 AND archived_at IS NULL FOR UPDATE`, wsID, key).Scan(&id, &curName, &curIcon)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		changes := map[string]any{}
		newName, newIcon := curName, curIcon
		if name.Set {
			if newName, err = text("name", name.Value, 1, 100); err != nil {
				return err
			}
			if newName != curName {
				changes["name"] = diff(curName, newName)
			}
		}
		if icon.Set {
			if newIcon, err = normalizeIcon(icon.Ptr()); err != nil {
				return err
			}
			if !equalPtr(curIcon, newIcon) {
				changes["icon"] = diff(curIcon, newIcon)
			}
		}
		if len(changes) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE item_types SET name = $1, icon = $2 WHERE id = $3`, newName, newIcon, id); err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, wsID, actorID, "item_type", id, "item_type.updated",
				map[string]any{"changes": changes}); err != nil {
				return err
			}
		}
		it, err = getItemType(ctx, tx, wsID, key)
		return err
	})
	return it, err
}

// ArchiveItemType hides an item type from new items; existing items keep it.
func (s *Store) ArchiveItemType(ctx context.Context, wsID, actorID uuid.UUID, key string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		// Lock all active types so two concurrent archives can't remove the last one.
		rows, err := tx.Query(ctx, `SELECT id, key FROM item_types WHERE workspace_id = $1 AND archived_at IS NULL FOR UPDATE`, wsID)
		if err != nil {
			return err
		}
		var target uuid.UUID
		active := 0
		for rows.Next() {
			var id uuid.UUID
			var k string
			if err := rows.Scan(&id, &k); err != nil {
				rows.Close()
				return err
			}
			active++
			if k == key {
				target = id
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if target == uuid.Nil {
			return ErrNotFound
		}
		if active <= 1 {
			return &ConflictError{Code: "last_item_type", Message: "a workspace must keep at least one active item type"}
		}
		if _, err := tx.Exec(ctx, `UPDATE item_types SET archived_at = now() WHERE id = $1`, target); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item_type", target, "item_type.archived", map[string]any{"key": key})
	})
}
