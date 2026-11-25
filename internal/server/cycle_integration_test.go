package server

import (
	"context"
	"testing"
	"time"
)

const cyclePath = "/api/v1/workspaces/acme/projects/ACME/cycle"

// cycleFields creates the ledger-like fields a monthly close works on.
func (e *p1env) cycleFields() {
	e.t.Helper()
	for _, f := range []map[string]any{
		{"key": "paid", "name": "Ödədi", "type": "boolean"},
		{"key": "monthly", "name": "Aylıq ödəniş", "type": "number"},
		{"key": "balance", "name": "Qalıq", "type": "number"},
		{"key": "note2", "name": "Qeyd", "type": "text"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
		e.must(200, "alice", "PUT", bindPath("task", f["key"].(string)), map[string]any{})
	}
}

func TestMonthlyCycleConfigAndAccess(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.cycleFields()
	today := e.today()

	c := e.must(200, "carol", "GET", cyclePath, nil).Body // member reads the default
	if c["enabled"] != false || c["day_of_month"] != 1.0 || c["updated_at"] != nil {
		t.Fatalf("default: %v", c)
	}
	if r := e.call("bob", "GET", cyclePath, nil); r.Status != 403 { // guest
		t.Fatalf("guest reads: %d", r.Status)
	}
	good := map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid"},
		"refill": []map[string]any{{"target": "balance", "source": "monthly"}}}
	e.must(403, "carol", "PUT", cyclePath, good)
	e.must(403, "carol", "POST", cyclePath+"/close", map[string]any{})
	e.must(403, "carol", "POST", cyclePath+"/preview", map[string]any{})

	for i, bad := range []map[string]any{
		{"enabled": true, "day_of_month": 0, "clear_fields": []string{"paid"}},
		{"enabled": true, "day_of_month": 29, "clear_fields": []string{"paid"}},
		{"enabled": true, "day_of_month": 1},
		{"enabled": true, "day_of_month": 1, "clear_fields": []string{"nope"}},
		{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid", "paid"}},
		{"enabled": true, "day_of_month": 1, "refill": []map[string]any{{"target": "balance", "source": "note2"}}},
		{"enabled": true, "day_of_month": 1, "refill": []map[string]any{{"target": "balance", "source": "balance"}}},
		{"enabled": true, "day_of_month": 1, "clear_fields": []string{"balance"}, "refill": []map[string]any{{"target": "balance", "source": "monthly"}}},
	} {
		if r := e.call("alice", "PUT", cyclePath, bad); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}

	c = e.must(200, "alice", "PUT", cyclePath, good).Body
	next := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	if c["enabled"] != true || c["next_run"] != ymd(next) || c["next_period"] != today.Format("2006-01") {
		t.Fatalf("saved: %v (today %s)", c, ymd(today))
	}
	// Changing only the fields keeps the date; disabling clears it.
	c = e.must(200, "alice", "PUT", cyclePath, map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid", "note2"},
		"refill": []map[string]any{{"target": "balance", "source": "monthly"}}}).Body
	if c["next_run"] != ymd(next) {
		t.Fatalf("next_run moved: %v", c)
	}
	c = e.must(200, "alice", "PUT", cyclePath, map[string]any{"enabled": false, "day_of_month": 1, "clear_fields": []string{"paid"}}).Body
	if c["next_run"] != nil {
		t.Fatalf("disabled keeps next_run: %v", c)
	}
	if n := e.countEvents(`action = 'cycle.updated'`); n != 3 {
		t.Fatalf("cycle.updated events: %d", n)
	}
}

