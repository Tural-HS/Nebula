package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/recur"
)

// Working-day calendar of a workspace (ADR-0027).

// Calendar entry kinds and limits.
const (
	CalendarHoliday = "holiday" // a non-working date
	CalendarWorkday = "workday" // a weekend date declared a working day
	// minCalendarYear..maxCalendarYear bound stored dates (also a DB CHECK).
	minCalendarYear = 2000
	maxCalendarYear = 2100
	// maxCalendarRange bounds a GET by from/to (a calendar view spans a few weeks).
	maxCalendarRange = 400
)

// CalendarDay is a holiday or a working-day transfer.
type CalendarDay struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// WorkCalendar is the calendar between two dates (inclusive).
type WorkCalendar struct {
	WeekendDays []int         `json:"weekend_days"`
	From        string        `json:"from"`
	To          string        `json:"to"`
	Days        []CalendarDay `json:"days"`
}

// GetCalendar returns the weekend days and the dates between from and to.
func (s *Store) GetCalendar(ctx context.Context, wsID uuid.UUID, from, to time.Time) (WorkCalendar, error) {
	from, to = recur.Date(from), recur.Date(to)
	if to.Before(from) {
		return WorkCalendar{}, invalid("to", "must not be before from")
	}
	if to.Sub(from).Hours()/24 >= maxCalendarRange {
		return WorkCalendar{}, invalid("to", "the range can be at most %d days", maxCalendarRange)
	}
	out := WorkCalendar{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Days: []CalendarDay{}}
	var weekend []int16
	if err := s.pool.QueryRow(ctx, `SELECT weekend_days FROM workspaces WHERE id = $1`, wsID).Scan(&weekend); err != nil {
		return WorkCalendar{}, err
	}
	out.WeekendDays = sortedWeekdays(weekend)
	rows, err := s.pool.Query(ctx, `SELECT day::text, kind, name FROM workspace_calendar_days
		WHERE workspace_id = $1 AND day BETWEEN $2::date AND $3::date ORDER BY day`,
		wsID, out.From, out.To)
	if err != nil {
		return WorkCalendar{}, err
	}
	days, err := pgx.CollectRows(rows, pgx.RowToStructByPos[CalendarDay])
	if err != nil {
		return WorkCalendar{}, err
	}
	if days != nil {
		out.Days = days
	}
	return out, nil
}

// sortedWeekdays returns weekday numbers Monday first (1..6, then 0).
func sortedWeekdays(in []int16) []int {
	out := make([]int, len(in))
	for i, d := range in {
		out[i] = int(d)
	}
	slices.SortFunc(out, func(a, b int) int { return (a+6)%7 - (b+6)%7 })
	return out
}

// loadCalendar reads a workspace's whole calendar for date computations. With
// lock, the workspace row is share-locked so a concurrent calendar change
// waits for (or is waited for by) the caller and next_due never goes stale.
func loadCalendar(ctx context.Context, q querier, wsID uuid.UUID, lock bool) (*recur.Calendar, error) {
	sql := `SELECT weekend_days FROM workspaces WHERE id = $1`
	if lock {
		sql += ` FOR SHARE`
	}
	var weekend []int16
	if err := q.QueryRow(ctx, sql, wsID).Scan(&weekend); err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT day, kind FROM workspace_calendar_days WHERE workspace_id = $1`, wsID)
	if err != nil {
		return nil, err
	}
	var holidays, workdays []time.Time
	var day time.Time
	var kind string
	if _, err := pgx.ForEachRow(rows, []any{&day, &kind}, func() error {
		if kind == CalendarWorkday {
			workdays = append(workdays, recur.Date(day))
		} else {
			holidays = append(holidays, recur.Date(day))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	wd := make([]time.Weekday, len(weekend))
	for i, d := range weekend {
		wd[i] = time.Weekday(d)
	}
	return recur.NewCalendar(wd, holidays, workdays)
}

// lockCalendar serializes calendar changes of a workspace and blocks rule
// computations (loadCalendar with lock) until the change commits. NO KEY
// UPDATE does not block foreign-key checks, so item writes are unaffected.
func lockCalendar(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) ([]int16, error) {
	var weekend []int16
	err := tx.QueryRow(ctx, `SELECT weekend_days FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, wsID).Scan(&weekend)
	return weekend, err
}

