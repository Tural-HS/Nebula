package server

import (
	"context"
	"testing"
	"time"
)

// dueCycleFields adds the loan date (the monthly due day) to cycleFields.
func (e *p1env) dueCycleFields() {
	e.t.Helper()
	e.cycleFields()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "loan_date", "name": "Kredit tarixi", "type": "date"})
	e.must(200, "alice", "PUT", bindPath("task", "loan_date"), map[string]any{})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "old_date", "name": "Köhnə tarix", "type": "date"})
	e.must(200, "alice", "PATCH", fieldsPath+"/old_date", map[string]any{"archived": true})
}

func dueCycle(extra map[string]any) map[string]any {
	c := map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid"},
		"refill": []map[string]any{{"target": "balance", "source": "monthly"}}, "due_day_field": "loan_date", "paid_field": "paid"}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// lastDayOrDay is day in month's month, clamped like the cycle does.
func lastDayOrDay(month time.Time, day int) string {
	last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	return ymd(time.Date(month.Year(), month.Month(), min(day, last), 0, 0, 0, 0, time.UTC))
}

func TestCycleDueDatesConfig(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.dueCycleFields()

	for i, bad := range []map[string]any{
		dueCycle(map[string]any{"due_day_field": "nope"}),
		dueCycle(map[string]any{"due_day_field": "paid"}),     // boolean, not date
		dueCycle(map[string]any{"due_day_field": "old_date"}), // archived
		dueCycle(map[string]any{"paid_field": "loan_date"}),   // date, not boolean
		dueCycle(map[string]any{"due_day_field": nil}),        // paid without due day
		dueCycle(map[string]any{"clear_fields": []string{"paid", "loan_date"}}),
		dueCycle(map[string]any{"clear_fields": []string{}}), // paid never cleared
	} {
		if r := e.call("alice", "PUT", cyclePath, bad); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}
	c := e.must(200, "alice", "PUT", cyclePath, dueCycle(nil)).Body
	if c["due_day_field"] != "loan_date" || c["paid_field"] != "paid" {
		t.Fatalf("saved: %v", c)
	}
	if n := e.countEvents(`action = 'cycle.updated' AND payload->>'due_day_field' = 'loan_date' AND payload->>'paid_field' = 'paid'`); n != 1 {
		t.Fatalf("audit of the due fields: %d", n)
	}
	// Due dates alone are enough for a cycle; empty strings turn them off.
	e.must(200, "alice", "PUT", cyclePath, map[string]any{"enabled": true, "day_of_month": 1, "due_day_field": "loan_date"})
	c = e.must(200, "alice", "PUT", cyclePath, dueCycle(map[string]any{"due_day_field": "", "paid_field": ""})).Body
	if c["due_day_field"] != nil || c["paid_field"] != nil {
		t.Fatalf("cleared: %v", c)
	}
}

