package server

import (
	"context"
	"strings"
	"testing"
)

// linksOf returns an item's links as "type:KEY" strings, oldest first.
func (e *p1env) linksOf(user, itemPath string) []string {
	e.t.Helper()
	var out []string
	for _, l := range e.must(200, user, "GET", itemPath+"/links", nil).Body["links"].([]any) {
		m := l.(map[string]any)
		out = append(out, m["type"].(string)+":"+m["item"].(map[string]any)["key"].(string))
	}
	return out
}

// watcherNames returns the watcher list of an item as seen by user.
func (e *p1env) watcherNames(user, itemPath string) (names []string, watching bool, count float64) {
	e.t.Helper()
	b := e.must(200, user, "GET", itemPath+"/watchers", nil).Body
	for _, w := range b["watchers"].([]any) {
		names = append(names, w.(map[string]any)["display_name"].(string))
	}
	return names, b["watching"].(bool), b["count"].(float64)
}

func linkActions(t *testing.T, e *p1env, user, itemPath string) []string {
	t.Helper()
	var out []string
	for _, a := range e.must(200, user, "GET", itemPath+"/activity", nil).Body["activity"].([]any) {
		m := a.(map[string]any)
		act := m["action"].(string)
		if !strings.HasPrefix(act, "link.") {
			continue
		}
		d := m["data"].(map[string]any)
		key := "?"
		if o, ok := d["other_item"].(map[string]any); ok {
			key = o["key"].(string)
		}
		out = append(out, act+":"+d["type"].(string)+":"+key)
	}
	return out
}

