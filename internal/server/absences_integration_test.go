package server

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Absences and approval delegation (ADR-0042). Same synthetic people as the
// approval tests: alice owner, bob guest, carol member (the preparer), eve
// and erin members (approvers), dave outsider.

const absencesPath = wsBase + "/absences"

// localDay is today+offset in a time zone, as YYYY-MM-DD.
func localDay(t *testing.T, tz string, offset int) string {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	return time.Now().In(loc).AddDate(0, 0, offset).Format(time.DateOnly)
}

// absenceBody is a create body; empty user and delegate are left out.
func absenceBody(user, delegate, from, to string) map[string]any {
	b := map[string]any{"starts_on": from, "ends_on": to}
	if user != "" {
		b["user_id"] = user
	}
	if delegate != "" {
		b["delegate_id"] = delegate
	}
	return b
}

// away records an absence of who (by actor) from today for days days.
func (e *p1env) away(actor, who, delegate string, days int) string {
	e.t.Helper()
	tz := e.must(200, "alice", "GET", wsBase, nil).Body["timezone"].(string)
	b := absenceBody(who, delegate, localDay(e.t, tz, 0), localDay(e.t, tz, days))
	return e.must(201, actor, "POST", absencesPath, b).Body["id"].(string)
}

func decisionsOf(a map[string]any) []map[string]any {
	var out []map[string]any
	for _, d := range a["decisions"].([]any) {
		out = append(out, d.(map[string]any))
	}
	return out
}

func names(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, x := range list {
		out = append(out, x.(map[string]any)["display_name"].(string))
	}
	return out
}

