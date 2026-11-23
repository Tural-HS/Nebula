// Package recur computes the dates of recurring items (ADR-0012). It works on
// calendar dates only (no clock, no time zone), so the same rule gives the
// same dates everywhere.
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

// LastDay as MonthDay means the last day of the month.
const LastDay = -1

// Schedule is a recurrence rule. Dates are calendar dates (time at midnight
// UTC, only year/month/day matter).
type Schedule struct {
	Freq     string         // daily | weekly | monthly | yearly
	Interval int            // every N units, >= 1
	Weekdays []time.Weekday // weekly: which days (at least one)
	MonthDay int            // monthly/yearly: 1..31 or LastDay
	Month    time.Month     // yearly: 1..12
	Start    time.Time      // first possible date (also the anchor for intervals)
	End      *time.Time     // last possible date, inclusive; nil = no end
}

// Validate reports the first problem with a schedule.
func (s Schedule) Validate() error {
	if s.Interval < 1 || s.Interval > 366 {
		return errors.New("interval must be 1 to 366")
	}
	if s.End != nil && s.End.Before(s.Start) {
		return errors.New("end date is before the start date")
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
		if s.MonthDay != LastDay && (s.MonthDay < 1 || s.MonthDay > 31) {
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

// Next returns the first occurrence on or after from, and false when the
// schedule has ended (or the schedule is invalid).
func (s Schedule) Next(from time.Time) (time.Time, bool) {
	if s.Validate() != nil {
		return time.Time{}, false
	}
	start := Date(s.Start)
	from = Date(from)
	if from.Before(start) {
		from = start
	}
	var d time.Time
	switch s.Freq {
	case Daily:
		days := int(from.Sub(start).Hours() / 24)
		steps := (days + s.Interval - 1) / s.Interval
		d = start.AddDate(0, 0, steps*s.Interval)
	case Weekly:
		d = s.nextWeekly(start, from)
	case Monthly:
		d = s.nextMonthly(start, from, s.Interval, func(y int, m time.Month) (int, time.Month) { return y, m })
	case Yearly:
		d = s.nextMonthly(start, from, 12*s.Interval, func(y int, _ time.Month) (int, time.Month) { return y, s.Month })
	}
	if s.End != nil && d.After(Date(*s.End)) {
		return time.Time{}, false
	}
	return d, true
}

// Upcoming returns up to n occurrences on or after from.
func (s Schedule) Upcoming(from time.Time, n int) []time.Time {
	var out []time.Time
	for len(out) < n {
		d, ok := s.Next(from)
		if !ok {
			break
		}
		out = append(out, d)
		from = d.AddDate(0, 0, 1)
	}
	return out
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
func (s Schedule) nextMonthly(start, from time.Time, step int, month func(int, time.Month) (int, time.Month)) time.Time {
	y, m := month(start.Year(), start.Month())
	cur := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	// Jump close to `from` without skipping a valid month.
	if gap := (from.Year()-cur.Year())*12 + int(from.Month()) - int(cur.Month()) - step; gap > 0 {
		cur = cur.AddDate(0, gap/step*step, 0)
	}
	for {
		d := dayIn(cur.Year(), cur.Month(), s.MonthDay)
		if !d.Before(from) && !d.Before(start) {
			return d
		}
		cur = cur.AddDate(0, step, 0)
	}
}

// dayIn clamps the day to the month's length (31 -> 30, 28 or 29).
func dayIn(y int, m time.Month, day int) time.Time {
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if day == LastDay || day > last {
		day = last
	}
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}
