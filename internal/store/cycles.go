package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Monthly period close (ADR-0051). A project can close each month on a
// chosen day: the values of all its items are saved for the month that
// ended (a snapshot people can read later), then on its open items the
// monthly marks are cleared and some fields are refilled from others, e.g.
// "Paid" cleared and balance := monthly payment. Every change goes through
// updateItemTx, so validation, approval locks and the per-item
// item.updated events (activity, webhooks, automations) behave as for a
// manual edit. One close per project and month (UNIQUE), so the scheduler
// running on several replicas, or a manual close, cannot apply it twice.

const (
	maxCycleFields = 20
	// maxCycleItems bounds one close: each reset item is a full edit (about a
	// dozen statements, ~7 ms), so 2000 items take ~15 s, well inside the
	// scheduler's close budget (engine.cycleBudget). A bigger project fails
	// with a clear error instead of timing out.
	maxCycleItems = 2000
	// maxLateDays: a scheduled close this many days late is not applied by
	// itself: resetting now would wipe marks people already set this month
	// and snapshot them under the old month. An admin closes it by hand.
	maxLateDays  = 3
	cycleBatch   = 50 // projects per scheduler tick
	periodLayout = "2006-01"
)

// CycleRefill sets Target to the value of Source when a month is closed.
type CycleRefill struct {
	Target string `json:"target"`
	Source string `json:"source"`
}

// Cycle is a project's monthly close configuration.
type Cycle struct {
	Enabled     bool          `json:"enabled"`
	DayOfMonth  int           `json:"day_of_month"`
	ClearFields []string      `json:"clear_fields"`
	Refill      []CycleRefill `json:"refill"`
	// IncludeClosed resets closed (done/canceled) items too.
	IncludeClosed bool `json:"include_closed"`
	// ResetStatusID is the status reset items get; nil keeps theirs.
	ResetStatusID *uuid.UUID `json:"reset_status_id"`
	// DueDayField is a date field (e.g. the loan date) whose day of the month
	// is every item's due date each month (ADR-0055); nil: due dates are not
	// managed. PaidField is a boolean field: unpaid late items are carried.
	DueDayField *string `json:"due_day_field"`
	// DueStartField (ADR-0060) is a date field (e.g. the loan date): the
	// first due date falls in the month after it, and its day is used when
	// the item's DueDayField is empty.
	DueStartField *string    `json:"due_start_field"`
	PaidField     *string    `json:"paid_field"`
	NextRun       *string    `json:"next_run"`    // YYYY-MM-DD of the next scheduled close
	NextPeriod    *string    `json:"next_period"` // YYYY-MM it will close
	LastError     *string    `json:"last_error"`  // why the last scheduled close failed
	UpdatedAt     *time.Time `json:"updated_at"`  // nil: never configured
}

// CycleInput replaces the configuration.
type CycleInput struct {
	Enabled       bool          `json:"enabled"`
	DayOfMonth    int           `json:"day_of_month"`
	ClearFields   []string      `json:"clear_fields"`
	Refill        []CycleRefill `json:"refill"`
	IncludeClosed bool          `json:"include_closed"`
	ResetStatusID *uuid.UUID    `json:"reset_status_id"`
	DueDayField   *string       `json:"due_day_field"`
	DueStartField *string       `json:"due_start_field"`
	PaidField     *string       `json:"paid_field"`
}

// PeriodClose is one closed month.
type PeriodClose struct {
	ID        uuid.UUID        `json:"id"`
	Period    string           `json:"period"` // YYYY-MM
	ClosedBy  *uuid.UUID       `json:"closed_by"`
	ItemCount int              `json:"item_count"` // items in the snapshot
	Changed   int              `json:"changed"`    // open items whose fields were reset
	Carried   int              `json:"carried"`    // unpaid late items left as they were (ADR-0055)
	Failed    []CycleItemError `json:"failed"`     // open items that could not be reset
	CreatedAt time.Time        `json:"created_at"`
}