func TestCycleDueDatesClose(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.dueCycleFields()
	today := e.today()
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC) // N: the month after the closed one
	yesterday := ymd(today.AddDate(0, 0, -1))

	mk := func(title string, fields map[string]any, due any) {
		e.must(201, "alice", "POST", items, map[string]any{"title": title, "fields": fields, "due_date": due})
	}
	mk("Ödəyib", map[string]any{"paid": true, "monthly": 100, "balance": 0, "loan_date": "2025-01-31"}, nil)                 // 1
	mk("Gecikən", map[string]any{"paid": false, "monthly": 200, "balance": 50, "loan_date": "2025-03-10"}, yesterday)        // 2: carried
	mk("Vaxtı çatmayıb", map[string]any{"paid": false, "monthly": 300, "balance": 0, "loan_date": "2025-02-15"}, ymd(today)) // 3
	mk("Tarixsiz", map[string]any{"paid": true, "monthly": 80, "balance": 0}, nil)                                           // 4

	// 5: done, paid never set, due in the past: not overdue, so it resets.
	mk("Tamamlanıb", map[string]any{"monthly": 90, "balance": 0, "loan_date": "2025-06-05"}, yesterday)
	var doneID string
	for _, it := range e.must(200, "carol", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			if s.(map[string]any)["name"] == "Done" && it.(map[string]any)["key"] == "task" {
				doneID = s.(map[string]any)["id"].(string)
			}
		}
	}
	e.must(200, "alice", "PATCH", items+"/5", map[string]any{"version": 1, "status_id": doneID})

	e.must(200, "alice", "PUT", cyclePath, dueCycle(map[string]any{"include_closed": true}))
	p := e.must(200, "alice", "POST", cyclePath+"/preview", map[string]any{}).Body
	if p["items"] != 5.0 || p["open"] != 5.0 || p["will_carry"] != 1.0 || p["will_reset"] != 4.0 {
		t.Fatalf("preview: %v", p)
	}
	cl := e.must(201, "alice", "POST", cyclePath+"/close", map[string]any{}).Body
	if cl["item_count"] != 5.0 || cl["changed"] != 4.0 || cl["carried"] != 1.0 {
		t.Fatalf("close: %v", cl)
	}
	get := func(n string) map[string]any { return e.must(200, "alice", "GET", items+"/"+n, nil).Body }
	if it := get("5"); it["due_date"] != lastDayOrDay(month, 5) || it["fields"].(map[string]any)["balance"] != 90.0 {
		t.Fatalf("done item was carried: %v", it)
	}
	if it := get("1"); it["due_date"] != lastDayOrDay(month, 31) || it["fields"].(map[string]any)["paid"] != nil {
		t.Fatalf("item 1: %v", it)
	}
	if it := get("2"); it["due_date"] != yesterday || it["fields"].(map[string]any)["paid"] != false ||
		it["fields"].(map[string]any)["balance"] != 50.0 {
		t.Fatalf("carried item changed: %v", it)
	}
	if it := get("3"); it["due_date"] != lastDayOrDay(month, 15) || it["fields"].(map[string]any)["balance"] != 300.0 {
		t.Fatalf("item 3: %v", it)
	}
	if it := get("4"); it["due_date"] != nil || it["fields"].(map[string]any)["balance"] != 80.0 {
		t.Fatalf("item 4: %v", it)
	}
	// The due date change is an ordinary tagged item.updated.
	if n := e.countEvents(`action = 'item.updated' AND payload ? 'cycle' AND payload->'changes' ? 'due_date'`); n != 3 {
		t.Fatalf("due date events: %d", n)
	}
	list := e.must(200, "carol", "GET", cyclePath+"/closes", nil).Body["closes"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["carried"] != 1.0 {
		t.Fatalf("closes: %v", list)
	}
	snap := e.must(200, "carol", "GET", cyclePath+"/closes/"+month.AddDate(0, -1, 0).Format("2006-01"), nil).Body
	if len(snap["items"].([]any)) != 5 {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestCycleDueDatesDailyStep(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.dueCycleFields()
	ctx := context.Background()
	today := e.today()
	thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	lateDue := ymd(thisMonth.AddDate(0, -1, 11)) // the 12th of last month

	e.must(201, "alice", "POST", items, map[string]any{"title": "Yeni", "fields": map[string]any{"loan_date": "2025-01-31", "monthly": 10}})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Gecikən", "due_date": lateDue,
		"fields": map[string]any{"loan_date": "2025-04-12", "paid": true, "monthly": 120, "balance": 40}})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Tarixsiz"})
	e.must(200, "alice", "PUT", cyclePath, dueCycle(nil))
	get := func(n string) map[string]any { return e.must(200, "alice", "GET", items+"/"+n, nil).Body }

	// A fixed day in a 30-day month: 31 is clamped.
	nov := time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC)
	if n, err := e.st.EmitCycleDueDates(ctx, nov); err != nil || n != 1 {
		t.Fatalf("fill: n=%d err=%v", n, err)
	}
	if it := get("1"); it["due_date"] != "2026-11-30" {
		t.Fatalf("filled due date: %v", it)
	}
	if it := get("3"); it["due_date"] != nil {
		t.Fatalf("item without a date got a due date: %v", it)
	}
	// Again: nothing to do (no tick repeats an edit).
	if n, err := e.st.EmitCycleDueDates(ctx, nov); err != nil || n != 0 {
		t.Fatalf("second run: n=%d err=%v", n, err)
	}

	// Paid but late, and last month is not closed yet: not rolled, so the
	// month's snapshot still gets its values.
	now := time.Now()
	if n, err := e.st.EmitCycleDueDates(ctx, now); err != nil {
		t.Fatal(err)
	} else if it := get("2"); it["due_date"] != lateDue || it["fields"].(map[string]any)["paid"] != true || n > 1 {
		t.Fatalf("rolled before the close: n=%d %v", n, it)
	}

	// Unpaid at the close: carried.
	e.must(200, "alice", "PATCH", items+"/2", map[string]any{"version": get("2")["version"], "fields": map[string]any{"paid": false}})
	if cl := e.must(201, "alice", "POST", cyclePath+"/close", map[string]any{}).Body; cl["carried"] != 1.0 {
		t.Fatalf("close: %v", cl)
	}
	if it := get("2"); it["due_date"] != lateDue || it["fields"].(map[string]any)["balance"] != 40.0 {
		t.Fatalf("carried item changed: %v", it)
	}

	// Paid later: the daily step applies the skipped reset and moves the due date.
	e.must(200, "alice", "PATCH", items+"/2", map[string]any{"version": get("2")["version"], "fields": map[string]any{"paid": true}})
	if n, err := e.st.EmitCycleDueDates(ctx, now); err != nil || n != 1 {
		t.Fatalf("roll: n=%d err=%v", n, err)
	}
	it := get("2")
	f := it["fields"].(map[string]any)
	if it["due_date"] != lastDayOrDay(thisMonth, 12) || f["paid"] != nil || f["balance"] != 120.0 {
		t.Fatalf("rolled item: %v", it)
	}
	if n := e.countEvents(`action = 'item.updated' AND payload->'cycle'->>'roll' = 'true'`); n != 1 {
		t.Fatalf("roll events: %d", n)
	}
	if n, err := e.st.EmitCycleDueDates(ctx, now); err != nil || n != 0 {
		t.Fatalf("roll repeated: n=%d err=%v", n, err)
	}
}
