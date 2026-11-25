package store

import (
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/recur"
)

func TestSLADueWorkingDays(t *testing.T) {
	baku, err := time.LoadLocation("Asia/Baku")
	if err != nil {
		t.Fatal(err)
	}
	d := func(s string) time.Time { x, _ := time.Parse(time.DateOnly, s); return x }
	// 2026-03-20..24 Novruz (holidays), 2026-03-21/22 a weekend.
	cal, err := recur.NewCalendar(recur.DefaultWeekend,
		[]time.Time{d("2026-03-20"), d("2026-03-23"), d("2026-03-24")}, []time.Time{d("2026-03-28")})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		start time.Time
		days  int
		want  string // local midnight that ends the due day
	}{
		{"friday +1 is end of monday", time.Date(2026, 1, 9, 15, 0, 0, 0, baku), 1, "2026-01-13T00:00:00+04:00"},
		{"same weekday +3", time.Date(2026, 1, 6, 9, 0, 0, 0, baku), 3, "2026-01-10T00:00:00+04:00"},
		{"saturday arrival counts from monday", time.Date(2026, 1, 10, 9, 0, 0, 0, baku), 1, "2026-01-13T00:00:00+04:00"},
		{"across Novruz holidays", time.Date(2026, 3, 19, 10, 0, 0, 0, baku), 1, "2026-03-26T00:00:00+04:00"},
		{"transferred working saturday counts", time.Date(2026, 3, 27, 10, 0, 0, 0, baku), 1, "2026-03-29T00:00:00+04:00"},
		// 22:30 UTC on Jan 8 is already Friday Jan 9 in Baku: the local date decides.
		{"workspace time zone decides the day", time.Date(2026, 1, 8, 22, 30, 0, 0, time.UTC), 1, "2026-01-13T00:00:00+04:00"},
	} {
		got, ok := SLADue(c.start, baku, cal, c.days)
		if !ok || got.Format(time.RFC3339) != c.want {
			t.Errorf("%s: %s %v, want %s", c.name, got.Format(time.RFC3339), ok, c.want)
		}
	}
}

func TestSLAState(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	at := func(h int) *time.Time { x := now.Add(time.Duration(h) * time.Hour); return &x }
	for _, c := range []struct {
		name string
		s    ItemSLA
		want string
	}{
		{"plenty of time", ItemSLA{StartedAt: at(-10), ResolutionDue: at(30)}, SLAOnTrack},
		{"under a quarter left", ItemSLA{StartedAt: at(-30), ResolutionDue: at(9)}, SLAAtRisk},
		{"past due", ItemSLA{StartedAt: at(-30), ResolutionDue: at(-1)}, SLABreached},
		{"resolved in time", ItemSLA{StartedAt: at(-30), ResolutionDue: at(-1), ResolvedAt: at(-2)}, SLAMet},
		{"resolved late", ItemSLA{StartedAt: at(-30), ResolutionDue: at(-5), ResolvedAt: at(-2)}, SLABreached},
		{"response breached, resolution fine", ItemSLA{StartedAt: at(-30), ResponseDue: at(-1), ResolutionDue: at(100)}, SLABreached},
		{"responded, resolution on track", ItemSLA{StartedAt: at(-1), ResponseDue: at(5), RespondedAt: at(0), ResolutionDue: at(100)}, SLAOnTrack},
	} {
		if got := c.s.state(now); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	if _, err := slaFilter("late"); err == nil {
		t.Error("unknown sla filter accepted")
	}
}