func TestAbsenceManagement(t *testing.T) {
	e := newP1Env(t)
	e.approvalSetup()
	const tz = "Asia/Baku"
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": tz})
	carol, eve, erin, bob, dave := e.userID("carol"), e.userID("eve"), e.userID("erin"), e.userID("bob"), e.userID("dave")
	today, in3 := localDay(t, tz, 0), localDay(t, tz, 3)

	b := absenceBody("", eve, today, in3)
	b["note"] = "Xəstəlik vərəqəsi"
	r := e.must(201, "carol", "POST", absencesPath, b)
	id := r.Body["id"].(string)
	if r.Body["active"] != true || r.Body["note"] != "Xəstəlik vərəqəsi" || r.Body["can_edit"] != true ||
		r.Body["delegate"].(map[string]any)["id"] != eve || r.Body["user"].(map[string]any)["id"] != carol {
		t.Fatalf("created: %s", r.Raw)
	}

	t.Run("validation and authorization", func(t *testing.T) {
		for name, c := range map[string]struct {
			body   map[string]any
			status int
			code   string
		}{
			"overlap":          {body: absenceBody("", "", in3, localDay(t, tz, 5)), status: 409, code: "absence_overlap"},
			"self delegate":    {body: absenceBody("", carol, localDay(t, tz, 10), localDay(t, tz, 11)), status: 422},
			"guest delegate":   {body: absenceBody("", bob, localDay(t, tz, 10), localDay(t, tz, 11)), status: 422},
			"outsider":         {body: absenceBody("", dave, localDay(t, tz, 10), localDay(t, tz, 11)), status: 422},
			"backwards":        {body: absenceBody("", "", localDay(t, tz, 11), localDay(t, tz, 10)), status: 422},
			"past start":       {body: absenceBody("", "", localDay(t, tz, -1), localDay(t, tz, 10)), status: 422},
			"too long":         {body: absenceBody("", "", localDay(t, tz, 10), localDay(t, tz, 400)), status: 422},
			"too far ahead":    {body: absenceBody("", "", localDay(t, tz, 366), localDay(t, tz, 367)), status: 422},
			"bad date":         {body: absenceBody("", "", "25.11.2026", localDay(t, tz, 1)), status: 422},
			"for someone":      {body: absenceBody(eve, "", localDay(t, tz, 10), localDay(t, tz, 11)), status: 403},
			"unknown member":   {body: absenceBody(uuid.NewString(), "", localDay(t, tz, 10), localDay(t, tz, 11)), status: 403},
			"note too long":    {body: map[string]any{"starts_on": localDay(t, tz, 10), "ends_on": localDay(t, tz, 11), "note": strings.Repeat("x", 501)}, status: 422},
			"unknown key":      {body: map[string]any{"starts_on": localDay(t, tz, 10), "ends_on": localDay(t, tz, 11), "reason": "x"}, status: 400},
			"missing dates":    {body: map[string]any{}, status: 422},
			"unknown delegate": {body: absenceBody("", uuid.NewString(), localDay(t, tz, 10), localDay(t, tz, 11)), status: 422},
		} {
			got := e.call("carol", "POST", absencesPath, c.body)
			if got.Status != c.status || (c.code != "" && errCode(got) != c.code) {
				t.Errorf("%s: %d %s", name, got.Status, got.Raw)
			}
		}
		if r := e.call("bob", "POST", absencesPath, absenceBody("", "", today, today)); r.Status != 403 {
			t.Errorf("guest records an absence: %d", r.Status)
		}
		if r := e.call("bob", "GET", absencesPath, nil); r.Status != 403 {
			t.Errorf("guest lists absences: %d", r.Status)
		}
		if r := e.call("dave", "GET", absencesPath, nil); r.Status != 404 {
			t.Errorf("outsider lists absences: %d", r.Status)
		}
		// An admin may record one for a guest? No: guests have nothing to hand over.
		if r := e.call("alice", "POST", absencesPath, absenceBody(bob, "", today, today)); r.Status != 422 {
			t.Errorf("absence for a guest: %d %s", r.Status, r.Raw)
		}
		if r := e.call("eve", "PATCH", absencesPath+"/"+id, map[string]any{"ends_on": localDay(t, tz, 4)}); r.Status != 403 {
			t.Errorf("someone else edits: %d", r.Status)
		}
		if r := e.call("eve", "POST", absencesPath+"/"+id+"/end", nil); r.Status != 403 {
			t.Errorf("someone else ends: %d", r.Status)
		}
		if r := e.call("carol", "PATCH", absencesPath+"/"+id, map[string]any{"starts_on": localDay(t, tz, 1)}); r.Status != 422 {
			t.Errorf("moving the start of a begun absence: %d %s", r.Status, r.Raw)
		}
		if r := e.call("carol", "PATCH", absencesPath+"/"+uuid.NewString(), map[string]any{"note": "x"}); r.Status != 404 {
			t.Errorf("unknown absence: %d", r.Status)
		}
	})

	// The admin records a sudden sick leave for erin.
	sick := e.must(201, "alice", "POST", absencesPath, absenceBody(erin, carol, today, today)).Body
	if sick["created_by"].(map[string]any)["display_name"] == nil || sick["user"].(map[string]any)["id"] != erin {
		t.Fatalf("admin-set absence: %v", sick)
	}
	// Notes: the member, the delegate and admins see them, others do not.
	for user, want := range map[string]bool{"carol": true, "eve": true, "alice": true, "erin": false} {
		var found map[string]any
		for _, a := range e.must(200, user, "GET", absencesPath, nil).Body["absences"].([]any) {
			if a.(map[string]any)["id"] == id {
				found = a.(map[string]any)
			}
		}
		if found == nil || (found["note"] != nil) != want {
			t.Errorf("%s sees note=%v (want %v)", user, found, want)
		}
	}
	if got := e.must(200, "eve", "GET", absencesPath+"?user_id="+erin, nil).Body["absences"].([]any); len(got) != 1 {
		t.Fatalf("filter by member: %v", got)
	}
	if r := e.call("eve", "GET", absencesPath+"?user_id=x", nil); r.Status != 400 {
		t.Fatalf("bad filter: %d", r.Status)
	}

	// Changing the delegate and the end, then ending early.
	r = e.must(200, "carol", "PATCH", absencesPath+"/"+id, map[string]any{"delegate_id": erin, "ends_on": localDay(t, tz, 2)})
	if r.Body["delegate"].(map[string]any)["id"] != erin || r.Body["ends_on"] != localDay(t, tz, 2) {
		t.Fatalf("patched: %s", r.Raw)
	}
	// erin is away today too: carol is warned that nobody stands in then.
	if r.Body["delegate_absent"] != true {
		t.Fatalf("delegate absence warning: %s", r.Raw)
	}
	e.must(200, "carol", "PATCH", absencesPath+"/"+id, map[string]any{"delegate_id": nil})
	r = e.must(200, "carol", "POST", absencesPath+"/"+id+"/end", nil)
	if r.Body["ended_at"] == nil || r.Body["active"] != false || r.Body["can_edit"] != false {
		t.Fatalf("ended: %s", r.Raw)
	}
	if r := e.call("carol", "POST", absencesPath+"/"+id+"/end", nil); r.Status != 409 || errCode(r) != "absence_closed" {
		t.Fatalf("end twice: %d %s", r.Status, r.Raw)
	}
	if r := e.call("carol", "PATCH", absencesPath+"/"+id, map[string]any{"note": "x"}); r.Status != 409 || errCode(r) != "absence_closed" {
		t.Fatalf("edit an ended absence: %d %s", r.Status, r.Raw)
	}
	// An ended absence frees its dates.
	up := e.must(201, "carol", "POST", absencesPath, absenceBody("", eve, localDay(t, tz, 1), localDay(t, tz, 2))).Body
	if up["active"] != false {
		t.Fatalf("an upcoming absence is not active: %v", up)
	}
	// An upcoming one can still move its start; the admin cancels it.
	e.must(200, "carol", "PATCH", absencesPath+"/"+up["id"].(string), map[string]any{"starts_on": localDay(t, tz, 2)})
	e.must(200, "alice", "POST", absencesPath+"/"+up["id"].(string)+"/end", nil)

	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw
	for _, a := range []string{"absence.created", "absence.updated", "absence.ended", `"set_by_admin":true`, `"has_note":true`} {
		if !contains(audit, a) {
			t.Errorf("audit log lacks %q", a)
		}
	}
	if contains(audit, "Xəstəlik") {
		t.Error("the note leaked into the audit log")
	}
}

