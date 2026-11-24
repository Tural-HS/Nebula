package recur

import (
	"errors"
	"time"
)

// Calendar is a workspace's working-day calendar (ADR-0027): weekend days,
// non-working dates (holidays) and working-day transfers (a weekend date
// declared a working day). A nil *Calendar is the default: Saturday and
// Sunday off, no holidays.
type Calendar struct {
	weekend [7]bool
	// override: date key -> true (declared working day) / false (holiday).
	override map[int]bool
}

// Limits keep every working-day search bounded (see MaxSearchDays).
const (
	MaxWeekendDays = 3
	// MaxDaysPerYear bounds holidays + transfers in one calendar year.
	MaxDaysPerYear = 60
	// MaxSearchDays is how far a shift or working-day search looks. With at
	// most 3 weekend days and 60 holidays per year, the longest possible run
	// of non-working days is below 2*60/4 weeks (≈ 210 days), so a search of
	// this length always finds a working day.
	MaxSearchDays = 400
)

// DefaultWeekend is Saturday and Sunday.
var DefaultWeekend = []time.Weekday{time.Saturday, time.Sunday}

func dateKey(t time.Time) int { return t.Year()*10000 + int(t.Month())*100 + t.Day() }

// NewCalendar builds a calendar. A date in both holidays and workdays is a
// working day (the caller's store prevents it: one row per date).
func NewCalendar(weekend []time.Weekday, holidays, workdays []time.Time) (*Calendar, error) {
	c := &Calendar{override: make(map[int]bool, len(holidays)+len(workdays))}
	n := 0
	for _, wd := range weekend {
		if wd < time.Sunday || wd > time.Saturday {
			return nil, errors.New("weekday must be 0 (Sunday) to 6 (Saturday)")
		}
		if !c.weekend[wd] {
			n++
		}
		c.weekend[wd] = true
	}
	if n > MaxWeekendDays {
		return nil, errors.New("a week can have at most 3 weekend days")
	}
	perYear := map[int]int{}
	add := func(t time.Time, working bool) error {
		perYear[t.Year()]++
		if perYear[t.Year()] > MaxDaysPerYear {
			return errors.New("too many calendar dates in one year")
		}
		c.override[dateKey(t)] = working
		return nil
	}
	for _, h := range holidays {
		if err := add(h, false); err != nil {
			return nil, err
		}
	}
	for _, w := range workdays {
		if err := add(w, true); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// IsWeekend reports whether the weekday is a weekend day of the calendar.
func (c *Calendar) IsWeekend(wd time.Weekday) bool {
	if c == nil {
		return wd == time.Saturday || wd == time.Sunday
	}
	return c.weekend[wd]
}

// IsWorkday reports whether the calendar date of t is a working day.
func (c *Calendar) IsWorkday(t time.Time) bool {
	if c != nil {
		if working, ok := c.override[dateKey(t)]; ok {
			return working
		}
	}
	return !c.IsWeekend(t.Weekday())
}

// shift moves d to the nearest working day in direction dir (-1 or +1), d
// itself included. ok is false when none is found within MaxSearchDays.
func (c *Calendar) shift(d time.Time, dir int) (time.Time, bool) {
	for range MaxSearchDays {
		if c.IsWorkday(d) {
			return d, true
		}
		d = d.AddDate(0, 0, dir)
	}
	return time.Time{}, false
}

// WorkdayIn returns the n-th working day of a month (n >= 1), or the last one
// (n == LastDay). A month with fewer than n working days gives its last
// working day, like the 31st clamps to the last day; ok is false only for a
// month without any working day.
func (c *Calendar) WorkdayIn(y int, m time.Month, n int) (time.Time, bool) {
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	var last time.Time
	found := 0
	for d := first; d.Month() == m; d = d.AddDate(0, 0, 1) {
		if !c.IsWorkday(d) {
			continue
		}
		found++
		last = d
		if found == n {
			return d, true
		}
	}
	return last, found > 0
}

// Today is the calendar date of now in loc: the workspace's "today".
func Today(now time.Time, loc *time.Location) time.Time {
	return Date(now.In(loc))
}

// WorkdaysBetween counts the working days d with from < d <= to, or minus
// the count for to < d <= from when to is before from, so that
// WorkdaysBetween(today, due) is "working days left" and is 0 for today.
// It costs O(holidays + transfers), not O(days): whole weeks are counted
// arithmetically, then each override inside the range adjusts the count.
func (c *Calendar) WorkdaysBetween(from, to time.Time) int {
	from, to = Date(from), Date(to)
	if to.Before(from) {
		return -c.WorkdaysBetween(to, from)
	}
	n := int((to.Unix() - from.Unix()) / 86400)
	weekend := 0
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		if c.IsWeekend(wd) {
			weekend++
		}
	}
	count := n / 7 * (7 - weekend)
	wd := from.Weekday()
	for range n % 7 {
		wd = (wd + 1) % 7
		if !c.IsWeekend(wd) {
			count++
		}
	}
	if c == nil {
		return count
	}
	lo, hi := dateKey(from), dateKey(to)
	for k, working := range c.override {
		if k <= lo || k > hi {
			continue
		}
		d := time.Date(k/10000, time.Month(k/100%100), k%100, 0, 0, 0, 0, time.UTC)
		switch weekendDay := c.IsWeekend(d.Weekday()); {
		case working && weekendDay:
			count++
		case !working && !weekendDay:
			count--
		}
	}
	return count
}

// AddWorkdays returns the n-th working day after from (n >= 1): from itself
// never counts, so a deadline of one working day set on a Friday is the next
// Monday, and one set on a holiday is the first working day after it. It is
// the inverse of WorkdaysBetween: WorkdaysBetween(from, d) == n and d is a
// working day. ok is false if the search runs past n*MaxSearchDays days,
// which a valid calendar never does (see MaxSearchDays).
func (c *Calendar) AddWorkdays(from time.Time, n int) (time.Time, bool) {
	d := Date(from)
	if n < 1 {
		return d, false
	}
	for range n * MaxSearchDays {
		d = d.AddDate(0, 0, 1)
		if c.IsWorkday(d) {
			n--
			if n == 0 {
				return d, true
			}
		}
	}
	return time.Time{}, false
}
