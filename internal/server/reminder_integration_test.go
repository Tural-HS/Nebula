package server

import (
	"context"
	"slices"
	"testing"
)

// Yearly date reminders (ADR-0054): a project picks a date field; on the
// day (month and day) everyone who can see the project gets a notification,
// once a day; guests and people outside a private project get none.
func TestDateReminders(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member
	ctx := context.Background()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "birthday", "name": "Ad günü", "type": "date"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "nv", "name": "NV", "type": "text"})
	e.must(200, "alice", "PUT", bindPath("task", "birthday"), map[string]any{"required": false})
	// The reminders go out at the digest hour: 0 means "already due".
	e.must(200, "alice", "PATCH", "/api/v1/workspaces/acme", map[string]any{"overdue_digest_hour": 0})
	today := e.wsToday("acme")
	md := today.Format("01-02")
	tomorrow := today.AddDate(0, 0, 1).Format("01-02")

	e.must(201, "alice", "POST", items, map[string]any{"title": "Bu gün", "fields": map[string]any{"birthday": "1990-" + md}})      // ACME-1
	e.must(201, "alice", "POST", items, map[string]any{"title": "Sabah", "fields": map[string]any{"birthday": "1985-" + tomorrow}}) // ACME-2
	e.must(201, "alice", "POST", items, map[string]any{"title": "Tarixsiz"})                                                        // ACME-3
	e.must(201, "alice", "POST", items, map[string]any{"title": "Silinmiş", "fields": map[string]any{"birthday": "2000-" + md}})    // ACME-4
	e.must(204, "alice", "DELETE", items+"/4", nil)

	rem := "/api/v1/workspaces/acme/projects/ACME/date-reminder"
	if b := e.must(200, "carol", "GET", rem, nil).Body; b["field"] != nil {
		t.Fatalf("off by default, got %v", b)
	}
	e.must(403, "carol", "PUT", rem, map[string]any{"field": "birthday"})
	e.must(422, "alice", "PUT", rem, map[string]any{"field": "nv"})      // not a date
	e.must(422, "alice", "PUT", rem, map[string]any{"field": "missing"}) // unknown
	e.must(422, "alice", "PUT", rem, map[string]any{})                   // field required
	e.must(200, "alice", "PUT", rem, map[string]any{"field": "birthday"})

	n, err := e.st.EmitDateReminders(ctx)
	if err != nil || n != 1 {
		t.Fatalf("emitted %d, %v; want 1 (today only, not deleted)", n, err)
	}
	e.drain()
	anniversaries := func(user string) []string {
		var out []string
		for _, k := range kinds(e.notes(user, "")) {
			if len(k) > 12 && k[:12] == "anniversary:" {
				out = append(out, k)
			}
		}
		return out
	}
	// Owner and member see the project; the guest is left out.
	for _, u := range []string{"alice", "carol"} {
		if got := anniversaries(u); !slices.Equal(got, []string{"anniversary:ACME-1"}) {
			t.Fatalf("%s: %v", u, got)
		}
	}
	if got := anniversaries("bob"); len(got) != 0 {
		t.Fatalf("guest got %v", got)
	}
	// Once a day, however often the scheduler runs (or the field is saved again).
	e.must(200, "alice", "PUT", rem, map[string]any{"field": "birthday"})
	if n, err := e.st.EmitDateReminders(ctx); err != nil || n != 0 {
		t.Fatalf("second run emitted %d, %v", n, err)
	}
	if b := e.must(200, "alice", "GET", rem, nil).Body; b["last_sent"] != today.Format("2006-01-02") || b["hour"] != float64(0) {
		t.Fatalf("state %v", b)
	}

	// A private project: carol is not a member, so only alice hears of it.
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Gizli", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli müştəri", "fields": map[string]any{"birthday": "1970-" + md}})
	e.must(200, "alice", "PUT", "/api/v1/workspaces/acme/projects/SEC/date-reminder", map[string]any{"field": "birthday"})
	if n, err := e.st.EmitDateReminders(ctx); err != nil || n != 1 {
		t.Fatalf("private run emitted %d, %v", n, err)
	}
	e.drain()
	if got := anniversaries("alice"); !slices.Contains(got, "anniversary:SEC-1") {
		t.Fatalf("alice: %v", got)
	}
	if got := anniversaries("carol"); slices.Contains(got, "anniversary:SEC-1") {
		t.Fatalf("carol saw the private project: %v", got)
	}

	// Off: null clears it.
	if b := e.must(200, "alice", "PUT", rem, map[string]any{"field": nil}).Body; b["field"] != nil {
		t.Fatalf("after clear %v", b)
	}
}
