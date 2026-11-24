package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/recur"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Approval improvements (ADR-0034): the workspace inbox, re-approval of
// changed content, deadlines and reminders. Same synthetic people as
// ADR-0026's tests: alice owner, bob guest, carol member (the preparer),
// eve and erin members (approvers), dave outsider.

const inboxPath = wsBase + "/approvals"

func inboxEntries(r resp) []map[string]any {
	var out []map[string]any
	for _, x := range r.Body["approvals"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func (e *p1env) inbox(user, query string) []map[string]any {
	e.t.Helper()
	return inboxEntries(e.must(200, user, "GET", inboxPath+"?state=pending"+query, nil))
}

func (e *p1env) inboxCount(user, query string) (count, overdue float64) {
	e.t.Helper()
	b := e.must(200, user, "GET", inboxPath+"/count?"+query, nil).Body
	return b["count"].(float64), b["overdue"].(float64)
}

func entryKeys(list []map[string]any) []string {
	var out []string
	for _, x := range list {
		out = append(out, x["item"].(map[string]any)["key"].(string))
	}
	return out
}

func TestApprovalInboxEligibilityPrivacyAndPagination(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	carol, erin := e.userID("carol"), e.userID("erin")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	for i := 1; i <= 3; i++ {
		e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş " + string(rune('0'+i))})
		e.must(201, "carol", "POST", items+"/"+string(rune('0'+i))+"/approvals", map[string]any{"version": 1, "to_status_id": done})
	}
	// A request in a private project only carol (and admins) can see.
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Xəzinə", "visibility": "private"})
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli ödəniş"})
	e.must(201, "carol", "POST", secItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})

	t.Run("who sees what", func(t *testing.T) {
		for _, c := range []struct {
			user, query string
			want        int
		}{
			{"erin", "&mine=true", 3},  // eligible, not the private one
			{"erin", "", 3},            // the private request does not exist for her
			{"eve", "&mine=true", 3},   //
			{"carol", "&mine=true", 0}, // the requester never approves
			{"carol", "", 4},           // ... but sees every pending request of her projects
			{"alice", "&mine=true", 0}, // owner is not in the policy's roles
			{"alice", "", 4},           // admins see private projects
			{"bob", "&mine=true", 0},   // guests never approve
		} {
			if got := e.inbox(c.user, c.query); len(got) != c.want {
				t.Errorf("%s %q: %v", c.user, c.query, entryKeys(got))
			}
		}
		for _, key := range entryKeys(e.inbox("erin", "")) {
			if key == "SEC-1" {
				t.Fatal("private request leaked into the inbox")
			}
		}
		if r := e.call("dave", "GET", inboxPath, nil); r.Status != 403 && r.Status != 404 {
			t.Fatalf("outsider: %d", r.Status)
		}
		if n, _ := e.inboxCount("erin", "mine=true"); n != 3 {
			t.Fatalf("badge: %v", n)
		}
		if n, _ := e.inboxCount("carol", "mine=true"); n != 0 {
			t.Fatalf("requester badge: %v", n)
		}
	})

	t.Run("entry content", func(t *testing.T) {
		first := e.inbox("erin", "&mine=true")[0]
		item := first["item"].(map[string]any)
		if item["key"] != "ACME-1" || item["number"] != 1.0 || item["project_key"] != "ACME" || item["title"] != "Ödəniş 1" {
			t.Fatalf("item: %v", item)
		}
		if first["requested_by"].(map[string]any)["id"] != carol || first["to_status"].(map[string]any)["name"] != "Done" ||
			first["from_status"].(map[string]any)["name"] != "To Do" || first["approvals"] != 0.0 || first["min_approvals"] != 1.0 ||
			first["can_decide"] != true || first["created_at"] == nil || first["due_on"] != nil || first["overdue"] != false {
			t.Fatalf("entry: %v", first)
		}
		if e.inbox("carol", "")[0]["can_decide"] != false {
			t.Fatal("requester can decide")
		}
	})

	t.Run("keyset pagination", func(t *testing.T) {
		r := e.must(200, "erin", "GET", inboxPath+"?state=pending&mine=true&limit=2", nil)
		page1 := inboxEntries(r)
		next, _ := r.Body["next_cursor"].(string)
		if len(page1) != 2 || next == "" {
			t.Fatalf("page 1: %v %q", entryKeys(page1), next)
		}
		r = e.must(200, "erin", "GET", inboxPath+"?state=pending&mine=true&limit=2&cursor="+next, nil)
		page2 := inboxEntries(r)
		if len(page2) != 1 || r.Body["next_cursor"] != nil {
			t.Fatalf("page 2: %v %v", entryKeys(page2), r.Body["next_cursor"])
		}
		got := append(entryKeys(page1), entryKeys(page2)...)
		if got[0] != "ACME-1" || got[1] != "ACME-2" || got[2] != "ACME-3" {
			t.Fatalf("order (oldest first): %v", got)
		}
		for _, q := range []string{"?state=approved", "?mine=maybe", "?cursor=not-a-cursor!", "?limit=101"} {
			if r := e.call("erin", "GET", inboxPath+q, nil); r.Status != 400 {
				t.Errorf("%s: %d", q, r.Status)
			}
		}
		if r := e.call("erin", "GET", inboxPath+"/count?mine=1", nil); r.Status != 400 {
			t.Errorf("count mine=1: %d", r.Status)
		}
	})

	t.Run("deciding and access changes", func(t *testing.T) {
		id := e.inbox("erin", "&mine=true")[0]["id"].(string)
		e.must(200, "erin", "POST", items+"/1/approvals/"+id+"/approve", map[string]any{})
		if n, _ := e.inboxCount("erin", "mine=true"); n != 2 {
			t.Fatalf("after deciding: %v", n)
		}
		if n, _ := e.inboxCount("eve", ""); n != 2 {
			t.Fatalf("closed requests leave the inbox: %v", n)
		}
		e.must(204, "alice", "PUT", secPath+"/members/"+erin, nil)
		if keys := entryKeys(e.inbox("erin", "&mine=true")); len(keys) != 3 || keys[2] != "SEC-1" {
			t.Fatalf("after project access: %v", keys)
		}
	})
}

// reapprovalSetup: an amount (watched) and a memo (not watched) field, and a
// policy on Done that re-approves changes of the amount only.
func (e *p1env) reapprovalSetup() (wf map[string]any, inProgress, done string) {
	e.t.Helper()
	wf, _, inProgress, done = e.approvalSetup()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "amount", "name": "Məbləğ", "type": "number"})
	e.must(200, "alice", "PUT", bindPath("task", "amount"), map[string]any{"required": false})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "memo", "name": "Qeyd", "type": "text"})
	e.must(200, "alice", "PUT", bindPath("task", "memo"), map[string]any{"required": false})
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"},
		"reapprove_on_change": true, "reapprove_fields": []string{"fields.amount"}})
	return wf, inProgress, done
}

