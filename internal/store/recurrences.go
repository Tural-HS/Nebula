package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/recur"
)

// Recurring items (ADR-0012).

const (
	maxRecurrencesPerWorkspace = 1000
	// maxCatchUpPerRule bounds how many missed dates one rule creates per
	// scheduler tick; the rest follow on the next ticks.
	maxCatchUpPerRule = 24
	upcomingPreview   = 5
)

// Recurrence is a rule that creates the next item of a series before it is due.
type Recurrence struct {
	ID          uuid.UUID      `json:"id"`
	Title       string         `json:"title"`
	Description *string        `json:"description"`
	Type        ItemTypeRef    `json:"type"`
	AssigneeID  *uuid.UUID     `json:"assignee_id"`
	Fields      map[string]any `json:"fields"`
	Freq        string         `json:"freq"`
	Every       int            `json:"every"`
	Weekdays    []int          `json:"weekdays"`
	MonthDay    *int           `json:"month_day"`
	WorkDay     *int           `json:"work_day"`
	Month       *int           `json:"month"`
	Shift       string         `json:"shift"`
	StartDate   string         `json:"start_date"`
	EndDate     *string        `json:"end_date"`
	LeadDays    int            `json:"lead_days"`
	NextDue     *string        `json:"next_due"`
	Upcoming    []string       `json:"upcoming"`
	Enabled     bool           `json:"enabled"`
	LastError   *string        `json:"last_error"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// RecurrenceInput creates or replaces a rule.
type RecurrenceInput struct {
	Title       string                     `json:"title"`
	Description *string                    `json:"description"`
	Type        string                     `json:"type"` // item type key; empty = default
	AssigneeID  *uuid.UUID                 `json:"assignee_id"`
	Fields      map[string]json.RawMessage `json:"fields"`
	ScheduleInput
	LeadDays *int  `json:"lead_days"` // default 3
	Enabled  *bool `json:"enabled"`   // default true
}

// ScheduleInput is the schedule part of a rule, as JSON.
type ScheduleInput struct {
	Freq      string  `json:"freq"`
	Every     int     `json:"every"`
	Weekdays  []int   `json:"weekdays"`
	MonthDay  *int    `json:"month_day"`
	WorkDay   *int    `json:"work_day"` // N-th working day 1..23 or -1 = last (ADR-0027)
	Month     *int    `json:"month"`
	Shift     string  `json:"shift"` // none (default) | previous | next
	StartDate string  `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

// Schedule converts and validates the schedule.
func (in ScheduleInput) Schedule() (recur.Schedule, error) {
	start, err := time.Parse(time.DateOnly, in.StartDate)
	if err != nil {
		return recur.Schedule{}, invalid("start_date", "must be a date YYYY-MM-DD")
	}
	s := recur.Schedule{Freq: in.Freq, Interval: in.Every, Start: start, Shift: in.shift()}
	if in.EndDate != nil {
		end, err := time.Parse(time.DateOnly, *in.EndDate)
		if err != nil {
			return recur.Schedule{}, invalid("end_date", "must be a date YYYY-MM-DD")
		}
		s.End = &end
	}
	for _, d := range in.Weekdays {
		s.Weekdays = append(s.Weekdays, time.Weekday(d))
	}
	if in.MonthDay != nil {
		s.MonthDay = *in.MonthDay
	}
	if in.WorkDay != nil {
		if *in.WorkDay == 0 {
			return recur.Schedule{}, invalid("work_day", "must be 1 to %d, -1 (last working day) or null", recur.MaxWorkDay)
		}
		s.WorkDay = *in.WorkDay
	}
	if in.Month != nil {
		s.Month = time.Month(*in.Month)
	}
	if start.Year() < 1900 || start.Year() > 2200 {
		return recur.Schedule{}, invalid("start_date", "must be between 1900 and 2200")
	}
	if err := s.Validate(); err != nil {
		return recur.Schedule{}, invalid("schedule", "%s", err.Error())
	}
	return s, nil
}

func (in ScheduleInput) shift() string {
	if in.Shift == "" {
		return recur.ShiftNone
	}
	return in.Shift
}

// PreviewSchedule returns the next dates of a schedule, from today in the
// workspace's time zone, without saving anything.
func (s *Store) PreviewSchedule(ctx context.Context, wsID uuid.UUID, in ScheduleInput) ([]string, error) {
	sched, err := in.Schedule()
	if err != nil {
		return nil, err
	}
	today, err := workspaceToday(ctx, s.pool, wsID)
	if err != nil {
		return nil, err
	}
	if sched.UsesCalendar() {
		if sched.Cal, err = loadCalendar(ctx, s.pool, wsID, false); err != nil {
			return nil, err
		}
	}
	return formatDates(sched.Upcoming(today, upcomingPreview)), nil
}

func workspaceToday(ctx context.Context, q querier, wsID uuid.UUID) (time.Time, error) {
	var today time.Time
	err := q.QueryRow(ctx, `SELECT (now() AT TIME ZONE timezone)::date FROM workspaces WHERE id = $1`, wsID).Scan(&today)
	return recur.Date(today), err
}

func formatDates(ds []time.Time) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Format(time.DateOnly)
	}
	return out
}

