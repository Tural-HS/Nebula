package server

import "testing"

// TestProjectDefaultView: a project opens with a shared view of its own
// (ADR-0014 addendum); personal and foreign views are refused, and deleting
// or unsharing the view clears the default.
func TestProjectDefaultView(t *testing.T) {
	e := newEnv(t)
	e.setup()
	acme := "/api/v1/workspaces/acme/projects/ACME"
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"})

	shared := e.must(201, "alice", "POST", acme+"/views", map[string]any{"name": "Ödənişlər", "kind": "table", "shared": true,
		"config": map[string]any{"columns": []string{"key", "title", "status"}}}).Body["id"].(string)
	personal := e.must(201, "carol", "POST", acme+"/views", map[string]any{"name": "Mənim", "kind": "table", "shared": false,
		"config": map[string]any{"columns": []string{"key", "title"}}}).Body["id"].(string)
	foreign := e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects/OPS/views", map[string]any{"name": "Ops", "kind": "table", "shared": true,
		"config": map[string]any{"columns": []string{"key", "title"}}}).Body["id"].(string)

	// Only admins, only shared views of this project.
	if r := e.call("carol", "PATCH", acme, map[string]any{"default_view_id": shared}); r.Status != 403 {
		t.Fatalf("member sets default: %d", r.Status)
	}
	for name, id := range map[string]string{"personal": personal, "other project": foreign, "unknown": "01a20000-0000-7000-8000-000000000000"} {
		if r := e.call("alice", "PATCH", acme, map[string]any{"default_view_id": id}); r.Status != 422 {
			t.Fatalf("%s view as default: %d %s", name, r.Status, r.Raw)
		}
	}
	p := e.must(200, "alice", "PATCH", acme, map[string]any{"default_view_id": shared}).Body
	if p["default_view_id"] != shared {
		t.Fatalf("default not set: %v", p["default_view_id"])
	}
	// Members see it in the project list.
	list := e.must(200, "carol", "GET", "/api/v1/workspaces/acme/projects", nil).Body["projects"].([]any)
	if got := list[0].(map[string]any)["default_view_id"]; got != shared {
		t.Fatalf("member sees default %v", got)
	}

	// Unsharing the view clears the default; so does clearing it explicitly or deleting the view.
	e.must(200, "alice", "PATCH", acme+"/views/"+shared, map[string]any{"shared": false})
	if got := e.must(200, "alice", "GET", acme, nil).Body["default_view_id"]; got != nil {
		t.Fatalf("unshared view still default: %v", got)
	}
	e.must(200, "alice", "PATCH", acme+"/views/"+shared, map[string]any{"shared": true})
	e.must(200, "alice", "PATCH", acme, map[string]any{"default_view_id": shared})
	e.must(200, "alice", "PATCH", acme, map[string]any{"default_view_id": nil})
	if got := e.must(200, "alice", "GET", acme, nil).Body["default_view_id"]; got != nil {
		t.Fatalf("cleared default: %v", got)
	}
	e.must(200, "alice", "PATCH", acme, map[string]any{"default_view_id": shared})
	e.must(204, "alice", "DELETE", acme+"/views/"+shared, nil)
	if got := e.must(200, "alice", "GET", acme, nil).Body["default_view_id"]; got != nil {
		t.Fatalf("deleted view still default: %v", got)
	}
}

// TestWorkspaceKindDefaultView: finance projects open with all items and the money columns.
func TestWorkspaceKindDefaultView(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fin-dv", "name": "Fin", "template": "az", "kind": "finance"})
	p := e.must(200, "alice", "GET", "/api/v1/workspaces/fin-dv/projects/FIN", nil).Body
	id, _ := p["default_view_id"].(string)
	if id == "" {
		t.Fatal("finance project has no default view")
	}
	views := e.must(200, "alice", "GET", "/api/v1/workspaces/fin-dv/projects/FIN/views", nil).Body["views"].([]any)
	for _, v := range views {
		m := v.(map[string]any)
		if m["id"] != id {
			continue
		}
		cfg := m["config"].(map[string]any)
		cols := cfg["columns"].([]any)
		hasAmount := false
		for _, c := range cols {
			hasAmount = hasAmount || c == "field.amount"
		}
		if m["name"] != "Bütün tapşırıqlar" || !hasAmount || len(cfg["filters"].(map[string]any)) != 0 {
			t.Fatalf("default view: %v", m)
		}
		return
	}
	t.Fatalf("default view %s not in the project's views", id)
}
