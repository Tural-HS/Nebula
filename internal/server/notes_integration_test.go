package server

import (
	"strings"
	"testing"
)

const stickyPath = "/api/v1/me/notes"

// Sticky notes (ADR-0052): private to their author, validated, capped.
func TestNotes(t *testing.T) {
	e := newEnv(t)
	e.setup()

	if l := e.must(200, "alice", "GET", stickyPath, nil).Body["notes"].([]any); len(l) != 0 {
		t.Fatalf("new user has notes: %v", l)
	}
	n := e.must(201, "alice", "POST", stickyPath, map[string]any{"body": "  Elmara zəng et  "}).Body
	if n["body"] != "Elmara zəng et" || n["color"] != "yellow" || n["pinned"] != false {
		t.Fatalf("created: %v", n)
	}
	id := n["id"].(string)
	e.must(201, "alice", "POST", stickyPath, map[string]any{"body": "Hesabat", "color": "green", "pinned": true})

	for i, bad := range []map[string]any{
		{"body": ""}, {"body": "   "}, {}, {"body": strings.Repeat("a", 2001)}, {"body": "x", "color": "red"},
	} {
		if r := e.call("alice", "POST", stickyPath, bad); r.Status != 422 {
			t.Errorf("case %d: %d %s", i, r.Status, r.Raw)
		}
	}

	// Pinned first.
	list := e.must(200, "alice", "GET", stickyPath, nil).Body["notes"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["body"] != "Hesabat" {
		t.Fatalf("order: %v", list)
	}

	// Another user can neither see nor touch it.
	if l := e.must(200, "carol", "GET", stickyPath, nil).Body["notes"].([]any); len(l) != 0 {
		t.Fatalf("carol sees alice's notes: %v", l)
	}
	e.must(404, "carol", "PATCH", stickyPath+"/"+id, map[string]any{"body": "hack"})
	e.must(404, "carol", "DELETE", stickyPath+"/"+id, nil)

	u := e.must(200, "alice", "PATCH", stickyPath+"/"+id, map[string]any{"color": "blue", "pinned": true}).Body
	if u["body"] != "Elmara zəng et" || u["color"] != "blue" || u["pinned"] != true {
		t.Fatalf("updated: %v", u)
	}
	e.must(422, "alice", "PATCH", stickyPath+"/"+id, map[string]any{"body": nil})
	e.must(404, "alice", "PATCH", stickyPath+"/not-a-uuid", map[string]any{"body": "x"})
	e.must(204, "alice", "DELETE", stickyPath+"/"+id, nil)
	e.must(404, "alice", "DELETE", stickyPath+"/"+id, nil)
}
