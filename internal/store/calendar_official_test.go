package store

import (
	"slices"
	"testing"
	"time"
)

// TestAzOfficialCalendars checks every shipped official calendar against the
// published number of working days (five-day week, Saturday and Sunday off),
// and that its dates are valid, in the year, unique and sorted.
func TestAzOfficialCalendars(t *testing.T) {
	for year, c := range azOfficialCalendars {
		off := map[string]bool{}
		prev := ""
		for _, d := range c.days {
			day, err := time.Parse(time.DateOnly, d.Date)
			if err != nil || day.Year() != year || d.Kind != CalendarHoliday || d.Name == "" {
				t.Fatalf("%d: bad entry %+v", year, d)
			}
			if d.Date <= prev {
				t.Fatalf("%d: %s not sorted or duplicated", year, d.Date)
			}
			prev = d.Date
			off[d.Date] = true
		}
		working := 0
		for day := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); day.Year() == year; day = day.AddDate(0, 0, 1) {
			if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday && !off[day.Format(time.DateOnly)] {
				working++
			}
		}
		if working != c.workingDays {
			t.Errorf("%d: %d working days, published %d (%s)", year, working, c.workingDays, c.source)
		}
	}
	if !slices.IsSorted(AzOfficialYears()) {
		t.Error("years not sorted")
	}
}
