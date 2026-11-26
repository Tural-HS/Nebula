package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Payment due dates driven by the monthly cycle (ADR-0055). A cycle may name
// a date field (due_day_field, e.g. the loan date): the day of the month of
// its value is the item's due date every month. The close moves the due date
// of every reset item into the new month. With a boolean paid_field, an item
// still unpaid after its due date is carried by the close: it keeps its
// status, fields and due date, so it stays overdue. A daily step fills
// missing due dates and, once a carried item is marked paid, applies the
// reset the close skipped (the "roll").

// dueStepBudget bounds the item edits of one scheduler tick (each a full
// edit, ~7 ms): a backlog (e.g. the first day after the setting is turned
// on in a big project) is worked off over a few ticks.
const dueStepBudget = 500

// monthDay is day in month's month, clamped to its length (31 → 30 November).
func monthDay(month time.Time, day int) time.Time {
	last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return time.Date(month.Year(), month.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}

// fieldDate reads a date field value ("YYYY-MM-DD").
func fieldDate(v json.RawMessage) (time.Time, bool) {
	var s string
	if len(v) == 0 || json.Unmarshal(v, &s) != nil {
		return time.Time{}, false
	}
	d, err := time.Parse(time.DateOnly, s)
	return d, err == nil
}

func isTrue(v json.RawMessage) bool { return string(bytes.TrimSpace(v)) == "true" }

// dueIn is the item's due date in month: the day of its due_day_field value
// (else of its due_start_field value, ADR-0060), clamped. Nothing falls due
// in or before the month of the start date (due_start_field, else the day
// field itself): a loan given on 5 January is first due on 5 February.
// false: due dates are off or the item has no usable date.
func (c CycleInput) dueIn(it cycleItem, month time.Time) (string, bool) {
	if c.DueDayField == nil {
		return "", false
	}
	day, ok := fieldDate(it.fields[*c.DueDayField])
	start, hasStart := time.Time{}, false
	if c.DueStartField != nil {
		start, hasStart = fieldDate(it.fields[*c.DueStartField])
	}
	if !ok {
		day, ok = start, hasStart
	}
	if !ok {
		return "", false
	}
	ref := day
	if hasStart {
		ref = start
	}
	if first := periodStart(ref).AddDate(0, 1, 0); month.Before(first) {
		month = first
	}
	return monthDay(month, day.Day()).Format(time.DateOnly), true
}

// carries reports an unpaid late item that a close on today leaves as it is:
// overdue by the server's own rule (open, i.e. todo or in_progress, and due
// before today) and paid not true. A done or canceled item is never carried,
// whatever its paid mark says: it resets like any other.
func (c CycleInput) carries(it cycleItem, today time.Time) bool {
	if c.DueDayField == nil || c.PaidField == nil || !it.open || it.due == nil || isTrue(it.fields[*c.PaidField]) {
		return false
	}

	due, err := time.Parse(time.DateOnly, *it.due)
	return err == nil && due.Before(today)
}

// resetUpdate is the edit the cycle's reset makes to it: cleared and
// refilled fields, the reset status and, with due dates, the due date in
// month. ok is false when nothing would change. The caller sets the
// workspace, project and actor.
func (c CycleInput) resetUpdate(it cycleItem, month time.Time) (u UpdateItemParams, in checkedUpdate, ok bool) {
	u = UpdateItemParams{Number: it.number, Version: it.version}
	if patch := cycleChanges(c, it.fields); len(patch) > 0 {
		u.Fields, ok = opt.Of(patch), true
	}
	if st, changes := c.statusChange(it); changes {
		u.StatusID, ok = opt.Of(st), true
	}
	if due, has := c.dueIn(it, month); has && !equalPtr(it.due, &due) {
		u.DueDate, in.due, ok = opt.Of(due), &due, true
	}
	return u, in, ok
}

// applyCycleUpdate makes one cycle edit in a savepoint. An item-level
// failure (locked by an approval, a transition the workflow forbids, a
// concurrent edit) is returned as ie and rolled back; err is anything else.
func applyCycleUpdate(ctx context.Context, tx pgx.Tx, u UpdateItemParams, in checkedUpdate, extra map[string]any) (changed bool, ie *CycleItemError, err error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, nil, err
	}
	_, changed, err = updateItemTx(ctx, sp, u, in, extra)
	if err != nil {
		_ = sp.Rollback(ctx)
		be, ok := bulkItemError(u.Number, err)
		if !ok {
			return false, nil, err
		}
		return false, &CycleItemError{Number: be.Number, Code: be.Code, Message: be.Message}, nil
	}
	return changed, nil, sp.Commit(ctx)
}

