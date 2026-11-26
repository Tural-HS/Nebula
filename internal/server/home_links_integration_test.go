package server

import (
	"slices"
	"testing"
)

// Home page links (ADR-0053): an admin picks projects; every member sees the
// ones they can open, in order; private projects stay hidden from others.
func TestHomeLinks(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ws := "/api/v1/workspaces/acme"
	sec := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Reyestr", "visibility": "private"}).Body["id"].(string)
	ops := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "OPS", "name": "Aylıq hesabat"}).Body["id"].(string)

	pinned := func(user string) []string {
		t.Helper()
		var out []string
		for _, x := range e.must(200, user, "GET", homePath, nil).Body["pinned"].([]any) {
			p := x.(map[string]any)
			out = append(out, p["workspace"].(map[string]any)["slug"].(string)+"/"+p["project"].(map[string]any)["key"].(string))
		}
		return out
	}
	if got := pinned("alice"); len(got) != 0 {
		t.Fatalf("no links yet, got %v", got)
	}

	// Members cannot change them; admins can, in their order.
	e.must(403, "carol", "PATCH", ws, map[string]any{"home_projects": []string{ops}})
	got := e.must(200, "alice", "PATCH", ws, map[string]any{"home_projects": []string{sec, ops}}).Body["home_projects"].([]any)
	if len(got) != 2 || got[0] != sec || got[1] != ops {
		t.Fatalf("stored %v", got)
	}
	if got := pinned("alice"); !slices.Equal(got, []string{"acme/SEC", "acme/OPS"}) {
		t.Fatalf("alice sees %v", got)
	}
	// carol is not in the private project: only the open one.
	if got := pinned("carol"); !slices.Equal(got, []string{"acme/OPS"}) {
		t.Fatalf("carol sees %v", got)
	}
	// dave is no member at all.
	if got := pinned("dave"); len(got) != 0 {
		t.Fatalf("dave sees %v", got)
	}

	// Validation: unknown or foreign ids, duplicates, more than eight.
	e.must(422, "alice", "PATCH", ws, map[string]any{"home_projects": []string{"0193b8a0-0000-7000-8000-000000000000"}})
	e.must(422, "alice", "PATCH", ws, map[string]any{"home_projects": []string{ops, ops}})
	nine := make([]string, 9)
	for i := range nine {
		nine[i] = ops
	}
	e.must(422, "alice", "PATCH", ws, map[string]any{"home_projects": nine})
	e.must(201, "dave", "POST", "/api/v1/workspaces", map[string]any{"slug": "other", "name": "Other"})
	foreign := e.must(201, "dave", "POST", "/api/v1/workspaces/other/projects", map[string]any{"key": "OTH", "name": "Other"}).Body["id"].(string)
	e.must(422, "alice", "PATCH", ws, map[string]any{"home_projects": []string{foreign}})

	// An archived project drops out of the home page; null clears the list.
	e.must(200, "alice", "POST", projectsPath+"/OPS/archive", nil)
	if got := pinned("alice"); !slices.Equal(got, []string{"acme/SEC"}) {
		t.Fatalf("after archive alice sees %v", got)
	}
	if b := e.must(200, "alice", "PATCH", ws, map[string]any{"home_projects": nil}).Body["home_projects"].([]any); len(b) != 0 {
		t.Fatalf("after clear %v", b)
	}
	if got := pinned("alice"); len(got) != 0 {
		t.Fatalf("after clear alice sees %v", got)
	}
}
