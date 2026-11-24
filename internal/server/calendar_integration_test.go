package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/recur"
)

const calPath = wsBase + "/calendar"

func TestCalendarAPIAccessAndValidation(t *testing.T) {
	e := newP1Env(t)
	e.setup()

	// Everyone in the workspace reads it (the calendar view shades holidays).
	c := e.must(200, "bob", "GET", calPath+"?year=2027", nil).Body // guest
	if fmt.Sprint(c["weekend_days"]) != "[6 0]" || c["from"] != "2027-01-01" || c["to"] != "2027-12-31" || len(c["days"].([]any)) != 0 {
		t.Fatalf("default calendar: %v", c)
	}
	if r := e.call("dave", "GET", calPath+"?year=2027", nil); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}
	for _, q := range []string{"", "?year=2027&from=2027-01-01&to=2027-01-02", "?from=2027-01-01", "?from=x&to=y", "?year=2027&year=2028"} {
		if r := e.call("carol", "GET", calPath+q, nil); r.Status != 400 {
			t.Errorf("GET %q: %d", q, r.Status)
		}
	}
	for _, q := range []string{"?year=1999", "?year=abc", "?from=2027-01-01&to=2028-03-01", "?from=2027-02-01&to=2027-01-01"} {
		if r := e.call("carol", "GET", calPath+q, nil); r.Status != 422 {
			t.Errorf("GET %q: %d", q, r.Status)
		}
	}

	// Writes are admin+.
	holiday := map[string]any{"kind": "holiday", "name": "Sınaq bayramı"}
	for _, who := range []string{"bob", "carol"} {
		if r := e.call(who, "PUT", calPath+"/days/2027-03-08", holiday); r.Status != 403 {
			t.Fatalf("%s sets a holiday: %d", who, r.Status)
		}
		if r := e.call(who, "PUT", calPath+"/weekend", map[string]any{"weekend_days": []int{0}}); r.Status != 403 {
			t.Fatalf("%s sets the weekend: %d", who, r.Status)
		}
		if r := e.call(who, "POST", calPath+"/presets/az-fixed", map[string]any{"year": 2027}); r.Status != 403 {
			t.Fatalf("%s applies the preset: %d", who, r.Status)
		}
	}

	bad := []struct {
		date string
		body map[string]any
	}{
		{"2027-02-30", holiday},
		{"27-03-08", holiday},
		{"1999-12-31", holiday},
		{"2027-03-08", map[string]any{"kind": "vacation", "name": "x"}},
		{"2027-03-08", map[string]any{"kind": "holiday", "name": "  "}},
		{"2027-03-08", map[string]any{"kind": "holiday", "name": string(make([]byte, 101))}},
		{"2027-03-08", map[string]any{"kind": "workday", "name": "Monday is already working"}}, // a Monday
	}
	for i, b := range bad {
		if r := e.call("alice", "PUT", calPath+"/days/"+b.date, b.body); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}
	for i, w := range [][]int{{0, 1, 2, 3}, {7}, {0, 0}} {
		if r := e.call("alice", "PUT", calPath+"/weekend", map[string]any{"weekend_days": w}); r.Status != 422 {
			t.Errorf("weekend case %d: %d", i, r.Status)
		}
	}
	if r := e.call("alice", "PUT", calPath+"/weekend", map[string]any{}); r.Status != 422 {
		t.Errorf("missing weekend_days: %d", r.Status)
	}

	e.must(200, "alice", "PUT", calPath+"/days/2027-03-08", holiday)
	e.must(200, "alice", "PUT", calPath+"/days/2027-03-08", map[string]any{"kind": "holiday", "name": "Yeni ad"})
	e.must(200, "alice", "PUT", calPath+"/days/2027-03-06", map[string]any{"kind": "workday", "name": "Köçürülmüş iş günü"}) // Saturday
	if r := e.call("alice", "DELETE", calPath+"/days/2027-04-01", nil); r.Status != 404 {
		t.Fatalf("delete missing: %d", r.Status)
	}
	got := e.must(200, "carol", "GET", calPath+"?from=2027-03-01&to=2027-03-31", nil).Body["days"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["kind"] != "workday" || got[1].(map[string]any)["name"] != "Yeni ad" {
		t.Fatalf("days: %v", got)
	}
	// Fri+Sat weekend.
	if w := e.must(200, "alice", "PUT", calPath+"/weekend", map[string]any{"weekend_days": []int{6, 5}}).Body; fmt.Sprint(w["weekend_days"]) != "[5 6]" {
		t.Fatalf("weekend: %v", w)
	}
	e.must(204, "alice", "DELETE", calPath+"/days/2027-03-08", nil)

	// The Azerbaijan preset adds only fixed-date holidays, once, and keeps
	// what the admin already entered.
	e.must(200, "alice", "PUT", calPath+"/days/2027-01-01", map[string]any{"kind": "holiday", "name": "Öz adımız"})
	added := e.must(200, "alice", "POST", calPath+"/presets/az-fixed", map[string]any{"year": 2027}).Body["added"].([]any)
	if len(added) != 10 { // 11 fixed dates, 1 January kept as the admin named it
		t.Fatalf("preset added %d: %v", len(added), added)
	}
	if again := e.must(200, "alice", "POST", calPath+"/presets/az-fixed", map[string]any{"year": 2027}).Body["added"].([]any); len(again) != 0 {
		t.Fatalf("preset applied twice: %v", again)
	}
	jan := e.must(200, "carol", "GET", calPath+"?from=2027-01-01&to=2027-01-02", nil).Body["days"].([]any)
	if jan[0].(map[string]any)["name"] != "Öz adımız" || jan[1].(map[string]any)["name"] != "Yeni il bayramı" {
		t.Fatalf("January: %v", jan)
	}
	if r := e.call("alice", "POST", calPath+"/presets/az-fixed", map[string]any{"year": 2200}); r.Status != 422 {
		t.Fatalf("preset year 2200: %d", r.Status)
	}

	// At most 60 entries per year: fill up from July (no preset dates in July-August).
	have := len(e.must(200, "carol", "GET", calPath+"?year=2027", nil).Body["days"].([]any))
	for i := range 60 - have {
		day := time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		if day.Weekday() == time.Friday || day.Weekday() == time.Saturday {
			e.must(200, "alice", "PUT", calPath+"/days/"+ymd(day), map[string]any{"kind": "workday", "name": "İş günü"})
		} else {
			e.must(200, "alice", "PUT", calPath+"/days/"+ymd(day), holiday)
		}
	}
	if r := e.call("alice", "PUT", calPath+"/days/2027-10-04", holiday); r.Status != 422 {
		t.Fatalf("61st entry: %d %s", r.Status, r.Raw)
	}
	// Replacing an existing date does not count twice.
	e.must(200, "alice", "PUT", calPath+"/days/2027-07-01", map[string]any{"kind": "holiday", "name": "Başqa ad"})
	// Other years are not affected.
	e.must(200, "alice", "PUT", calPath+"/days/2028-10-04", holiday)

	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw
	for _, a := range []string{"calendar.day_set", "calendar.day_deleted", "calendar.weekend_updated", "calendar.preset_applied"} {
		if !contains(audit, a) {
			t.Errorf("audit log has no %s", a)
		}
	}
}

