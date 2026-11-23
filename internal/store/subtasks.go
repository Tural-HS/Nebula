package store

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Subtasks (ADR-0021): one level of parent/child items within a project.
// The database enforces the rules (composite FK, items_check_parent trigger);
// the checks here only turn the common cases into clear validation errors.

// ItemParent is the embedded summary of an item's parent.
type ItemParent struct {
	Key    string `json:"key"`
	Number int64  `json:"number"`
	Title  string `json:"title"`
}

// SubtaskSummary is an item's live subtasks: Done counts those in a done or
// canceled status, so a canceled subtask does not keep progress below 100%.
type SubtaskSummary struct {
	Count int64 `json:"count"`
	Done  int64 `json:"done"`
}

func itemKey(projectKey string, number int64) string {
	return projectKey + "-" + strconv.FormatInt(number, 10)
}

func keyOrNil(projectKey string, number *int64) any {
	if number == nil {
		return nil
	}
	return itemKey(projectKey, *number)
}

type parentRef struct {
	id uuid.UUID
}

// lockParent resolves a parent by number in the same project and locks it
// FOR SHARE, like the trigger does, so it cannot be deleted or become a
// subtask itself before this transaction commits.
func lockParent(ctx context.Context, tx pgx.Tx, wsID, projectID uuid.UUID, number int64) (parentRef, error) {
	var ref parentRef
	var grandparent *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id, parent_id FROM items
		WHERE workspace_id = $1 AND project_id = $2 AND number = $3 AND deleted_at IS NULL
		FOR SHARE`, wsID, projectID, number).Scan(&ref.id, &grandparent)
	if errors.Is(err, pgx.ErrNoRows) {
		return parentRef{}, invalid("parent", "no such item in this project")
	}
	if err != nil {
		return parentRef{}, err
	}
	if grandparent != nil {
		return parentRef{}, invalid("parent", "a subtask cannot have subtasks")
	}
	return ref, nil
}

func requireNoSubtasks(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var has bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM items WHERE parent_id = $1 AND deleted_at IS NULL)`, id).
		Scan(&has); err != nil {
		return err
	}
	if has {
		return invalid("parent", "an item with subtasks cannot become a subtask")
	}
	return nil
}

// parentError maps the database guards to validation errors; they fire when
// a concurrent change slipped past the checks above.
func parentError(err error) error {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return err
	}
	switch {
	case pg.Code == "23514" && (pg.ConstraintName == "items_parent_depth" || pg.ConstraintName == "items_parent_live" ||
		pg.ConstraintName == "items_parent_not_self"):
		return invalid("parent", "%s", pg.Message)
	case pg.Code == "23503" && pg.ConstraintName == "items_parent_fkey":
		return invalid("parent", "no such item in this project")
	}
	return err
}

// detachSubtasks makes the subtasks of a deleted item top-level again, with
// an item.updated event each, so nothing points at a deleted parent and the
// change shows up in each subtask's history and automations.
func detachSubtasks(ctx context.Context, tx pgx.Tx, wsID, actorID, parentID uuid.UUID) error {
	var parentKey string
	if err := tx.QueryRow(ctx, `SELECT p.key || '-' || i.number FROM items i JOIN projects p ON p.id = i.project_id
		WHERE i.id = $1`, parentID).Scan(&parentKey); err != nil {
		return err
	}
	// Deleted subtasks are detached too (no event: they are gone already).
	rows, err := tx.Query(ctx, `UPDATE items SET parent_id = NULL,
			version = version + CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END
		WHERE parent_id = $1 RETURNING id, deleted_at IS NULL`, parentID)
	if err != nil {
		return err
	}
	var live []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		var isLive bool
		if err := rows.Scan(&id, &isLive); err != nil {
			rows.Close()
			return err
		}
		if isLive {
			live = append(live, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range live {
		item, err := scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, id))
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item", id, "item.updated", map[string]any{
			"changes":  map[string]any{"parent": diff(parentKey, nil)},
			"snapshot": Snapshot(item),
		}); err != nil {
			return err
		}
	}
	return nil
}
