package server

import (
	"fmt"
	"testing"
)

// ADR-0062: set_field can copy another field of the same item.
func TestSetFieldFrom(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	for _, f := range []map[string]any{
		{"key": "paid", "name": "Paid", "type": "boolean"},
		{"key": "balance", "name": "Balance", "type": "number"},
		{"key": "monthly", "name": "Monthly", "type": "number"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
		e.must(200, "alice", "PUT", bindPath("task", f["key"].(string)), map[string]any{})
	}
	base := "/api/v1/workspaces/acme/automations"
	for _, bad := range []map[string]any{
		{"type": "set_field", "key": "balance", "from": "nope"},
		{"type": "set_field", "key": "balance", "from": "balance"},
		{"type": "set_field", "key": "balance", "from": "monthly", "value": 1},
	} {
		if r := e.call("alice", "POST", base, map[string]any{"name": "v", "trigger": "item.updated", "actions": []any{bad}}); r.Status != 422 {
			t.Errorf("%v: %d %s", bad, r.Status, r.Raw)
		}
	}
	paidChanged := map[string]any{"field": "changed", "op": "includes", "value": "fields.paid"}
	e.must(201, "alice", "POST", base, map[string]any{"name": "clear", "trigger": "item.updated", "enabled": true,
		"conditions": []any{paidChanged, map[string]any{"field": "fields.paid", "op": "eq", "value": true}},
		"actions":    []any{map[string]any{"type": "set_field", "key": "balance", "value": nil}}})
	e.must(201, "alice", "POST", base, map[string]any{"name": "restore", "trigger": "item.updated", "enabled": true,
		"conditions": []any{paidChanged, map[string]any{"field": "fields.paid", "op": "neq", "value": true}},
		"actions":    []any{map[string]any{"type": "set_field", "key": "balance", "from": "monthly"}}})

	it := e.must(201, "carol", "POST", items, map[string]any{"title": "C", "fields": map[string]any{"balance": 120, "monthly": 120}}).Body
	path := items + "/" + fmt.Sprint(it["number"])
	e.must(200, "carol", "PATCH", path, map[string]any{"version": it["version"], "fields": map[string]any{"paid": true}})
	e.drain()
	got := e.must(200, "carol", "GET", path, nil).Body
	if f := got["fields"].(map[string]any); f["balance"] != nil {
		t.Fatalf("balance not cleared: %v", f)
	}
	e.must(200, "carol", "PATCH", path, map[string]any{"version": got["version"], "fields": map[string]any{"paid": false}})
	e.drain()
	if f := e.must(200, "carol", "GET", path, nil).Body["fields"].(map[string]any); f["balance"] != 120.0 {
		t.Fatalf("balance not restored from monthly: %v", f)
	}
}