// SetWeekend replaces the weekend days (0 = Sunday .. 6 = Saturday).
func (s *Store) SetWeekend(ctx context.Context, wsID, actorID uuid.UUID, days []int) error {
	if days == nil {
		return invalid("weekend_days", "is required (an empty list means no weekend)")
	}
	set := weekdaysArray(days)
	if len(set) != len(days) {
		return invalid("weekend_days", "must be distinct weekday numbers 0 (Sunday) to 6 (Saturday)")
	}
	if len(set) > recur.MaxWeekendDays {
		return invalid("weekend_days", "a week can have at most %d weekend days", recur.MaxWeekendDays)
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := lockCalendar(ctx, tx, wsID)
		if err != nil {
			return err
		}
		before, after := sortedWeekdays(cur), sortedWeekdays(set)
		if slices.Equal(before, after) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE workspaces SET weekend_days = $1 WHERE id = $2`, set, wsID); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "calendar", wsID, "calendar.weekend_updated",
			map[string]any{"changes": map[string]any{"weekend_days": diff(before, after)}}); err != nil {
			return err
		}
		return rescheduleRecurrences(ctx, tx, wsID, actorID)
	})
}

// CalendarDayInput sets one date.
type CalendarDayInput struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func parseCalendarDate(s string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, invalid("date", "must be a date YYYY-MM-DD")
	}
	if d.Year() < minCalendarYear || d.Year() > maxCalendarYear {
		return time.Time{}, invalid("date", "must be between %d and %d", minCalendarYear, maxCalendarYear)
	}
	return d, nil
}

// SetCalendarDay creates or replaces the entry of one date.
func (s *Store) SetCalendarDay(ctx context.Context, wsID, actorID uuid.UUID, date string, in CalendarDayInput) (CalendarDay, error) {
	d, err := parseCalendarDate(date)
	if err != nil {
		return CalendarDay{}, err
	}
	if in.Kind != CalendarHoliday && in.Kind != CalendarWorkday {
		return CalendarDay{}, invalid("kind", "must be holiday or workday")
	}
	name, err := text("name", in.Name, 1, 100)
	if err != nil {
		return CalendarDay{}, err
	}
	out := CalendarDay{Date: d.Format(time.DateOnly), Kind: in.Kind, Name: name}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		weekend, err := lockCalendar(ctx, tx, wsID)
		if err != nil {
			return err
		}
		if in.Kind == CalendarWorkday && !slices.Contains(weekend, int16(d.Weekday())) { // #nosec G115 -- 0..6
			return invalid("kind", "only a weekend date can be declared a working day")
		}
		var prev *CalendarDay
		var p CalendarDay
		err = tx.QueryRow(ctx, `SELECT day::text, kind, name FROM workspace_calendar_days WHERE workspace_id = $1 AND day = $2::date`,
			wsID, out.Date).Scan(&p.Date, &p.Kind, &p.Name)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if err := checkYearLimit(ctx, tx, wsID, d.Year(), 1); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			prev = &p
		}
		if prev != nil && *prev == out {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_calendar_days (workspace_id, day, kind, name, created_by)
			VALUES ($1, $2::date, $3, $4, $5)
			ON CONFLICT (workspace_id, day) DO UPDATE SET kind = EXCLUDED.kind, name = EXCLUDED.name`,
			wsID, out.Date, out.Kind, out.Name, actorID); err != nil {
			return err
		}
		payload := map[string]any{"date": out.Date, "kind": out.Kind, "name": out.Name}
		if prev != nil {
			payload["previous"] = map[string]any{"kind": prev.Kind, "name": prev.Name}
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "calendar", wsID, "calendar.day_set", payload); err != nil {
			return err
		}
		if prev != nil && prev.Kind == out.Kind {
			return nil // renamed only: no date moves
		}
		return rescheduleRecurrences(ctx, tx, wsID, actorID)
	})
	return out, err
}