func TestMonthlyCycleClose(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.cycleFields()
	ctx := context.Background()
	today := e.today()
	lastMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0).Format("2006-01")

	mk := func(title string, fields map[string]any) {
		e.must(201, "alice", "POST", items, map[string]any{"title": title, "fields": fields})
	}
	mk("Əli Məmmədov", map[string]any{"paid": true, "monthly": 150, "balance": 0})     // 1: paid
	mk("Aygün Həsənova", map[string]any{"paid": false, "monthly": 200, "balance": 50}) // 2: partly
	mk("Bağlı müştəri", map[string]any{"paid": true, "monthly": 100, "balance": 0})    // 3: will be done
	mk("Dəyişməyən", map[string]any{"monthly": 80, "balance": 80})                     // 4: nothing to reset
	var doneID string
	for _, it := range e.must(200, "carol", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			if s.(map[string]any)["name"] == "Done" && it.(map[string]any)["key"] == "task" {
				doneID = s.(map[string]any)["id"].(string)
			}
		}
	}
	e.must(200, "alice", "PATCH", items+"/3", map[string]any{"version": 1, "status_id": doneID})

	// Closing needs a configuration.
	e.must(422, "alice", "POST", cyclePath+"/close", map[string]any{})
	e.must(200, "alice", "PUT", cyclePath, map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid"},
		"refill": []map[string]any{{"target": "balance", "source": "monthly"}}})

	p := e.must(200, "alice", "POST", cyclePath+"/preview", map[string]any{}).Body
	if p["period"] != lastMonth || p["closed"] != false || p["items"] != 4.0 || p["open"] != 3.0 || p["will_reset"] != 2.0 {
		t.Fatalf("preview: %v", p)
	}
	e.must(422, "alice", "POST", cyclePath+"/close", map[string]any{"period": "2099-01"})
	e.must(422, "alice", "POST", cyclePath+"/close", map[string]any{"period": "11.2026"})

	cl := e.must(201, "alice", "POST", cyclePath+"/close", map[string]any{}).Body
	if cl["period"] != lastMonth || cl["item_count"] != 4.0 || cl["changed"] != 2.0 || len(cl["failed"].([]any)) != 0 || cl["closed_by"] == nil {
		t.Fatalf("close: %v", cl)
	}
	get := func(n string) map[string]any {
		return e.must(200, "alice", "GET", items+"/"+n, nil).Body["fields"].(map[string]any)
	}
	if f := get("1"); f["paid"] != nil || f["balance"] != 150.0 || f["monthly"] != 150.0 {
		t.Fatalf("item 1 after close: %v", f)
	}
	if f := get("2"); f["paid"] != nil || f["balance"] != 200.0 {
		t.Fatalf("item 2 after close: %v", f)
	}
	if f := get("3"); f["paid"] != true || f["balance"] != 0.0 { // done: untouched
		t.Fatalf("done item changed: %v", f)
	}
	// Each reset is an ordinary item.updated, tagged with the close.
	if n := e.countEvents(`action = 'item.updated' AND payload ? 'cycle'`); n != 2 {
		t.Fatalf("tagged item.updated events: %d", n)
	}

	// Twice is refused; the snapshot keeps last month's values.
	if r := e.call("alice", "POST", cyclePath+"/close", map[string]any{}); r.Status != 409 {
		t.Fatalf("second close: %d", r.Status)
	}
	snap := e.must(200, "carol", "GET", cyclePath+"/closes/"+lastMonth, nil).Body
	rows := snap["items"].([]any)
	if len(rows) != 4 {
		t.Fatalf("snapshot rows: %v", snap)
	}
	first := rows[0].(map[string]any)
	if first["number"] != 1.0 || first["title"] != "Əli Məmmədov" || first["fields"].(map[string]any)["paid"] != true ||
		first["fields"].(map[string]any)["balance"] != 0.0 {
		t.Fatalf("snapshot row 1: %v", first)
	}
	list := e.must(200, "carol", "GET", cyclePath+"/closes", nil).Body["closes"].([]any)
	if len(list) != 1 {
		t.Fatalf("closes: %v", list)
	}
	e.must(404, "carol", "GET", cyclePath+"/closes/2001-01", nil)
	e.must(403, "bob", "GET", cyclePath+"/closes/"+lastMonth, nil)

	// The scheduler: due today closes the month before today's month, unless
	// it is already closed; then moves to the next date.
	if _, err := e.pool.Exec(ctx, `UPDATE period_cycles SET next_run = $1`, today); err != nil {
		t.Fatal(err)
	}
	n, err := e.st.EmitCycles(ctx)
	if err != nil || n != 0 { // lastMonth was closed by hand: skipped
		t.Fatalf("scheduled run: n=%d err=%v", n, err)
	}
	c := e.must(200, "alice", "GET", cyclePath, nil).Body
	if c["next_run"] == ymd(today) || c["last_error"] != nil {
		t.Fatalf("next_run not advanced: %v", c)
	}
	// A month with a hand-made close removed is closed by the scheduler.
	if _, err := e.pool.Exec(ctx, `DELETE FROM period_closes`); err != nil {
		t.Fatal(err)
	}
	e.must(200, "alice", "PATCH", items+"/1", map[string]any{"version": 2, "fields": map[string]any{"paid": true, "balance": 0}})
	if _, err := e.pool.Exec(ctx, `UPDATE period_cycles SET next_run = $1`, today); err != nil {
		t.Fatal(err)
	}
	if n, err = e.st.EmitCycles(ctx); err != nil || n != 1 {
		t.Fatalf("scheduled close: n=%d err=%v", n, err)
	}
	if n, _ = e.st.EmitCycles(ctx); n != 0 {
		t.Fatalf("scheduler closed twice: %d", n)
	}
	if f := get("1"); f["paid"] != nil || f["balance"] != 150.0 {
		t.Fatalf("item 1 after scheduled close: %v", f)
	}
	list = e.must(200, "carol", "GET", cyclePath+"/closes", nil).Body["closes"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["closed_by"] != nil || list[0].(map[string]any)["period"] != lastMonth {
		t.Fatalf("scheduled close record: %v", list)
	}

	// When the status tracks the month: closed items too, and back to To Do.
	var todoID string
	for _, it := range e.must(200, "carol", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			if s.(map[string]any)["name"] == "To Do" && it.(map[string]any)["key"] == "task" {
				todoID = s.(map[string]any)["id"].(string)
			}
		}
	}
	e.must(422, "alice", "PUT", cyclePath, map[string]any{"enabled": true, "day_of_month": 1, "reset_status_id": "01a20727-0000-7000-8000-000000000000"})
	c = e.must(200, "alice", "PUT", cyclePath, map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid"},
		"refill": []map[string]any{{"target": "balance", "source": "monthly"}}, "include_closed": true, "reset_status_id": todoID}).Body
	if c["include_closed"] != true || c["reset_status_id"] != todoID {
		t.Fatalf("saved scope: %v", c)
	}
	p = e.must(200, "alice", "POST", cyclePath+"/preview", map[string]any{"period": today.Format("2006-01")}).Body
	if p["open"] != 4.0 || p["will_reset"] != 1.0 { // only the done item 3 changes: status and fields
		t.Fatalf("preview with closed items: %v", p)
	}
	e.must(201, "alice", "POST", cyclePath+"/close", map[string]any{"period": today.Format("2006-01")})
	it3 := e.must(200, "alice", "GET", items+"/3", nil).Body
	if it3["status"].(map[string]any)["id"] != todoID || it3["fields"].(map[string]any)["paid"] != nil || it3["fields"].(map[string]any)["balance"] != 100.0 {
		t.Fatalf("closed item after a close with include_closed: %v", it3)
	}
}
