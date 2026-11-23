package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxImportRows bounds one import (ADR-0011); the parser enforces the same limit.
const MaxImportRows = 20_000

// importBatchRows is how many items are sent to Postgres per pipelined batch.
const importBatchRows = 500

// ImportRow is one spreadsheet row converted into create parameters.
// WorkspaceID, ProjectID and ActorID of Item are ignored (taken from ImportParams).
type ImportRow struct {
	Line int // row number in the file, for error reports
	Item CreateItemParams
}

// ImportParams are the inputs for ImportItems.
type ImportParams struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	ActorID     uuid.UUID
	FieldKeys   []string // every custom field key the mapping can set
	Rows        []ImportRow
	DryRun      bool           // validate everything, write nothing
	Audit       map[string]any // extra payload for the project.imported event (file name, format, sheet)
}

// ImportRowError is a validation problem in one row.
type ImportRowError struct {
	Line    int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ImportResult reports what an import did or would do.
type ImportResult struct {
	Valid       int              `json:"valid"` // rows that passed validation
	Created     int              `json:"created"`
	FirstNumber int64            `json:"first_number,omitempty"`
	LastNumber  int64            `json:"last_number,omitempty"`
	Errors      []ImportRowError `json:"-"`
}

var errImportRollback = errors.New("import rolled back")

// ImportItems creates items from many rows in one transaction: either every
// row is valid and all are created, or nothing is written. Each row goes
// through the same validation as CreateItem (prepareItem); each created item
// writes its usual item.created event (and dispatch job), and the import as a
// whole writes one project.imported audit event.
//
// Rows are validated first, collecting every row error, so one dry run reports
// all problems at once. Inserts are pipelined in batches to keep a 20 000-row
// import to a few seconds.
func (s *Store) ImportItems(ctx context.Context, p ImportParams) (ImportResult, error) {
	if len(p.Rows) > MaxImportRows {
		return ImportResult{}, invalid("rows", "at most %d rows per import", MaxImportRows)
	}
	var res ImportResult
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Lock the project row first: imports and single creates on this project
		// serialize on it, so the reserved number range cannot be taken by anyone else.
		var projectKey string
		var next int64
		err := tx.QueryRow(ctx, `SELECT key, next_item_number FROM projects
			WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL FOR UPDATE`,
			p.ProjectID, p.WorkspaceID).Scan(&projectKey, &next)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		cache := newItemCache(p.WorkspaceID, p.FieldKeys)
		prepared := make([]preparedItem, 0, len(p.Rows))
		for i, row := range p.Rows {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			in := row.Item
			in.WorkspaceID, in.ProjectID, in.ActorID = p.WorkspaceID, p.ProjectID, p.ActorID
			prep, err := prepareItem(ctx, tx, cache, in)
			var ve *ValidationError
			if errors.As(err, &ve) {
				res.Errors = append(res.Errors, ImportRowError{Line: row.Line, Field: ve.Field, Message: ve.Message})
				continue
			}
			if err != nil {
				return err
			}
			prepared = append(prepared, prep)
		}
		res.Valid = len(prepared)
		if len(res.Errors) > 0 || p.DryRun || len(prepared) == 0 {
			return errImportRollback
		}

		actor := any(nil)
		if p.ActorID != uuid.Nil {
			actor = p.ActorID
		}
		src := eventSource{source: "user"}
		if v, ok := ctx.Value(sourceKey{}).(eventSource); ok {
			src = v
		}
		var rule any
		if src.ruleID != uuid.Nil {
			rule = src.ruleID
		}

		first := next
		for start := 0; start < len(prepared); start += importBatchRows {
			batch := &pgx.Batch{}
			for _, prep := range prepared[start:min(start+importBatchRows, len(prepared))] {
				id, number := newID(), next
				next++
				batch.Queue(insertItemSQL, id, p.WorkspaceID, p.ProjectID, prep.typ.ID, prep.status.ID, number,
					prep.title, prep.desc, prep.assignee, p.ActorID, prep.fields)
				reporter := p.ActorID
				item := Item{ID: id, Key: projectKey + "-" + strconv.FormatInt(number, 10), Number: number,
					ProjectID: p.ProjectID, Type: prep.typ.ItemTypeRef, Status: prep.status, Title: prep.title,
					Description: prep.desc, AssigneeID: prep.assignee, ReporterID: &reporter, Fields: prep.fields, Version: 1}
				// Same rows appendEvent writes: the event plus its dispatch job (outbox).
				batch.Queue(`WITH e AS (
						INSERT INTO events (workspace_id, actor_id, entity_type, entity_id, action, payload, source, automation_rule_id)
						VALUES ($1, $2, 'item', $3, 'item.created', $4, $5, $6) RETURNING id)
					INSERT INTO jobs (kind, payload, max_attempts)
					SELECT $7, jsonb_build_object('event_id', e.id), 5 FROM e`,
					p.WorkspaceID, actor, id, createdPayload(item), src.source, rule, JobDispatchEvent)
			}
			if err := tx.SendBatch(ctx, batch).Close(); err != nil {
				return fmt.Errorf("import batch at row %d: %w", start, err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE projects SET next_item_number = $1 WHERE id = $2`, next, p.ProjectID); err != nil {
			return err
		}
		res.Created, res.FirstNumber, res.LastNumber = len(prepared), first, next-1

		audit := map[string]any{}
		for k, v := range p.Audit {
			audit[k] = v
		}
		audit["created"] = res.Created
		audit["first_key"] = projectKey + "-" + strconv.FormatInt(first, 10)
		audit["last_key"] = projectKey + "-" + strconv.FormatInt(next-1, 10)
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "project", p.ProjectID, "project.imported", audit)
	})
	if errors.Is(err, errImportRollback) {
		res.Created, res.FirstNumber, res.LastNumber = 0, 0, 0
		return res, nil
	}
	return res, err
}