var azMonths = [...]string{"yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avqust", "sentyabr", "oktyabr", "noyabr", "dekabr"}

// RenderRecurrenceTitle fills {{date}} (dd.mm.yyyy), {{month}} and {{year}}.
func RenderRecurrenceTitle(tmpl string, due time.Time) string {
	return strings.NewReplacer(
		"{{date}}", due.Format("02.01.2006"),
		"{{month}}", azMonths[due.Month()-1],
		"{{year}}", due.Format("2006"),
	).Replace(tmpl)
}

// validateRecurrence checks the schedule and that the item it would create is
// valid today (type, required fields, assignee), using the same validation as
// a manual create.
func validateRecurrence(ctx context.Context, q querier, wsID, projectID uuid.UUID, in RecurrenceInput) (recur.Schedule, preparedItem, int, error) {
	sched, err := in.Schedule()
	if err != nil {
		return recur.Schedule{}, preparedItem{}, 0, err
	}
	lead := 3
	if in.LeadDays != nil {
		lead = *in.LeadDays
	}
	if lead < 0 || lead > 365 {
		return recur.Schedule{}, preparedItem{}, 0, invalid("lead_days", "must be 0 to 365")
	}
	sample, _ := sched.Next(sched.Start)
	prep, err := prepareItem(ctx, q, newItemCache(wsID, nil), CreateItemParams{
		WorkspaceID: wsID, ProjectID: projectID, Title: RenderRecurrenceTitle(in.Title, sample),
		Description: in.Description, TypeKey: in.Type, AssigneeID: in.AssigneeID, Fields: in.Fields,
	})
	if err != nil {
		return recur.Schedule{}, preparedItem{}, 0, err
	}
	if _, err := text("title", in.Title, 1, maxTitleLen); err != nil {
		return recur.Schedule{}, preparedItem{}, 0, err
	}
	return sched, prep, lead, nil
}

// firstDue is the due date of the first occurrence from today on that comes
// after the last one with an item (see recur.Schedule.NextAfter).
func firstDue(sched recur.Schedule, today time.Time, after *recur.Occurrence) *time.Time {
	o, ok := sched.NextAfter(today, after)
	if !ok {
		return nil
	}
	return &o.Due
}

const recurrenceSelect = `SELECT r.id, r.title, r.description, t.id, t.key, t.name, r.assignee_id, r.fields,
	r.freq, r.every, r.weekdays, r.month_day, r.work_day, r.month, r.shift, r.start_date::text, r.end_date::text, r.lead_days,
	r.next_due::text, r.is_enabled, r.last_error, r.created_at, r.updated_at
	FROM recurrences r JOIN item_types t ON t.id = r.item_type_id`