// approveInto creates an item and has erin approve its move into done.
func (e *p1env) approveInto(n, done string) {
	e.t.Helper()
	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş " + n, "fields": map[string]any{"amount": 100}})
	id := lastApproval(e.must(201, "carol", "POST", items+"/"+n+"/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	e.must(200, "erin", "POST", items+"/"+n+"/approvals/"+id+"/approve", map[string]any{})
}

func TestApprovalPolicyReapprovalAndDeadlineValidation(t *testing.T) {
	e := newP1Env(t)
	wf, _, done := e.reapprovalSetup()
	path := policyPath(wf, done)
	base := map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	for name, body := range map[string]map[string]any{
		"unknown field":         with(map[string]any{"reapprove_on_change": true, "reapprove_fields": []string{"fields.nope"}}),
		"bad token":             with(map[string]any{"reapprove_on_change": true, "reapprove_fields": []string{"status"}}),
		"fields without option": with(map[string]any{"reapprove_fields": []string{"title"}}),
		"deadline zero":         with(map[string]any{"due_in_working_days": 0}),
		"deadline 31":           with(map[string]any{"due_in_working_days": 31}),
	} {
		if r := e.call("alice", "PUT", path, body); r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	got := e.must(200, "alice", "PUT", path, with(map[string]any{"reapprove_on_change": true,
		"reapprove_fields": []string{"title", "fields.amount", "title"}, "due_in_working_days": 3})).Body
	pol := got["approval_policies"].([]any)[0].(map[string]any)
	if pol["reapprove_on_change"] != true || len(pol["reapprove_fields"].([]any)) != 2 || pol["due_in_working_days"] != 3.0 {
		t.Fatalf("policy: %v", pol)
	}
	// Defaults: off, nothing watched, no deadline.
	pol = e.must(200, "alice", "PUT", path, base).Body["approval_policies"].([]any)[0].(map[string]any)
	if pol["reapprove_on_change"] != false || len(pol["reapprove_fields"].([]any)) != 0 || pol["due_in_working_days"] != nil {
		t.Fatalf("defaults: %v", pol)
	}
}

func TestApprovalReapprovalOnWatchedChange(t *testing.T) {
	e := newP1Env(t)
	wf, inProgress, done := e.reapprovalSetup()
	carol, erin := e.userID("carol"), e.userID("erin")
	ctx := context.Background()
	e.approveInto("1", done)
	one := items + "/1"
	it := e.must(200, "carol", "GET", one, nil).Body
	if it["status"].(map[string]any)["id"] != done {
		t.Fatalf("not approved: %v", it["status"])
	}
	firstID := lastApproval(e.must(200, "carol", "GET", one+"/approvals", nil))["id"].(string)

	t.Run("unwatched changes keep the approval", func(t *testing.T) {
		e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "fields": map[string]any{"memo": "bank köçürməsi"}})
		e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "title": "Ödəniş 1 (yeni ad)"})
		e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "due_date": "2031-03-20"})
		it := e.must(200, "carol", "GET", one, nil).Body
		if it["status"].(map[string]any)["id"] != done || it["approval"] != nil {
			t.Fatalf("unwatched change moved the item: %v %v", it["status"], it["approval"])
		}
		if n := e.countEvents(`action = 'approval.invalidated'`); n != 0 {
			t.Fatalf("invalidated events: %d", n)
		}
	})

	t.Run("bulk edits and automation are refused", func(t *testing.T) {
		v := int(e.version("1"))
		r := e.call("carol", "POST", bulkPath, map[string]any{"items": targets(1, v), "fields": map[string]any{"amount": 5}})
		if r.Status != 422 || bulkErrors(r)[1] != "reapproval_required" {
			t.Fatalf("bulk: %d %s", r.Status, r.Raw)
		}
		ws, proj := e.wsProject()
		_, err := e.st.UpdateItem(ctx, store.UpdateItemParams{WorkspaceID: ws, ProjectID: proj, Number: 1, Version: v,
			Fields: opt.Of(map[string]json.RawMessage{"amount": json.RawMessage("7")})})
		var ce *store.ConflictError
		if !errors.As(err, &ce) || ce.Code != "reapproval_required" {
			t.Fatalf("system actor: %v", err)
		}
		if int(e.version("1")) != v {
			t.Fatal("item changed")
		}
	})

	t.Run("a watched change sends the item back for a new approval", func(t *testing.T) {
		r := e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "fields": map[string]any{"amount": 250}})
		// The item returns to the status it was approved from (To Do here,
		// not In Progress: approveInto requests from To Do).
		if st := r.Body["status"].(map[string]any); st["name"] != "To Do" || st["id"] == inProgress {
			t.Fatalf("status: %v", r.Body["status"])
		}
		ap, _ := r.Body["approval"].(map[string]any)
		if ap == nil || ap["requested_by"] != carol || ap["to_status"].(map[string]any)["id"] != done {
			t.Fatalf("new pending request: %v", ap)
		}
		if r.Body["fields"].(map[string]any)["amount"] != 250.0 {
			t.Fatalf("amount: %v", r.Body["fields"])
		}
		list := e.must(200, "carol", "GET", one+"/approvals", nil).Body["approvals"].([]any)
		newest, old := list[0].(map[string]any), list[1].(map[string]any)
		if newest["state"] != "pending" || newest["reopened_from"] != firstID || old["id"] != firstID ||
			old["state"] != "approved" || old["invalidated_at"] == nil {
			t.Fatalf("history: %v / %v", newest, old)
		}
		if n := e.countEvents(`action = 'approval.invalidated'`); n != 1 {
			t.Fatalf("invalidated events: %d", n)
		}
		if n := e.countEvents(`action = 'approval.requested'`); n != 2 {
			t.Fatalf("requested events: %d", n)
		}
		var changed []string
		if err := e.pool.QueryRow(ctx, `SELECT ARRAY(SELECT jsonb_array_elements_text(payload -> 'changed'))
			FROM events WHERE action = 'approval.invalidated'`).Scan(&changed); err != nil || len(changed) != 1 || changed[0] != "fields.amount" {
			t.Fatalf("changed: %v %v", changed, err)
		}
		// The content is locked again while the new request is pending.
		if r := e.call("carol", "PATCH", one, map[string]any{"version": e.version("1"), "fields": map[string]any{"amount": 1}}); errCode(r) != "approval_pending" {
			t.Fatalf("locked: %d %s", r.Status, r.Raw)
		}
		// The earlier approver hears that their approval no longer holds.
		e.drain()
		if got := kinds(e.notes("erin", "")); !containsStr(got, "approval_invalidated") || !containsStr(got, "approval_requested") {
			t.Fatalf("erin's notifications: %v", got)
		}
		// Four eyes again: the editor cannot approve the changed content.
		id := newest["id"].(string)
		if r := e.call("carol", "POST", one+"/approvals/"+id+"/approve", map[string]any{}); r.Status != 403 || errCode(r) != "self_approval" {
			t.Fatalf("self approval: %d %s", r.Status, r.Raw)
		}
		e.must(200, "erin", "POST", one+"/approvals/"+id+"/approve", map[string]any{})
		if e.must(200, "carol", "GET", one, nil).Body["status"].(map[string]any)["id"] != done {
			t.Fatal("re-approved item is not in Done")
		}
	})

	t.Run("an approver who edits becomes the requester", func(t *testing.T) {
		r := e.must(200, "erin", "PATCH", one, map[string]any{"version": e.version("1"), "fields": map[string]any{"amount": 300}})
		ap := r.Body["approval"].(map[string]any)
		if ap["requested_by"] != erin {
			t.Fatalf("requester: %v", ap)
		}
		if r := e.call("erin", "POST", one+"/approvals/"+ap["id"].(string)+"/approve", map[string]any{}); errCode(r) != "self_approval" {
			t.Fatalf("erin approves her own change: %d", r.Status)
		}
		e.must(200, "eve", "POST", one+"/approvals/"+ap["id"].(string)+"/approve", map[string]any{})
	})

	t.Run("the rule is the one the approval was given under", func(t *testing.T) {
		// Widen the policy to the default (title, description, all fields):
		// item 1 was approved under "amount only", so a title change still passes.
		e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"},
			"reapprove_on_change": true})
		r := e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "title": "Ödəniş 1 (son)"})
		if r.Body["status"].(map[string]any)["id"] != done || r.Body["approval"] != nil {
			t.Fatalf("snapshot ignored: %v", r.Body["status"])
		}
		// An item approved under the wider rule re-approves a title change.
		e.approveInto("2", done)
		r = e.must(200, "carol", "PATCH", items+"/2", map[string]any{"version": e.version("2"), "title": "Ödəniş 2 (dəyişdi)"})
		if r.Body["approval"] == nil || r.Body["status"].(map[string]any)["name"] != "To Do" {
			t.Fatalf("default watch list: %v %v", r.Body["status"], r.Body["approval"])
		}
		e.must(200, "carol", "POST", items+"/2/approvals/"+r.Body["approval"].(map[string]any)["id"].(string)+"/cancel", nil)
	})

	t.Run("no policy, no re-approval", func(t *testing.T) {
		e.must(200, "alice", "DELETE", policyPath(wf, done), nil)
		r := e.must(200, "carol", "PATCH", one, map[string]any{"version": e.version("1"), "fields": map[string]any{"amount": 999}})
		if r.Body["status"].(map[string]any)["id"] != done || r.Body["approval"] != nil {
			t.Fatalf("unguarded status re-approves: %v", r.Body["status"])
		}
	})
}

