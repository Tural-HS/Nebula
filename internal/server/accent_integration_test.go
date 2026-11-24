package server

import (
	"context"
	"encoding/json"
	"testing"
)

// Workspace accent colour (ADR-0035, migration 00028).
func TestWorkspaceAccent(t *testing.T) {
	e := newEnv(t)
	e.setup()

	if ws := e.must(200, "carol", "GET", wsBase, nil).Body; ws["accent"] != nil {
		t.Fatalf("new workspace accent = %v, want null (default)", ws["accent"])
	}
	for _, who := range []string{"bob", "carol"} { // guest, member
		if r := e.call(who, "PATCH", wsBase, map[string]any{"accent": "violet"}); r.Status != 403 {
			t.Fatalf("%s changes the accent: %d", who, r.Status)
		}
	}
	for _, bad := range []any{"red", "VIOLET", "", "#ff0000", 3, "url(javascript:alert(1))"} {
		want := 422 // unknown key
		if bad == 3 {
			want = 400 // not a string: rejected by the JSON decoder
		}
		if r := e.call("alice", "PATCH", wsBase, map[string]any{"accent": bad}); r.Status != want {
			t.Errorf("accent %v: %d %s", bad, r.Status, r.Raw)
		}
	}

	ws := e.must(200, "alice", "PATCH", wsBase, map[string]any{"accent": "violet"}).Body
	if ws["accent"] != "violet" {
		t.Fatalf("accent not applied: %v", ws)
	}
	// Every member sees it (it is a workspace setting, not a personal one).
	if got := e.must(200, "carol", "GET", wsBase, nil).Body["accent"]; got != "violet" {
		t.Fatalf("member sees accent %v", got)
	}
	// "teal" is the default and is stored as NULL, like null itself.
	if got := e.must(200, "alice", "PATCH", wsBase, map[string]any{"accent": "teal"}).Body["accent"]; got != nil {
		t.Fatalf("teal stored as %v, want null", got)
	}
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"accent": "forest"})
	if got := e.must(200, "alice", "PATCH", wsBase, map[string]any{"accent": nil}).Body["accent"]; got != nil {
		t.Fatalf("null accent = %v", got)
	}
	// Setting the same value again records nothing.
	before := e.count(`SELECT count(*) FROM events WHERE action = 'workspace.updated' AND payload->'changes' ? 'accent'`)
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"accent": nil})
	if after := e.count(`SELECT count(*) FROM events WHERE action = 'workspace.updated' AND payload->'changes' ? 'accent'`); after != before {
		t.Fatalf("no-op accent change appended an event (%d → %d)", before, after)
	}

	// workspace.updated carries from/to with "teal" for the default.
	rows, err := e.pool.Query(context.Background(),
		`SELECT payload->'changes'->'accent' FROM events WHERE action = 'workspace.updated' AND payload->'changes' ? 'accent' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []map[string]string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var c map[string]string
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("payload %s: %v", raw, err)
		}
		got = append(got, c)
	}
	want := []map[string]string{
		{"from": "teal", "to": "violet"},
		{"from": "violet", "to": "teal"},
		{"from": "teal", "to": "forest"},
		{"from": "forest", "to": "teal"},
	}
	if len(got) != len(want) {
		t.Fatalf("accent events: %v", got)
	}
	for i := range want {
		if got[i]["from"] != want[i]["from"] || got[i]["to"] != want[i]["to"] {
			t.Errorf("event %d: %v, want %v", i, got[i], want[i])
		}
	}
}