func TestItemLinks(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider
	for _, title := range []string{"Close the month", "Reconcile bank", "Send report"} {
		e.must(201, "carol", "POST", items, map[string]any{"title": title}) // ACME-1..3
	}
	one, two, three := items+"/1", items+"/2", items+"/3"

	l := e.must(201, "carol", "POST", two+"/links", map[string]any{"type": "blocks", "target": "acme-1"}).Body
	if l["type"] != "blocks" || l["item"].(map[string]any)["key"] != "ACME-1" {
		t.Fatalf("created link: %v", l)
	}
	if got := e.linksOf("bob", one); !sameList(got, []string{"blocked_by:ACME-2"}) { // guests read links
		t.Fatalf("seen from the target: %v", got)
	}
	if got := e.linksOf("carol", two); !sameList(got, []string{"blocks:ACME-1"}) {
		t.Fatalf("seen from the source: %v", got)
	}

	t.Run("refused writes", func(t *testing.T) {
		cases := []struct {
			name, path string
			body       map[string]any
			status     int
		}{
			{"same link again", two, map[string]any{"type": "blocks", "target": "ACME-1"}, 409},
			{"same pair, other direction", one, map[string]any{"type": "blocks", "target": "ACME-2"}, 409},
			{"inverse name of the same link", one, map[string]any{"type": "blocked_by", "target": "ACME-2"}, 409},
			{"self link", one, map[string]any{"type": "relates", "target": "ACME-1"}, 422},
			{"no such item", one, map[string]any{"type": "relates", "target": "ACME-99"}, 422},
			{"not a key", one, map[string]any{"type": "relates", "target": "1"}, 422},
			{"bad type", one, map[string]any{"type": "parent_of", "target": "ACME-3"}, 422},
		}
		for _, c := range cases {
			if r := e.call("carol", "POST", c.path+"/links", c.body); r.Status != c.status {
				t.Errorf("%s: %d %s", c.name, r.Status, r.Raw)
			}
		}
		if r := e.call("bob", "POST", one+"/links", map[string]any{"type": "relates", "target": "ACME-3"}); r.Status != 403 {
			t.Errorf("guest links: %d", r.Status)
		}
		if r := e.call("dave", "GET", one+"/links", nil); r.Status != 404 {
			t.Errorf("outsider reads links: %d", r.Status)
		}
	})

	// Another type for the same pair is a different link.
	e.must(201, "carol", "POST", one+"/links", map[string]any{"type": "relates", "target": "ACME-2"})
	e.must(201, "carol", "POST", three+"/links", map[string]any{"type": "duplicated_by", "target": "ACME-1"})
	if got := e.linksOf("carol", one); !sameList(got, []string{"blocked_by:ACME-2", "relates:ACME-2", "duplicates:ACME-3"}) {
		t.Fatalf("links of ACME-1: %v", got)
	}

	t.Run("blocked indicator and filter", func(t *testing.T) {
		list := e.must(200, "carol", "GET", items, nil).Body["items"].([]any)
		blocked := map[string]bool{}
		for _, it := range list {
			m := it.(map[string]any)
			blocked[m["key"].(string)] = m["blocked"].(bool)
		}
		if !blocked["ACME-1"] || blocked["ACME-2"] || blocked["ACME-3"] {
			t.Fatalf("blocked flags: %v", blocked)
		}
		if got := listKeys(t, e.must(200, "carol", "GET", items+"?blocked=true", nil)); !sameList(got, []string{"ACME-1"}) {
			t.Fatalf("?blocked=true: %v", got)
		}
		if c := e.must(200, "carol", "GET", items+"/counts?blocked=true", nil).Body["total"]; c != 1.0 {
			t.Fatalf("counts ?blocked=true: %v", c)
		}
		if r := e.call("carol", "GET", items+"?blocked=yes", nil); r.Status != 400 {
			t.Fatalf("?blocked=yes: %d", r.Status)
		}
		// A done blocker no longer blocks.
		item := e.must(200, "carol", "GET", two, nil).Body
		e.must(200, "carol", "PATCH", two, map[string]any{"version": item["version"], "status_id": taskStatusID(t, e.env, "Done")})
		if got := listKeys(t, e.must(200, "carol", "GET", items+"?blocked=true", nil)); len(got) != 0 {
			t.Fatalf("done blocker still blocks: %v", got)
		}
		links := e.must(200, "carol", "GET", one+"/links", nil).Body["links"].([]any)
		if st := links[0].(map[string]any)["item"].(map[string]any)["status"].(map[string]any); st["category"] != "done" {
			t.Fatalf("linked item status: %v", st)
		}
	})

	t.Run("activity on both items", func(t *testing.T) {
		if got := linkActions(t, e, "carol", one); !sameList(got, []string{
			"link.added:duplicates:ACME-3", "link.added:relates:ACME-2", "link.added:blocked_by:ACME-2"}) {
			t.Fatalf("ACME-1 activity: %v", got)
		}
		if got := linkActions(t, e, "carol", three); !sameList(got, []string{"link.added:duplicated_by:ACME-1"}) {
			t.Fatalf("ACME-3 activity: %v", got)
		}
	})

	t.Run("remove", func(t *testing.T) {
		id := l["id"].(string)
		if r := e.call("bob", "DELETE", one+"/links/"+id, nil); r.Status != 403 {
			t.Fatalf("guest removes: %d", r.Status)
		}
		if r := e.call("carol", "DELETE", three+"/links/"+id, nil); r.Status != 404 {
			t.Fatalf("removing through an item that is not part of the link: %d", r.Status)
		}
		e.must(204, "carol", "DELETE", one+"/links/"+id, nil) // either side may remove it
		if r := e.call("carol", "DELETE", one+"/links/"+id, nil); r.Status != 404 {
			t.Fatalf("second delete: %d", r.Status)
		}
		if got := linkActions(t, e, "carol", two)[0]; got != "link.removed:blocks:ACME-1" {
			t.Fatalf("ACME-2 history: %v", got)
		}
	})

	t.Run("deleting an item removes its links", func(t *testing.T) {
		e.must(204, "carol", "DELETE", three, nil)
		if got := e.linksOf("carol", one); !sameList(got, []string{"relates:ACME-2"}) {
			t.Fatalf("after delete: %v", got)
		}
		var n int
		if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM item_links l JOIN items i ON i.id IN (l.source_id, l.target_id)
			WHERE i.deleted_at IS NOT NULL`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("links to deleted items: %d %v", n, err)
		}
		if got := linkActions(t, e, "carol", one)[0]; got != "link.removed:duplicates:ACME-3" {
			t.Fatalf("ACME-1 history after delete: %v", got)
		}
	})
}

func TestItemLinksPrivacyAndTenancy(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	ctx := context.Background()
	e.must(201, "carol", "POST", items, map[string]any{"title": "Public work"}) // ACME-1
	e.must(201, "carol", "POST", items, map[string]any{"title": "Public too"})  // ACME-2
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hidden", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Confidential case"}) // SEC-1

	// alice (owner) sees both projects and links across them.
	e.must(201, "alice", "POST", items+"/1/links", map[string]any{"type": "relates", "target": "SEC-1"})
	hidden := e.must(201, "alice", "POST", secItems+"/1/links", map[string]any{"type": "blocks", "target": "ACME-2"}).Body
	e.must(201, "alice", "POST", items+"/1/links", map[string]any{"type": "relates", "target": "ACME-2"})
	if got := e.linksOf("alice", items+"/1"); !sameList(got, []string{"relates:SEC-1", "relates:ACME-2"}) {
		t.Fatalf("owner: %v", got)
	}

	t.Run("hidden side is invisible", func(t *testing.T) {
		for _, path := range []string{items + "/1/links", items + "/2/links", items + "/1/activity", items + "/2/activity", items} {
			r := e.must(200, "carol", "GET", path, nil)
			if strings.Contains(r.Raw, "SEC") || strings.Contains(r.Raw, "Confidential") {
				t.Errorf("%s leaks the private item: %s", path, r.Raw)
			}
		}
		if got := e.linksOf("carol", items+"/1"); !sameList(got, []string{"relates:ACME-2"}) {
			t.Fatalf("carol: %v", got)
		}
		if got := e.linksOf("carol", items+"/2"); !sameList(got, []string{"relates:ACME-1"}) {
			t.Fatalf("carol on ACME-2: %v", got)
		}
		// Only a visible blocker counts: ACME-2 is blocked for alice, not for carol.
		for user, want := range map[string]bool{"alice": true, "carol": false} {
			list := e.must(200, user, "GET", items+"?blocked=true", nil)
			if got := len(listKeys(t, list)) == 1; got != want {
				t.Errorf("%s ?blocked=true: %v", user, listKeys(t, list))
			}
		}
		if got := linkActions(t, e, "alice", items+"/2"); !sameList(got, []string{"link.added:relates:ACME-1", "link.added:blocked_by:SEC-1"}) {
			t.Fatalf("owner's history: %v", got)
		}
	})

	t.Run("writes refused like a missing item", func(t *testing.T) {
		missing := e.call("carol", "POST", items+"/2/links", map[string]any{"type": "relates", "target": "NOPE-1"})
		hiddenTarget := e.call("carol", "POST", items+"/2/links", map[string]any{"type": "relates", "target": "SEC-1"})
		if hiddenTarget.Status != 422 || missing.Status != 422 || hiddenTarget.Raw != missing.Raw {
			t.Fatalf("hidden target distinguishable: %d %s vs %d %s", hiddenTarget.Status, hiddenTarget.Raw, missing.Status, missing.Raw)
		}
		if r := e.call("carol", "DELETE", items+"/2/links/"+hidden["id"].(string), nil); r.Status != 404 {
			t.Fatalf("removing a link to a hidden item: %d", r.Status)
		}
		if r := e.call("carol", "GET", secItems+"/1/links", nil); r.Status != 404 {
			t.Fatalf("links of a hidden item: %d", r.Status)
		}
	})

	t.Run("never across workspaces", func(t *testing.T) {
		e.must(201, "dave", "POST", "/api/v1/workspaces", map[string]any{"slug": "other", "name": "Other"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/other/projects", map[string]any{"key": "OTH", "name": "Other"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/other/projects/OTH/items", map[string]any{"title": "Elsewhere"})
		e.must(201, "dave", "POST", "/api/v1/workspaces/other/members", map[string]any{"email": "alice@example.com", "role": "member"})
		// alice can see OTH-1, but it lives in another workspace.
		if r := e.call("alice", "POST", items+"/1/links", map[string]any{"type": "relates", "target": "OTH-1"}); r.Status != 422 {
			t.Fatalf("cross-workspace link: %d %s", r.Status, r.Raw)
		}
		// The schema refuses it too (composite FKs).
		_, err := e.pool.Exec(ctx, `INSERT INTO item_links (id, workspace_id, source_id, target_id, type)
			SELECT gen_random_uuid(), a.workspace_id, a.id, o.id, 'relates'
			FROM items a, items o WHERE a.workspace_id <> o.workspace_id LIMIT 1`)
		if err == nil || !strings.Contains(err.Error(), "foreign key") {
			t.Fatalf("schema allowed a cross-workspace link: %v", err)
		}
	})
}

func TestWatchers(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider
	bob, carol, dave := e.userID("bob"), e.userID("carol"), e.userID("dave")
	one := items + "/1"

	// Auto-watch: reporter on create, assignee on assignment, commenters, mentions.
	e.must(201, "carol", "POST", items, map[string]any{"title": "Quarter close"}) // ACME-1
	if names, watching, n := e.watcherNames("carol", one); !sameList(names, []string{"Carol"}) || !watching || n != 1 {
		t.Fatalf("reporter: %v %v %v", names, watching, n)
	}
	e.must(200, "carol", "PATCH", one, map[string]any{"version": 1, "assignee_id": bob})
	erin := e.userID("erin")
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "erin@example.com", "role": "member"})
	e.must(201, "alice", "POST", one+"/comments", map[string]any{"body": "FYI " + mention(erin)})
	names, watching, _ := e.watcherNames("bob", one)
	if !sameList(names, []string{"Alice", "Bob", "Carol", "Erin"}) || watching != true {
		t.Fatalf("auto-watchers: %v (bob watching %v)", names, watching)
	}

	t.Run("watch and unwatch", func(t *testing.T) {
		e.must(201, "carol", "POST", items, map[string]any{"title": "Other"}) // ACME-2
		two := items + "/2"
		if b := e.must(200, "bob", "PUT", two+"/watchers/me", nil).Body; b["watching"] != true || b["count"] != 2.0 {
			t.Fatalf("guest watches: %v", b)
		}
		e.must(200, "bob", "PUT", two+"/watchers/me", nil) // idempotent
		if b := e.must(200, "bob", "DELETE", two+"/watchers/me", nil).Body; b["watching"] != false || b["count"] != 1.0 {
			t.Fatalf("unwatch: %v", b)
		}
		if r := e.call("carol", "PUT", two+"/watchers/"+bob, nil); r.Status != 403 {
			t.Fatalf("member adds someone else: %d", r.Status)
		}
		e.must(200, "alice", "PUT", two+"/watchers/"+bob, nil)
		e.must(200, "alice", "DELETE", two+"/watchers/"+bob, nil)
		if r := e.call("alice", "PUT", two+"/watchers/"+dave, nil); r.Status != 422 {
			t.Fatalf("admin adds an outsider: %d", r.Status)
		}
		if r := e.call("dave", "PUT", two+"/watchers/me", nil); r.Status != 404 {
			t.Fatalf("outsider watches: %d", r.Status)
		}
		// Explicit changes are audited, but stay out of the item's feed.
		var n int
		if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE action LIKE 'watcher.%'`).Scan(&n); err != nil || n != 4 {
			t.Fatalf("watcher events: %d %v", n, err)
		}
		for _, a := range e.must(200, "alice", "GET", two+"/activity", nil).Body["activity"].([]any) {
			if strings.HasPrefix(a.(map[string]any)["action"].(string), "watcher.") {
				t.Fatalf("watcher event in the feed: %v", a)
			}
		}
	})

	t.Run("fan-out without duplicates or self-notification", func(t *testing.T) {
		e.drain()
		if _, err := e.pool.Exec(context.Background(), `DELETE FROM notifications`); err != nil {
			t.Fatal(err)
		}
		item := e.must(200, "carol", "GET", one, nil).Body
		v := item["version"].(float64)
		// alice moves it on and gives it to carol: carol (reporter, watcher,
		// new assignee) gets one notification per event.
		e.must(200, "alice", "PATCH", one, map[string]any{"version": v, "status_id": taskStatusID(t, e.env, "In Progress")})
		e.must(200, "alice", "PATCH", one, map[string]any{"version": v + 1, "assignee_id": carol})
		e.must(200, "alice", "PATCH", one, map[string]any{"version": v + 2, "due_date": "2030-01-31"})
		e.must(201, "carol", "POST", one+"/comments", map[string]any{"body": "On it"})
		e.must(201, "carol", "POST", items, map[string]any{"title": "Prerequisite"}) // ACME-3
		e.must(201, "alice", "POST", one+"/links", map[string]any{"type": "blocked_by", "target": "ACME-3"})
		e.drain()
		if got := kinds(e.notes("carol", "")); !sameList(got, []string{
			"link_added:ACME-3", "due_changed:ACME-1", "assigned:ACME-1", "status_changed:ACME-1"}) {
			// carol watches both linked items: one notification, from the source (ACME-3 blocks ACME-1).
			t.Fatalf("carol: %v", got)
		}
		if got := kinds(e.notes("bob", "")); !sameList(got, []string{
			"link_added:ACME-1", "comment:ACME-1", "due_changed:ACME-1", "assignee_changed:ACME-1", "status_changed:ACME-1"}) {
			t.Fatalf("bob (former assignee, watcher): %v", got)
		}
		if got := kinds(e.notes("erin", "")); !sameList(got, []string{
			"link_added:ACME-1", "comment:ACME-1", "due_changed:ACME-1", "assignee_changed:ACME-1", "status_changed:ACME-1"}) {
			t.Fatalf("mentioned watcher: %v", got)
		}
		// alice watches (she commented), but made the changes herself.
		if got := kinds(e.notes("alice", "")); !sameList(got, []string{"comment:ACME-1"}) {
			t.Fatalf("alice: %v", got)
		}
	})

	t.Run("bulk edits do not notify watchers", func(t *testing.T) {
		e.drain()
		if _, err := e.pool.Exec(context.Background(), `DELETE FROM notifications`); err != nil {
			t.Fatal(err)
		}
		item := e.must(200, "carol", "GET", one, nil).Body
		e.must(200, "alice", "POST", items+"/bulk", map[string]any{
			"items": []map[string]any{{"number": 1, "version": item["version"]}}, "due_date": "2031-03-01"})
		e.drain()
		var n int
		if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("bulk notified %d", n)
		}
	})
}