func TestApprovalDeadlineAcrossHolidayAndReminder(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	ctx := context.Background()
	today := e.today()
	// The next weekday is a holiday: a one-working-day deadline skips it
	// (and any weekend) and lands on the weekday after.
	nextWeekday := func(d time.Time) time.Time {
		for {
			d = d.AddDate(0, 0, 1)
			if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
				return d
			}
		}
	}
	holiday := nextWeekday(today)
	want := nextWeekday(holiday)
	e.must(200, "alice", "PUT", wsBase+"/calendar/days/"+ymd(holiday), map[string]any{"kind": "holiday", "name": "Sınaq bayramı"})
	cal, _ := recur.NewCalendar(recur.DefaultWeekend, []time.Time{holiday}, nil)
	if got, _ := cal.AddWorkdays(today, 1); !got.Equal(want) {
		t.Fatalf("test oracle disagrees: %s vs %s", got, want)
	}
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_roles": []string{"member"}, "due_in_working_days": 1})

	for _, n := range []string{"1", "2"} {
		e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş " + n})
	}
	r := e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})
	a := lastApproval(r)
	if a["due_on"] != ymd(want) || a["overdue"] != false {
		t.Fatalf("deadline across the holiday: %v (want %s)", a["due_on"], ymd(want))
	}
	if ap := r.Body["item"].(map[string]any)["approval"].(map[string]any); ap["due_on"] != ymd(want) || ap["overdue"] != false {
		t.Fatalf("item summary: %v", ap)
	}
	id1 := a["id"].(string)
	id2 := lastApproval(e.must(201, "carol", "POST", items+"/2/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	// eve decides on request 1 before the deadline: she gets no reminder for it.
	e.must(200, "eve", "POST", items+"/1/approvals/"+id1+"/approve", map[string]any{})

	if n, err := e.st.EmitApprovalOverdue(ctx, 100); err != nil || n != 0 {
		t.Fatalf("nothing is overdue yet: %d %v", n, err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE approval_requests SET due_on = $1 WHERE id = ANY($2)`,
		today.AddDate(0, 0, -1), []string{id1, id2}); err != nil {
		t.Fatal(err)
	}
	if a := lastApproval(e.must(200, "carol", "GET", items+"/1/approvals", nil)); a["overdue"] != true {
		t.Fatalf("drawer overdue: %v", a)
	}
	if got := e.inbox("erin", "&mine=true"); len(got) != 2 || got[0]["overdue"] != true {
		t.Fatalf("inbox overdue: %v", got)
	}
	if n, o := e.inboxCount("erin", "mine=true"); n != 2 || o != 2 {
		t.Fatalf("badge: %v %v", n, o)
	}

	// Two replicas tick at once: each request is reminded of exactly once.
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for range 2 {
		wg.Go(func() {
			n, err := e.st.EmitApprovalOverdue(ctx, 100)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		})
	}
	wg.Wait()
	if total != 2 {
		t.Fatalf("emitted %d, want 2", total)
	}
	if n, err := e.st.EmitApprovalOverdue(ctx, 100); err != nil || n != 0 {
		t.Fatalf("second tick: %d %v", n, err)
	}
	if n := e.countEvents(`action = 'approval.overdue' AND actor_id IS NULL`); n != 2 {
		t.Fatalf("overdue events: %d", n)
	}
	e.drain()
	countKind := func(user string) int {
		n := 0
		for _, k := range kinds(e.notes(user, "")) {
			if strings.HasPrefix(k, "approval_overdue:") {
				n++
			}
		}
		return n
	}
	for user, want := range map[string]int{"erin": 2, "eve": 1, "carol": 2, "alice": 0, "bob": 0} {
		if got := countKind(user); got != want {
			t.Errorf("%s: %d approval_overdue, want %d", user, got, want)
		}
	}
	// Deciding takes the request out of the overdue list; closed requests are never reminded.
	e.must(200, "erin", "POST", items+"/1/approvals/"+id1+"/approve", map[string]any{})
	if n, o := e.inboxCount("erin", "mine=true"); n != 1 || o != 1 {
		t.Fatalf("after deciding: %v %v", n, o)
	}
}

// --- small helpers -------------------------------------------------------

// containsStr reports whether kinds() output has a notification of kind s
// (entries read "kind:ITEM-KEY").
func containsStr(list []string, s string) bool {
	for _, x := range list {
		if strings.HasPrefix(x, s+":") {
			return true
		}
	}
	return false
}

func (e *p1env) wsProject() (uuid.UUID, uuid.UUID) {
	e.t.Helper()
	var ws, proj uuid.UUID
	if err := e.pool.QueryRow(context.Background(), `SELECT w.id, p.id FROM workspaces w JOIN projects p ON p.workspace_id = w.id
		WHERE w.slug = 'acme' AND p.key = 'ACME'`).Scan(&ws, &proj); err != nil {
		e.t.Fatal(err)
	}
	return ws, proj
}
