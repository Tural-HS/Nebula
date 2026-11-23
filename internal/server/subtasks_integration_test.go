package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func subtaskSummary(t *testing.T, item map[string]any) (count, done float64) {
	t.Helper()
	s, ok := item["subtasks"].(map[string]any)
	if !ok {
		t.Fatalf("no subtasks summary: %v", item)
	}
	return s["count"].(float64), s["done"].(float64)
}

func taskStatusID(t *testing.T, e *env, name string) string {
	t.Helper()
	for _, it := range e.must(200, "carol", "GET", "/api/v1/workspaces/acme/item-types", nil).Body["item_types"].([]any) {
		if it.(map[string]any)["key"] != "task" {
			continue
		}
		for _, s := range it.(map[string]any)["statuses"].([]any) {
			if s.(map[string]any)["name"] == name {
				return s.(map[string]any)["id"].(string)
			}
		}
	}
	t.Fatalf("no task status %q", name)
	return ""
}

func listKeys(t *testing.T, r resp) []string {
	t.Helper()
	var keys []string
	for _, it := range r.Body["items"].([]any) {
		keys = append(keys, it.(map[string]any)["key"].(string))
	}
	return keys
}

func fieldOf(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["field"].(string)
		return s
	}
	return ""
}

func TestSubtasks(t *testing.T) {
	e := newEnv(t)
	e.setup() // alice owner, bob guest, carol member

	parent := e.must(201, "carol", "POST", items, map[string]any{"title": "Release 2.0"}).Body
	if parent["parent"] != nil {
		t.Fatalf("top-level item has a parent: %v", parent["parent"])
	}
	if n, d := subtaskSummary(t, parent); n != 0 || d != 0 {
		t.Fatalf("new item summary: %v/%v", d, n)
	}
	// The parent can be referenced by number, number string or key.
	for i, ref := range []any{1, "1", "ACME-1", "acme-1"} {
		r := e.must(201, "carol", "POST", items, map[string]any{"title": fmt.Sprintf("Step %d", i+1), "parent": ref}).Body
		p, ok := r["parent"].(map[string]any)
		if !ok || p["key"] != "ACME-1" || p["number"] != 1.0 || p["title"] != "Release 2.0" {
			t.Fatalf("parent %v: %v", ref, r["parent"])
		}
	}
	e.must(201, "carol", "POST", items, map[string]any{"title": "Unrelated"}) // ACME-6

	doneID := taskStatusID(t, e, "Done")
	sub := e.must(200, "carol", "GET", items+"/2", nil).Body
	e.must(200, "carol", "PATCH", items+"/2", map[string]any{"version": sub["version"], "status_id": doneID})

	got := e.must(200, "bob", "GET", items+"/1", nil).Body // guests read subtasks too
	if n, d := subtaskSummary(t, got); n != 4 || d != 1 {
		t.Fatalf("summary after one done: %v/%v", d, n)
	}

	t.Run("list filters", func(t *testing.T) {
		if keys := listKeys(t, e.must(200, "bob", "GET", items+"?parent=ACME-1", nil)); len(keys) != 4 {
			t.Fatalf("?parent: %v", keys)
		}
		if keys := listKeys(t, e.must(200, "bob", "GET", items+"?parent=6", nil)); len(keys) != 0 {
			t.Fatalf("?parent of an item without subtasks: %v", keys)
		}
		top := listKeys(t, e.must(200, "bob", "GET", items+"?top_level=true", nil))
		if len(top) != 2 || top[0] != "ACME-6" || top[1] != "ACME-1" {
			t.Fatalf("?top_level: %v", top)
		}
		counts := e.must(200, "bob", "GET", items+"/counts?top_level=true", nil).Body
		if counts["total"] != 2.0 {
			t.Fatalf("counts with top_level: %v", counts)
		}
		if list := e.must(200, "bob", "GET", items+"?top_level=true", nil).Body["items"].([]any); true {
			if n, _ := subtaskSummary(t, list[1].(map[string]any)); n != 4 {
				t.Fatalf("list row summary: %v", list[1])
			}
		}
		for _, q := range []string{"?parent=OTHER-1", "?parent=abc", "?parent=0", "?top_level=yes", "?parent=1&top_level=true"} {
			if r := e.call("bob", "GET", items+q, nil); r.Status != 400 {
				t.Errorf("GET %s: %d %s", q, r.Status, r.Raw)
			}
		}
	})

	t.Run("validation and depth", func(t *testing.T) {
		e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"})
		e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects/OPS/items", map[string]any{"title": "Ops 1"})
		cases := map[string]any{
			"subtask of a subtask": 2,
			"missing parent":       999,
			"other project key":    "OPS-1",
			"garbage":              "x-y-z",
			"zero":                 0,
			"boolean":              true,
		}
		for name, ref := range cases {
			r := e.call("carol", "POST", items, map[string]any{"title": "x", "parent": ref})
			want := 422
			if name == "boolean" {
				want = 400 // not a JSON number or string
			}
			if r.Status != want || (want == 422 && fieldOf(r) != "parent") {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
		if r := e.call("bob", "POST", items, map[string]any{"title": "x", "parent": 1}); r.Status != 403 {
			t.Errorf("guest creates a subtask: %d", r.Status)
		}

		one := e.must(200, "carol", "GET", items+"/1", nil).Body
		six := e.must(200, "carol", "GET", items+"/6", nil).Body
		patches := map[string]struct {
			path string
			body map[string]any
		}{
			"parent with subtasks becomes a subtask": {items + "/1", map[string]any{"version": one["version"], "parent": 6}},
			"own parent":                             {items + "/6", map[string]any{"version": six["version"], "parent": "ACME-6"}},
			"under a subtask":                        {items + "/6", map[string]any{"version": six["version"], "parent": 3}},
		}
		for name, p := range patches {
			if r := e.call("carol", "PATCH", p.path, p.body); r.Status != 422 || fieldOf(r) != "parent" {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("move, detach and events", func(t *testing.T) {
		three := e.must(200, "carol", "GET", items+"/3", nil).Body
		moved := e.must(200, "carol", "PATCH", items+"/3", map[string]any{"version": three["version"], "parent": "ACME-6"}).Body
		if moved["parent"].(map[string]any)["key"] != "ACME-6" {
			t.Fatalf("moved: %v", moved["parent"])
		}
		// Setting the same parent again is not a change.
		same := e.must(200, "carol", "PATCH", items+"/3", map[string]any{"version": moved["version"], "parent": 6}).Body
		if same["version"] != moved["version"] {
			t.Fatalf("no-op parent change bumped the version")
		}
		cleared := e.must(200, "carol", "PATCH", items+"/3", map[string]any{"version": same["version"], "parent": nil}).Body
		if cleared["parent"] != nil {
			t.Fatalf("cleared: %v", cleared["parent"])
		}
		var changes []string
		rows, err := e.pool.Query(context.Background(), `SELECT coalesce(payload->'changes'->'parent'->>'from', '-') || '>' ||
				coalesce(payload->'changes'->'parent'->>'to', '-') || ' ' || coalesce(payload->'snapshot'->>'parent', '-')
			FROM events e JOIN items i ON i.id = e.entity_id
			WHERE i.number = 3 AND e.action = 'item.updated' AND payload->'changes' ? 'parent' ORDER BY e.id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			changes = append(changes, s)
		}
		rows.Close()
		if fmt.Sprint(changes) != "[ACME-1>ACME-6 ACME-6 ACME-6>- -]" {
			t.Fatalf("parent change events: %v", changes)
		}

		// Deleting a parent makes its subtasks top-level, each with an event.
		e.must(204, "carol", "DELETE", items+"/1", nil)
		for _, n := range []int{2, 4, 5} {
			it := e.must(200, "carol", "GET", fmt.Sprintf("%s/%d", items, n), nil).Body
			if it["parent"] != nil {
				t.Fatalf("ACME-%d still has a parent: %v", n, it["parent"])
			}
		}
		var detached int
		_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE action = 'item.updated'
			AND payload->'changes'->'parent'->>'from' = 'ACME-1' AND payload->'changes'->'parent'->'to' = 'null'::jsonb`).Scan(&detached)
		if detached != 3 {
			t.Fatalf("detach events: %d", detached)
		}
		if r := e.call("carol", "POST", items, map[string]any{"title": "x", "parent": 1}); r.Status != 422 {
			t.Fatalf("subtask of a deleted item: %d", r.Status)
		}
	})
}

// TestSubtasksDatabaseGuards checks the schema rules on their own, as the
// runtime role, bypassing the store's friendlier checks.
func TestSubtasksDatabaseGuards(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for _, title := range []string{"A", "B", "C"} {
		e.must(201, "carol", "POST", items, map[string]any{"title": title})
	}
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "OPS", "name": "Ops"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects/OPS/items", map[string]any{"title": "Ops 1"})

	ctx := context.Background()
	setParent := func(child, parent string) error {
		_, err := e.pool.Exec(ctx, `UPDATE items c SET parent_id = p.id FROM items p, projects cp, projects pp
			WHERE cp.id = c.project_id AND pp.id = p.project_id
			  AND cp.key || '-' || c.number = $1 AND pp.key || '-' || p.number = $2`, child, parent)
		return err
	}
	wantConstraint := func(err error, name string) {
		t.Helper()
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.ConstraintName != name {
			t.Fatalf("want violation of %s, got %v", name, err)
		}
	}
	wantConstraint(setParent("ACME-1", "OPS-1"), "items_parent_fkey") // other project
	wantConstraint(setParent("ACME-1", "ACME-1"), "items_parent_not_self")
	if err := setParent("ACME-2", "ACME-1"); err != nil {
		t.Fatal(err)
	}
	wantConstraint(setParent("ACME-3", "ACME-2"), "items_parent_depth") // under a subtask
	wantConstraint(setParent("ACME-1", "ACME-3"), "items_parent_depth") // parent becomes a subtask
	if _, err := e.pool.Exec(ctx, `UPDATE items SET deleted_at = now() WHERE number = 3`); err != nil {
		t.Fatal(err)
	}
	wantConstraint(setParent("ACME-1", "ACME-3"), "items_parent_live")
}

// TestSubtasksConcurrentDepth races "X under Y" against "Z under X": only
// one may win, otherwise the tree would get a second level.
func TestSubtasksConcurrentDepth(t *testing.T) {
	e := newEnv(t)
	e.setup()
	const rounds = 15
	for i := 0; i < rounds*3; i++ {
		e.must(201, "carol", "POST", items, map[string]any{"title": fmt.Sprintf("item %d", i+1)})
	}
	for i := range rounds {
		y, x, z := 3*i+1, 3*i+2, 3*i+3
		var wg sync.WaitGroup
		statuses := make([]int, 2)
		wg.Go(func() {
			statuses[0] = e.call("carol", "PATCH", fmt.Sprintf("%s/%d", items, x), map[string]any{"version": 1, "parent": y}).Status
		})
		wg.Go(func() {
			statuses[1] = e.call("alice", "PATCH", fmt.Sprintf("%s/%d", items, z), map[string]any{"version": 1, "parent": x}).Status
		})
		wg.Wait()
		if statuses[0] == 200 && statuses[1] == 200 {
			t.Fatalf("round %d: both changes won", i)
		}
		if statuses[0] == 500 || statuses[1] == 500 {
			t.Fatalf("round %d: statuses %v", i, statuses)
		}
	}
	var deep int
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM items c JOIN items p ON p.id = c.parent_id
		WHERE p.parent_id IS NOT NULL`).Scan(&deep)
	if deep != 0 {
		t.Fatalf("%d items at depth 2", deep)
	}
}