func checkYearLimit(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, year, adding int) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workspace_calendar_days
		WHERE workspace_id = $1 AND day >= make_date($2, 1, 1) AND day < make_date($2 + 1, 1, 1)`, wsID, year).Scan(&n); err != nil {
		return err
	}
	if n+adding > recur.MaxDaysPerYear {
		return invalid("date", "a year can have at most %d holidays and working-day transfers", recur.MaxDaysPerYear)
	}
	return nil
}

// DeleteCalendarDay removes the entry of one date.
func (s *Store) DeleteCalendarDay(ctx context.Context, wsID, actorID uuid.UUID, date string) error {
	d, err := parseCalendarDate(date)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := lockCalendar(ctx, tx, wsID); err != nil {
			return err
		}
		var kind, name string
		err := tx.QueryRow(ctx, `DELETE FROM workspace_calendar_days WHERE workspace_id = $1 AND day = $2::date RETURNING kind, name`,
			wsID, d.Format(time.DateOnly)).Scan(&kind, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "calendar", wsID, "calendar.day_deleted",
			map[string]any{"date": d.Format(time.DateOnly), "kind": kind, "name": name}); err != nil {
			return err
		}
		return rescheduleRecurrences(ctx, tx, wsID, actorID)
	})
}

// azFixedHolidays are the fixed-date non-working days of Azerbaijan, Labour
// Code art. 105.2 as published by the Cabinet of Ministers (ADR-0027 cites the
// source). Novruz, Ramazan and Qurban bayramı and the day transfers are
// announced every year and are not here: the admin adds them.
var azFixedHolidays = []struct {
	month time.Month
	day   int
	name  string
}{
	{time.January, 1, "Yeni il bayramı"},
	{time.January, 2, "Yeni il bayramı"},
	{time.March, 8, "Qadınlar günü"},
	{time.May, 9, "Faşizm üzərində qələbə günü"},
	{time.May, 28, "Müstəqillik Günü"},
	{time.June, 15, "Azərbaycan xalqının milli qurtuluş günü"},
	{time.June, 26, "Azərbaycan Respublikasının Silahlı Qüvvələri günü"},
	{time.November, 8, "Zəfər Günü"},
	{time.November, 9, "Azərbaycan Respublikasının Dövlət bayrağı günü"},
	{time.December, 31, "Dünya azərbaycanlılarının həmrəyliyi günü"},
}

// AddAzerbaijanFixedHolidays adds the fixed-date holidays of one year. Dates
// that already have an entry (the admin's own name, or a working-day
// transfer) are left alone. Returns the added dates.
func (s *Store) AddAzerbaijanFixedHolidays(ctx context.Context, wsID, actorID uuid.UUID, year int) ([]CalendarDay, error) {
	if year < minCalendarYear || year > maxCalendarYear {
		return nil, invalid("year", "must be between %d and %d", minCalendarYear, maxCalendarYear)
	}
	added := []CalendarDay{}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := lockCalendar(ctx, tx, wsID); err != nil {
			return err
		}
		var candidates []CalendarDay
		for _, h := range azFixedHolidays {
			candidates = append(candidates, CalendarDay{
				Date: time.Date(year, h.month, h.day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly),
				Kind: CalendarHoliday, Name: h.name,
			})
		}
		rows, err := tx.Query(ctx, `SELECT day::text FROM workspace_calendar_days
			WHERE workspace_id = $1 AND day >= make_date($2, 1, 1) AND day < make_date($2 + 1, 1, 1)`, wsID, year)
		if err != nil {
			return err
		}
		existing, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, c := range candidates {
			if !slices.Contains(existing, c.Date) {
				added = append(added, c)
			}
		}
		if len(added) == 0 {
			return nil
		}
		if err := checkYearLimit(ctx, tx, wsID, year, len(added)); err != nil {
			return err
		}
		dates := make([]string, len(added))
		for i, c := range added {
			dates[i] = c.Date
			if _, err := tx.Exec(ctx, `INSERT INTO workspace_calendar_days (workspace_id, day, kind, name, created_by)
				VALUES ($1, $2::date, $3, $4, $5)`, wsID, c.Date, c.Kind, c.Name, actorID); err != nil {
				return err
			}
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "calendar", wsID, "calendar.preset_applied",
			map[string]any{"preset": "az-fixed", "year": year, "dates": dates}); err != nil {
			return err
		}
		return rescheduleRecurrences(ctx, tx, wsID, actorID)
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

// rescheduleRecurrences recomputes next_due of the workspace's rules whose
// dates depend on the calendar, after a calendar change in tx. Like editing a
// rule it starts from today: it never creates items for past dates, and never
// a second item for a date the rule already produced (lastRun).
func rescheduleRecurrences(ctx context.Context, tx pgx.Tx, wsID, actorID uuid.UUID) error {
	// Formulas using workdays() follow the calendar too (ADR-0030).
	if err := requestFormulaRecompute(ctx, tx, wsID, "calendar"); err != nil {
		return err
	}
	cal, err := loadCalendar(ctx, tx, wsID, false)
	if err != nil {
		return err
	}
	today, err := workspaceToday(ctx, tx, wsID)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, recurrenceSelect+` WHERE r.workspace_id = $1 AND r.next_due IS NOT NULL
		AND (r.work_day IS NOT NULL OR r.shift <> 'none') ORDER BY r.id FOR UPDATE OF r`, wsID)
	if err != nil {
		return err
	}
	var rules []Recurrence
	for rows.Next() {
		r, err := scanRecurrence(rows, nil)
		if err != nil {
			rows.Close()
			return err
		}
		rules = append(rules, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range rules {
		sched, err := r.input().Schedule()
		if err != nil {
			continue // a stored rule that no longer validates keeps its date
		}
		sched.Cal = cal
		last, err := lastRun(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		next := firstDue(sched, today, last)
		var nextDue *string
		if next != nil {
			v := next.Format(time.DateOnly)
			nextDue = &v
		}
		if equalPtr(nextDue, r.NextDue) {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE recurrences SET next_due = $1::date WHERE id = $2`, nextDue, r.ID); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "recurrence", r.ID, "recurrence.rescheduled",
			map[string]any{"title": r.Title, "reason": "calendar", "changes": map[string]any{"next_due": diff(r.NextDue, nextDue)}}); err != nil {
			return err
		}
	}
	return nil
}

