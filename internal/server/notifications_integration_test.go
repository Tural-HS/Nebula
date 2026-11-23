package server

import (
	"context"
	"fmt"
	"testing"
	"time"
)

const notesPath = "/api/v1/me/notifications"

// notes returns a user's notifications (newest first) for the given query.
func (e *p1env) notes(user, query string) []map[string]any {
	e.t.Helper()
	path := notesPath
	if query != "" {
		path += "?" + query
	}
	var out []map[string]any
	for _, n := range e.must(200, user, "GET", path, nil).Body["notifications"].([]any) {
		out = append(out, n.(map[string]any))
	}
	return out
}

func (e *p1env) unread(user string) int {
	e.t.Helper()
	return int(e.must(200, user, "GET", notesPath+"/unread-count", nil).Body["unread"].(float64))
}

// kinds summarizes notifications as "kind:ITEM-KEY" strings, newest first.
func kinds(list []map[string]any) []string {
	out := make([]string, len(list))
	for i, n := range list {
		out[i] = fmt.Sprintf("%s:%s", n["kind"], n["item_key"])
	}
	return out
}

func sameList(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestNotifications(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider
	ctx := context.Background()
	alice, bob, carol := e.userID("alice"), e.userID("bob"), e.userID("carol")

	// assigned: on create (not when assigning yourself) and on assignee change.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Fix login", "assignee_id": bob}) // ACME-1
	e.must(201, "carol", "POST", items, map[string]any{"title": "Mine", "assignee_id": carol})    // ACME-2
	e.must(200, "alice", "PATCH", items+"/2", map[string]any{"version": 1, "assignee_id": alice}) // self-assign by actor
	e.must(200, "alice", "PATCH", items+"/2", map[string]any{"version": 2, "assignee_id": carol})
	e.must(200, "alice", "PATCH", items+"/2", map[string]any{"version": 3, "title": "Renamed"}) // no assignee change
	e.drain()

	if got := kinds(e.notes("bob", "")); !sameList(got, []string{"assigned:ACME-1"}) {
		t.Fatalf("bob: %v", got)
	}
	if got := kinds(e.notes("carol", "")); !sameList(got, []string{"assigned:ACME-2"}) {
		t.Fatalf("carol (self-assignment must not notify): %v", got)
	}
	if got := e.notes("alice", ""); len(got) != 0 {
		t.Fatalf("alice notified about her own actions: %v", kinds(got))
	}
	n := e.notes("carol", "")[0]
	if n["actor_name"] != "Alice" || n["workspace_slug"] != "acme" || n["project_key"] != "ACME" ||
		n["item_number"] != 2.0 || n["item_title"] != "Renamed" || n["read"] != false {
		t.Fatalf("entry does not carry what the UI needs (title as of now): %v", n)
	}

	// comment and mention: one notification per person; the actor gets none.
	// ACME-1: reporter carol, assignee bob.
	e.must(201, "alice", "POST", items+"/1/comments", map[string]any{"body": "Look " + mention(bob) + " " + mention(alice)})
	e.drain()
	if got := kinds(e.notes("bob", "")); !sameList(got, []string{"mention:ACME-1", "assigned:ACME-1"}) {
		t.Fatalf("bob after mention (mention wins over comment): %v", got)
	}
	if got := kinds(e.notes("carol", "")); !sameList(got, []string{"comment:ACME-1", "assigned:ACME-2"}) {
		t.Fatalf("carol as reporter: %v", got)
	}
	if got := e.notes("alice", ""); len(got) != 0 {
		t.Fatalf("author mentioned herself: %v", kinds(got))
	}

	// Editing a comment notifies only newly added mentions.
	c := e.must(201, "carol", "POST", items+"/1/comments", map[string]any{"body": "Working on it"}).Body
	e.drain()
	e.must(200, "carol", "PATCH", items+"/1/comments/"+c["id"].(string), map[string]any{"body": "Working on it, " + mention(alice)})
	e.drain()
	e.must(200, "carol", "PATCH", items+"/1/comments/"+c["id"].(string), map[string]any{"body": "Working on it!! " + mention(alice)})
	e.drain()
	if got := kinds(e.notes("alice", "")); !sameList(got, []string{"mention:ACME-1"}) {
		t.Fatalf("alice after edit: %v", got)
	}
	if got := kinds(e.notes("bob", "")); !sameList(got, []string{"comment:ACME-1", "mention:ACME-1", "assigned:ACME-1"}) {
		t.Fatalf("bob as assignee of a commented item: %v", got)
	}

	// overdue: the scheduler event notifies the assignee; there is no actor.
	e.must(201, "alice", "POST", items, map[string]any{"title": "Late", "due_date": "2020-01-15", "assignee_id": bob}) // ACME-3
	if _, err := e.st.EmitOverdue(ctx, 100); err != nil {
		t.Fatal(err)
	}
	e.drain()
	bobs := e.notes("bob", "")
	if got := kinds(bobs[:2]); !sameList(got, []string{"overdue:ACME-3", "assigned:ACME-3"}) {
		t.Fatalf("bob overdue: %v", kinds(bobs))
	}
	if bobs[0]["actor_name"] != nil || bobs[0]["actor_id"] != nil {
		t.Fatalf("overdue has no actor: %v", bobs[0])
	}

	t.Run("dedupe on job retry", func(t *testing.T) {
		var before, after int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&before)
		// A worker died after the work but before marking the jobs done.
		if _, err := e.pool.Exec(ctx, `UPDATE jobs SET state = 'pending', run_at = now(), finished_at = NULL, attempts = 0
			WHERE kind = 'notifications.create'`); err != nil {
			t.Fatal(err)
		}
		e.drain()
		// And the dispatch ran again: FanOut must not enqueue a second job.
		var eventID int64
		_ = e.pool.QueryRow(ctx, `SELECT event_id FROM notifications ORDER BY id LIMIT 1`).Scan(&eventID)
		ev, err := e.st.GetEvent(ctx, eventID)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.st.FanOut(ctx, ev); err != nil {
			t.Fatal(err)
		}
		if n, err := e.st.CreateNotifications(ctx, ev); err != nil || n != 0 {
			t.Fatalf("direct re-run created %d (%v)", n, err)
		}
		e.drain()
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&after)
		var jobs int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = 'notifications.create' AND payload->>'event_id' = $1`,
			fmt.Sprint(eventID)).Scan(&jobs)
		if before != after || jobs != 1 {
			t.Fatalf("retry duplicated: %d -> %d notifications, %d jobs for one event", before, after, jobs)
		}
	})

	t.Run("unread count, mark read, mark all", func(t *testing.T) {
		if got := e.unread("bob"); got != 5 {
			t.Fatalf("bob unread: %d", got)
		}
		first := e.notes("bob", "")[0]
		id := fmt.Sprint(int64(first["id"].(float64)))
		if r := e.call("carol", "POST", notesPath+"/"+id+"/read", nil); r.Status != 404 {
			t.Fatalf("someone else's notification: %d", r.Status)
		}
		e.must(204, "bob", "POST", notesPath+"/"+id+"/read", nil)
		e.must(204, "bob", "POST", notesPath+"/"+id+"/read", nil) // idempotent
		if r := e.call("bob", "POST", notesPath+"/abc/read", nil); r.Status != 404 {
			t.Fatalf("bad id: %d", r.Status)
		}
		if got := e.unread("bob"); got != 4 {
			t.Fatalf("after mark read: %d", got)
		}
		unread := e.notes("bob", "unread=true")
		if len(unread) != 4 || unread[0]["id"] == first["id"] {
			t.Fatalf("unread filter: %v", kinds(unread))
		}
		if r := e.call("bob", "GET", notesPath+"?unread=yes", nil); r.Status != 400 {
			t.Fatalf("bad unread: %d", r.Status)
		}
		if r := e.call("bob", "GET", notesPath+"?cursor=x", nil); r.Status != 400 {
			t.Fatalf("bad cursor: %d", r.Status)
		}
		if r := e.call("bob", "POST", notesPath+"/read-all?workspace=nope", nil); r.Status != 404 {
			t.Fatalf("read-all in unknown workspace: %d", r.Status)
		}
		if m := e.must(200, "bob", "POST", notesPath+"/read-all", nil).Body["marked"]; m != 4.0 {
			t.Fatalf("marked: %v", m)
		}
		if got := e.unread("bob"); got != 0 || len(e.notes("bob", "")) != 5 {
			t.Fatalf("after read-all: unread %d", got)
		}
		if got := e.unread("carol"); got != 2 {
			t.Fatalf("read-all touched another user: carol %d", got)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		all := e.notes("bob", "")
		var paged []map[string]any
		cursor := ""
		for range 10 {
			q := "limit=2"
			if cursor != "" {
				q += "&cursor=" + cursor
			}
			body := e.must(200, "bob", "GET", notesPath+"?"+q, nil).Body
			for _, n := range body["notifications"].([]any) {
				paged = append(paged, n.(map[string]any))
			}
			next, _ := body["next_cursor"].(string)
			if next == "" {
				break
			}
			cursor = next
		}
		if !sameList(kinds(paged), kinds(all)) || len(paged) != 5 {
			t.Fatalf("pages %v != list %v", kinds(paged), kinds(all))
		}
		for i := 1; i < len(paged); i++ {
			if paged[i]["id"].(float64) >= paged[i-1]["id"].(float64) {
				t.Fatal("not newest first")
			}
		}
	})

	t.Run("privacy", func(t *testing.T) {
		e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Secret", "visibility": "private"})
		e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
		e.must(201, "carol", "POST", secItems, map[string]any{"title": "Müqavilə"}) // SEC-1, reporter carol
		e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli", "assignee_id": carol})
		e.drain()
		if got := kinds(e.notes("carol", "")); got[0] != "assigned:SEC-2" {
			t.Fatalf("project member: %v", got)
		}
		unreadBefore := e.unread("carol")

		// Access revoked: old notifications disappear, and new events on her
		// item no longer reach her, although she is still its reporter.
		e.must(204, "alice", "DELETE", secPath+"/members/"+carol, nil)
		e.must(201, "alice", "POST", secItems+"/1/comments", map[string]any{"body": "Yeni şərh"})
		e.drain()
		for _, n := range e.notes("carol", "") {
			if n["project_key"] == "SEC" {
				t.Fatalf("private project's notification visible after access was revoked: %v", n)
			}
		}
		if got := e.unread("carol"); got != unreadBefore-1 {
			t.Fatalf("unread count still includes hidden notifications: %d (was %d)", got, unreadBefore)
		}
		var stored int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id = $1 AND kind = 'comment'
			AND item_id = (SELECT id FROM items WHERE title = 'Müqavilə')`, carol).Scan(&stored)
		if stored != 0 {
			t.Fatal("a notification was created for someone who cannot see the project")
		}
		// Access restored: visible again (it was hidden, not deleted).
		e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
		if got := e.unread("carol"); got != unreadBefore {
			t.Fatalf("restored access: %d", got)
		}

		// Leaving the workspace deletes them (membership FK).
		e.must(204, "alice", "DELETE", "/api/v1/workspaces/acme/members/"+carol, nil)
		var left int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id = $1`, carol).Scan(&left)
		if left != 0 || len(e.notes("carol", "")) != 0 {
			t.Fatalf("notifications survived leaving the workspace: %d", left)
		}

		// Deleted items drop out of the list.
		before := len(e.notes("bob", ""))
		e.must(204, "alice", "DELETE", items+"/3", nil)
		if got := len(e.notes("bob", "")); got != before-2 {
			t.Fatalf("deleted item still listed: %d -> %d", before, got)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		e.must(201, "dave", "POST", "/api/v1/workspaces", map[string]any{"slug": "beta", "name": "Beta"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/beta/projects", map[string]any{"key": "BETA", "name": "Beta"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/beta/members", map[string]any{"email": "bob@example.com", "role": "member"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/beta/projects/BETA/items", map[string]any{"title": "B", "assignee_id": bob})
		e.drain()

		list := e.notes("bob", "")
		if list[0]["workspace_slug"] != "beta" || list[0]["item_key"] != "BETA-1" || list[1]["workspace_slug"] != "acme" {
			t.Fatalf("bob across workspaces: %v", kinds(list))
		}
		if len(e.notes("dave", "")) != 0 {
			t.Fatal("dave sees others' notifications")
		}
		e.must(201, "alice", "POST", items, map[string]any{"title": "Acme again", "assignee_id": bob})
		e.drain()
		if got := e.unread("bob"); got != 2 {
			t.Fatalf("bob unread in two workspaces: %d", got)
		}
		if r := e.call("dave", "POST", notesPath+"/read-all?workspace=acme", nil); r.Status != 404 {
			t.Fatalf("read-all for a workspace one is not in: %d", r.Status)
		}
		if m := e.must(200, "bob", "POST", notesPath+"/read-all?workspace=beta", nil).Body["marked"]; m != 1.0 {
			t.Fatalf("per-workspace read-all: %v", m)
		}
		if u := e.notes("bob", "unread=true"); len(u) != 1 || u[0]["workspace_slug"] != "acme" {
			t.Fatalf("after beta read-all: %v", kinds(u))
		}
	})

	t.Run("retention", func(t *testing.T) {
		// bob: everything read except one unread; age them.
		if _, err := e.pool.Exec(ctx, `UPDATE notifications SET created_at = now() - interval '100 days' WHERE recipient_id = $1`, bob); err != nil {
			t.Fatal(err)
		}
		var total, unreadOld int
		_ = e.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE read_at IS NULL) FROM notifications WHERE recipient_id = $1`, bob).
			Scan(&total, &unreadOld)
		if _, err := e.st.PurgeFinished(ctx, 7*24*time.Hour, 30*24*time.Hour); err != nil {
			t.Fatal(err)
		}
		var left int
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id = $1`, bob).Scan(&left)
		if unreadOld == 0 || left != unreadOld {
			t.Fatalf("100 days: %d rows, %d unread, %d left (only unread may stay)", total, unreadOld, left)
		}
		if _, err := e.pool.Exec(ctx, `UPDATE notifications SET created_at = now() - interval '181 days' WHERE recipient_id = $1`, bob); err != nil {
			t.Fatal(err)
		}
		if _, err := e.st.PurgeNotifications(ctx, 90*24*time.Hour, 180*24*time.Hour, 1); err != nil {
			t.Fatal(err)
		}
		_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id = $1`, bob).Scan(&left)
		if left != 0 {
			t.Fatalf("181 days: %d left", left)
		}
		if len(e.notes("alice", "")) == 0 {
			t.Fatal("retention removed recent notifications of others")
		}
	})
}
