// Package recur computes the dates of recurring items (ADR-0012) and their
// working-day options (ADR-0027). It works on calendar dates only (no clock,
// no time zone), so the same rule gives the same dates everywhere; the caller
// supplies "today" in the workspace's time zone (see Today).
package recur

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Frequencies.
const (
	Daily   = "daily"
	Weekly  = "weekly"
	Monthly = "monthly"
	Yearly  = "yearly"
)

// LastDay as MonthDay means the last day of the month; as WorkDay, the last
// working day of the month.
const LastDay = -1

// Shift policies for a date that lands on a non-working day (ADR-0027).
const (
	ShiftNone     = "none"
	ShiftPrevious = "previous"
	ShiftNext     = "next"
)

// MaxWorkDay bounds "the N-th working day of the month".
const MaxWorkDay = 23

// Occurrence is one date of a schedule. Nominal is the date the rule names
// (the 31st, every Monday, ...); Due is the date the item is due after the
// shift policy (equal to Nominal without a shift). For working-day rules
// Nominal is the first day of the month the rule names, so the occurrence
// keeps its identity when a calendar change moves its working day.
type Occurrence struct {
	Nominal time.Time
	Due     time.Time
}

// Schedule is a recurrence rule. Dates are calendar dates (time at midnight
// UTC, only year/month/day matter).
type Schedule struct {
	Freq     string         // daily | weekly | monthly | yearly
	Interval int            // every N units, >= 1
	Weekdays []time.Weekday // weekly: which days (at least one)
	MonthDay int            // monthly/yearly: 1..31 or LastDay (0 with WorkDay)
	WorkDay  int            // monthly/yearly: N-th working day 1..MaxWorkDay or LastDay; 0 = not used
	Month    time.Month     // yearly: 1..12
	Shift    string         // ShiftNone ("" too), ShiftPrevious, ShiftNext; not with WorkDay
	Start    time.Time      // first possible nominal date (also the anchor for intervals)
	End      *time.Time     // last possible nominal date, inclusive; nil = no end
	Cal      *Calendar      // working days; nil = Saturday and Sunday off
}

// usesCalendar reports whether the dates depend on the working-day calendar.
func (s Schedule) usesCalendar() bool {
	return s.WorkDay != 0 || (s.Shift != "" && s.Shift != ShiftNone)
}

// UsesCalendar reports whether a calendar change can move this schedule's dates.
func (s Schedule) UsesCalendar() bool { return s.usesCalendar() }

// Validate reports the first problem with a schedule.
func (s Schedule) Validate() error {
	if s.Interval < 1 || s.Interval > 366 {
		return errors.New("interval must be 1 to 366")
	}
	if s.End != nil && s.End.Before(s.Start) {
		return errors.New("end date is before the start date")
	}
	switch s.Shift {
	case "", ShiftNone, ShiftPrevious, ShiftNext:
	default:
		return fmt.Errorf("unknown shift %q", s.Shift)
	}
	if s.WorkDay != 0 {
		if s.Freq != Monthly && s.Freq != Yearly {
			return errors.New("working-day rules need a monthly or yearly frequency")
		}
		if s.WorkDay != LastDay && (s.WorkDay < 1 || s.WorkDay > MaxWorkDay) {
			return fmt.Errorf("working day must be 1 to %d or -1 (last working day)", MaxWorkDay)
		}
		if s.MonthDay != 0 {
			return errors.New("use either a month day or a working day, not both")
		}
		if s.usesShift() {
			return errors.New("a working-day rule cannot shift: it is always on a working day")
		}
	}
	switch s.Freq {
	case Daily:
	case Weekly:
		if len(s.Weekdays) == 0 || len(s.Weekdays) > 7 {
			return errors.New("weekly schedules need 1 to 7 weekdays")
		}
		for _, d := range s.Weekdays {
			if d < time.Sunday || d > time.Saturday {
				return errors.New("weekday must be 0 (Sunday) to 6 (Saturday)")
			}
		}
	case Monthly, Yearly:
		if s.WorkDay == 0 && s.MonthDay != LastDay && (s.MonthDay < 1 || s.MonthDay > 31) {
			return errors.New("month day must be 1 to 31 or -1 (last day)")
		}
		if s.Freq == Yearly && (s.Month < time.January || s.Month > time.December) {
			return errors.New("month must be 1 to 12")
		}
	default:
		return fmt.Errorf("unknown frequency %q", s.Freq)
	}
	return nil
}

// Date normalizes t to a calendar date (midnight UTC).
func Date(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s Schedule) usesShift() bool { return s.Shift == ShiftPrevious || s.Shift == ShiftNext }

// maxScan bounds the occurrences NextAfter looks at: the look-back window of
// a forward shift plus occurrences absorbed by a long non-working stretch.
const maxScan = 4 * MaxSearchDays