// dueCandidates selects the items the daily step works on (aliases i, s, c
// for items, statuses, period_cycles and w for workspaces; $1 is now): a
// valid day field and either no due date yet, or paid while due before this
// month and that month already closed (so its snapshot holds the late
// month's values before the roll resets them).
const dueCandidates = `i.deleted_at IS NULL
	AND (((i.fields ->> c.due_day_field) ~ '^\d{4}-\d{2}-\d{2}$' AND jsonb_typeof(i.fields -> c.due_day_field) = 'string')
		OR (c.due_start_field IS NOT NULL AND jsonb_typeof(i.fields -> c.due_start_field) = 'string'
			AND (i.fields ->> c.due_start_field) ~ '^\d{4}-\d{2}-\d{2}$'))
	AND (i.due_date IS NULL OR (c.paid_field IS NOT NULL
		AND i.fields -> c.paid_field = 'true'::jsonb
		AND i.due_date < date_trunc('month', $1::timestamptz AT TIME ZONE w.timezone)::date
		AND (c.include_closed OR s.category NOT IN ('done', 'canceled'))
		AND EXISTS (SELECT 1 FROM period_closes pc WHERE pc.project_id = c.project_id
			AND pc.period >= date_trunc('month', i.due_date::timestamp)::date)))`

// EmitCycleDueDates runs the daily due-date step (ADR-0055) at now for the
// projects whose cycle manages due dates and that have work to do: fills
// missing due dates (this month, on the day of the item's date field) and
// rolls paid late items (the cycle's reset plus this month's due date).
// Returns the number of items changed. It is idempotent: a changed item is
// no longer a candidate, so every tick may run it; a failing project does
// not stop the others.
func (s *Store) EmitCycleDueDates(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.project_id FROM period_cycles c
		JOIN workspaces w ON w.id = c.workspace_id AND w.archived_at IS NULL AND w.deletion_requested_at IS NULL
		JOIN projects p ON p.id = c.project_id AND p.archived_at IS NULL AND p.deletion_requested_at IS NULL
		WHERE c.is_enabled AND c.due_day_field IS NOT NULL
		  AND EXISTS (SELECT 1 FROM items i JOIN statuses s ON s.id = i.status_id
			WHERE i.workspace_id = c.workspace_id AND i.project_id = c.project_id AND `+dueCandidates+`)
		ORDER BY c.project_id LIMIT $2`, now, cycleBatch)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	total := 0
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil || total >= dueStepBudget {
			break
		}
		n, err := s.runCycleDueDates(ctx, id, now, dueStepBudget-total)
		if err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", id, err))
			continue
		}
		total += n
	}
	return total, errors.Join(errs...)
}

func (s *Store) runCycleDueDates(ctx context.Context, projectID uuid.UUID, now time.Time, limit int) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var wsID uuid.UUID
		var c CycleInput
		var refill []byte
		var today time.Time
		// The cycle row lock (SKIP LOCKED) keeps this off a project that
		// another replica, or the monthly close itself, is working on.
		err := tx.QueryRow(ctx, `SELECT c.workspace_id, c.day_of_month, c.clear_fields, c.refill, c.include_closed,
				c.reset_status_id, c.due_day_field, c.due_start_field, c.paid_field, ($2::timestamptz AT TIME ZONE w.timezone)::date
			FROM period_cycles c JOIN workspaces w ON w.id = c.workspace_id
			WHERE c.project_id = $1 AND c.is_enabled AND c.due_day_field IS NOT NULL
			FOR UPDATE OF c SKIP LOCKED`, projectID, now).
			Scan(&wsID, &c.DayOfMonth, &c.ClearFields, &refill, &c.IncludeClosed, &c.ResetStatusID, &c.DueDayField, &c.DueStartField, &c.PaidField, &today)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(refill, &c.Refill); err != nil {
			return err
		}
		c.Enabled = true
		// A field archived since it was saved: the close reports it; do nothing.
		var ve *ValidationError
		if err := validateCycle(ctx, tx, wsID, &c); errors.As(err, &ve) {
			return nil
		} else if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+cycleItemCols+`
			FROM items i JOIN statuses s ON s.id = i.status_id
			JOIN period_cycles c ON c.project_id = i.project_id
			JOIN workspaces w ON w.id = i.workspace_id
			WHERE i.workspace_id = $2 AND i.project_id = $3 AND `+dueCandidates+`
			ORDER BY i.number LIMIT $4 FOR UPDATE OF i SKIP LOCKED`, now, wsID, projectID, limit)
		if err != nil {
			return err
		}
		items, err := scanCycleItems(rows)
		if err != nil {
			return err
		}
		month := periodStart(today)
		for _, it := range items {
			var u UpdateItemParams
			var in checkedUpdate
			var extra map[string]any
			if it.due == nil {
				due, ok := c.dueIn(it, month)
				if !ok {
					continue
				}
				u = UpdateItemParams{Number: it.number, Version: it.version, DueDate: opt.Of(due)}
				in.due = &due
				extra = map[string]any{"cycle": map[string]any{"due_date": true}}
			} else {
				if !c.inScope(it) {
					continue
				}
				var ok bool
				if u, in, ok = c.resetUpdate(it, month); !ok {
					continue
				}
				extra = map[string]any{"cycle": map[string]any{"roll": true, "period": (*it.due)[:7]}}
			}
			u.WorkspaceID, u.ProjectID = wsID, projectID
			// Item-level failures are retried on the next tick.
			changed, _, err := applyCycleUpdate(ctx, tx, u, in, extra)
			if err != nil {
				return err
			}
			if changed {
				n++
			}
		}
		return nil
	})
	return n, err
}
