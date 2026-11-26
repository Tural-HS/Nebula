package server

import (
	"testing"
)

// copy_fields automation action (ADR-0056): REY-style source project (here
// ACME) pushes field values to the items of a MAH-style target project that
// share the nv_number value.

const mahItems = "/api/v1/workspaces/acme/projects/MAH/items"

func TestCopyFieldsAction(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	base := "/api/v1/workspaces/acme/automations"
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "MAH", "name": "Monthly"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OLD", "name": "Old"})
	e.must(200, "alice", "POST", "/api/v1/workspaces/acme/projects/OLD/archive", nil)
	for _, f := range []map[string]any{
		{"key": "nv_number", "name": "NV", "type": "text"},
		{"key": "balance", "name": "Balance", "type": "number"},
		{"key": "monthly_payment", "name": "Monthly payment", "type": "number"},
		{"key": "note", "name": "Note", "type": "text"},
		{"key": "signed", "name": "Signed", "type": "date"},
		{"key": "gone", "name": "Gone", "type": "text"},
		formulaBody("double", "balance * 2", 2),
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
	}
	e.must(200, "alice", "PATCH", fieldsPath+"/gone", map[string]any{"archived": true})
	for _, k := range []string{"nv_number", "balance", "monthly_payment", "note"} {
		e.must(200, "alice", "PUT", bindPath("task", k), map[string]any{})
	}
	// Bugs do not use monthly_payment: a bug target only gets the rest.
	for _, k := range []string{"nv_number", "balance"} {
		e.must(200, "alice", "PUT", bindPath("bug", k), map[string]any{})
	}

	copyAction := func(project, match string, fields ...string) map[string]any {
		return map[string]any{"type": "copy_fields", "project": project, "match": match, "fields": fields}
	}
	t.Run("validation", func(t *testing.T) {
		cases := map[string]struct {
			actions []any
			field   string
		}{
			"unknown project":   {[]any{copyAction("NOPE", "nv_number", "balance")}, "actions[0].project"},
			"bad project key":   {[]any{copyAction("mah", "nv_number", "balance")}, "actions[0].project"},
			"archived project":  {[]any{copyAction("OLD", "nv_number", "balance")}, "actions[0].project"},
			"unknown match":     {[]any{copyAction("MAH", "nope", "balance")}, "actions[0].match"},
			"archived match":    {[]any{copyAction("MAH", "gone", "balance")}, "actions[0].match"},
			"date match":        {[]any{copyAction("MAH", "signed", "balance")}, "actions[0].match"},
			"no fields":         {[]any{copyAction("MAH", "nv_number")}, "actions[0].fields"},
			"unknown field":     {[]any{copyAction("MAH", "nv_number", "balance", "nope")}, "actions[0].fields[1]"},
			"archived field":    {[]any{copyAction("MAH", "nv_number", "gone")}, "actions[0].fields[0]"},
			"formula field":     {[]any{copyAction("MAH", "nv_number", "double")}, "actions[0].fields[0]"},
			"match copied":      {[]any{copyAction("MAH", "nv_number", "nv_number")}, "actions[0].fields[0]"},
			"duplicate field":   {[]any{copyAction("MAH", "nv_number", "balance", "balance")}, "actions[0].fields[1]"},
			"same target twice": {[]any{copyAction("MAH", "nv_number", "balance"), copyAction("MAH", "nv_number", "note")}, "actions[1]"},
		}
		many := make([]string, 31)
		for i := range many {
			many[i] = "balance"
		}
		cases["too many fields"] = struct {
			actions []any
			field   string
		}{[]any{copyAction("MAH", "nv_number", many...)}, "actions[0].fields"}
		for name, c := range cases {
			r := e.call("alice", "POST", base, map[string]any{"name": "v", "trigger": "item.updated", "actions": c.actions})
			if r.Status != 422 || errField(r) != c.field {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	rule := e.must(201, "alice", "POST", base, map[string]any{
		"name": "REY to MAH", "trigger": "item.updated",
		"conditions": []any{map[string]any{"field": "project", "op": "eq", "value": "ACME"}},
		"actions":    []any{copyAction("MAH", "nv_number", "balance", "monthly_payment")},
	}).Body
	if a := rule["actions"].([]any)[0].(map[string]any); a["project"] != "MAH" || a["match"] != "nv_number" || len(a["fields"].([]any)) != 2 {
		t.Fatalf("stored action: %v", a)
	}

	// Targets: MAH-1 (task) and MAH-3 (bug) share NV-1; MAH-2 is another customer.
	e.must(201, "alice", "POST", mahItems, map[string]any{"title": "Customer 1", "fields": map[string]any{"nv_number": "NV-1", "balance": 100, "note": "keep"}})
	e.must(201, "alice", "POST", mahItems, map[string]any{"title": "Customer 2", "fields": map[string]any{"nv_number": "NV-2", "balance": 7}})
	e.must(201, "alice", "POST", mahItems, map[string]any{"title": "Customer 1 bug", "type": "bug", "fields": map[string]any{"nv_number": "NV-1", "balance": 100}})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Lease 1", "fields": map[string]any{"nv_number": "NV-1", "balance": 100}})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Lease 9", "fields": map[string]any{"nv_number": "NV-9", "balance": 1}})
	e.drain()

	mahEvents := func() int {
		return e.count(`SELECT count(*) FROM events ev JOIN items i ON i.id = ev.entity_id JOIN projects p ON p.id = i.project_id
			WHERE p.key = 'MAH' AND ev.action = 'item.updated'`)
	}
	get := func(path string) map[string]any { return e.must(200, "alice", "GET", path, nil).Body }

	t.Run("copies changed listed fields", func(t *testing.T) {
		e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1,
			"fields": map[string]any{"balance": 250.5, "monthly_payment": 40, "note": "changed"}})
		e.drain()
		m1 := get(mahItems + "/1")
		f := m1["fields"].(map[string]any)
		if f["balance"] != 250.5 || f["monthly_payment"] != 40.0 || f["note"] != "keep" || f["nv_number"] != "NV-1" {
			t.Fatalf("MAH-1 fields: %v", f)
		}
		if m1["version"] != 2.0 {
			t.Fatalf("MAH-1 version: %v", m1["version"])
		}
		m3 := get(mahItems + "/3")["fields"].(map[string]any)
		if m3["balance"] != 250.5 || m3["monthly_payment"] != nil {
			t.Fatalf("MAH-3 (bug, no monthly_payment) fields: %v", m3)
		}
		if f := get(mahItems + "/2")["fields"].(map[string]any); f["balance"] != 7.0 {
			t.Fatalf("other customer touched: %v", f)
		}
		n := e.count(`SELECT count(*) FROM events ev JOIN items i ON i.id = ev.entity_id JOIN projects p ON p.id = i.project_id
			WHERE p.key = 'MAH' AND i.number = 1 AND ev.action = 'item.updated' AND ev.source = 'automation'
			AND ev.automation_rule_id = $1::uuid`, rule["id"])
		if n != 1 {
			t.Fatalf("automation-tagged update events on MAH-1: %d", n)
		}
	})

	t.Run("empty patch writes nothing", func(t *testing.T) {
		before := mahEvents()
		e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 2, "title": "Lease 1 renamed", "fields": map[string]any{"note": "again"}})
		e.drain()
		if got := mahEvents(); got != before {
			t.Fatalf("MAH events %d -> %d", before, got)
		}
		if v := get(mahItems + "/1")["version"]; v != 2.0 {
			t.Fatalf("MAH-1 version bumped: %v", v)
		}
	})

	t.Run("cleared source clears target", func(t *testing.T) {
		e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 3, "fields": map[string]any{"monthly_payment": nil}})
		e.drain()
		if f := get(mahItems + "/1")["fields"].(map[string]any); f["monthly_payment"] != nil || f["balance"] != 250.5 {
			t.Fatalf("MAH-1 fields after clear: %v", f)
		}
	})

	t.Run("no match", func(t *testing.T) {
		before := mahEvents()
		e.must(200, "carol", "PATCH", items+"/2", map[string]any{"version": 1, "fields": map[string]any{"balance": 999}})
		e.drain()
		if got := mahEvents(); got != before {
			t.Fatalf("MAH events %d -> %d", before, got)
		}
	})

	t.Run("reverse rule does not loop", func(t *testing.T) {
		e.must(201, "alice", "POST", base, map[string]any{
			"name": "MAH to REY", "trigger": "item.updated",
			"conditions": []any{map[string]any{"field": "project", "op": "eq", "value": "MAH"}},
			"actions":    []any{copyAction("ACME", "nv_number", "balance", "monthly_payment")},
		})
		acmeVersion := get(items + "/1")["version"].(float64)
		m1 := get(mahItems + "/1")
		e.must(200, "carol", "PATCH", mahItems+"/1", map[string]any{"version": m1["version"], "fields": map[string]any{"balance": 300}})
		e.drain()
		a1 := get(items + "/1")
		if a1["fields"].(map[string]any)["balance"] != 300.0 || a1["version"].(float64) != acmeVersion+1 {
			t.Fatalf("ACME-1 after reverse copy: %v v%v", a1["fields"], a1["version"])
		}
		// The ACME write came from automation: the REY→MAH rule does not run
		// again, and MAH-3 is not yet updated by this change.
		if f := get(mahItems + "/3")["fields"].(map[string]any); f["balance"] != 250.5 {
			t.Fatalf("MAH-3 changed by an automation-caused event: %v", f)
		}
	})

	if n := e.count(`SELECT count(*) FROM jobs WHERE state <> 'done'`); n != 0 {
		t.Fatalf("unfinished or failed jobs: %d", n)
	}
}