// monthStart is the first day of the month `ahead` months after today.
func monthStart(today time.Time, ahead int) time.Time {
	return time.Date(today.Year(), today.Month()+time.Month(ahead), 1, 0, 0, 0, 0, time.UTC)
}

func TestCalendarChangeReschedulesRecurrences(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	start := monthStart(e.today(), 2)

	lastWD := e.must(201, "alice", "POST", recPath, map[string]any{
		"title": "Hesabat {{month}}", "freq": "monthly", "every": 1, "work_day": -1, "start_date": ymd(start), "lead_days": 0,
	}).Body
	want, _ := recur.Schedule{Freq: recur.Monthly, Interval: 1, WorkDay: recur.LastDay, Start: start}.Next(start)
	if lastWD["next_due"] != ymd(want) || lastWD["work_day"] != -1.0 || lastWD["month_day"] != nil || lastWD["shift"] != "none" {
		t.Fatalf("last working day rule: %v (want %s)", lastWD, ymd(want))
	}
	// A plain date with "next working day": the 15th of the month.
	fifteenth := start.AddDate(0, 0, 14)
	shifted := e.must(201, "alice", "POST", recPath, map[string]any{
		"title": "Ödəniş", "freq": "monthly", "every": 1, "month_day": 15, "shift": "next", "start_date": ymd(start),
	}).Body
	plain := e.must(201, "alice", "POST", recPath, map[string]any{
		"title": "Sadə", "freq": "monthly", "every": 1, "month_day": 15, "start_date": ymd(start),
	}).Body
	wantShift, _ := recur.Schedule{Freq: recur.Monthly, Interval: 1, MonthDay: 15, Shift: recur.ShiftNext, Start: start}.Next(start)
	if shifted["next_due"] != ymd(wantShift) || plain["next_due"] != ymd(fifteenth) {
		t.Fatalf("shifted %v, plain %v", shifted["next_due"], plain["next_due"])
	}

	// Validation of the new options.
	for i, b := range []map[string]any{
		{"title": "x", "freq": "monthly", "every": 1, "work_day": 24, "start_date": ymd(start)},
		{"title": "x", "freq": "monthly", "every": 1, "work_day": 0, "start_date": ymd(start)},
		{"title": "x", "freq": "monthly", "every": 1, "work_day": 1, "month_day": 1, "start_date": ymd(start)},
		{"title": "x", "freq": "weekly", "every": 1, "weekdays": []int{1}, "work_day": 1, "start_date": ymd(start)},
		{"title": "x", "freq": "monthly", "every": 1, "work_day": 1, "shift": "next", "start_date": ymd(start)},
		{"title": "x", "freq": "monthly", "every": 1, "month_day": 1, "shift": "later", "start_date": ymd(start)},
	} {
		if r := e.call("alice", "POST", recPath, b); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}

	// The last working day becomes a holiday: the rule moves to the day before.
	e.must(200, "alice", "PUT", calPath+"/days/"+ymd(want), map[string]any{"kind": "holiday", "name": "Sınaq"})
	cal, _ := recur.NewCalendar(recur.DefaultWeekend, []time.Time{want}, nil)
	want2, _ := recur.Schedule{Freq: recur.Monthly, Interval: 1, WorkDay: recur.LastDay, Start: start, Cal: cal}.Next(start)
	if got := e.must(200, "carol", "GET", recPath+"/"+lastWD["id"].(string), nil).Body; got["next_due"] != ymd(want2) || !want2.Before(want) {
		t.Fatalf("after holiday: %v, want %s", got["next_due"], ymd(want2))
	}
	// The 15th becomes a holiday (if it is not already a weekend day): the
	// shifted rule moves, the plain one does not.
	e.must(200, "alice", "PUT", calPath+"/days/"+ymd(fifteenth), map[string]any{"kind": "holiday", "name": "Sınaq 2"})
	cal, _ = recur.NewCalendar(recur.DefaultWeekend, []time.Time{want, fifteenth}, nil)
	wantShift2, _ := recur.Schedule{Freq: recur.Monthly, Interval: 1, MonthDay: 15, Shift: recur.ShiftNext, Start: start, Cal: cal}.Next(start)
	if got := e.must(200, "carol", "GET", recPath+"/"+shifted["id"].(string), nil).Body; got["next_due"] != ymd(wantShift2) {
		t.Fatalf("shifted after holiday: %v, want %s", got["next_due"], ymd(wantShift2))
	}
	if got := e.must(200, "carol", "GET", recPath+"/"+plain["id"].(string), nil).Body; got["next_due"] != ymd(fifteenth) {
		t.Fatalf("plain rule moved: %v", got["next_due"])
	}
	// Preview uses the calendar too.
	prev := e.must(200, "carol", "POST", recPath+"/preview", map[string]any{
		"freq": "monthly", "every": 1, "month_day": 15, "shift": "next", "start_date": ymd(start),
	}).Body["upcoming"].([]any)
	if prev[0] != ymd(wantShift2) {
		t.Fatalf("preview: %v", prev)
	}
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw
	if !contains(audit, "recurrence.rescheduled") {
		t.Fatal("no recurrence.rescheduled event")
	}

	// Removing the holiday moves it back.
	e.must(204, "alice", "DELETE", calPath+"/days/"+ymd(want), nil)
	if got := e.must(200, "carol", "GET", recPath+"/"+lastWD["id"].(string), nil).Body; got["next_due"] != ymd(want) {
		t.Fatalf("after removing the holiday: %v", got["next_due"])
	}

	// No retroactive items, no duplicates: the item for this month already
	// exists (lead time reached); a calendar change that moves this month's
	// last working day does not create a second item for the same month.
	if _, err := e.pool.Exec(ctx, `UPDATE recurrences SET lead_days = 365 WHERE id = $1`, lastWD["id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE recurrences SET is_enabled = false WHERE id <> $1`, lastWD["id"]); err != nil {
		t.Fatal(err)
	}
	n, err := e.st.EmitRecurrences(ctx, 100)
	if err != nil || n < 1 {
		t.Fatalf("emit: %d %v", n, err)
	}
	var nominal, due string
	if err := e.pool.QueryRow(ctx, `SELECT nominal_date::text, due_date::text FROM recurrence_runs WHERE recurrence_id = $1 ORDER BY due_date LIMIT 1`,
		lastWD["id"]).Scan(&nominal, &due); err != nil || nominal != ymd(start) || due != ymd(want) {
		t.Fatalf("first run: nominal %s due %s (%v)", nominal, due, err)
	}
	var runsBefore int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM recurrence_runs WHERE recurrence_id = $1`, lastWD["id"]).Scan(&runsBefore)
	e.must(200, "alice", "PUT", calPath+"/days/"+ymd(want), map[string]any{"kind": "holiday", "name": "Sınaq"})
	if _, err := e.st.EmitRecurrences(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var dupes int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM recurrence_runs WHERE recurrence_id = $1 AND nominal_date = $2::date`,
		lastWD["id"], ymd(start)).Scan(&dupes)
	if dupes != 1 {
		t.Fatalf("items for the first month: %d", dupes)
	}
	var runsAfter int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM recurrence_runs WHERE recurrence_id = $1`, lastWD["id"]).Scan(&runsAfter)
	if runsAfter != runsBefore {
		t.Fatalf("calendar change created items: %d -> %d", runsBefore, runsAfter)
	}
}

func TestOverdueDigestSkipsNonWorkingDays(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	today := ymd(e.today())
	e.must(201, "alice", "POST", items, map[string]any{"title": "Ödəniş gecikir", "due_date": "2020-03-01"})
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"overdue_digest_hour": 0})
	// No weekend: only the holiday decides.
	e.must(200, "alice", "PUT", calPath+"/weekend", map[string]any{"weekend_days": []int{}})
	e.must(200, "alice", "PUT", calPath+"/days/"+today, map[string]any{"kind": "holiday", "name": "Sınaq"})
	if n, err := e.st.EmitOverdueDigests(ctx); err != nil || n != 0 {
		t.Fatalf("digest on a holiday: %d %v", n, err)
	}
	// The day is done: removing the holiday later the same day does not
	// send it twice... but a fresh day (no run yet) that is working sends it.
	e.must(204, "alice", "DELETE", calPath+"/days/"+today, nil)
	if n, _ := e.st.EmitOverdueDigests(ctx); n != 0 {
		t.Fatalf("digest after the day was marked: %d", n)
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM workspace_digest_runs`); err != nil {
		t.Fatal(err)
	}
	if n, err := e.st.EmitOverdueDigests(ctx); err != nil || n != 1 {
		t.Fatalf("digest on a working day: %d %v", n, err)
	}
}

// TestCalendarOfficialPreset: for a year with a shipped official calendar the
// preset adds every holiday and transferred rest day (ADR-0027).
func TestCalendarOfficialPreset(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	cal := e.must(200, "alice", "GET", calPath+"?year=2026", nil).Body
	if years, _ := cal["az_official_years"].([]any); len(years) == 0 || years[0] != float64(2026) {
		t.Fatalf("official years: %v", cal["az_official_years"])
	}
	added := e.must(200, "alice", "POST", calPath+"/presets/az-fixed", map[string]any{"year": 2026}).Body["added"].([]any)
	if len(added) != 25 {
		t.Fatalf("official 2026 added %d", len(added))
	}
	days := e.must(200, "alice", "GET", calPath+"?year=2026", nil).Body["days"].([]any)
	found := map[string]bool{}
	for _, d := range days {
		found[d.(map[string]any)["date"].(string)] = true
	}
	for _, want := range []string{"2026-03-20", "2026-03-30", "2026-05-29", "2026-11-10", "2026-01-20"} {
		if !found[want] {
			t.Errorf("missing %s", want)
		}
	}
}