// CycleItemError is an item a close could not reset (e.g. locked by a
// pending approval). Its values are in the snapshot; reset it by hand.
type CycleItemError struct {
	Number  int64  `json:"number"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PeriodSnapshotRow is an item as it was when its month was closed.
type PeriodSnapshotRow struct {
	Number     int64          `json:"number"`
	Title      string         `json:"title"`
	StatusID   uuid.UUID      `json:"status_id"`
	StatusName string         `json:"status_name"` // as it is named now
	AssigneeID *uuid.UUID     `json:"assignee_id"`
	Fields     map[string]any `json:"fields"`
}

// CyclePreview tells what closing a month now would do.
type CyclePreview struct {
	Period    string `json:"period"`     // YYYY-MM that would be closed
	Closed    bool   `json:"closed"`     // that month is already closed
	Items     int    `json:"items"`      // items that would be saved in the snapshot
	Open      int    `json:"open"`       // items the reset applies to (open ones, or all with include_closed)
	WillReset int    `json:"will_reset"` // open items whose values would change
	WillCarry int    `json:"will_carry"` // unpaid late items that would be left as they are (ADR-0055)
}

// periodStart is the first day of t's month.
func periodStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// nextRunAfter is the first date after today that falls on day.
func nextRunAfter(today time.Time, day int) time.Time {
	d := time.Date(today.Year(), today.Month(), day, 0, 0, 0, 0, time.UTC)
	if !d.After(today) {
		d = d.AddDate(0, 1, 0)
	}
	return d
}

// closedPeriodFor is the month a close on run date closes: the one before.
func closedPeriodFor(run time.Time) time.Time {
	return periodStart(run).AddDate(0, -1, 0)
}

func strPtr(s string) *string { return &s }

// GetCycle returns the project's configuration (disabled when never set).
func (s *Store) GetCycle(ctx context.Context, wsID, projectID uuid.UUID) (Cycle, error) {
	c := Cycle{DayOfMonth: 1, ClearFields: []string{}, Refill: []CycleRefill{}}
	var refill []byte
	var next *time.Time
	var updated time.Time
	err := s.pool.QueryRow(ctx, `SELECT is_enabled, day_of_month, clear_fields, refill, include_closed, reset_status_id,
			due_day_field, due_start_field, paid_field, next_run, last_error, updated_at
		FROM period_cycles WHERE workspace_id = $1 AND project_id = $2`, wsID, projectID).
		Scan(&c.Enabled, &c.DayOfMonth, &c.ClearFields, &refill, &c.IncludeClosed, &c.ResetStatusID,
			&c.DueDayField, &c.DueStartField, &c.PaidField, &next, &c.LastError, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return Cycle{}, err
	}
	if err := json.Unmarshal(refill, &c.Refill); err != nil {
		return Cycle{}, err
	}
	if next != nil {
		c.NextRun = strPtr(next.Format(time.DateOnly))
		c.NextPeriod = strPtr(closedPeriodFor(*next).Format(periodLayout))
	}
	c.UpdatedAt = &updated
	return c, nil
}

// validateCycle checks the fields: known, not computed, refills of one type.
func validateCycle(ctx context.Context, q querier, wsID uuid.UUID, in *CycleInput) error {
	if in.DayOfMonth < 1 || in.DayOfMonth > 28 {
		return invalid("day_of_month", "must be 1..28 (every month has these days)")
	}
	if len(in.ClearFields) > maxCycleFields {
		return invalid("clear_fields", "at most %d fields", maxCycleFields)
	}
	if len(in.Refill) > maxCycleFields {
		return invalid("refill", "at most %d fields", maxCycleFields)
	}
	if in.ClearFields == nil {
		in.ClearFields = []string{}
	}
	if in.Refill == nil {
		in.Refill = []CycleRefill{}
	}
	if in.DueDayField != nil && *in.DueDayField == "" {
		in.DueDayField = nil
	}
	if in.PaidField != nil && *in.PaidField == "" {
		in.PaidField = nil
	}
	if in.Enabled && !in.resets() {
		return invalid("clear_fields", "choose at least one field to clear or refill, or a status")
	}
	if in.ResetStatusID != nil {
		var ok bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE workspace_id = $1 AND id = $2)`,
			wsID, *in.ResetStatusID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return invalid("reset_status_id", "unknown status")
		}
	}
	keys := slices.Clone(in.ClearFields)
	for _, r := range in.Refill {
		keys = append(keys, r.Target, r.Source)
	}
	types, err := fieldTypesByKey(ctx, q, wsID, keys)
	if err != nil {
		return err
	}
	check := func(path, key string) (string, error) {
		t, ok := types[key]
		switch {
		case !ok:
			return "", invalid(path, "unknown field %q", key)
		case strings.HasPrefix(t, FieldFormula+":"):
			return "", invalid(path, "%q is a formula field: its value is computed", key)
		}
		return t, nil
	}
	touched := map[string]bool{}
	for i, k := range in.ClearFields {
		if _, err := check(fmt.Sprintf("clear_fields[%d]", i), k); err != nil {
			return err
		}
		if touched[k] {
			return invalid(fmt.Sprintf("clear_fields[%d]", i), "%q is listed twice", k)
		}
		touched[k] = true
	}
	for i, r := range in.Refill {
		path := fmt.Sprintf("refill[%d]", i)
		tt, err := check(path+".target", r.Target)
		if err != nil {
			return err
		}
		st, err := check(path+".source", r.Source)
		if err != nil {
			return err
		}
		if r.Target == r.Source {
			return invalid(path+".source", "must differ from the target")
		}
		if tt != st {
			return invalid(path+".source", "must have the same type as the target (%s, not %s)", tt, st)
		}
		if touched[r.Target] {
			return invalid(path+".target", "%q is already cleared or refilled", r.Target)
		}
		touched[r.Target] = true
	}
	return validateDueFields(ctx, q, wsID, in, touched)
}