// NextAfter returns the first occurrence that
//   - is not before from: its nominal or its due date is on or after from (a
//     date moved back to a working day before from still counts while the
//     date the rule names has not passed, so a late calendar change never
//     silently drops a deadline), and
//   - comes after the occurrence `after` (nil = none): a later nominal date
//     and a later due date. Occurrences whose due date collapses onto an
//     earlier one (Saturday and Sunday both moved to Monday) are skipped, so
//     there is at most one item per due date.
//
// ok is false when the schedule has ended or is invalid.
func (s Schedule) NextAfter(from time.Time, after *Occurrence) (Occurrence, bool) {
	if s.Validate() != nil {
		return Occurrence{}, false
	}
	from = Date(from)
	scan := from
	if s.Shift == ShiftNext {
		// A nominal date up to MaxSearchDays earlier can move forward onto from.
		scan = from.AddDate(0, 0, -MaxSearchDays)
	}
	if after != nil && !scan.After(after.Nominal) {
		scan = Date(after.Nominal).AddDate(0, 0, 1)
	}
	for range maxScan {
		n, ok := s.nominal(scan)
		if !ok {
			return Occurrence{}, false
		}
		o := Occurrence{Nominal: n, Due: s.due(n)}
		key := o.Due
		if o.Nominal.After(key) {
			key = o.Nominal
		}
		if s.WorkDay != 0 {
			// The rule names a month, not a date: a calendar change that moves
			// "the 1st working day" from the 2nd to the 5th is the same
			// occurrence, never a second one.
			o.Nominal = time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		if (after == nil || (o.Nominal.After(Date(after.Nominal)) && o.Due.After(Date(after.Due)))) && !key.Before(from) {
			return o, true
		}
		scan = n.AddDate(0, 0, 1)
	}
	return Occurrence{}, false
}

// Next returns the due date of the first occurrence on or after from (see
// NextAfter), and false when the schedule has ended (or is invalid).
func (s Schedule) Next(from time.Time) (time.Time, bool) {
	o, ok := s.NextAfter(from, nil)
	return o.Due, ok
}

// Upcoming returns the due dates of up to n occurrences on or after from.
func (s Schedule) Upcoming(from time.Time, n int) []time.Time {
	var out []time.Time
	var after *Occurrence
	for len(out) < n {
		o, ok := s.NextAfter(from, after)
		if !ok {
			break
		}
		out = append(out, o.Due)
		after = &o
	}
	return out
}

// due applies the shift policy to a nominal date. Working-day rules are
// already on a working day.
func (s Schedule) due(n time.Time) time.Time {
	dir := 0
	switch s.Shift {
	case ShiftPrevious:
		dir = -1
	case ShiftNext:
		dir = 1
	}
	if dir == 0 {
		return n
	}
	if d, ok := s.Cal.shift(n, dir); ok {
		return d
	}
	return n // unreachable within the calendar limits
}

// nominal returns the first nominal date on or after from (and on or after
// the start), and false when the schedule has ended.
func (s Schedule) nominal(from time.Time) (time.Time, bool) {
	start := Date(s.Start)
	if from.Before(start) {
		from = start
	}
	var d time.Time
	ok := true
	switch s.Freq {
	case Daily:
		days := int(from.Sub(start).Hours() / 24)
		steps := (days + s.Interval - 1) / s.Interval
		d = start.AddDate(0, 0, steps*s.Interval)
	case Weekly:
		d = s.nextWeekly(start, from)
	case Monthly:
		d, ok = s.nextMonthly(start, from, s.Interval, func(y int, m time.Month) (int, time.Month) { return y, m })
	case Yearly:
		d, ok = s.nextMonthly(start, from, 12*s.Interval, func(y int, _ time.Month) (int, time.Month) { return y, s.Month })
	}
	if !ok || (s.End != nil && d.After(Date(*s.End))) {
		return time.Time{}, false
	}
	return d, true
}

// nextWeekly: weeks are counted from the Monday of the start date's week, so
// "every 2 weeks on Mon and Thu" keeps a stable rhythm.
func (s Schedule) nextWeekly(start, from time.Time) time.Time {
	weekStart := func(t time.Time) time.Time {
		offset := (int(t.Weekday()) + 6) % 7 // Monday = 0
		return t.AddDate(0, 0, -offset)
	}
	anchor := weekStart(start)
	days := slices.Clone(s.Weekdays)
	slices.SortFunc(days, func(a, b time.Weekday) int { return (int(a)+6)%7 - (int(b)+6)%7 })
	for w := weekStart(from); ; w = w.AddDate(0, 0, 7) {
		weeks := int(w.Sub(anchor).Hours() / 24 / 7)
		if weeks%s.Interval != 0 {
			continue
		}
		for _, wd := range days {
			d := w.AddDate(0, 0, (int(wd)+6)%7)
			if !d.Before(from) && !d.Before(start) {
				return d
			}
		}
	}
}

// nextMonthly walks months from the start month in steps of `step` months.
// month maps (year, month) to the month to use (yearly rules pin it).
func (s Schedule) nextMonthly(start, from time.Time, step int, month func(int, time.Month) (int, time.Month)) (time.Time, bool) {
	y, m := month(start.Year(), start.Month())
	cur := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	// Jump close to `from` without skipping a valid month.
	if gap := (from.Year()-cur.Year())*12 + int(from.Month()) - int(cur.Month()) - step; gap > 0 {
		cur = cur.AddDate(0, gap/step*step, 0)
	}
	// Bounded: a working-day rule skips months without any working day, and a
	// calendar cannot block more than a few of them in a row.
	for range 2400 {
		d, ok := s.dayIn(cur.Year(), cur.Month())
		if ok && !d.Before(from) && !d.Before(start) {
			return d, true
		}
		cur = cur.AddDate(0, step, 0)
	}
	return time.Time{}, false
}

// dayIn is the rule's day in a month: a month day or a working day.
func (s Schedule) dayIn(y int, m time.Month) (time.Time, bool) {
	if s.WorkDay != 0 {
		return s.Cal.WorkdayIn(y, m, s.WorkDay)
	}
	return dayIn(y, m, s.MonthDay), true
}

// dayIn clamps the day to the month's length (31 -> 30, 28 or 29).
func dayIn(y int, m time.Month, day int) time.Time {
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day == LastDay || day > last {
		day = last
	}
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}