// scanRecurrence reads a rule; its upcoming dates use cal (nil = default
// calendar; only rules with working-day options depend on it).
func scanRecurrence(row pgx.Row, cal *recur.Calendar) (Recurrence, error) {
	var r Recurrence
	var weekdays []int16
	var monthDay, workDay, month *int16
	var every, lead int16
	err := row.Scan(&r.ID, &r.Title, &r.Description, &r.Type.ID, &r.Type.Key, &r.Type.Name, &r.AssigneeID, &r.Fields,
		&r.Freq, &every, &weekdays, &monthDay, &workDay, &month, &r.Shift, &r.StartDate, &r.EndDate, &lead,
		&r.NextDue, &r.Enabled, &r.LastError, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recurrence{}, ErrNotFound
	}
	if err != nil {
		return Recurrence{}, err
	}
	r.Every, r.LeadDays = int(every), int(lead)
	r.Weekdays = make([]int, len(weekdays))
	for i, d := range weekdays {
		r.Weekdays[i] = int(d)
	}
	if monthDay != nil {
		v := int(*monthDay)
		r.MonthDay = &v
	}
	if workDay != nil {
		v := int(*workDay)
		r.WorkDay = &v
	}
	if month != nil {
		v := int(*month)
		r.Month = &v
	}
	r.Upcoming = []string{}
	if r.NextDue != nil {
		if sched, err := r.input().Schedule(); err == nil {
			sched.Cal = cal
			next, _ := time.Parse(time.DateOnly, *r.NextDue)
			r.Upcoming = formatDates(sched.Upcoming(next, upcomingPreview))
		}
	}
	return r, nil
}

func (r Recurrence) input() ScheduleInput {
	return ScheduleInput{Freq: r.Freq, Every: r.Every, Weekdays: r.Weekdays, MonthDay: r.MonthDay, WorkDay: r.WorkDay,
		Month: r.Month, Shift: r.Shift, StartDate: r.StartDate, EndDate: r.EndDate}
}