// validateDueFields checks the due-date settings (ADR-0055): an active date
// field that the close itself leaves alone, and a paid flag that the close
// clears or refills (else an item paid once would never be late again).
func validateDueFields(ctx context.Context, q querier, wsID uuid.UUID, in *CycleInput, touched map[string]bool) error {
	if in.PaidField != nil && in.DueDayField == nil {
		return invalid("paid_field", "needs due_day_field")
	}
	check := func(path string, key *string, typ string) error {
		if key == nil {
			return nil
		}
		var t string
		var archived bool
		err := q.QueryRow(ctx, `SELECT field_type, archived_at IS NOT NULL FROM field_definitions WHERE workspace_id = $1 AND key = $2`,
			wsID, *key).Scan(&t, &archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (t != typ || archived)) {
			return invalid(path, "must be an active %s field of this workspace", typ)
		}
		return err
	}
	if err := check("due_day_field", in.DueDayField, FieldDate); err != nil {
		return err
	}
	if err := check("paid_field", in.PaidField, FieldBoolean); err != nil {
		return err
	}
	if in.DueStartField != nil && in.DueDayField == nil {
		return invalid("due_start_field", "needs due_day_field")
	}
	if err := check("due_start_field", in.DueStartField, FieldDate); err != nil {
		return err
	}
	if in.DueStartField != nil && touched[*in.DueStartField] {
		return invalid("due_start_field", "%q is cleared or refilled every month", *in.DueStartField)
	}
	if in.DueDayField != nil && touched[*in.DueDayField] {
		return invalid("due_day_field", "%q is cleared or refilled every month", *in.DueDayField)
	}
	if in.PaidField != nil && !touched[*in.PaidField] {
		return invalid("paid_field", "%q must be cleared or refilled every month", *in.PaidField)
	}
	return nil
}

// resets reports whether the configuration changes anything on close.
func (c CycleInput) resets() bool {
	return len(c.ClearFields) > 0 || len(c.Refill) > 0 || c.ResetStatusID != nil || c.DueDayField != nil
}

