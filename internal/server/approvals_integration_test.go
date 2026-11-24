package server

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Four-eyes approvals (ADR-0026). Synthetic people only: alice owner, bob
// guest, carol member (the preparer), eve and erin members (approvers),
// dave outsider.

func (e *p1env) approvalSetup() (wf map[string]any, todo, inProgress, done string) {
	e.t.Helper()
	e.setup()
	for _, u := range []string{"eve", "erin"} {
		e.must(200, u, "GET", "/api/v1/me", nil)
	}
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "eve@example.com", "role": "member"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/members", map[string]any{"email": "erin@example.com", "role": "member"})
	wf = e.defaultWorkflow()
	return wf, statusID(e.t, wf, "To Do"), statusID(e.t, wf, "In Progress"), statusID(e.t, wf, "Done")
}

func policyPath(wf map[string]any, status string) string {
	return wfBase + "/" + wf["id"].(string) + "/statuses/" + status + "/approval-policy"
}

// pending returns the item's open approval summary (nil if none).
func (e *p1env) pending(path string) map[string]any {
	e.t.Helper()
	a, _ := e.must(200, "carol", "GET", path, nil).Body["approval"].(map[string]any)
	return a
}

func lastApproval(r resp) map[string]any {
	return r.Body["approvals"].([]any)[0].(map[string]any)
}

func TestApprovalPolicyValidation(t *testing.T) {
	e := newP1Env(t)
	wf, todo, _, done := e.approvalSetup()
	path := policyPath(wf, done)
	if r := e.call("carol", "PUT", path, map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}}); r.Status != 403 {
		t.Fatalf("member sets a policy: %d", r.Status)
	}
	dave := e.userID("dave")
	eve := e.userID("eve")
	bob := e.userID("bob")
	for name, body := range map[string]map[string]any{
		"zero":          {"min_approvals": 0, "approver_roles": []string{"member"}},
		"six":           {"min_approvals": 6, "approver_roles": []string{"member"}},
		"guest role":    {"min_approvals": 1, "approver_roles": []string{"guest"}},
		"nobody":        {"min_approvals": 1},
		"outsider":      {"min_approvals": 1, "approver_user_ids": []string{dave}},
		"guest user":    {"min_approvals": 1, "approver_user_ids": []string{bob}},
		"unsatisfiable": {"min_approvals": 2, "approver_user_ids": []string{eve}},
	} {
		if r := e.call("alice", "PUT", path, body); r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	if r := e.call("alice", "PUT", policyPath(wf, "00000000-0000-0000-0000-000000000000"), map[string]any{"min_approvals": 1, "approver_roles": []string{"admin"}}); r.Status != 404 {
		t.Fatalf("unknown status: %d", r.Status)
	}
	got := e.must(200, "alice", "PUT", path, map[string]any{"min_approvals": 2, "approver_roles": []string{"member", "member"}, "approver_user_ids": []string{eve}}).Body
	pols := got["approval_policies"].([]any)
	if len(pols) != 1 || pols[0].(map[string]any)["status_id"] != done || pols[0].(map[string]any)["min_approvals"] != 2.0 ||
		len(pols[0].(map[string]any)["approver_roles"].([]any)) != 1 {
		t.Fatalf("policy: %v", pols)
	}
	// Replacing is an upsert; a policy on the initial status is allowed.
	e.must(200, "alice", "PUT", path, map[string]any{"min_approvals": 1, "approver_roles": []string{"admin"}})
	e.must(200, "alice", "PUT", policyPath(wf, todo), map[string]any{"min_approvals": 1, "approver_roles": []string{"admin"}})
	if n := len(e.must(200, "carol", "GET", wfBase, nil).Body["workflows"].([]any)[0].(map[string]any)["approval_policies"].([]any)); n != 2 {
		t.Fatalf("policies visible to members: %d", n)
	}
	e.must(200, "alice", "DELETE", policyPath(wf, todo), nil)
	if r := e.call("alice", "DELETE", policyPath(wf, todo), nil); r.Status != 404 {
		t.Fatalf("delete twice: %d", r.Status)
	}
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw
	if !contains(audit, "approval_policy.set") || !contains(audit, "approval_policy.deleted") {
		t.Fatalf("policy changes are audited: %s", audit)
	}
}

