package server

import (
	"context"
	"encoding/json"
	"testing"
)

// types of the events a receiver got, in order.
func (rc *receiver) types(t *testing.T) []string {
	t.Helper()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]string, 0, len(rc.bodies))
	for _, b := range rc.bodies {
		var p struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p.Type)
	}
	return out
}

func (rc *receiver) last(t *testing.T) map[string]any {
	t.Helper()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var p map[string]any
	if err := json.Unmarshal(rc.bodies[len(rc.bodies)-1], &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWebhookProjectFilter(t *testing.T) {
	e := newP1Env(t)
	e.setup() // project ACME (workspace-visible)
	ctx := context.Background()
	hooks := wsBase + "/webhooks"
	acme := e.must(200, "alice", "GET", projectsPath+"/ACME", nil).Body["id"].(string)
	sec := e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq", "visibility": "private"}).Body["id"].(string)

	for _, bad := range [][]string{{"01a20727-0000-7000-8000-000000000000"}, {"nope"}} {
		if r := e.call("alice", "POST", hooks, map[string]any{"url": "http://127.0.0.1:1/x", "events": []string{"*"}, "project_ids": bad}); r.Status != 422 && r.Status != 400 {
			t.Errorf("project_ids %v: %d %s", bad, r.Status, r.Raw)
		}
	}

	all := newReceiver(t, 200)     // no filter: workspace-visible projects + workspace events
	secOnly := newReceiver(t, 200) // explicitly the private project
	acmeOnly := newReceiver(t, 200)
	create := func(rc *receiver, projects []string) {
		t.Helper()
		e.must(201, "alice", "POST", hooks, map[string]any{"url": rc.srv.URL + "/h", "events": []string{"*"}, "project_ids": projects})
	}
	create(all, nil)
	create(secOnly, []string{sec})
	create(acmeOnly, []string{acme})
	listed := e.must(200, "alice", "GET", hooks, nil).Body["webhooks"].([]any)
	if ids := listed[1].(map[string]any)["project_ids"].([]any); len(ids) != 1 || ids[0] != sec {
		t.Fatalf("project_ids not stored: %v", listed[1])
	}
	e.drain() // webhook.created events: workspace-level, everyone may get them
	base := map[*receiver]int{all: all.count(), secOnly: secOnly.count(), acmeOnly: acmeOnly.count()}
	got := func(rc *receiver) []string { return rc.types(t)[base[rc]:] }

	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq iş"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Məhkəmə işi"})
	e.must(201, "alice", "POST", secItems+"/1/comments", map[string]any{"body": "gizli şərh"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "branch", "name": "Filial", "type": "text"})
	e.drain()

	if g := got(all); len(g) != 2 || g[0] != "item.created" || g[1] != "field.created" {
		t.Fatalf("unfiltered webhook: %v (must not see the private project)", g)
	}
	if g := got(secOnly); len(g) != 3 || g[0] != "item.created" || g[1] != "comment.created" || g[2] != "field.created" {
		t.Fatalf("SEC webhook: %v", g)
	}
	if g := got(acmeOnly); len(g) != 2 || g[0] != "item.created" {
		t.Fatalf("ACME webhook: %v", g)
	}

	// The project turns private after the event but before delivery: the
	// unfiltered webhook must not receive it.
	for rc := range base {
		base[rc] = rc.count()
	}
	e.must(201, "alice", "POST", items, map[string]any{"title": "Son açıq iş"})
	e.must(200, "alice", "PATCH", projectsPath+"/ACME", map[string]any{"visibility": "private"})
	e.drain()
	if g := got(all); len(g) != 0 {
		t.Fatalf("unfiltered webhook got a now-private project's event: %v", g)
	}
	if g := got(acmeOnly); len(g) != 2 || g[0] != "item.created" || g[1] != "project.updated" {
		t.Fatalf("ACME webhook: %v", g)
	}
	e.must(200, "alice", "PATCH", projectsPath+"/ACME", map[string]any{"visibility": "workspace"})

	// Deleting a recurrence keeps its project, so the filter still applies.
	rec := e.must(201, "alice", "POST", secPath+"/recurrences", map[string]any{"title": "Gizli qayda", "freq": "daily", "every": 1, "start_date": "2026-11-23"}).Body
	e.drain()
	for rc := range base {
		base[rc] = rc.count()
	}
	e.must(204, "alice", "DELETE", secPath+"/recurrences/"+rec["id"].(string), nil)
	e.drain()
	if g := got(all); len(g) != 0 {
		t.Fatalf("unfiltered webhook got a private recurrence deletion: %v", g)
	}
	if g := got(secOnly); len(g) != 1 || g[0] != "recurrence.deleted" {
		t.Fatalf("SEC webhook: %v", g)
	}

	// The daily digest is cut per webhook: each sees only its projects' items.
	e.must(201, "alice", "POST", items, map[string]any{"title": "Gecikən açıq", "due_date": "2020-01-01"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gecikən gizli", "due_date": "2020-01-01"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gecikən gizli 2", "due_date": "2020-02-01"})
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"overdue_digest_hour": 0})
	e.drain()
	if n, err := e.st.EmitOverdueDigests(ctx); err != nil || n != 1 {
		t.Fatalf("digest: %d %v", n, err)
	}
	e.drain()
	check := func(rc *receiver, wantTotal float64, wantKeys ...string) {
		t.Helper()
		p := rc.last(t)
		if p["type"] != "workspace.overdue_digest" {
			t.Fatalf("last event %v", p["type"])
		}
		data := p["data"].(map[string]any)
		if data["total"] != wantTotal {
			t.Fatalf("digest total %v, want %v", data["total"], wantTotal)
		}
		var keys []string
		for _, it := range data["items"].([]any) {
			keys = append(keys, it.(map[string]any)["project"].(string))
		}
		for _, k := range keys {
			ok := false
			for _, w := range wantKeys {
				ok = ok || k == w
			}
			if !ok {
				t.Fatalf("digest leaked project %s (allowed %v)", k, wantKeys)
			}
		}
	}
	check(all, 1, "ACME")
	check(secOnly, 2, "SEC")
	check(acmeOnly, 1, "ACME")
}