// SaveCycle replaces the configuration. Enabling it schedules the next close
// on the next matching day after today, so turning it on mid-month never
// closes a past month at once.
func (s *Store) SaveCycle(ctx context.Context, wsID, projectID, actorID uuid.UUID, in CycleInput) (Cycle, error) {
	if err := validateCycle(ctx, s.pool, wsID, &in); err != nil {
		return Cycle{}, err
	}
	refill, err := json.Marshal(in.Refill)
	if err != nil {
		return Cycle{}, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		var next *time.Time
		if in.Enabled {
			// Keep the scheduled date when only the fields change.
			var cur *time.Time
			var curDay int
			err := tx.QueryRow(ctx, `SELECT next_run, day_of_month FROM period_cycles
				WHERE workspace_id = $1 AND project_id = $2 AND is_enabled FOR UPDATE`, wsID, projectID).Scan(&cur, &curDay)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if cur != nil && curDay == in.DayOfMonth {
				next = cur
			} else {
				n := nextRunAfter(today, in.DayOfMonth)
				next = &n
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO period_cycles (project_id, workspace_id, is_enabled, day_of_month,
				clear_fields, refill, include_closed, reset_status_id, due_day_field, paid_field, next_run, updated_by, due_start_field)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (project_id) DO UPDATE SET is_enabled = EXCLUDED.is_enabled, day_of_month = EXCLUDED.day_of_month,
				clear_fields = EXCLUDED.clear_fields, refill = EXCLUDED.refill, include_closed = EXCLUDED.include_closed,
				reset_status_id = EXCLUDED.reset_status_id, due_day_field = EXCLUDED.due_day_field,
				due_start_field = EXCLUDED.due_start_field,
				paid_field = EXCLUDED.paid_field, next_run = EXCLUDED.next_run,
				updated_by = EXCLUDED.updated_by, last_error = NULL`,
			projectID, wsID, in.Enabled, in.DayOfMonth, in.ClearFields, refill, in.IncludeClosed, in.ResetStatusID,
			in.DueDayField, in.PaidField, next, nullUUID(actorID), in.DueStartField); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "cycle.updated", map[string]any{
			"enabled": in.Enabled, "day_of_month": in.DayOfMonth, "clear_fields": in.ClearFields, "refill": in.Refill,
			"include_closed": in.IncludeClosed, "reset_status_id": in.ResetStatusID,
			"due_day_field": in.DueDayField, "due_start_field": in.DueStartField, "paid_field": in.PaidField,
		})
	})
	if err != nil {
		return Cycle{}, err
	}
	return s.GetCycle(ctx, wsID, projectID)
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// cycleChanges is the field patch a close applies to an item with fields cur.
// Only keys whose value would change are included.
func cycleChanges(c CycleInput, cur map[string]json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	isNull := func(v json.RawMessage) bool { return len(v) == 0 || string(v) == "null" }
	for _, k := range c.ClearFields {
		if !isNull(cur[k]) {
			out[k] = json.RawMessage("null")
		}
	}
	for _, r := range c.Refill {
		src, dst := cur[r.Source], cur[r.Target]
		switch {
		case isNull(src) && isNull(dst):
		case isNull(src):
			out[r.Target] = json.RawMessage("null")
		case isNull(dst) || !jsonEqual(src, dst):
			out[r.Target] = src
		}
	}
	return out
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

type cycleItem struct {
	number  int64
	version int
	open    bool
	status  uuid.UUID
	due     *string // YYYY-MM-DD
	fields  map[string]json.RawMessage
}

// inScope reports whether the reset applies to the item.
func (c CycleInput) inScope(it cycleItem) bool { return it.open || c.IncludeClosed }

// statusChange is the status the item gets, if it changes.
func (c CycleInput) statusChange(it cycleItem) (uuid.UUID, bool) {
	if c.ResetStatusID == nil || *c.ResetStatusID == it.status {
		return uuid.Nil, false
	}
	return *c.ResetStatusID, true
}

func loadCycleItems(ctx context.Context, q querier, wsID, projectID uuid.UUID) ([]cycleItem, error) {
	rows, err := q.Query(ctx, `SELECT `+cycleItemCols+`
		FROM items i JOIN statuses s ON s.id = i.status_id
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.deleted_at IS NULL
		ORDER BY i.number LIMIT $3`, wsID, projectID, maxCycleItems+1)
	if err != nil {
		return nil, err
	}
	out, err := scanCycleItems(rows)
	if err != nil {
		return nil, err
	}
	if len(out) > maxCycleItems {
		return nil, invalid("project", "a monthly close handles at most %d items", maxCycleItems)
	}
	return out, nil
}

// cycleItemCols are the columns scanCycleItems reads (items i, statuses s).
const cycleItemCols = `i.number, i.version, s.category NOT IN ('done', 'canceled'), i.status_id, i.due_date::text, i.fields`

func scanCycleItems(rows pgx.Rows) ([]cycleItem, error) {
	defer rows.Close()
	var out []cycleItem
	for rows.Next() {
		var it cycleItem
		var raw []byte
		if err := rows.Scan(&it.number, &it.version, &it.open, &it.status, &it.due, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &it.fields); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (c Cycle) input() CycleInput {
	return CycleInput{Enabled: c.Enabled, DayOfMonth: c.DayOfMonth, ClearFields: c.ClearFields, Refill: c.Refill,
		IncludeClosed: c.IncludeClosed, ResetStatusID: c.ResetStatusID, DueDayField: c.DueDayField, DueStartField: c.DueStartField, PaidField: c.PaidField}
}

// parsePeriod reads an explicit YYYY-MM for a manual close: last month or
// the current one (closing it early on purpose; its scheduled close is then
// skipped). Older months are refused: closing them now would snapshot
// today's values under their names and reset this month's marks.
func parsePeriod(v string, today time.Time) (time.Time, error) {
	p, err := time.Parse(periodLayout, v)
	if err != nil {
		return time.Time{}, invalid("period", "must be YYYY-MM")
	}
	if p.After(periodStart(today)) {
		return time.Time{}, invalid("period", "cannot close a future month")
	}
	if p.Before(periodStart(today).AddDate(0, -1, 0)) {
		return time.Time{}, invalid("period", "only last month or the current one can be closed")
	}
	return p, nil
}

// notBeforeLatest refuses a month older than the latest closed one: closes
// go forward, so a snapshot never mixes a later month's values into an
// earlier one.
func notBeforeLatest(ctx context.Context, q querier, wsID, projectID uuid.UUID, p time.Time) error {
	var latest *time.Time
	if err := q.QueryRow(ctx, `SELECT max(period) FROM period_closes WHERE workspace_id = $1 AND project_id = $2`,
		wsID, projectID).Scan(&latest); err != nil {
		return err
	}
	if latest != nil && p.Before(*latest) {
		return invalid("period", "%s is already closed; months are closed in order", latest.Format(periodLayout))
	}
	return nil
}

// PreviewCycle tells what closing period (YYYY-MM; empty = last month) now would do.
func (s *Store) PreviewCycle(ctx context.Context, wsID, projectID uuid.UUID, period string) (CyclePreview, error) {
	c, err := s.GetCycle(ctx, wsID, projectID)
	if err != nil {
		return CyclePreview{}, err
	}
	today, err := workspaceToday(ctx, s.pool, wsID)
	if err != nil {
		return CyclePreview{}, err
	}
	p := periodStart(today).AddDate(0, -1, 0)
	if period != "" {
		if p, err = parsePeriod(period, today); err != nil {
			return CyclePreview{}, err
		}
	}
	out := CyclePreview{Period: p.Format(periodLayout)}
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM period_closes WHERE workspace_id = $1 AND project_id = $2 AND period = $3)`,
		wsID, projectID, p).Scan(&out.Closed); err != nil {
		return CyclePreview{}, err
	}
	items, err := loadCycleItems(ctx, s.pool, wsID, projectID)
	if err != nil {
		return CyclePreview{}, err
	}
	in := c.input()
	month := p.AddDate(0, 1, 0)
	for _, it := range items {
		out.Items++
		if !in.inScope(it) {
			continue
		}
		out.Open++
		if in.carries(it, today) {
			out.WillCarry++
			continue
		}
		if _, _, ok := in.resetUpdate(it, month); ok {
			out.WillReset++
		}
	}
	return out, nil
}

// CloseCycle closes period (YYYY-MM; empty = last month) now, for an admin.
func (s *Store) CloseCycle(ctx context.Context, wsID, projectID, actorID uuid.UUID, period string) (PeriodClose, error) {
	c, err := s.GetCycle(ctx, wsID, projectID)
	if err != nil {
		return PeriodClose{}, err
	}
	if c.UpdatedAt == nil || !c.input().resets() {
		return PeriodClose{}, invalid("cycle", "set up the fields to clear or refill first")
	}
	// Fields may have changed since the configuration was saved.
	in := c.input()
	if err := validateCycle(ctx, s.pool, wsID, &in); err != nil {
		return PeriodClose{}, err
	}
	var out PeriodClose
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		p := periodStart(today).AddDate(0, -1, 0)
		if period != "" {
			if p, err = parsePeriod(period, today); err != nil {
				return err
			}
		}
		if err := notBeforeLatest(ctx, tx, wsID, projectID, p); err != nil {
			return err
		}
		out, err = closePeriodTx(ctx, tx, wsID, projectID, actorID, p, today, in)
		return err
	})
	return out, err
}