func TestApprovalFlow(t *testing.T) {
	e := newP1Env(t)
	wf, todo, inProgress, done := e.approvalSetup()
	carol, erin := e.userID("carol"), e.userID("erin")
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "amount", "name": "Məbləğ", "type": "number"})
	e.must(200, "alice", "PUT", bindPath("task", "amount"), map[string]any{"required": false})
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 2, "approver_roles": []string{"member"}})

	e.must(201, "carol", "POST", items, map[string]any{"title": "Ödəniş 1", "fields": map[string]any{"amount": 1250.5}}) // ACME-1
	one := items + "/1"
	appr := one + "/approvals"

	// The transitions endpoint tells the UI which moves need approval.
	for _, s := range e.must(200, "carol", "GET", one+"/transitions", nil).Body["statuses"].([]any) {
		m := s.(map[string]any)
		if (m["id"] == done) != (m["requires_approval"] == true) {
			t.Fatalf("requires_approval: %v", m)
		}
	}

	t.Run("guarded moves are refused everywhere else", func(t *testing.T) {
		if r := e.call("carol", "PATCH", one, map[string]any{"version": 1, "status_id": done}); r.Status != 409 || errCode(r) != "approval_required" {
			t.Fatalf("direct move: %d %s", r.Status, r.Raw)
		}
		if r := e.call("carol", "POST", items, map[string]any{"title": "x", "status_id": done}); r.Status != 422 || !contains(r.Raw, "status_id") {
			t.Fatalf("create in guarded status: %d %s", r.Status, r.Raw)
		}
		r := e.call("alice", "POST", bulkPath, map[string]any{"items": targets(1, 1), "status_id": done})
		if r.Status != 422 || bulkErrors(r)[1] != "approval_required" {
			t.Fatalf("bulk move: %d %s", r.Status, r.Raw)
		}
		if e.version("1") != 1 {
			t.Fatal("item changed")
		}
		// Other moves stay free.
		e.must(200, "carol", "PATCH", one, map[string]any{"version": 1, "status_id": inProgress})
	})

	// --- request -------------------------------------------------------------
	if r := e.call("bob", "POST", appr, map[string]any{"version": 2, "to_status_id": done}); r.Status != 403 {
		t.Fatalf("guest requests: %d", r.Status)
	}
	if r := e.call("carol", "POST", appr, map[string]any{"version": 1, "to_status_id": done}); r.Status != 409 || errCode(r) != "version_conflict" {
		t.Fatalf("stale request: %d %s", r.Status, r.Raw)
	}
	if r := e.call("carol", "POST", appr, map[string]any{"version": 2, "to_status_id": todo}); r.Status != 422 {
		t.Fatalf("request for an unguarded status: %d %s", r.Status, r.Raw)
	}
	r := e.must(201, "carol", "POST", appr, map[string]any{"version": 2, "to_status_id": done, "comment": "Qəbz əlavə olunub"})
	req := lastApproval(r)
	reqID := req["id"].(string)
	if req["state"] != "pending" || req["can_cancel"] != true || req["can_approve"] != false || len(req["approvers"].([]any)) != 2 {
		t.Fatalf("request as seen by the requester: %v", req)
	}
	item := r.Body["item"].(map[string]any)
	if item["status"].(map[string]any)["id"] != inProgress || item["approval"] == nil {
		t.Fatalf("item after request: %v", item)
	}
	// Eligible approvers are asked in-app; the requester and non-approvers are not.
	e.drain()
	for user, want := range map[string]bool{"eve": true, "erin": true, "alice": false, "carol": false} {
		if got := strings.Join(kinds(e.notes(user, "")), ","); contains(got, "approval_requested:ACME-1") != want {
			t.Errorf("%s notifications %s (want asked=%v)", user, got, want)
		}
	}
	if r := e.call("carol", "POST", appr, map[string]any{"version": 2, "to_status_id": done}); r.Status != 409 || errCode(r) != "approval_pending" {
		t.Fatalf("second request: %d %s", r.Status, r.Raw)
	}
	if got := e.must(200, "eve", "GET", items+"?approval=mine", nil).Body["items"].([]any); len(got) != 1 {
		t.Fatalf("pending my approval (eve): %v", got)
	}
	if got := e.must(200, "carol", "GET", items+"?approval=mine", nil).Body["items"].([]any); len(got) != 0 {
		t.Fatalf("the requester has nothing to approve: %v", got)
	}
	if got := e.must(200, "carol", "GET", items+"?approval=pending", nil).Body["items"].([]any); len(got) != 1 {
		t.Fatalf("pending filter: %v", got)
	}
	if n := e.must(200, "carol", "GET", items+"/queue-counts", nil).Body["approval"]; n != 1.0 {
		t.Fatalf("queue count of pending approvals: %v", n)
	}
	if r := e.call("carol", "GET", items+"?approval=bogus", nil); r.Status != 400 {
		t.Fatalf("bad filter: %d", r.Status)
	}

	t.Run("locked while pending", func(t *testing.T) {
		for name, patch := range map[string]map[string]any{
			"title":       {"title": "Başqa"},
			"description": {"description": "dəyişdi"},
			"field":       {"fields": map[string]any{"amount": 9999}},
			"status":      {"status_id": todo},
		} {
			patch["version"] = 2
			if r := e.call("carol", "PATCH", one, patch); r.Status != 409 || errCode(r) != "approval_pending" {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
		if r := e.call("alice", "DELETE", one, nil); r.Status != 409 || errCode(r) != "approval_pending" {
			t.Errorf("delete: %d %s", r.Status, r.Raw)
		}
		if r := e.call("alice", "POST", bulkPath, map[string]any{"items": targets(1, 2), "delete": true}); r.Status != 422 || bulkErrors(r)[1] != "approval_pending" {
			t.Errorf("bulk delete: %d %s", r.Status, r.Raw)
		}
		// Item files are signed off too; files on comments are discussion.
		var wsID, projectID uuid.UUID
		if err := e.pool.QueryRow(context.Background(), `SELECT workspace_id, project_id FROM items WHERE number = 1`).Scan(&wsID, &projectID); err != nil {
			t.Fatal(err)
		}
		err := e.st.CheckAttachmentTarget(context.Background(), wsID, projectID, 1, nil, uuid.MustParse(carol))
		var ce *store.ConflictError
		if !errors.As(err, &ce) || ce.Code != "approval_pending" {
			t.Errorf("attachment upload while pending: %v", err)
		}
		// Operational fields stay editable; a no-op title is not a change.
		e.must(200, "carol", "PATCH", one, map[string]any{"version": 2, "assignee_id": carol, "title": "Ödəniş 1"})
	})

	// --- decisions -----------------------------------------------------------
	approve := appr + "/" + reqID + "/approve"
	reject := appr + "/" + reqID + "/reject"
	if r := e.call("carol", "POST", approve, map[string]any{}); r.Status != 403 || errCode(r) != "self_approval" {
		t.Fatalf("self approval: %d %s", r.Status, r.Raw)
	}
	if r := e.call("carol", "POST", reject, map[string]any{"reason": "x"}); r.Status != 403 || errCode(r) != "self_approval" {
		t.Fatalf("self rejection: %d %s", r.Status, r.Raw)
	}
	if r := e.call("bob", "POST", approve, map[string]any{}); r.Status != 403 {
		t.Fatalf("guest approves: %d", r.Status)
	}
	if r := e.call("alice", "POST", approve, map[string]any{}); r.Status != 403 || errCode(r) != "not_an_approver" {
		// The policy names the member role only: even the owner is not an approver.
		t.Fatalf("owner outside the approver set: %d %s", r.Status, r.Raw)
	}
	if r := e.call("dave", "POST", approve, map[string]any{}); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}
	if r := e.call("eve", "POST", reject, map[string]any{"reason": "  "}); r.Status != 422 {
		t.Fatalf("reject without reason: %d %s", r.Status, r.Raw)
	}
	r = e.must(200, "eve", "POST", approve, map[string]any{"comment": "Yoxlandı"})
	if a := lastApproval(r); a["approvals"] != 1.0 || a["state"] != "pending" || len(a["approvers"].([]any)) != 1 {
		t.Fatalf("after one of two approvals: %v", a)
	}
	if r := e.call("eve", "POST", approve, map[string]any{}); r.Status != 409 || errCode(r) != "already_decided" {
		t.Fatalf("approving twice: %d %s", r.Status, r.Raw)
	}
	if p := e.pending(one); p == nil || p["approvals"] != 1.0 || p["min_approvals"] != 2.0 {
		t.Fatalf("badge after one approval: %v", p)
	}
	r = e.must(200, "erin", "POST", approve, map[string]any{})
	if a := lastApproval(r); a["state"] != "approved" || a["closed_by"].(map[string]any)["id"] != erin || len(a["decisions"].([]any)) != 2 {
		t.Fatalf("approved request: %v", a)
	}
	item = r.Body["item"].(map[string]any)
	if item["status"].(map[string]any)["id"] != done || item["approval"] != nil || item["version"] != 4.0 {
		t.Fatalf("item after the final approval: %v", item)
	}
	if r := e.call("carol", "POST", appr+"/"+reqID+"/cancel", nil); r.Status != 409 || errCode(r) != "approval_closed" {
		t.Fatalf("cancel a decided request: %d %s", r.Status, r.Raw)
	}
	// Unlocked again.
	e.must(200, "carol", "PATCH", one, map[string]any{"version": 4, "title": "Ödəniş 1 (ödənilib)"})

	// --- audit trail and notifications ---------------------------------------
	act := e.must(200, "carol", "GET", one+"/activity?limit=100", nil).Raw
	for _, a := range []string{"approval.requested", "approval.approved", "approval.applied", "Qəbz əlavə olunub", "Yoxlandı", reqID} {
		if !contains(act, a) {
			t.Errorf("activity lacks %q", a)
		}
	}
	if n := e.count(`SELECT count(*) FROM events WHERE action = 'item.updated' AND payload->'approval'->>'request_id' = $1`, reqID); n != 1 {
		t.Fatalf("status change events tagged with the request: %d", n)
	}
	e.drain()
	if got := strings.Join(kinds(e.notes("carol", "")), ","); !contains(got, "approval_applied:ACME-1") {
		t.Errorf("requester notifications: %s", got)
	}
}