func TestDelegatedApprovalFlow(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku"})
	alice, eve, erin := e.userID("alice"), e.userID("eve"), e.userID("erin")
	// Only eve and erin may approve; two approvals are needed.
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_user_ids": []string{eve, erin}})
	absence := e.away("eve", "", alice, 5) // eve is away; alice (owner, not in the policy) stands in

	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş 7001"})
	appr := items + "/1/approvals"
	reqID := lastApproval(e.must(201, "carol", "POST", appr, map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	approve, reject := appr+"/"+reqID+"/approve", appr+"/"+reqID+"/reject"

	// Notifications: the delegate is asked; the absent approver still gets
	// the request (it waits for her), the requester does not.
	e.drain()
	for user, want := range map[string]bool{"alice": true, "eve": true, "erin": true, "carol": false} {
		if got := strings.Join(kinds(e.notes(user, "")), ","); contains(got, "approval_requested:ACME-1") != want {
			t.Errorf("%s notifications %s (want %v)", user, got, want)
		}
	}

	// What alice sees: no own approval right, but eve's on her behalf.
	a := lastApproval(e.must(200, "alice", "GET", appr, nil))
	if a["can_approve"] != false || strings.Join(names(a["on_behalf_of"]), ",") != "Eve" {
		t.Fatalf("delegate view: %v", a)
	}
	abs := a["absences"].([]any)
	if len(abs) != 1 || abs[0].(map[string]any)["user"].(map[string]any)["id"] != eve ||
		abs[0].(map[string]any)["delegate"].(map[string]any)["id"] != alice {
		t.Fatalf("absences on the panel: %v", abs)
	}
	got := e.inbox("alice", "&mine=true")
	if len(got) != 1 || got[0]["can_decide"] != true || strings.Join(names(got[0]["on_behalf_of"]), ",") != "Eve" {
		t.Fatalf("delegate inbox: %v", got)
	}
	if n, _ := e.inboxCount("alice", "mine=true"); n != 1 {
		t.Fatalf("delegate badge: %v", n)
	}
	// eve's own view: she may still decide herself (she is away, not barred).
	if a := lastApproval(e.must(200, "eve", "GET", appr, nil)); a["can_approve"] != true || len(names(a["on_behalf_of"])) != 0 {
		t.Fatalf("absent approver view: %v", a)
	}

	if r := e.call("alice", "POST", approve, map[string]any{}); r.Status != 403 || errCode(r) != "not_an_approver" {
		t.Fatalf("delegate approves on her own account: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", approve, map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "not_a_delegate" {
		t.Fatalf("on behalf of someone who is not away: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", approve, map[string]any{"on_behalf_of": alice}); r.Status != 422 {
		t.Fatalf("on her own behalf: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", reject, map[string]any{"on_behalf_of": eve}); r.Status != 422 {
		t.Fatalf("delegated rejection without a reason: %d %s", r.Status, r.Raw)
	}
	r := e.must(200, "alice", "POST", approve, map[string]any{"on_behalf_of": eve, "comment": "Eve adına yoxlandı"})
	a = lastApproval(r)
	ds := decisionsOf(a)
	if a["approvals"] != 1.0 || len(ds) != 1 || ds[0]["user"].(map[string]any)["id"] != alice ||
		ds[0]["on_behalf_of"].(map[string]any)["id"] != eve {
		t.Fatalf("delegated approval: %v", a)
	}
	// eve's slot is filled: neither she nor alice can fill it again.
	if r := e.call("eve", "POST", approve, map[string]any{}); r.Status != 409 || errCode(r) != "already_decided" {
		t.Fatalf("absent approver approves after her delegate: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", approve, map[string]any{"on_behalf_of": eve}); r.Status != 409 || errCode(r) != "already_decided" {
		t.Fatalf("delegate approves twice: %d %s", r.Status, r.Raw)
	}
	if a := lastApproval(e.must(200, "eve", "GET", appr, nil)); a["can_approve"] != false {
		t.Fatalf("eve's slot is used: %v", a)
	}
	if got := e.inbox("alice", "&mine=true"); len(got) != 0 {
		t.Fatalf("delegate inbox after deciding: %v", got)
	}
	if n := e.count(`SELECT count(*) FROM approval_decisions WHERE request_id = $1 AND user_id = $2 AND on_behalf_of = $3`, reqID, alice, eve); n != 1 {
		t.Fatalf("decision row: %d", n)
	}
	r = e.must(200, "erin", "POST", approve, map[string]any{})
	if a := lastApproval(r); a["state"] != "approved" {
		t.Fatalf("applied: %v", a)
	}

	// Audit: the event's actor is alice, the payload names eve and the absence.
	if n := e.countEvents(`action = 'approval.approved' AND actor_id = $1 AND payload->>'on_behalf_of' = $2 AND payload->>'absence_id' = $3`,
		alice, eve, absence); n != 1 {
		t.Fatalf("approval.approved on behalf: %d", n)
	}
	// Evidence (ADR-0029): the decision names both people.
	ws, err := e.st.WorkspaceForMember(context.Background(), "acme", uuid.MustParse(alice))
	if err != nil {
		t.Fatal(err)
	}
	today := localDay(t, "Asia/Baku", 0)
	set, err := e.st.CollectEvidence(context.Background(), store.EvidenceQuery{WorkspaceID: ws.ID,
		Viewer: store.Viewer{UserID: uuid.MustParse(alice), Role: ws.Role}, From: today, To: today})
	if err != nil {
		t.Fatal(err)
	}
	var onBehalf int
	for _, ap := range set.Approvals {
		for _, d := range ap.Decisions {
			if d.OnBehalfOf != nil && d.OnBehalfOf.String() == eve && d.UserID.String() == alice {
				onBehalf++
			}
		}
	}
	absenceEvents := 0
	for _, ev := range set.Events {
		if ev.EntityType == "absence" {
			absenceEvents++
		}
	}
	people := map[string]bool{}
	for _, p := range set.People {
		people[p.ID.String()] = true
	}
	if onBehalf != 1 || absenceEvents != 1 || !people[eve] || !people[alice] {
		t.Fatalf("evidence: on behalf %d, absence events %d, people %v", onBehalf, absenceEvents, people)
	}
}

// TestDelegationFourEyes: one person never provides two approvals of one
// request, the requester never approves through a delegation, and there
// are no chains.
func TestDelegationFourEyes(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku"})
	alice, carol, eve, erin := e.userID("alice"), e.userID("carol"), e.userID("eve"), e.userID("erin")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_roles": []string{"member"}})
	erinAway := e.away("erin", "", eve, 3) // erin away, eve stands in

	open := func(by, title string) (string, string) {
		r := e.must(201, by, "POST", items, map[string]any{"title": title})
		path := items + "/" + strconv.Itoa(int(r.Body["number"].(float64)))
		id := lastApproval(e.must(201, by, "POST", path+"/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
		return path + "/approvals/" + id, id
	}

	t.Run("own then delegated", func(t *testing.T) {
		base, _ := open("carol", "Bir")
		e.must(200, "eve", "POST", base+"/approve", map[string]any{})
		if r := e.call("eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 409 || errCode(r) != "already_decided" {
			t.Fatalf("own + delegated: %d %s", r.Status, r.Raw)
		}
		if a := lastApproval(e.must(200, "eve", "GET", strings.Split(base, "/approvals/")[0]+"/approvals", nil)); len(names(a["on_behalf_of"])) != 0 {
			t.Fatalf("no delegated action is offered after voting: %v", a)
		}
	})
	t.Run("delegated then own", func(t *testing.T) {
		base, _ := open("carol", "İki")
		e.must(200, "eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin})
		if r := e.call("eve", "POST", base+"/approve", map[string]any{}); r.Status != 409 || errCode(r) != "already_decided" {
			t.Fatalf("delegated + own: %d %s", r.Status, r.Raw)
		}
		if r := e.call("eve", "POST", base+"/reject", map[string]any{"reason": "x"}); r.Status != 409 || errCode(r) != "already_decided" {
			t.Fatalf("delegated approve + own reject: %d %s", r.Status, r.Raw)
		}
	})
	t.Run("the requester never approves through a delegation", func(t *testing.T) {
		// eve requests; erin (away, delegate eve) could approve it, eve cannot for her.
		base, _ := open("eve", "Üç")
		if r := e.call("eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "self_approval" {
			t.Fatalf("requester as delegate: %d %s", r.Status, r.Raw)
		}
		if got := e.inbox("eve", "&mine=true"); len(got) != 0 {
			t.Fatalf("requester's inbox offers her own request: %v", got)
		}
		// Nor does a request of the absent approver herself count through her delegate.
		base, _ = open("erin", "Dörd")
		if r := e.call("eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "self_approval" {
			t.Fatalf("on behalf of the requester: %d %s", r.Status, r.Raw)
		}
	})
	t.Run("no chains", func(t *testing.T) {
		base, _ := open("carol", "Beş")
		eveAway := e.away("eve", "", alice, 2) // the delegate is away too
		if r := e.call("eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "not_a_delegate" {
			t.Fatalf("an absent delegate acts: %d %s", r.Status, r.Raw)
		}
		// alice stands in for eve, not (through eve) for erin.
		if r := e.call("alice", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "not_a_delegate" {
			t.Fatalf("chained delegation: %d %s", r.Status, r.Raw)
		}
		a := lastApproval(e.must(200, "carol", "GET", strings.Split(base, "/approvals/")[0]+"/approvals", nil))
		for _, x := range a["absences"].([]any) {
			m := x.(map[string]any)
			if m["user"].(map[string]any)["id"] == erin && m["delegate"] != nil {
				t.Fatalf("erin's delegate is away: nobody stands in: %v", m)
			}
		}
		// alice may act for eve (a member approver) on this request.
		e.must(200, "alice", "POST", base+"/approve", map[string]any{"on_behalf_of": eve})
		e.must(200, "eve", "POST", absencesPath+"/"+eveAway+"/end", nil)
	})
	t.Run("ended early", func(t *testing.T) {
		base, _ := open("carol", "Altı")
		e.must(200, "erin", "POST", absencesPath+"/"+erinAway+"/end", nil)
		if r := e.call("eve", "POST", base+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 403 || errCode(r) != "not_a_delegate" {
			t.Fatalf("delegate after the absence ended: %d %s", r.Status, r.Raw)
		}
		if got := e.inbox("eve", "&mine=true"); len(got) == 0 {
			t.Fatal("eve still decides on her own account")
		} else {
			for _, x := range got {
				if len(names(x["on_behalf_of"])) != 0 {
					t.Fatalf("delegation shown after the end: %v", x)
				}
			}
		}
	})
	t.Run("private project: the delegate must see it", func(t *testing.T) {
		e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Xəzinə", "visibility": "private"})
		e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
		e.must(204, "alice", "PUT", secPath+"/members/"+erin, nil)
		e.away("erin", "", eve, 1) // eve cannot see SEC
		e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli"})
		id := lastApproval(e.must(201, "carol", "POST", secItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
		if r := e.call("eve", "POST", secItems+"/1/approvals/"+id+"/approve", map[string]any{"on_behalf_of": erin}); r.Status != 404 {
			t.Fatalf("delegate outside the private project: %d", r.Status)
		}
		for _, x := range e.inbox("eve", "") {
			if strings.HasPrefix(x["item"].(map[string]any)["key"].(string), "SEC") {
				t.Fatalf("private request leaked to the delegate: %v", x)
			}
		}
		a := lastApproval(e.must(200, "carol", "GET", secItems+"/1/approvals", nil))
		abs := a["absences"].([]any)
		if len(abs) != 1 || abs[0].(map[string]any)["delegate"] != nil {
			t.Fatalf("a delegate who cannot see the project stands in for nobody: %v", abs)
		}
		e.drain()
		if got := strings.Join(kinds(e.notes("eve", "")), ","); contains(got, "SEC-1") {
			t.Fatalf("delegate notified about a private item: %s", got)
		}
	})
}

// TestDelegationWindowInWorkspaceTimeZone: an absence is in effect on its
// dates in the workspace's time zone, not the server's.
func TestDelegationWindowInWorkspaceTimeZone(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	alice, eve := e.userID("alice"), e.userID("eve")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_user_ids": []string{eve}})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Saat qurşağı"})
	id := lastApproval(e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	appr := items + "/1/approvals"

	// UTC+14: "today" there is always a later date than in UTC-11.
	const east, west = "Pacific/Kiritimati", "Pacific/Pago_Pago"
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": east})
	day := localDay(t, east, 0)
	absID := e.must(201, "eve", "POST", absencesPath, absenceBody("", alice, day, day)).Body["id"].(string)
	canAct := func() bool {
		return len(names(lastApproval(e.must(200, "alice", "GET", appr, nil))["on_behalf_of"])) == 1
	}
	if !canAct() {
		t.Fatal("the absence is in effect on its local day")
	}
	// The same dates in a time zone where that day has not begun yet.
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": west})
	if canAct() {
		t.Fatal("an absence that starts tomorrow (local) is not in effect")
	}
	if r := e.call("alice", "POST", appr+"/"+id+"/approve", map[string]any{"on_behalf_of": eve}); r.Status != 403 || errCode(r) != "not_a_delegate" {
		t.Fatalf("before the window: %d %s", r.Status, r.Raw)
	}
	// After the window: the last day was yesterday (local).
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": east})
	if _, err := e.pool.Exec(context.Background(), `UPDATE workspace_absences SET starts_on = $2::date - 3, ends_on = $2::date - 1 WHERE id = $1`,
		absID, day); err != nil {
		t.Fatal(err)
	}
	if canAct() {
		t.Fatal("an absence that ended yesterday (local) is not in effect")
	}
	if got := e.must(200, "alice", "GET", absencesPath, nil).Body["absences"].([]any); len(got) != 0 {
		t.Fatalf("past absences are not listed: %v", got)
	}
	// Last day = today (local): in effect until local midnight.
	if _, err := e.pool.Exec(context.Background(), `UPDATE workspace_absences SET ends_on = $2::date WHERE id = $1`, absID, day); err != nil {
		t.Fatal(err)
	}
	if !canAct() {
		t.Fatal("the last day is included")
	}
	e.must(200, "alice", "POST", appr+"/"+id+"/approve", map[string]any{"on_behalf_of": eve})
}

func TestDelegationReminders(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Asia/Baku"})
	alice, eve, erin := e.userID("alice"), e.userID("eve"), e.userID("erin")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_user_ids": []string{eve, erin},
		"due_in_working_days": 1})
	e.away("eve", "", alice, 3)
	e.must(201, "carol", "POST", items, map[string]any{"title": "Gecikən"})
	id := lastApproval(e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	e.drain()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `UPDATE approval_requests SET due_on = (now() AT TIME ZONE 'Asia/Baku')::date - 1 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, err := e.st.EmitApprovalOverdue(ctx, 100); err != nil || n != 1 {
		t.Fatalf("overdue: %d %v", n, err)
	}
	e.drain()
	// The reminder goes to the delegate and the present approver, not to the
	// absent one (no nagging), and to the requester as before.
	for user, want := range map[string]bool{"alice": true, "erin": true, "carol": true, "eve": false} {
		if got := strings.Join(kinds(e.notes(user, "")), ","); contains(got, "approval_overdue:ACME-1") != want {
			t.Errorf("%s reminders %s (want %v)", user, got, want)
		}
	}
}