// lastRun is the latest occurrence a rule already created an item for.
func lastRun(ctx context.Context, q querier, id uuid.UUID) (*recur.Occurrence, error) {
	var nominal, due *time.Time
	err := q.QueryRow(ctx, `SELECT coalesce(nominal_date, due_date), due_date FROM recurrence_runs
		WHERE recurrence_id = $1 ORDER BY coalesce(nominal_date, due_date) DESC, due_date DESC LIMIT 1`, id).Scan(&nominal, &due)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &recur.Occurrence{Nominal: recur.Date(*nominal), Due: recur.Date(*due)}, nil
}

// ParseCalendarYear parses a ?year= value.
func ParseCalendarYear(v string) (int, error) {
	y, err := strconv.Atoi(v)
	if err != nil || y < minCalendarYear || y > maxCalendarYear {
		return 0, invalid("year", "must be a year between %d and %d", minCalendarYear, maxCalendarYear)
	}
	return y, nil
}

// isWorkday reports whether a date (YYYY-MM-DD) is a working day of the workspace.
func isWorkday(ctx context.Context, q querier, wsID uuid.UUID, date string) (bool, error) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return false, err
	}
	cal, err := loadCalendar(ctx, q, wsID, false)
	if err != nil {
		return false, err
	}
	return cal.IsWorkday(d), nil
}