func TestApprovalRejectAndCancel(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_user_ids": []string{e.userID("eve"), e.userID("erin")}})
	for _, title := range []string{"Rədd", "Ləğv", "Admin ləğvi"} {
		e.must(201, "carol", "POST", items, map[string]any{"title": title})
	}
	// Reject: closes the request with the reason, item unchanged and unlocked.
	id := lastApproval(e.must(201, "carol", "POST", items+"/1/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	r := e.must(200, "eve", "POST", items+"/1/approvals/"+id+"/reject", map[string]any{"reason": "Məbləğ müqavilə ilə uyğun deyil"})
	a := lastApproval(r)
	if a["state"] != "rejected" || r.Body["item"].(map[string]any)["status"].(map[string]any)["id"] == done {
		t.Fatalf("rejected: %v", r.Raw)
	}
	if d := a["decisions"].([]any)[0].(map[string]any); d["decision"] != "reject" || d["comment"] != "Məbləğ müqavilə ilə uyğun deyil" {
		t.Fatalf("decision: %v", d)
	}
	if r := e.call("erin", "POST", items+"/1/approvals/"+id+"/approve", map[string]any{}); r.Status != 409 || errCode(r) != "approval_closed" {
		t.Fatalf("approve after rejection: %d %s", r.Status, r.Raw)
	}
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "title": "Rədd (düzəldildi)"})

	// Cancel: only the requester (or an admin).
	id = lastApproval(e.must(201, "carol", "POST", items+"/2/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	if r := e.call("eve", "POST", items+"/2/approvals/"+id+"/cancel", nil); r.Status != 403 {
		t.Fatalf("approver cancels: %d", r.Status)
	}
	if a := lastApproval(e.must(200, "carol", "POST", items+"/2/approvals/"+id+"/cancel", nil)); a["state"] != "canceled" {
		t.Fatalf("canceled: %v", a)
	}
	id = lastApproval(e.must(201, "carol", "POST", items+"/3/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)
	e.must(200, "alice", "POST", items+"/3/approvals/"+id+"/cancel", nil)

	// A pending request blocks deleting its target status.
	e.must(201, "carol", "POST", items+"/2/approvals", map[string]any{"version": 1, "to_status_id": done})
	if r := e.call("alice", "DELETE", wfBase+"/"+wf["id"].(string)+"/statuses/"+done, nil); r.Status != 409 {
		t.Fatalf("delete a status with pending requests: %d %s", r.Status, r.Raw)
	}

	e.drain()
	got := strings.Join(kinds(e.notes("carol", "")), ",")
	if !contains(got, "approval_rejected:ACME-1") || !contains(got, "approval_canceled:ACME-3") || contains(got, "approval_canceled:ACME-2") {
		t.Fatalf("requester notifications (own cancel is silent): %s", got)
	}
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log?limit=100", nil).Raw
	for _, a := range []string{"approval.rejected", "approval.canceled", "Məbləğ müqavilə ilə uyğun deyil"} {
		if !contains(audit, a) {
			t.Errorf("audit log lacks %q", a)
		}
	}
}

func TestApprovalConcurrentApprovesApplyOnce(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	for _, min := range []int{1, 2} {
		e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": min, "approver_roles": []string{"member"}})
		r := e.must(201, "carol", "POST", items, map[string]any{"title": "Eyni anda"})
		path := items + "/" + strconv.Itoa(int(r.Body["number"].(float64)))
		id := lastApproval(e.must(201, "carol", "POST", path+"/approvals", map[string]any{"version": 1, "to_status_id": done}))["id"].(string)

		var wg sync.WaitGroup
		codes := make([]int, 2)
		for i, u := range []string{"eve", "erin"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = e.call(u, "POST", path+"/approvals/"+id+"/approve", map[string]any{}).Status
			}()
		}
		wg.Wait()
		ok := 0
		for _, c := range codes {
			if c == 200 {
				ok++
			} else if c != 409 {
				t.Fatalf("min %d: unexpected status %d", min, c)
			}
		}
		if ok != min {
			t.Fatalf("min %d: %d approvals accepted (%v)", min, ok, codes)
		}
		item := e.must(200, "carol", "GET", path, nil).Body
		if item["status"].(map[string]any)["id"] != done || item["version"] != 2.0 {
			t.Fatalf("min %d: item %v", min, item)
		}
		if c := e.count(`SELECT count(*) FROM events WHERE action = 'approval.applied' AND payload->>'request_id' = $1`, id); c != 1 {
			t.Fatalf("min %d: applied %d times", min, c)
		}
	}
}

func TestApprovalPrivateProjectEligibility(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	carol, erin := e.userID("carol"), e.userID("erin")
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Xəzinə", "visibility": "private"})
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	e.must(201, "carol", "POST", secItems, map[string]any{"title": "Gizli ödəniş"})
	r := e.must(201, "carol", "POST", secItems+"/1/approvals", map[string]any{"version": 1, "to_status_id": done})
	a := lastApproval(r)
	if len(a["approvers"].([]any)) != 0 {
		t.Fatalf("members outside the private project are listed as approvers: %v", a["approvers"])
	}
	id := a["id"].(string)
	e.drain()
	for _, u := range []string{"eve", "erin"} {
		if got := kinds(e.notes(u, "")); len(got) != 0 {
			t.Fatalf("%s was notified about a private item: %v", u, got)
		}
		if r := e.call(u, "POST", secItems+"/1/approvals/"+id+"/approve", map[string]any{}); r.Status != 404 {
			t.Fatalf("%s approves in a project they cannot see: %d", u, r.Status)
		}
	}
	// Once erin may see the project she becomes eligible.
	e.must(204, "alice", "PUT", secPath+"/members/"+erin, nil)
	if got := e.must(200, "erin", "GET", secItems+"?approval=mine", nil).Body["items"].([]any); len(got) != 1 {
		t.Fatalf("pending my approval after access: %v", got)
	}
	e.must(200, "erin", "POST", secItems+"/1/approvals/"+id+"/approve", map[string]any{})
	// The public project's lists never show the private request.
	if got := e.must(200, "eve", "GET", items+"?approval=pending", nil).Body["items"].([]any); len(got) != 0 {
		t.Fatalf("leak: %v", got)
	}
}

func TestApprovalImportRefusesGuardedStatus(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"admin"}})
	csv := []byte("Başlıq;Status\nAçıq;To Do\nBağlı;Done\n")
	mapping := map[string]any{"columns": []map[string]any{{"column": 0, "target": "title"}, {"column": 1, "target": "status"}}}
	r := e.upload("alice", importsPath+"?dry_run=true", "a.csv", csv, "", mapping)
	if r.Status != 200 || r.Body["error_count"] != 1.0 {
		t.Fatalf("dry run: %d %s", r.Status, r.Raw)
	}
	if x := r.Body["errors"].([]any)[0].(map[string]any); x["row"] != 3.0 || x["field"] != "status_id" || !contains(x["message"].(string), "approval") {
		t.Fatalf("row error: %v", x)
	}
	if r := e.upload("alice", importsPath, "a.csv", csv, "", mapping); r.Status != 422 {
		t.Fatalf("import: %d %s", r.Status, r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM items`); n != 0 {
		t.Fatalf("import wrote %d items", n)
	}
}

func TestApprovalAutomationCannotBypass(t *testing.T) {
	e := newP1Env(t)
	wf, _, _, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	e.must(201, "alice", "POST", wsBase+"/automations", map[string]any{
		"name": "Avto bağla", "trigger": "item.created",
		"conditions": []any{}, "actions": []map[string]any{{"type": "set_status", "status_id": done}},
	})
	e.must(201, "carol", "POST", items, map[string]any{"title": "Avtomat"})
	e.drain()
	if s := e.must(200, "carol", "GET", items+"/1", nil).Body["status"].(map[string]any)["id"]; s == done {
		t.Fatal("automation moved the item into a guarded status")
	}
	if n := e.count(`SELECT count(*) FROM jobs WHERE kind = 'automation.run' AND state = 'failed'`); n != 1 {
		t.Fatalf("the rule run should fail permanently, failed jobs: %d", n)
	}
}
