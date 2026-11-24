package store_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Migration 00020 Down refuses to drop working-day rules the previous release
// cannot run; without them it goes down and up again cleanly.
func TestWorkingCalendarMigrationDownGuard(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := f.st.SetCalendarDay(ctx, f.wsID, f.userID, "2030-01-01", store.CalendarDayInput{Kind: store.CalendarHoliday, Name: "Sınaq"}); err != nil {
		t.Fatal(err)
	}
	last := -1
	rec, err := f.st.SaveRecurrence(ctx, f.wsID, f.project, f.userID, [16]byte{}, store.RecurrenceInput{
		Title: "Hesabat", ScheduleInput: store.ScheduleInput{Freq: "monthly", Every: 1, WorkDay: &last, StartDate: "2030-01-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.NextDue == nil || *rec.NextDue != "2030-01-31" {
		t.Fatalf("next_due = %v", rec.NextDue)
	}
	// Later migrations (other features) are undone first.
	version := func() int {
		var v int
		if err := f.owner.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for version() > 20 {
		if err := database.Migrate(ctx, f.owner, "down", logger); err != nil {
			t.Fatal(err)
		}
	}
	err = database.Migrate(ctx, f.owner, "down", logger)
	if err == nil || !strings.Contains(err.Error(), "working-day options") {
		t.Fatalf("down with a working-day rule: %v", err)
	}
	if err := f.st.DeleteRecurrence(ctx, f.wsID, f.project, f.userID, rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, f.owner, "down", logger); err != nil {
		t.Fatal(err)
	}
	if v := version(); v >= 20 {
		t.Fatalf("still at %d", v)
	}
	var tables int
	if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE tablename = 'workspace_calendar_days'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("calendar table after down: %d %v", tables, err)
	}
	if err := database.Migrate(ctx, f.owner, "up", logger); err != nil {
		t.Fatal(err)
	}
}
