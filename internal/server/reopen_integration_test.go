package server

import (
	"fmt"
	"testing"
)

// ADR-0060: only admins reopen closed items when the workflow says so; the
// "month" due filter; the cycle's due start field.
func TestReopenAdminOnlyAndMonthFilter(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	wfs := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/workflows", nil).Body["workflows"].([]any)
	wf := wfs[0].(map[string]any)
	var todo, done string
	for _, s := range wf["statuses"].([]any) {
		st := s.(map[string]any)
		switch st["category"] {
		case "todo":
			if todo == "" {
				todo = st["id"].(string)
			}
		case "done":
			if done == "" {
				done = st["id"].(string)
			}
		}
	}
	wfPath := fmt.Sprintf("/api/v1/workspaces/acme/workflows/%s", wf["id"])
	if r := e.call("carol", "PATCH", wfPath, map[string]any{"reopen_admin_only": true}); r.Status != 403 {
		t.Fatalf("member changed the workflow: %d", r.Status)
	}
	got := e.must(200, "alice", "PATCH", wfPath, map[string]any{"reopen_admin_only": true}).Body
	if got["reopen_admin_only"] != true {
		t.Fatalf("setting not stored: %v", got)
	}

	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Paid customer"}).Body
	n := fmt.Sprint(it["number"])
	it = e.must(200, "carol", "PATCH", items+"/"+n, map[string]any{"version": it["version"], "status_id": done}).Body

	// The member closed it but cannot reopen it, and is not offered to.
	r := e.call("carol", "PATCH", items+"/"+n, map[string]any{"version": it["version"], "status_id": todo})
	if r.Status != 403 || errCode(r) != "reopen_admin_only" {
		t.Fatalf("member reopened: %d %s", r.Status, r.Raw)
	}
	for _, s := range e.must(200, "carol", "GET", items+"/"+n+"/transitions", nil).Body["statuses"].([]any) {
		if c := s.(map[string]any)["category"]; c == "todo" || c == "in_progress" {
			t.Fatalf("member offered an open status: %v", s)
		}
	}
	// The admin can.
	open := false
	for _, s := range e.must(200, "alice", "GET", items+"/"+n+"/transitions", nil).Body["statuses"].([]any) {
		if s.(map[string]any)["id"] == todo {
			open = true
		}
	}
	if !open {
		t.Fatal("admin not offered the open status")
	}
	e.must(200, "alice", "PATCH", items+"/"+n, map[string]any{"version": it["version"], "status_id": todo})

	// Off again: members reopen as before.
	e.must(200, "alice", "PATCH", wfPath, map[string]any{"reopen_admin_only": false})
	it = e.must(200, "alice", "GET", items+"/"+n, nil).Body
	it = e.must(200, "carol", "PATCH", items+"/"+n, map[string]any{"version": it["version"], "status_id": done}).Body
	e.must(200, "carol", "PATCH", items+"/"+n, map[string]any{"version": it["version"], "status_id": todo})

	// due=month: due by the end of this month (and earlier), not later.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Late", "due_date": "2020-01-05"})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Far", "due_date": "2199-01-05"})
	list := e.must(200, "carol", "GET", items+"?due=month", nil).Body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "Late" {
		t.Fatalf("due=month: %v", list)
	}

	// Cycle: due_start_field needs due_day_field and a date field.
	for _, f := range []map[string]any{
		{"key": "payment_day", "name": "Payment day", "type": "date"},
		{"key": "loan_date", "name": "Loan date", "type": "date"},
		{"key": "paid", "name": "Paid", "type": "boolean"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
	}
	cycle := "/api/v1/workspaces/acme/projects/ACME/cycle"
	base := map[string]any{"enabled": true, "day_of_month": 1, "clear_fields": []string{"paid"}, "refill": []any{}, "include_closed": true}
	with := func(extra map[string]any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if r := e.call("alice", "PUT", cycle, with(map[string]any{"due_start_field": "loan_date"})); r.Status != 422 {
		t.Fatalf("start without day field: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "PUT", cycle, with(map[string]any{"due_day_field": "payment_day", "due_start_field": "paid"})); r.Status != 422 {
		t.Fatalf("boolean start field: %d %s", r.Status, r.Raw)
	}
	c := e.must(200, "alice", "PUT", cycle, with(map[string]any{"due_day_field": "payment_day", "due_start_field": "loan_date", "paid_field": "paid"})).Body
	if c["due_start_field"] != "loan_date" {
		t.Fatalf("start field not stored: %v", c)
	}
}