// ListRecurrences returns a project's rules, soonest first.
func (s *Store) ListRecurrences(ctx context.Context, wsID, projectID uuid.UUID) ([]Recurrence, error) {
	cal, err := loadCalendar(ctx, s.pool, wsID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, recurrenceSelect+` WHERE r.workspace_id = $1 AND r.project_id = $2
		ORDER BY r.next_due NULLS LAST, r.title, r.id`, wsID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recurrence{}
	for rows.Next() {
		r, err := scanRecurrence(rows, cal)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRecurrence returns one rule of a project.
func (s *Store) GetRecurrence(ctx context.Context, wsID, projectID, id uuid.UUID) (Recurrence, error) {
	cal, err := loadCalendar(ctx, s.pool, wsID, false)
	if err != nil {
		return Recurrence{}, err
	}
	return scanRecurrence(s.pool.QueryRow(ctx, recurrenceSelect+` WHERE r.workspace_id = $1 AND r.project_id = $2 AND r.id = $3`,
		wsID, projectID, id), cal)
}

func weekdaysArray(in []int) []int16 {
	out := make([]int16, 0, len(in))
	seen := map[int]bool{}
	for _, d := range in {
		if !seen[d] && d >= 0 && d <= 6 {
			seen[d] = true
			out = append(out, int16(d)) // #nosec G115 -- range checked
		}
	}
	return out
}

// SaveRecurrence creates (id == uuid.Nil) or replaces a rule.
func (s *Store) SaveRecurrence(ctx context.Context, wsID, projectID, actorID, id uuid.UUID, in RecurrenceInput) (Recurrence, error) {
	enabled := in.Enabled == nil || *in.Enabled
	var saved uuid.UUID
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		sched, prep, lead, err := validateRecurrence(ctx, tx, wsID, projectID, in)
		if err != nil {
			return err
		}
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		if sched.Cal, err = loadCalendar(ctx, tx, wsID, true); err != nil {
			return err
		}
		action := "recurrence.created"
		var last *recur.Occurrence
		if id == uuid.Nil {
			var n int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM recurrences WHERE workspace_id = $1`, wsID).Scan(&n); err != nil {
				return err
			}
			if n >= maxRecurrencesPerWorkspace {
				return invalid("recurrence", "a workspace can have at most %d recurring items", maxRecurrencesPerWorkspace)
			}
			id = newID()
		} else {
			action = "recurrence.updated"
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT true FROM recurrences WHERE workspace_id = $1 AND project_id = $2 AND id = $3 FOR UPDATE`,
				wsID, projectID, id).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
			// Never schedule a date that already has an item.
			if last, err = lastRun(ctx, tx, id); err != nil {
				return err
			}
		}
		next := firstDue(sched, today, last)
		var end, monthDay, workDay, month any
		if sched.End != nil {
			end = sched.End.Format(time.DateOnly)
		}
		if in.MonthDay != nil {
			monthDay = *in.MonthDay
		}
		if in.WorkDay != nil {
			workDay = *in.WorkDay
		}
		if in.Month != nil {
			month = *in.Month
		}
		var nextDue any
		if next != nil {
			nextDue = next.Format(time.DateOnly)
		}
		_, err = tx.Exec(ctx, `INSERT INTO recurrences (id, workspace_id, project_id, title, description, item_type_id,
				assignee_id, fields, freq, every, weekdays, month_day, month, start_date, end_date, lead_days, next_due,
				is_enabled, last_error, created_by, work_day, shift)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::date, $15::date, $16, $17::date, $18, NULL, $19,
				$20, $21)
			ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, description = EXCLUDED.description,
				item_type_id = EXCLUDED.item_type_id, assignee_id = EXCLUDED.assignee_id, fields = EXCLUDED.fields,
				freq = EXCLUDED.freq, every = EXCLUDED.every, weekdays = EXCLUDED.weekdays, month_day = EXCLUDED.month_day,
				month = EXCLUDED.month, start_date = EXCLUDED.start_date, end_date = EXCLUDED.end_date,
				lead_days = EXCLUDED.lead_days, next_due = EXCLUDED.next_due, is_enabled = EXCLUDED.is_enabled,
				last_error = NULL, work_day = EXCLUDED.work_day, shift = EXCLUDED.shift`,
			id, wsID, projectID, strings.TrimSpace(in.Title), prep.desc, prep.typ.ID, prep.assignee, prep.fields,
			in.Freq, in.Every, weekdaysArray(in.Weekdays), monthDay, month, in.StartDate, end, lead, nextDue,
			enabled, actorID, workDay, sched.Shift)
		if err != nil {
			return err
		}
		saved = id
		return appendEvent(ctx, tx, wsID, actorID, "recurrence", id, action,
			map[string]any{"title": in.Title, "freq": in.Freq, "work_day": workDay, "shift": sched.Shift,
				"next_due": nextDue, "enabled": enabled})
	})
	if err != nil {
		return Recurrence{}, err
	}
	return s.GetRecurrence(ctx, wsID, projectID, saved)
}

// DeleteRecurrence removes a rule; items it created stay.
func (s *Store) DeleteRecurrence(ctx context.Context, wsID, projectID, actorID, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var title string
		err := tx.QueryRow(ctx, `DELETE FROM recurrences WHERE workspace_id = $1 AND project_id = $2 AND id = $3 RETURNING title`,
			wsID, projectID, id).Scan(&title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// The row is gone: carry the project so webhook project filters still apply.
		return appendEvent(ctx, tx, wsID, actorID, "recurrence", id, "recurrence.deleted",
			map[string]any{"title": title, "project_id": projectID.String()})
	})
}