// closePeriodTx saves the snapshot and resets the open items. It fails with
// period_closed when that month is already closed. today is the run date:
// with due dates (ADR-0055) an unpaid item due before it is carried, and the
// reset items fall due in the month after period.
func closePeriodTx(ctx context.Context, tx pgx.Tx, wsID, projectID, actorID uuid.UUID, period, today time.Time, c CycleInput) (PeriodClose, error) {
	out := PeriodClose{ID: newID(), Period: period.Format(periodLayout), ClosedBy: nullUUID(actorID), Failed: []CycleItemError{}}
	tag, err := tx.Exec(ctx, `INSERT INTO period_closes (id, workspace_id, project_id, period, closed_by, item_count, changed)
		VALUES ($1, $2, $3, $4, $5, 0, 0) ON CONFLICT (project_id, period) DO NOTHING`,
		out.ID, wsID, projectID, period, out.ClosedBy)
	if err != nil {
		return PeriodClose{}, err
	}
	if tag.RowsAffected() == 0 {
		return PeriodClose{}, &ConflictError{Code: "period_closed", Message: fmt.Sprintf("%s is already closed", out.Period)}
	}
	items, err := loadCycleItems(ctx, tx, wsID, projectID)
	if err != nil {
		return PeriodClose{}, err
	}
	snap, err := tx.Exec(ctx, `INSERT INTO period_snapshots (workspace_id, close_id, item_id, number, title, status_id, assignee_id, fields)
		SELECT i.workspace_id, $3, i.id, i.number, i.title, i.status_id, i.assignee_id, i.fields
		FROM items i WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.deleted_at IS NULL`, wsID, projectID, out.ID)
	if err != nil {
		return PeriodClose{}, err
	}
	out.ItemCount = int(snap.RowsAffected())
	extra := map[string]any{"cycle": map[string]any{"close_id": out.ID, "period": out.Period}}
	month := period.AddDate(0, 1, 0)
	for _, it := range items {
		if !c.inScope(it) {
			continue
		}
		if c.carries(it, today) {
			out.Carried++
			continue
		}
		u, in, ok := c.resetUpdate(it, month)
		if !ok {
			continue
		}
		u.WorkspaceID, u.ProjectID, u.ActorID = wsID, projectID, actorID
		changed, ie, err := applyCycleUpdate(ctx, tx, u, in, extra)
		if err != nil {
			return PeriodClose{}, err
		}
		if ie != nil {
			out.Failed = append(out.Failed, *ie)
		} else if changed {
			out.Changed++
		}
	}
	failed, err := json.Marshal(out.Failed)
	if err != nil {
		return PeriodClose{}, err
	}
	if err := tx.QueryRow(ctx, `UPDATE period_closes SET item_count = $2, changed = $3, failed = $4, carried = $5 WHERE id = $1 RETURNING created_at`,
		out.ID, out.ItemCount, out.Changed, failed, out.Carried).Scan(&out.CreatedAt); err != nil {
		return PeriodClose{}, err
	}
	var projectKey, projectName string
	if err := tx.QueryRow(ctx, `SELECT key, name FROM projects WHERE id = $1`, projectID).Scan(&projectKey, &projectName); err != nil {
		return PeriodClose{}, err
	}
	err = appendEvent(ctx, tx, wsID, actorID, "project", projectID, "period.closed", map[string]any{
		"close_id": out.ID, "period": out.Period, "items": out.ItemCount, "changed": out.Changed, "failed": len(out.Failed), "carried": out.Carried,
		"project_key": projectKey, "project_name": projectName,
	})
	return out, err
}