func TestWatchersLoseAccess(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	carol := e.userID("carol")
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hidden", "visibility": "private"})
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Audit file"}) // SEC-1
	e.must(201, "alice", "POST", items, map[string]any{"title": "Open item"})     // ACME-1
	sec := secItems + "/1"
	e.must(200, "carol", "PUT", sec+"/watchers/me", nil)
	e.must(200, "carol", "PUT", items+"/1/watchers/me", nil)

	// A link to an item carol cannot see does not notify her.
	e.must(204, "alice", "DELETE", secPath+"/members/"+carol, nil)
	e.must(201, "alice", "POST", items+"/1/links", map[string]any{"type": "relates", "target": "SEC-1"})
	item := e.must(200, "alice", "GET", sec, nil).Body
	e.must(200, "alice", "PATCH", sec, map[string]any{"version": item["version"], "title": "Audit file v2", "due_date": "2030-05-05"})
	e.drain()
	if got := e.notes("carol", ""); len(got) != 0 {
		t.Fatalf("carol lost access but was notified: %v", kinds(got))
	}
	var rows int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE recipient_id = $1`, carol).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("notification rows for carol: %d %v", rows, err)
	}
	if names, _, n := e.watcherNames("alice", sec); len(names) != 1 || n != 1 {
		t.Fatalf("watcher list still shows carol: %v", names)
	}
	// Access restored: the kept row works again.
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	item = e.must(200, "alice", "GET", sec, nil).Body
	e.must(200, "alice", "PATCH", sec, map[string]any{"version": item["version"], "due_date": "2030-06-06"})
	e.drain()
	if got := kinds(e.notes("carol", "")); !sameList(got, []string{"due_changed:SEC-1"}) {
		t.Fatalf("after access restored: %v", got)
	}
}