// EmitRecurrences creates the items of rules whose next date is within their
// lead time, for up to limit rules, and returns how many items it created.
// Each rule runs in its own transaction so one broken rule cannot block others.
func (s *Store) EmitRecurrences(ctx context.Context, limit int) (int, error) {
	// Archived projects and workspaces create nothing (ADR-0039); their
	// rules catch up after a restore, bounded like any backlog.
	rows, err := s.pool.Query(ctx, `SELECT r.id FROM recurrences r
		JOIN workspaces w ON w.id = r.workspace_id AND w.archived_at IS NULL
		JOIN projects p ON p.id = r.project_id AND p.archived_at IS NULL
		WHERE r.is_enabled AND r.next_due IS NOT NULL
		  AND r.next_due - r.lead_days <= (now() AT TIME ZONE w.timezone)::date
		ORDER BY r.next_due, r.id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	created := 0
	for _, id := range ids {
		n, err := s.runRecurrence(ctx, id)
		created += n
		if err != nil {
			return created, err
		}
	}
	return created, nil
}

// runRecurrence creates every due item of one rule (bounded), advancing
// next_due after each. A validation failure disables the rule.
func (s *Store) runRecurrence(ctx context.Context, id uuid.UUID) (int, error) {
	ctx = withSystem(ctx)
	created := 0
	var failure *ValidationError
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var (
			wsID, projectID uuid.UUID
			r               Recurrence
		)
		// Lock order everywhere: workspace (calendar) first, then the rule, so a
		// concurrent calendar change (rescheduleRecurrences) cannot deadlock.
		if err := tx.QueryRow(ctx, `SELECT workspace_id FROM recurrences WHERE id = $1`, id).Scan(&wsID); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		cal, err := loadCalendar(ctx, tx, wsID, true)
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `SELECT r.project_id FROM recurrences r WHERE r.id = $1 AND r.is_enabled
			FOR UPDATE SKIP LOCKED`, id)
		if err := row.Scan(&projectID); errors.Is(err, pgx.ErrNoRows) {
			return nil // disabled, deleted or being run by another replica
		} else if err != nil {
			return err
		}
		r, err = scanRecurrence(tx.QueryRow(ctx, recurrenceSelect+` WHERE r.id = $1`, id), cal)
		if err != nil {
			return err
		}
		sched, err := r.input().Schedule()
		if err != nil {
			return err
		}
		sched.Cal = cal
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		fields := map[string]json.RawMessage{}
		for k, v := range r.Fields {
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			fields[k] = raw
		}
		// Missed dates from next_due on are caught up; a date that already has
		// an item (same due date, or same nominal date before a calendar
		// change moved it) is skipped.
		var after *recur.Occurrence
		var next *string
		if r.NextDue != nil {
			from, err := time.Parse(time.DateOnly, *r.NextDue)
			if err != nil {
				return err
			}
			for i := 0; ; i++ {
				occ, ok := sched.NextAfter(from, after)
				if !ok {
					break
				}
				v := occ.Due.Format(time.DateOnly)
				next = &v
				if i == maxCatchUpPerRule || occ.Due.AddDate(0, 0, -r.LeadDays).After(today) {
					break
				}
				var done bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recurrence_runs WHERE recurrence_id = $1
						AND (due_date = $2::date OR coalesce(nominal_date, due_date) = $3::date))`,
					id, v, occ.Nominal.Format(time.DateOnly)).Scan(&done); err != nil {
					return err
				}
				if !done {
					item, err := createItemTx(ctx, tx, CreateItemParams{
						WorkspaceID: wsID, ProjectID: projectID, Title: RenderRecurrenceTitle(r.Title, occ.Due),
						Description: r.Description, TypeKey: r.Type.Key, AssigneeID: r.AssigneeID, DueDate: &v, Fields: fields,
					}, map[string]any{"recurrence_id": id})
					if errors.As(err, &failure) {
						return err
					}
					if err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `INSERT INTO recurrence_runs (workspace_id, recurrence_id, due_date, item_id, nominal_date)
						VALUES ($1, $2, $3::date, $4, $5::date)`, wsID, id, v, item.ID, occ.Nominal.Format(time.DateOnly)); err != nil {
						return err
					}
					created++
				}
				after = &occ
				next = nil
			}
		}
		_, err = tx.Exec(ctx, `UPDATE recurrences SET next_due = $1::date, last_error = NULL WHERE id = $2`, next, id)
		return err
	})
	if failure == nil {
		return created, err
	}
	// The rule no longer produces a valid item (e.g. a field became required,
	// the assignee left): disable it once instead of failing every minute.
	msg := fmt.Sprintf("%s: %s", failure.Field, failure.Message)
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var wsID uuid.UUID
		if err := tx.QueryRow(ctx, `UPDATE recurrences SET is_enabled = false, last_error = $1 WHERE id = $2 RETURNING workspace_id`,
			msg, id).Scan(&wsID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, uuid.Nil, "recurrence", id, "recurrence.failed", map[string]any{"error": msg})
	})
	return 0, err
}