// ListCloses returns the project's closed months, newest first.
func (s *Store) ListCloses(ctx context.Context, wsID, projectID uuid.UUID) ([]PeriodClose, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+closeCols+`
		FROM period_closes WHERE workspace_id = $1 AND project_id = $2 ORDER BY period DESC LIMIT 240`, wsID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PeriodClose{}
	for rows.Next() {
		c, err := scanClose(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const closeCols = `id, period, closed_by, item_count, changed, carried, failed, created_at`

func scanClose(row pgx.Row) (PeriodClose, error) {
	var c PeriodClose
	var period time.Time
	var failed []byte
	if err := row.Scan(&c.ID, &period, &c.ClosedBy, &c.ItemCount, &c.Changed, &c.Carried, &failed, &c.CreatedAt); err != nil {
		return PeriodClose{}, err
	}
	c.Period = period.Format(periodLayout)
	if err := json.Unmarshal(failed, &c.Failed); err != nil {
		return PeriodClose{}, err
	}
	return c, nil
}

// GetCloseSnapshot returns one closed month and its items, by number.
func (s *Store) GetCloseSnapshot(ctx context.Context, wsID, projectID uuid.UUID, period string) (PeriodClose, []PeriodSnapshotRow, error) {
	p, err := time.Parse(periodLayout, period)
	if err != nil {
		return PeriodClose{}, nil, invalid("period", "must be YYYY-MM")
	}
	c, err := scanClose(s.pool.QueryRow(ctx, `SELECT `+closeCols+`
		FROM period_closes WHERE workspace_id = $1 AND project_id = $2 AND period = $3`, wsID, projectID, p))
	if errors.Is(err, pgx.ErrNoRows) {
		return PeriodClose{}, nil, ErrNotFound
	}
	if err != nil {
		return PeriodClose{}, nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT ps.number, ps.title, ps.status_id, coalesce(st.name, ''), ps.assignee_id, ps.fields
		FROM period_snapshots ps LEFT JOIN statuses st ON st.id = ps.status_id
		WHERE ps.workspace_id = $1 AND ps.close_id = $2 ORDER BY ps.number`, wsID, c.ID)
	if err != nil {
		return PeriodClose{}, nil, err
	}
	defer rows.Close()
	out := []PeriodSnapshotRow{}
	for rows.Next() {
		var r PeriodSnapshotRow
		if err := rows.Scan(&r.Number, &r.Title, &r.StatusID, &r.StatusName, &r.AssigneeID, &r.Fields); err != nil {
			return PeriodClose{}, nil, err
		}
		out = append(out, r)
	}
	return c, out, rows.Err()
}

// EmitCycles runs the scheduled closes that are due, for up to cycleBatch
// projects, and returns how many months it closed. Each project runs in its
// own transaction; one that fails records last_error and the loop goes on,
// so a broken project never holds back the others.
func (s *Store) EmitCycles(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.project_id FROM period_cycles c
		JOIN workspaces w ON w.id = c.workspace_id AND w.archived_at IS NULL AND w.deletion_requested_at IS NULL
		JOIN projects p ON p.id = c.project_id AND p.archived_at IS NULL AND p.deletion_requested_at IS NULL
		WHERE c.is_enabled AND c.next_run IS NOT NULL AND c.next_run <= (now() AT TIME ZONE w.timezone)::date
		ORDER BY c.next_run, c.project_id LIMIT $1`, cycleBatch)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	closed := 0
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		ok, err := s.runCycle(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", id, err))
			s.recordCycleError(ctx, id, err)
			continue
		}
		if ok {
			closed++
		}
	}
	return closed, errors.Join(errs...)
}

// recordCycleError notes an unexpected failure (database error, timeout) on
// the project's cycle so admins see it; the scheduler retries next tick. It
// uses its own short context: the tick's may be what ran out.
func (s *Store) recordCycleError(ctx context.Context, projectID uuid.UUID, cause error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	msg := "the monthly close failed and will be retried: " + cause.Error()
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		msg = "the monthly close took too long and will be retried; if this repeats, close the month by hand"
	}
	_, _ = s.pool.Exec(ctx, `UPDATE period_cycles SET last_error = $2 WHERE project_id = $1`, projectID, msg)
}

func (s *Store) runCycle(ctx context.Context, projectID uuid.UUID) (bool, error) {
	ctx = withSystem(ctx)
	var failure error
	var wsID uuid.UUID
	var next time.Time
	closed := false
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var refill []byte
		var c CycleInput
		var run time.Time
		err := tx.QueryRow(ctx, `SELECT workspace_id, day_of_month, clear_fields, refill, include_closed, reset_status_id,
				due_day_field, due_start_field, paid_field, next_run
			FROM period_cycles WHERE project_id = $1 AND is_enabled AND next_run IS NOT NULL FOR UPDATE SKIP LOCKED`, projectID).
			Scan(&wsID, &c.DayOfMonth, &c.ClearFields, &refill, &c.IncludeClosed, &c.ResetStatusID, &c.DueDayField, &c.DueStartField, &c.PaidField, &run)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // disabled or being run by another replica
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(refill, &c.Refill); err != nil {
			return err
		}
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		if run.After(today) {
			return nil // another replica closed it and moved on meanwhile
		}
		// The month due is the one before the scheduled date (not before
		// today): after downtime the right month is closed, or none.
		period := closedPeriodFor(run)
		next = nextRunAfter(today, c.DayOfMonth)
		if late := int(today.Sub(run).Hours() / 24); late > maxLateDays {
			failure = fmt.Errorf("the close of %s was due on %s, %d days ago; it was not applied automatically so this month's marks are kept: close %s by hand if it is still needed",
				period.Format(periodLayout), run.Format(time.DateOnly), late, period.Format(periodLayout))
			return nil
		}
		if verr := validateCycle(ctx, tx, wsID, &c); verr != nil {
			failure = verr
			return nil
		}
		sp, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		_, cerr := closePeriodTx(ctx, sp, wsID, projectID, uuid.Nil, period, today, c)

		var ce *ConflictError
		switch {
		case errors.As(cerr, &ce) && ce.Code == "period_closed":
			_ = sp.Rollback(ctx) // closed by hand already
		case cerr != nil:
			_ = sp.Rollback(ctx)
			var ve *ValidationError
			if !errors.As(cerr, &ve) {
				return cerr
			}
			failure = cerr
		default:
			if err := sp.Commit(ctx); err != nil {
				return err
			}
			closed = true
		}
		if failure != nil {
			return nil // recorded below, outside the rolled-back work
		}
		_, err = tx.Exec(ctx, `UPDATE period_cycles SET next_run = $2, last_error = NULL WHERE project_id = $1`, projectID, next)
		return err
	})
	if err != nil {
		return false, err
	}
	if failure != nil {
		if err := s.inTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE period_cycles SET next_run = $2, last_error = $3 WHERE project_id = $1`,
				projectID, next, failure.Error()); err != nil {
				return err
			}
			return appendEvent(ctx, tx, wsID, uuid.Nil, "project", projectID, "period.close_failed", map[string]any{"error": failure.Error()})
		}); err != nil {
			return false, err
		}
	}
	return closed, nil
}
