package recur

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func dates(ts []time.Time) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Format(time.DateOnly)
	}
	return out
}

func eq(t *testing.T, name string, got []time.Time, want ...string) {
	t.Helper()
	g := dates(got)
	if len(g) != len(want) {
		t.Fatalf("%s: got %v, want %v", name, g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("%s: got %v, want %v", name, g, want)
		}
	}
}

func TestMonthly(t *testing.T) {
	// FinTez: a customer pays on the 9th of every month.
	s := Schedule{Freq: Monthly, Interval: 1, MonthDay: 9, Start: d("2026-11-23")}
	eq(t, "9th", s.Upcoming(d("2026-11-23"), 3), "2026-12-09", "2027-01-09", "2027-02-09")
	eq(t, "on the day itself", s.Upcoming(d("2026-12-09"), 1), "2026-12-09")

	// The 31st clamps to the month's last day, then returns to 31.
	s = Schedule{Freq: Monthly, Interval: 1, MonthDay: 31, Start: d("2027-01-01")}
	eq(t, "31st", s.Upcoming(d("2027-01-01"), 4), "2027-01-31", "2027-02-28", "2027-03-31", "2027-04-30")
	s.Start = d("2028-02-01")
	eq(t, "leap year", s.Upcoming(d("2028-02-01"), 1), "2028-02-29")

	s = Schedule{Freq: Monthly, Interval: 1, MonthDay: LastDay, Start: d("2026-11-01")}
	eq(t, "last day", s.Upcoming(d("2026-11-01"), 3), "2026-11-30", "2026-12-31", "2027-01-31")

	// Every 3 months keeps the rhythm of the start month even when asked far later.
	s = Schedule{Freq: Monthly, Interval: 3, MonthDay: 15, Start: d("2026-01-01")}
	eq(t, "quarterly", s.Upcoming(d("2026-01-01"), 3), "2026-01-15", "2026-04-15", "2026-07-15")
	eq(t, "quarterly far ahead", s.Upcoming(d("2030-05-20"), 2), "2030-07-15", "2030-10-15")
}

func TestDailyWeeklyYearly(t *testing.T) {
	s := Schedule{Freq: Daily, Interval: 2, Start: d("2026-11-23")}
	eq(t, "every 2 days", s.Upcoming(d("2026-11-24"), 3), "2026-11-25", "2026-11-27", "2026-11-29")

	// 2026-11-23 is a Monday.
	s = Schedule{Freq: Weekly, Interval: 1, Weekdays: []time.Weekday{time.Thursday, time.Monday}, Start: d("2026-11-23")}
	eq(t, "Mon+Thu", s.Upcoming(d("2026-11-23"), 4), "2026-11-23", "2026-11-26", "2026-11-30", "2026-12-03")
	s.Interval = 2
	eq(t, "every 2 weeks", s.Upcoming(d("2026-11-24"), 3), "2026-11-26", "2026-12-07", "2026-12-10")
	s = Schedule{Freq: Weekly, Interval: 1, Weekdays: []time.Weekday{time.Sunday}, Start: d("2026-11-23")}
	eq(t, "Sunday ends the week", s.Upcoming(d("2026-11-23"), 2), "2026-11-29", "2026-12-06")

	s = Schedule{Freq: Yearly, Interval: 1, Month: time.February, MonthDay: 29, Start: d("2026-01-01")}
	eq(t, "29 Feb yearly", s.Upcoming(d("2026-01-01"), 3), "2026-02-28", "2027-02-28", "2028-02-29")
	s = Schedule{Freq: Yearly, Interval: 1, Month: time.March, MonthDay: 30, Start: d("2026-11-23")}
	eq(t, "birthday after start", s.Upcoming(d("2026-11-23"), 2), "2027-03-30", "2028-03-30")
}

func TestStartAndEnd(t *testing.T) {
	end := d("2027-02-09")
	s := Schedule{Freq: Monthly, Interval: 1, MonthDay: 9, Start: d("2026-12-01"), End: &end}
	eq(t, "until end (inclusive)", s.Upcoming(d("2020-01-01"), 10), "2026-12-09", "2027-01-09", "2027-02-09")
	if _, ok := s.Next(d("2027-02-10")); ok {
		t.Fatal("next after end")
	}
}

func TestValidate(t *testing.T) {
	end := d("2026-01-01")
	bad := []Schedule{
		{Freq: "hourly", Interval: 1},
		{Freq: Daily, Interval: 0},
		{Freq: Weekly, Interval: 1},
		{Freq: Weekly, Interval: 1, Weekdays: []time.Weekday{7}},
		{Freq: Monthly, Interval: 1, MonthDay: 32},
		{Freq: Monthly, Interval: 1, MonthDay: 0},
		{Freq: Yearly, Interval: 1, MonthDay: 1, Month: 13},
		{Freq: Daily, Interval: 1, Start: d("2026-02-01"), End: &end},
	}
	for i, s := range bad {
		if s.Validate() == nil {
			t.Errorf("case %d accepted: %+v", i, s)
		}
		if _, ok := s.Next(d("2026-01-01")); ok {
			t.Errorf("case %d: Next on invalid schedule", i)
		}
	}
}

func FuzzNextIsOnOrAfterAndValid(f *testing.F) {
	f.Add(3, 1, 9, 2, 0)
	f.Add(2, 2, 31, 2, 400)
	f.Fuzz(func(t *testing.T, freq, interval, day, month, offset int) {
		s := Schedule{
			Freq: []string{Daily, Weekly, Monthly, Yearly}[((freq%4)+4)%4], Interval: ((interval%12)+12)%12 + 1,
			Weekdays: []time.Weekday{time.Weekday(((day % 7) + 7) % 7)}, MonthDay: ((day%31)+31)%31 + 1,
			Month: time.Month(((month%12)+12)%12 + 1), Start: d("2026-01-31"),
		}
		from := d("2026-01-31").AddDate(0, 0, ((offset%2000)+2000)%2000)
		got, ok := s.Next(from)
		if !ok || got.Before(from) {
			t.Fatalf("%+v from %s: %s %v", s, from.Format(time.DateOnly), got.Format(time.DateOnly), ok)
		}
		if again, _ := s.Next(got); !again.Equal(got) {
			t.Fatalf("not stable: %s then %s", got, again)
		}
	})
}
