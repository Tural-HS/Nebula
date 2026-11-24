package recur

import (
	"testing"
	"time"
)

// azLike is a synthetic calendar shaped like a typical year: Sat+Sun off,
// a few weekday holidays and one Saturday declared a working day.
func azLike(t *testing.T) *Calendar {
	t.Helper()
	c, err := NewCalendar(DefaultWeekend,
		[]time.Time{d("2026-01-01"), d("2026-01-02"), d("2026-03-20"), d("2026-04-30"), d("2026-07-31"), d("2026-12-31")},
		[]time.Time{d("2026-03-28")}) // Saturday 28 Mar declared a working day
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCalendarIsWorkday(t *testing.T) {
	c := azLike(t)
	cases := map[string]bool{
		"2026-01-01": false, // holiday (Thu)
		"2026-01-05": true,  // Monday
		"2026-01-03": false, // Saturday
		"2026-03-28": true,  // Saturday, transfer
		"2026-03-29": false, // Sunday
	}
	for s, want := range cases {
		if got := c.IsWorkday(d(s)); got != want {
			t.Errorf("%s: got %v, want %v", s, got, want)
		}
	}
	var none *Calendar
	if none.IsWorkday(d("2026-01-03")) || !none.IsWorkday(d("2026-01-01")) {
		t.Error("nil calendar: Sat/Sun off, no holidays")
	}
	// Custom weekend: Friday + Saturday.
	fs, err := NewCalendar([]time.Weekday{time.Friday, time.Saturday}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fs.IsWorkday(d("2026-01-02")) || !fs.IsWorkday(d("2026-01-04")) {
		t.Error("Fri+Sat weekend")
	}
}

func TestNewCalendarLimits(t *testing.T) {
	if _, err := NewCalendar([]time.Weekday{0, 1, 2, 3}, nil, nil); err == nil {
		t.Error("4 weekend days accepted")
	}
	if _, err := NewCalendar([]time.Weekday{7}, nil, nil); err == nil {
		t.Error("weekday 7 accepted")
	}
	var many []time.Time
	for i := range MaxDaysPerYear + 1 {
		many = append(many, d("2026-01-01").AddDate(0, 0, i))
	}
	if _, err := NewCalendar(DefaultWeekend, many, nil); err == nil {
		t.Error("too many dates in a year accepted")
	}
	if _, err := NewCalendar(DefaultWeekend, many[:MaxDaysPerYear], nil); err != nil {
		t.Errorf("exactly the limit: %v", err)
	}
}

func TestLastWorkingDay(t *testing.T) {
	c := azLike(t)
	s := Schedule{Freq: Monthly, Interval: 1, WorkDay: LastDay, Start: d("2026-01-01"), Cal: c}
	eq(t, "last working day", s.Upcoming(d("2026-01-01"), 12),
		"2026-01-30", // 31 Jan is a Saturday
		"2026-02-27", // 28 Feb Saturday
		"2026-03-31",
		"2026-04-29", // 30 Apr holiday
		"2026-05-29", // 31 May Sunday, 30 Saturday
		"2026-06-30",
		"2026-07-30", // 31 Jul holiday (Friday)
		"2026-08-31",
		"2026-09-30",
		"2026-10-30", // 31 Oct Saturday
		"2026-11-30",
		"2026-12-30", // 31 Dec holiday
	)
	// Leap year February: 29 Feb 2028 is a Tuesday.
	s = Schedule{Freq: Monthly, Interval: 1, WorkDay: LastDay, Start: d("2028-02-01")}
	eq(t, "leap February", s.Upcoming(d("2028-02-01"), 1), "2028-02-29")
	// Non-leap February 2027: 28 Feb is a Sunday.
	s.Start = d("2027-02-01")
	eq(t, "February 2027", s.Upcoming(d("2027-02-01"), 1), "2027-02-26")
	// Yearly: last working day of December.
	s = Schedule{Freq: Yearly, Interval: 1, Month: time.December, WorkDay: LastDay, Start: d("2026-01-01"), Cal: c}
	eq(t, "yearly December", s.Upcoming(d("2026-01-01"), 2), "2026-12-30", "2027-12-31")
}

func TestNthWorkingDay(t *testing.T) {
	c := azLike(t)
	s := Schedule{Freq: Monthly, Interval: 1, WorkDay: 1, Start: d("2026-01-01"), Cal: c}
	// 1-2 Jan holidays, 3-4 weekend: first working day is Monday 5 Jan.
	eq(t, "1st", s.Upcoming(d("2026-01-01"), 2), "2026-01-05", "2026-02-02")
	s.WorkDay = 5
	eq(t, "5th", s.Upcoming(d("2026-01-01"), 1), "2026-01-09")
	// March: 20th holiday, Saturday 28th is a working day.
	// Working days: 2..6, 9..13, 16..19, 23..27, 28, 30, 31 = 22.
	s.WorkDay = 19
	eq(t, "19th", s.Upcoming(d("2026-03-01"), 1), "2026-03-27")
	s.WorkDay = 20
	eq(t, "20th is the Saturday transfer", s.Upcoming(d("2026-03-01"), 1), "2026-03-28")
	s.WorkDay = 22
	eq(t, "22nd = 31 Mar", s.Upcoming(d("2026-03-01"), 1), "2026-03-31")
	s.WorkDay = 23
	eq(t, "23rd clamps to 31 Mar", s.Upcoming(d("2026-03-01"), 1), "2026-03-31")
	// February 2026 has 20 working days: the 23rd clamps to the last one.
	eq(t, "clamp", s.Upcoming(d("2026-02-01"), 1), "2026-02-27")
	// Every 3 months on the 10th working day.
	s = Schedule{Freq: Monthly, Interval: 3, WorkDay: 10, Start: d("2026-01-01")}
	eq(t, "quarterly", s.Upcoming(d("2026-01-01"), 3), "2026-01-14", "2026-04-14", "2026-07-14")
}

func TestShiftPolicies(t *testing.T) {
	c := azLike(t)
	base := Schedule{Freq: Monthly, Interval: 1, MonthDay: 31, Start: d("2026-01-01"), Cal: c}
	// 31 Jan Sat, 28 Feb Sat (clamped), 31 Mar Tue.
	none := base
	eq(t, "none", none.Upcoming(d("2026-01-01"), 3), "2026-01-31", "2026-02-28", "2026-03-31")
	prev := base
	prev.Shift = ShiftPrevious
	eq(t, "previous", prev.Upcoming(d("2026-01-01"), 3), "2026-01-30", "2026-02-27", "2026-03-31")
	next := base
	next.Shift = ShiftNext
	eq(t, "next", next.Upcoming(d("2026-01-01"), 3), "2026-02-02", "2026-03-02", "2026-03-31")
	// Holiday on the date itself: 31 Dec 2026 (Thursday).
	eq(t, "previous over holiday", prev.Upcoming(d("2026-12-01"), 1), "2026-12-30")
	// 31 Dec holiday, 1-2 Jan holidays, 3-4 Jan weekend -> Monday 5 Jan.
	eq(t, "next over holidays and year end", next.Upcoming(d("2026-12-01"), 1), "2027-01-01")

	// A Saturday transfer is a working day: no shift.
	s := Schedule{Freq: Weekly, Interval: 1, Weekdays: []time.Weekday{time.Saturday}, Start: d("2026-03-01"), Shift: ShiftNext, Cal: c}
	eq(t, "weekly Saturday next", s.Upcoming(d("2026-03-21"), 3), "2026-03-23", "2026-03-28", "2026-04-06")

	// Daily with a forward shift: Sat and Sun collapse onto Monday, one item.
	s = Schedule{Freq: Daily, Interval: 1, Start: d("2026-01-01"), Shift: ShiftNext, Cal: c}
	eq(t, "daily next collapses", s.Upcoming(d("2026-01-01"), 4), "2026-01-05", "2026-01-06", "2026-01-07", "2026-01-08")
	s.Shift = ShiftPrevious
	eq(t, "daily previous collapses", s.Upcoming(d("2026-01-08"), 3), "2026-01-08", "2026-01-09", "2026-01-12")
}

func TestNextAfterAndFrom(t *testing.T) {
	c := azLike(t)
	// 31 Jan 2026 (Sat) moves back to Friday 30 Jan. On Saturday the 31st the
	// deadline still counts: nominal date has not passed.
	s := Schedule{Freq: Monthly, Interval: 1, MonthDay: 31, Start: d("2026-01-01"), Shift: ShiftPrevious, Cal: c}
	o, ok := s.NextAfter(d("2026-01-31"), nil)
	if !ok || o.Due != d("2026-01-30") || o.Nominal != d("2026-01-31") {
		t.Fatalf("got %+v", o)
	}
	// After that occurrence the next one is February.
	o2, _ := s.NextAfter(d("2026-01-31"), &o)
	if o2.Nominal != d("2026-02-28") || o2.Due != d("2026-02-27") {
		t.Fatalf("got %+v", o2)
	}
	// An item already exists for nominal 31 Jan due 30 Jan. If the calendar
	// later moves the due date to 31 Jan (no shift), the same nominal date
	// is not produced again.
	plain := s
	plain.Shift = ShiftNone
	o3, _ := plain.NextAfter(d("2026-01-29"), &o)
	if o3.Nominal != d("2026-02-28") {
		t.Fatalf("duplicate of an existing nominal date: %+v", o3)
	}
	// Forward shift: a nominal Saturday before `from` lands on Monday after it.
	next := s
	next.Shift = ShiftNext
	o4, _ := next.NextAfter(d("2026-02-01"), nil)
	if o4.Nominal != d("2026-01-31") || o4.Due != d("2026-02-02") {
		t.Fatalf("got %+v", o4)
	}
}

func TestWorkingDayIdentityAcrossCalendarChange(t *testing.T) {
	// An item exists for "1st working day of February 2027" = Mon 1 Feb.
	s := Schedule{Freq: Monthly, Interval: 1, WorkDay: 1, Start: d("2027-01-01")}
	o, _ := s.NextAfter(d("2027-01-20"), nil)
	if o.Due != d("2027-02-01") || o.Nominal != d("2027-02-01") { // Feb 1st is also the month start
		t.Fatalf("got %+v", o)
	}
	// 1 Feb becomes a holiday: the 1st working day is now the 2nd, but it
	// is the same month's occurrence, so the next one is March.
	cal, _ := NewCalendar(DefaultWeekend, []time.Time{d("2027-02-01")}, nil)
	s.Cal = cal
	next, _ := s.NextAfter(d("2027-01-20"), &o)
	if next.Due != d("2027-03-01") {
		t.Fatalf("second item for February: %+v", next)
	}
}

func TestEndAppliesToNominalDate(t *testing.T) {
	end := d("2026-01-31")
	s := Schedule{Freq: Monthly, Interval: 1, MonthDay: 31, Start: d("2026-01-01"), End: &end, Shift: ShiftNext}
	eq(t, "nominal within end", s.Upcoming(d("2026-01-01"), 5), "2026-02-02")
}

func TestValidateWorkingDayOptions(t *testing.T) {
	bad := []Schedule{
		{Freq: Daily, Interval: 1, WorkDay: 1},
		{Freq: Monthly, Interval: 1, WorkDay: 24},
		{Freq: Monthly, Interval: 1, WorkDay: -2},
		{Freq: Monthly, Interval: 1, WorkDay: 1, MonthDay: 5},
		{Freq: Monthly, Interval: 1, WorkDay: LastDay, Shift: ShiftNext},
		{Freq: Monthly, Interval: 1, MonthDay: 1, Shift: "sideways"},
	}
	for i, s := range bad {
		if s.Validate() == nil {
			t.Errorf("case %d accepted: %+v", i, s)
		}
	}
	good := []Schedule{
		{Freq: Monthly, Interval: 1, WorkDay: MaxWorkDay},
		{Freq: Yearly, Interval: 1, Month: 12, WorkDay: LastDay},
		{Freq: Weekly, Interval: 1, Weekdays: []time.Weekday{1}, Shift: ShiftPrevious},
		{Freq: Daily, Interval: 1, Shift: ShiftNone},
	}
	for i, s := range good {
		if err := s.Validate(); err != nil {
			t.Errorf("case %d rejected: %v", i, err)
		}
	}
}

func TestToday(t *testing.T) {
	baku, err := time.LoadLocation("Asia/Baku") // UTC+4, no DST
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	cases := []struct {
		now  string
		loc  *time.Location
		want string
	}{
		{"2026-01-31T19:59:59Z", baku, "2026-01-31"},
		{"2026-01-31T20:00:00Z", baku, "2026-02-01"}, // midnight in Baku
		{"2026-03-01T04:59:59Z", ny, "2026-02-28"},
		{"2026-03-01T05:00:00Z", ny, "2026-03-01"},
		{"2028-02-29T23:30:00Z", time.UTC, "2028-02-29"},
		{"2026-11-01T05:30:00Z", ny, "2026-11-01"}, // DST ends that night
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.now)
		if got := Today(now, c.loc).Format(time.DateOnly); got != c.want {
			t.Errorf("%s in %s: got %s, want %s", c.now, c.loc, got, c.want)
		}
	}
	// Midnight in Baku on 1 Feb: "last working day of January" is already past.
	s := Schedule{Freq: Monthly, Interval: 1, WorkDay: LastDay, Start: d("2026-01-01")}
	now, _ := time.Parse(time.RFC3339, "2026-01-31T20:00:00Z")
	if got, _ := s.Next(Today(now, baku)); got != d("2026-02-27") {
		t.Errorf("after Baku midnight: %s", got.Format(time.DateOnly))
	}
	if got, _ := s.Next(Today(now, time.UTC)); got != d("2026-02-27") {
		// 31 Jan 2026 is a Saturday; the last working day was 30 Jan.
		t.Errorf("UTC: %s", got.Format(time.DateOnly))
	}
}

// FuzzWorkingDayResults: whatever the calendar (within its limits), a
// working-day rule or a shifted rule never lands on a non-working day, and
// results never go backwards.
func FuzzWorkingDayResults(f *testing.F) {
	f.Add(uint64(0x5a5a), uint8(6), int8(-1), uint8(0), uint16(0), uint8(1))
	f.Add(uint64(0xffffffffffff), uint8(0x41), int8(23), uint8(1), uint16(700), uint8(2))
	f.Add(uint64(1), uint8(0x7f), int8(1), uint8(2), uint16(30), uint8(0))
	f.Fuzz(func(t *testing.T, seed uint64, weekendMask uint8, workDay int8, mode uint8, offset uint16, block uint8) {
		var weekend []time.Weekday
		for wd := range 7 {
			if weekendMask&(1<<wd) != 0 && len(weekend) < MaxWeekendDays {
				weekend = append(weekend, time.Weekday(wd))
			}
		}
		// Holidays: a pseudo-random spread plus one contiguous block, per year.
		var holidays, workdays []time.Time
		x := seed | 1
		for _, y := range []int{2026, 2027} {
			start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
			blockLen := int(block % 40)
			for i := range blockLen {
				holidays = append(holidays, start.AddDate(0, 0, 300+i))
			}
			for range MaxDaysPerYear - blockLen - 2 {
				x ^= x << 13
				x ^= x >> 7
				x ^= x << 17
				day := start.AddDate(0, 0, int(x%300))
				if !containsDate(holidays, day) {
					holidays = append(holidays, day)
				}
			}
			for i := range 2 { // two Saturday transfers if not holidays
				sat := start.AddDate(0, 0, (6-int(start.Weekday())+7)%7+7*(20+i))
				if !containsDate(holidays, sat) {
					workdays = append(workdays, sat)
				}
			}
		}
		cal, err := NewCalendar(weekend, holidays, workdays)
		if err != nil {
			t.Fatal(err)
		}
		s := Schedule{Interval: 1, Start: d("2026-01-01"), Cal: cal}
		switch mode % 4 {
		case 0, 1:
			s.Freq = Monthly
			s.WorkDay = int(workDay%MaxWorkDay+MaxWorkDay)%MaxWorkDay + 1
			if workDay < 0 {
				s.WorkDay = LastDay
			}
			if mode%4 == 1 {
				s.Freq, s.Month = Yearly, time.Month(int(seed%12)+1)
			}
		case 2:
			s.Freq, s.MonthDay, s.Shift = Monthly, int(seed%31)+1, ShiftPrevious
		case 3:
			s.Freq, s.Shift = Daily, ShiftNext
		}
		from := d("2026-01-01").AddDate(0, 0, int(offset%730))
		var after *Occurrence
		for range 6 {
			o, ok := s.NextAfter(from, after)
			if !ok {
				t.Fatalf("%+v ended from %s", s, from.Format(time.DateOnly))
			}
			if !cal.IsWorkday(o.Due) {
				t.Fatalf("%+v: %s is not a working day", s, o.Due.Format(time.DateOnly))
			}
			if after != nil && (!o.Due.After(after.Due) || !o.Nominal.After(after.Nominal)) {
				t.Fatalf("not increasing: %+v then %+v", *after, o)
			}
			after = &o
		}
	})
}

func containsDate(ds []time.Time, t time.Time) bool {
	for _, x := range ds {
		if x.Equal(t) {
			return true
		}
	}
	return false
}
