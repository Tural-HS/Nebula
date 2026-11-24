package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Bulk edits (ADR-0023): one request changes or deletes many items of one
// project. Every item goes through the same code as a single edit
// (updateItemTx / deleteItemTx), so validation, workflow transitions,
// assignee access, field rules and the per-item item.updated / item.deleted
// events (webhooks, automations, activity) behave exactly as for single
// edits. The request is all-or-nothing: if any item fails, nothing is
// changed and the caller gets the list of failing items.

// MaxBulkItems caps one bulk request. 500 items is a few thousand
// statements in one transaction: well inside the request and statement
// timeouts, and the web UI selects at most this many.
const MaxBulkItems = 500

// BulkTarget is one selected item and the version the caller last saw. The
// version makes a bulk edit as safe as a single one: an item changed by
// someone else since it was loaded fails with version_conflict instead of
// being overwritten. It also makes a blind retry harmless (ADR-0023): a
// request that did commit cannot apply twice, since every version moved on.
type BulkTarget struct {
	Number  int64 `json:"number"`
	Version int   `json:"version"`
}

// BulkItemsParams describe a bulk change. Delete excludes the other fields.
type BulkItemsParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	ActorID     uuid.UUID
	Items       []BulkTarget
	Delete      bool
	StatusID    opt.Field[uuid.UUID]
	AssigneeID  opt.Field[uuid.UUID]
	DueDate     opt.Field[string]
	Fields      opt.Field[map[string]json.RawMessage]
}

// BulkResult is a committed bulk change.
type BulkResult struct {
	BulkID    uuid.UUID `json:"bulk_id"`   // recorded in every event of this change
	Items     []Item    `json:"items"`     // updated items as now stored (empty for a delete)
	Changed   int       `json:"changed"`   // items actually changed or deleted
	Unchanged int       `json:"unchanged"` // items that already had the requested values
}

// BulkItemError explains why one item of a bulk change failed.
type BulkItemError struct {
	Number  int64  `json:"number"`
	Code    string `json:"code"` // not_found, version_conflict, validation_failed, ...
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// BulkError is a rolled-back bulk change: nothing was applied.
type BulkError struct {
	Items []BulkItemError
}

func (e *BulkError) Error() string {
	return fmt.Sprintf("bulk change failed for %d item(s)", len(e.Items))
}

// BulkItems applies one change to every target in a single transaction.
func (s *Store) BulkItems(ctx context.Context, p BulkItemsParams) (BulkResult, error) {
	if len(p.Items) == 0 {
		return BulkResult{}, invalid("items", "select at least one item")
	}
	if len(p.Items) > MaxBulkItems {
		return BulkResult{}, invalid("items", "at most %d items per request", MaxBulkItems)
	}
	seen := make(map[int64]bool, len(p.Items))
	for _, t := range p.Items {
		if t.Number < 1 || t.Version < 1 {
			return BulkResult{}, invalid("items", "every item needs a number and the version you last read")
		}
		if seen[t.Number] {
			return BulkResult{}, invalid("items", "item %d is listed twice", t.Number)
		}
		seen[t.Number] = true
	}
	update := UpdateItemParams{
		WorkspaceID: p.WorkspaceID, ProjectID: p.ProjectID, ActorID: p.ActorID,
		StatusID: p.StatusID, AssigneeID: p.AssigneeID, DueDate: p.DueDate, Fields: p.Fields,
	}
	hasChange := p.StatusID.Set || p.AssigneeID.Set || p.DueDate.Set || (p.Fields.Set && len(p.Fields.Value) > 0)
	if p.Delete == hasChange {
		return BulkResult{}, invalid("action", "send either delete or at least one of status_id, assignee_id, due_date, fields")
	}
	if p.Fields.Set && len(p.Fields.Value) > maxBulkFields {
		return BulkResult{}, invalid("fields", "at most %d fields per bulk change", maxBulkFields)
	}
	checked, err := checkUpdate(update)
	if err != nil {
		return BulkResult{}, err
	}
	if p.Fields.Set && len(p.Fields.Value) > 0 {
		// Refuse up front instead of failing every item (ADR-0030).
		types, err := fieldTypesByKey(ctx, s.pool, p.WorkspaceID, fieldKeys(p.Fields.Value))
		if err != nil {
			return BulkResult{}, err
		}
		for _, k := range fieldKeys(p.Fields.Value) {
			if strings.HasPrefix(types[k], FieldFormula+":") {
				return BulkResult{}, invalid("fields."+k, "is a formula field: its value is computed and cannot be set")
			}
		}
	}

	// Lock rows in one order (by number): two overlapping bulk edits then
	// wait for each other instead of deadlocking.
	targets := slices.Clone(p.Items)
	slices.SortFunc(targets, func(a, b BulkTarget) int { return cmp.Compare(a.Number, b.Number) })

	res := BulkResult{BulkID: newID(), Items: []Item{}}
	extra := map[string]any{"bulk": map[string]any{"id": res.BulkID, "size": len(targets)}}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var failed []BulkItemError
		for _, t := range targets {
			// A savepoint per item: a failing statement aborts only this
			// item's work, so every failure can be reported at once.
			sp, err := tx.Begin(ctx)
			if err != nil {
				return err
			}
			var item Item
			var changed bool
			if p.Delete {
				v := t.Version
				err = deleteItemTx(ctx, sp, p.WorkspaceID, p.ProjectID, t.Number, &v, p.ActorID, extra)
				changed = err == nil
			} else {
				u := update
				u.Number, u.Version = t.Number, t.Version
				item, changed, err = updateItemTx(ctx, sp, u, checked, extra)
			}
			if err != nil {
				_ = sp.Rollback(ctx)
				ie, ok := bulkItemError(t.Number, err)
				if !ok {
					return err // database or other unexpected error: abort as is
				}
				failed = append(failed, ie)
				continue
			}
			if err := sp.Commit(ctx); err != nil {
				return err
			}
			if changed {
				res.Changed++
			} else {
				res.Unchanged++
			}
			if !p.Delete {
				res.Items = append(res.Items, item)
			}
		}
		if len(failed) > 0 {
			return &BulkError{Items: failed}
		}
		return nil
	})
	if err != nil {
		return BulkResult{}, err
	}
	return res, nil
}

const maxBulkFields = 20

// bulkItemError turns a client-caused failure of one item into its report.
func bulkItemError(n int64, err error) (BulkItemError, bool) {
	var ve *ValidationError
	var ce *ConflictError
	switch {
	case errors.As(err, &ve):
		return BulkItemError{Number: n, Code: "validation_failed", Message: ve.Message, Field: ve.Field}, true
	case errors.As(err, &ce):
		return BulkItemError{Number: n, Code: ce.Code, Message: ce.Message}, true
	case errors.Is(err, ErrNotFound):
		return BulkItemError{Number: n, Code: "not_found", Message: "item not found"}, true
	}
	return BulkItemError{}, false
}
