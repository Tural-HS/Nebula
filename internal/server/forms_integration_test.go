package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// Intake forms (ADR-0032). Synthetic people only: alice owner, bob guest,
// carol member, dave signed in but not a member of acme.

const formsPath = "/api/v1/workspaces/acme/projects/ACME/forms"

// formSetup creates the custom fields a payment request uses and binds them
// to "task": vendor (text), amount (number, required by the item type),
// category (select) and approver (user: never allowed on a form).
func (e *p1env) formSetup() (categories map[string]string) {
	e.t.Helper()
	e.setup()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "vendor", "name": "Vendor", "type": "text"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "amount", "name": "Amount", "type": "number"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "confirm", "name": "Confirmed", "type": "boolean"})
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "approver", "name": "Approver", "type": "user"})
	cat := e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "category", "name": "Category", "type": "select",
		"options": map[string]any{"choices": []map[string]any{{"label": "Utilities"}, {"label": "Travel"}}}})
	for _, k := range []string{"vendor", "confirm", "approver", "category"} {
		e.must(200, "alice", "PUT", bindPath("task", k), map[string]any{})
	}
	e.must(200, "alice", "PUT", bindPath("task", "amount"), map[string]any{"required": true})
	return choiceIDs(e.t, cat)
}

func paymentForm(overrides map[string]any) map[string]any {
	f := map[string]any{
		"slug": "payment-request", "title": "Payment request", "item_type": "task",
		"intro":          "For payments to external vendors.",
		"title_template": "Ödəniş tələbi: {{field.vendor}}",
		"fields": []map[string]any{
			{"key": "vendor", "label": "Vendor name", "help": "As on the invoice", "required": true},
			{"key": "amount", "label": "Amount (AZN)", "required": true},
			{"key": "category", "label": "Category"},
			{"key": "description", "label": "Details"},
			{"key": "due_date", "label": "Pay by"},
		},
		"audience": "members",
	}
	for k, v := range overrides {
		f[k] = v
	}
	return f
}

func submit(values map[string]any) map[string]any { return map[string]any{"values": values} }

func (e *p1env) countEvents(where string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE `+where, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestFormBuilder(t *testing.T) {
	e := newP1Env(t)
	e.formSetup()
	if r := e.call("carol", "POST", formsPath, paymentForm(nil)); r.Status != 403 {
		t.Fatalf("member creates a form: %d", r.Status)
	}
	if r := e.call("carol", "GET", formsPath, nil); r.Status != 403 {
		t.Fatalf("member lists forms: %d", r.Status)
	}
	wf := e.defaultWorkflow()
	daveID := e.userID("dave")
	cases := map[string]struct {
		over  map[string]any
		field string
	}{
		"bad slug":          {map[string]any{"slug": "Payment Request"}, "slug"},
		"short slug":        {map[string]any{"slug": "pr"}, "slug"},
		"no title":          {map[string]any{"title": " "}, "title"},
		"unknown type":      {map[string]any{"item_type": "invoice"}, "item_type"},
		"bad audience":      {map[string]any{"audience": "public"}, "audience"},
		"too many files":    {map[string]any{"max_files": 11}, "max_files"},
		"unknown field":     {map[string]any{"fields": []map[string]any{{"key": "amount", "label": "A", "required": true}, {"key": "iban", "label": "IBAN"}}}, "fields[1].key"},
		"user field":        {map[string]any{"fields": []map[string]any{{"key": "amount", "label": "A", "required": true}, {"key": "approver", "label": "Approver"}}}, "fields[1].key"},
		"duplicate field":   {map[string]any{"fields": []map[string]any{{"key": "amount", "label": "A", "required": true}, {"key": "amount", "label": "B", "required": true}}}, "fields[1].key"},
		"missing label":     {map[string]any{"fields": []map[string]any{{"key": "amount", "label": "", "required": true}}}, "fields[0].label"},
		"type requires it":  {map[string]any{"fields": []map[string]any{{"key": "vendor", "label": "V"}}, "title_template": "Payment"}, "fields"},
		"optional required": {map[string]any{"fields": []map[string]any{{"key": "amount", "label": "A"}}, "title_template": "Payment"}, "fields[0].required"},
		"bad placeholder":   {map[string]any{"title_template": "Pay {{field.iban}}"}, "title_template"},
		"placeholder typo":  {map[string]any{"title_template": "Pay {{vendor}}"}, "title_template"},
		"unbalanced":        {map[string]any{"title_template": "Pay {{field.vendor"}, "title_template"},
		"foreign status":    {map[string]any{"initial_status_id": "01a20727-0000-7000-8000-000000000000"}, "initial_status_id"},
		"outsider assignee": {map[string]any{"default_assignee_id": daveID}, "default_assignee_id"},
	}
	for name, c := range cases {
		r := e.call("alice", "POST", formsPath, paymentForm(c.over))
		if r.Status != 422 || errField(r) != c.field {
			t.Errorf("%s: %d field %q, want 422 %q; %s", name, r.Status, errField(r), c.field, r.Raw)
		}
	}

	f := e.must(201, "alice", "POST", formsPath, paymentForm(map[string]any{
		"initial_status_id": statusID(t, wf, "In Progress"), "default_assignee_id": e.userID("carol")})).Body
	if f["enabled"] != true || f["slug"] != "payment-request" || f["project_key"] != "ACME" || len(f["fields"].([]any)) != 5 {
		t.Fatalf("created form: %v", f)
	}
	if r := e.call("alice", "POST", formsPath, paymentForm(nil)); r.Status != 409 || errCode(r) != "slug_taken" {
		t.Fatalf("duplicate slug: %d %s", r.Status, r.Raw)
	}
	id := f["id"].(string)
	upd := e.must(200, "alice", "PUT", formsPath+"/"+id, paymentForm(map[string]any{"title": "Vendor payment", "enabled": false})).Body
	if upd["title"] != "Vendor payment" || upd["enabled"] != false || upd["initial_status_id"] != nil {
		t.Fatalf("updated form: %v", upd)
	}
	if n := len(e.must(200, "alice", "GET", formsPath, nil).Body["forms"].([]any)); n != 1 {
		t.Fatalf("forms listed: %d", n)
	}
	e.must(204, "alice", "DELETE", formsPath+"/"+id, nil)
	e.must(404, "alice", "GET", formsPath+"/"+id, nil)
	for _, action := range []string{"form.created", "form.updated", "form.deleted"} {
		if n := e.countEvents(`entity_type = 'form' AND entity_id = $1 AND action = $2 AND payload->>'project_id' IS NOT NULL`, id, action); n != 1 {
			t.Errorf("%s events: %d", action, n)
		}
	}
}

func TestFormSubmission(t *testing.T) {
	e := newP1Env(t)
	cats := e.formSetup()
	carolID := e.userID("carol")
	form := e.must(201, "alice", "POST", formsPath, paymentForm(map[string]any{"default_assignee_id": carolID})).Body
	formID := form["id"].(string)
	pub := "/api/v1/forms/payment-request"

	t.Run("audience: members", func(t *testing.T) {
		got := e.must(200, "bob", "GET", pub, nil).Body["form"].(map[string]any) // guests are members
		if got["open"] != true || got["workspace"] != "Acme Inc" || len(got["fields"].([]any)) != 5 || got["can_manage"] != false {
			t.Fatalf("public form: %v", got)
		}
		cat := got["fields"].([]any)[2].(map[string]any)
		if cat["type"] != "select" || len(cat["choices"].([]any)) != 2 {
			t.Fatalf("category input: %v", cat)
		}
		if _, leaks := got["project_key"]; leaks {
			t.Fatal("the public form names the project")
		}
		e.must(200, "bob", "GET", "/api/v1/forms/"+formID, nil) // by id too
		if e.must(200, "alice", "GET", pub, nil).Body["form"].(map[string]any)["can_manage"] != true {
			t.Fatal("admins manage the form")
		}
		// dave is not a member: the form does not exist for him.
		for _, r := range []resp{e.call("dave", "GET", pub, nil), e.call("dave", "POST", pub+"/submissions", submit(map[string]any{"vendor": "x", "amount": 1}))} {
			if r.Status != 404 {
				t.Fatalf("non-member: %d %s", r.Status, r.Raw)
			}
		}
		if r := e.call("bob", "GET", "/api/v1/forms/no-such-form", nil); r.Status != 404 {
			t.Fatalf("unknown slug: %d", r.Status)
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := map[string]struct {
			values map[string]any
			field  string
		}{
			"required missing":  {map[string]any{"amount": 120}, "values.vendor"},
			"required blank":    {map[string]any{"vendor": "   ", "amount": 120}, "values.vendor"},
			"required null":     {map[string]any{"vendor": "V", "amount": nil}, "values.amount"},
			"wrong type":        {map[string]any{"vendor": "V", "amount": "120"}, "values.amount"},
			"unknown choice":    {map[string]any{"vendor": "V", "amount": 1, "category": "nope"}, "values.category"},
			"not on the form":   {map[string]any{"vendor": "V", "amount": 1, "approver": carolID}, "values.approver"},
			"bad date":          {map[string]any{"vendor": "V", "amount": 1, "due_date": "31.12.2026"}, "values.due_date"},
			"description shape": {map[string]any{"vendor": "V", "amount": 1, "description": 5}, "values.description"},
		}
		for name, c := range cases {
			r := e.call("bob", "POST", pub+"/submissions", submit(c.values))
			if r.Status != 422 || errField(r) != c.field {
				t.Errorf("%s: %d %q; %s", name, r.Status, errField(r), r.Raw)
			}
		}
		if n := e.count(`SELECT count(*) FROM items`); n != 0 {
			t.Fatalf("refused submissions created %d items", n)
		}
	})

	// Form-created items run automations like any user change (unlike imports).
	e.must(201, "alice", "POST", wsBase+"/automations", map[string]any{
		"name": "Tag form items", "trigger": "item.created",
		"actions": []any{map[string]any{"type": "set_field", "key": "confirm", "value": true}},
	})

	sub := e.must(201, "bob", "POST", pub+"/submissions", submit(map[string]any{
		"vendor": "  Vendor-A   Test\nMMC ", "amount": 120.5, "category": cats["Travel"], "description": "Invoice INV-0001", "due_date": "2026-12-31",
	})).Body
	if sub["key"] != "ACME-1" || sub["title"] != "Ödəniş tələbi: Vendor-A Test MMC" {
		t.Fatalf("submission: %v", sub)
	}
	e.drain()
	item := e.must(200, "carol", "GET", items+"/1", nil).Body
	fields := item["fields"].(map[string]any)
	if item["reporter_id"] != e.userID("bob") || item["assignee_id"] != carolID || item["due_date"] != "2026-12-31" ||
		fields["amount"] != 120.5 || fields["category"] != cats["Travel"] || item["description"] != "Invoice INV-0001" {
		t.Fatalf("item: %v", item)
	}
	if fields["confirm"] != true {
		t.Fatalf("the item.created automation did not run on a form item: %v", fields)
	}
	if n := e.countEvents(`action = 'item.created' AND source = 'form' AND actor_id = $1 AND payload->>'form_id' = $2
		AND payload->>'submission_id' = $3`, e.userID("bob"), formID, sub["id"]); n != 1 {
		t.Fatalf("item.created with source form: %d", n)
	}
	// The default "notify the assignee" rule ran too: the event is dispatched.
	if n := len(e.notes("carol", "")); n != 1 {
		t.Fatalf("assignee notifications: %d", n)
	}

	t.Run("item side", func(t *testing.T) {
		req := e.must(200, "carol", "GET", items+"/1/request", nil).Body
		rq := req["requester"].(map[string]any)
		if req["form_title"] != "Payment request" || rq["email"] != "bob@example.com" || rq["member"] != true {
			t.Fatalf("item request: %v", req)
		}
		if r := e.call("bob", "GET", items+"/1/request", nil); r.Status != 403 {
			t.Fatalf("guest reads the requester: %d", r.Status)
		}
		e.must(201, "carol", "POST", items, map[string]any{"title": "Not from a form", "fields": map[string]any{"amount": 1}})
		e.must(404, "carol", "GET", items+"/2/request", nil)
	})

	t.Run("disabled form", func(t *testing.T) {
		e.must(200, "alice", "PUT", formsPath+"/"+formID, paymentForm(map[string]any{"enabled": false}))
		got := e.must(200, "bob", "GET", pub, nil).Body["form"].(map[string]any)
		if got["open"] != false || len(got["fields"].([]any)) != 0 || got["intro"] != nil {
			t.Fatalf("closed form shows its inputs: %v", got)
		}
		if adm := e.must(200, "alice", "GET", pub, nil).Body["form"].(map[string]any); len(adm["fields"].([]any)) != 5 {
			t.Fatalf("admins preview a closed form: %v", adm)
		}
		if r := e.call("bob", "POST", pub+"/submissions", submit(map[string]any{"vendor": "V", "amount": 1})); r.Status != 409 || errCode(r) != "form_closed" {
			t.Fatalf("closed form: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("audience: accounts", func(t *testing.T) {
		e.must(200, "alice", "PUT", formsPath+"/"+formID, paymentForm(map[string]any{"audience": "accounts"}))
		e.must(200, "dave", "GET", pub, nil)
		s := e.must(201, "dave", "POST", pub+"/submissions", submit(map[string]any{"vendor": "Vendor-B", "amount": 7})).Body
		if s["key"] != "ACME-3" {
			t.Fatalf("outsider submission: %v", s)
		}
		// A requester gains no access to the workspace.
		for _, p := range []string{"/api/v1/workspaces/acme", items + "/3", items + "/3/request"} {
			if r := e.call("dave", "GET", p, nil); r.Status != 404 {
				t.Errorf("outsider GET %s: %d", p, r.Status)
			}
		}
		if rq := e.must(200, "carol", "GET", items+"/3/request", nil).Body["requester"].(map[string]any); rq["member"] != false {
			t.Fatalf("outsider marked as member: %v", rq)
		}
		// Not a member, so no watcher row (watchers belong to memberships).
		if n := e.count(`SELECT count(*) FROM item_watchers w JOIN users u ON u.id = w.user_id WHERE u.email = 'dave@example.com'`); n != 0 {
			t.Fatalf("outsider watches the item: %d", n)
		}
	})
}

func TestFormPrivateProject(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "alice", "POST", "/api/v1/workspaces/acme/projects", map[string]any{"key": "SEC", "name": "Treasury", "visibility": "private"})
	secForms := "/api/v1/workspaces/acme/projects/SEC/forms"
	e.must(201, "alice", "POST", secForms, map[string]any{
		"slug": "access-request", "title": "Access request", "item_type": "task", "title_template": "Access: {{requester}}",
		"fields": []map[string]any{{"key": "description", "label": "What do you need?", "required": true}},
	})
	s := e.must(201, "carol", "POST", "/api/v1/forms/access-request/submissions",
		submit(map[string]any{"description": "Read access to the ledger"})).Body
	if s["key"] != "SEC-1" || s["title"] != "Access: Carol" {
		t.Fatalf("submission into a private project: %v", s)
	}
	// carol is a member but not in SEC: the project stays hidden.
	for _, p := range []string{"/api/v1/workspaces/acme/projects/SEC", "/api/v1/workspaces/acme/projects/SEC/items/1"} {
		if r := e.call("carol", "GET", p, nil); r.Status != 404 {
			t.Errorf("carol GET %s: %d", p, r.Status)
		}
	}
	if keys := e.visibleSearch("alice", "ledger"); keys != 1 { // the control: the item is indexed
		t.Errorf("admin search hits: %d", keys)
	}
	if keys := e.visibleSearch("carol", "ledger"); keys != 0 {
		t.Errorf("search finds the private item: %d", keys)
	}
	e.drain()
	if n := len(e.notes("carol", "")); n != 0 {
		t.Errorf("notifications about a hidden project: %d", n)
	}
	list := e.must(200, "carol", "GET", "/api/v1/me/requests", nil).Body
	reqs := list["requests"].([]any)
	if len(reqs) != 1 {
		t.Fatalf("my requests: %s", e.call("carol", "GET", "/api/v1/me/requests", nil).Raw)
	}
	mine := reqs[0].(map[string]any)
	if mine["key"] != "SEC-1" || mine["title"] != "Access: Carol" || mine["form_title"] != "Access request" ||
		mine["status"].(map[string]any)["name"] != "To Do" {
		t.Fatalf("my request: %v", mine)
	}
	raw := e.must(200, "carol", "GET", "/api/v1/me/requests/"+mine["id"].(string), nil).Raw
	for _, leak := range []string{"Treasury", "project_id", "assignee", "reporter"} {
		if strings.Contains(raw, leak) {
			t.Errorf("request detail leaks %q: %s", leak, raw)
		}
	}
}

// visibleSearch counts workspace search hits for q as user (ADR-0024).
func (e *p1env) visibleSearch(user, q string) int {
	e.t.Helper()
	r := e.must(200, user, "GET", "/api/v1/workspaces/acme/search?q="+q, nil)
	list, _ := r.Body["hits"].([]any)
	return len(list)
}

func TestMyRequestsIsolationAndUpdates(t *testing.T) {
	e := newP1Env(t)
	e.formSetup()
	wf := e.defaultWorkflow()
	form := e.must(201, "alice", "POST", formsPath, paymentForm(nil)).Body
	pub := "/api/v1/forms/payment-request/submissions"
	bobs := e.must(201, "bob", "POST", pub, submit(map[string]any{"vendor": "Vendor-C", "amount": 10, "due_date": ""})).Body
	e.must(201, "bob", "POST", pub, submit(map[string]any{"vendor": "Vendor-D", "amount": 20}))
	carols := e.must(201, "carol", "POST", pub, submit(map[string]any{"vendor": "Vendor-E", "amount": 30})).Body
	bobID := bobs["id"].(string)
	me := "/api/v1/me/requests"

	// Nobody reads someone else's request, not even an admin of the workspace.
	for _, u := range []string{"carol", "alice", "dave"} {
		if r := e.call(u, "GET", me+"/"+bobID, nil); r.Status != 404 {
			t.Errorf("%s reads bob's request: %d", u, r.Status)
		}
		if r := e.call(u, "POST", me+"/"+bobID+"/seen", nil); r.Status != 404 {
			t.Errorf("%s marks bob's request: %d", u, r.Status)
		}
	}
	page := e.must(200, "bob", "GET", me+"?limit=1", nil).Body
	first := page["requests"].([]any)
	if len(first) != 1 || first[0].(map[string]any)["title"] != "Ödəniş tələbi: Vendor-D" || page["next_cursor"] == nil {
		t.Fatalf("first page: %v", page)
	}
	second := e.must(200, "bob", "GET", me+"?limit=1&cursor="+page["next_cursor"].(string), nil).Body
	if l := second["requests"].([]any); len(l) != 1 || l[0].(map[string]any)["id"] != bobID {
		t.Fatalf("second page: %v", second)
	}
	if n := len(e.must(200, "carol", "GET", me, nil).Body["requests"].([]any)); n != 1 {
		t.Fatalf("carol's list: %d", n)
	}
	if r := e.call("bob", "GET", me+"?cursor=bogus", nil); r.Status != 400 {
		t.Fatalf("bad cursor: %d", r.Status)
	}

	detail := e.must(200, "bob", "GET", me+"/"+bobID, nil).Body
	answers := detail["answers"].([]any)
	if len(answers) != 2 || answers[0].(map[string]any)["label"] != "Vendor name" || answers[1].(map[string]any)["display"] != "10" {
		t.Fatalf("answers: %v", answers)
	}
	if detail["unseen"] != false || detail["message"] != nil {
		t.Fatalf("fresh request: %v", detail)
	}

	// The team moves the item and writes to the requester.
	e.must(200, "carol", "PATCH", items+"/1", map[string]any{"version": 1, "status_id": statusID(t, wf, "In Progress")})
	page = e.must(200, "bob", "GET", me, nil).Body
	if page["unseen"] != 1.0 {
		t.Fatalf("unseen after a status change: %v", page["unseen"])
	}
	// Internal edits are not news for the requester.
	e.must(200, "carol", "PATCH", items+"/2", map[string]any{"version": 1, "title": "Renamed internally"})
	l := e.must(200, "bob", "GET", me, nil).Body["requests"].([]any)
	if l[0].(map[string]any)["title"] != "Ödəniş tələbi: Vendor-D" || l[0].(map[string]any)["unseen"] != false {
		t.Fatalf("internal rename shows: %v", l[0])
	}
	e.must(204, "bob", "POST", me+"/"+bobID+"/seen", nil)
	if u := e.must(200, "bob", "GET", me, nil).Body["unseen"]; u != 0.0 {
		t.Fatalf("unseen after opening: %v", u)
	}
	if r := e.call("bob", "PUT", items+"/1/request/message", map[string]any{"message": "x"}); r.Status != 403 {
		t.Fatalf("guest writes to the requester: %d", r.Status)
	}
	msg := e.must(200, "carol", "PUT", items+"/1/request/message", map[string]any{"message": "Paid on Friday."}).Body
	if msg["message"] != "Paid on Friday." || msg["message_by"].(map[string]any)["display_name"] != "Carol" {
		t.Fatalf("message: %v", msg)
	}
	detail = e.must(200, "bob", "GET", me+"/"+bobID, nil).Body
	if detail["message"] != "Paid on Friday." || detail["unseen"] != true || detail["status"].(map[string]any)["name"] != "In Progress" {
		t.Fatalf("detail after message: %v", detail)
	}
	if n := e.countEvents(`action = 'request.message_set' AND entity_type = 'item'`); n != 1 {
		t.Fatalf("message events: %d", n)
	}
	e.must(200, "carol", "PUT", items+"/1/request/message", map[string]any{"message": "Paid on Friday."}) // no-op
	if n := e.countEvents(`action = 'request.message_set'`); n != 1 {
		t.Fatalf("a repeated message wrote an event: %d", n)
	}

	// Deleting the item or the form keeps the requester's record.
	e.must(204, "carol", "DELETE", items+"/3", nil)
	e.must(204, "alice", "DELETE", formsPath+"/"+form["id"].(string), nil)
	cd := e.must(200, "carol", "GET", me+"/"+carols["id"].(string), nil).Body
	if cd["removed"] != true || cd["status"] != nil || cd["form_title"] != "Payment request" || cd["unseen"] != true {
		t.Fatalf("removed request: %v", cd)
	}
}

func TestFormRateLimit(t *testing.T) {
	e := newP1Env(t)
	e.formSetup()
	e.must(201, "alice", "POST", formsPath, paymentForm(nil))
	pub := "/api/v1/forms/payment-request/submissions"
	for i := range formSubmitBurst {
		e.must(201, "carol", "POST", pub, submit(map[string]any{"vendor": "Vendor", "amount": i + 1}))
	}
	r := e.call("carol", "POST", pub, submit(map[string]any{"vendor": "Vendor", "amount": 99}))
	if r.Status != 429 || errCode(r) != "rate_limited" {
		t.Fatalf("over the burst: %d %s", r.Status, r.Raw)
	}
	e.must(201, "bob", "POST", pub, submit(map[string]any{"vendor": "Vendor", "amount": 1})) // per user
	big := strings.Repeat("x", maxBodyBytes)
	if r := e.call("bob", "POST", pub, `{"values": {"description": "`+big+`"}}`); r.Status != 413 {
		t.Fatalf("oversized body: %d", r.Status)
	}
}

func TestFormApprovalPolicy(t *testing.T) {
	e := newP1Env(t)
	wf, todo, inProgress, done := e.approvalSetup()
	e.must(200, "alice", "PUT", policyPath(wf, done), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	e.must(200, "alice", "PUT", policyPath(wf, todo), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	base := map[string]any{"slug": "expense", "title": "Expense approval", "item_type": "task", "title_template": "Expense",
		"fields": []map[string]any{{"key": "description", "label": "What for?"}}}
	with := func(status string) map[string]any {
		b := map[string]any{}
		for k, v := range base {
			b[k] = v
		}
		b["initial_status_id"] = status
		return b
	}
	// Straight into a guarded status would skip its approval.
	if r := e.call("alice", "POST", formsPath, with(done)); r.Status != 422 || errField(r) != "initial_status_id" {
		t.Fatalf("guarded initial status: %d %s", r.Status, r.Raw)
	}
	// The workflow's initial status is exempt, as for items (ADR-0026).
	e.must(201, "alice", "POST", formsPath, with(todo))
	f := e.must(200, "alice", "GET", formsPath, nil).Body["forms"].([]any)[0].(map[string]any)
	e.must(200, "alice", "PUT", formsPath+"/"+f["id"].(string), with(inProgress))
	e.must(201, "carol", "POST", "/api/v1/forms/expense/submissions", submit(map[string]any{"description": "Taxi"}))
	item := e.must(200, "carol", "GET", items+"/1", nil).Body
	if item["status"].(map[string]any)["id"] != inProgress {
		t.Fatalf("initial status: %v", item["status"])
	}
	// Approvals then apply as usual.
	if r := e.call("eve", "PATCH", items+"/1", map[string]any{"version": item["version"], "status_id": done}); r.Status != 409 || errCode(r) != "approval_required" {
		t.Fatalf("move into a guarded status: %d %s", r.Status, r.Raw)
	}
	// A policy added after the form was saved: no submission skips it.
	e.must(200, "alice", "PUT", policyPath(wf, inProgress), map[string]any{"min_approvals": 1, "approver_roles": []string{"member"}})
	r := e.call("carol", "POST", "/api/v1/forms/expense/submissions", submit(map[string]any{"description": "Hotel"}))
	if r.Status != 409 || errCode(r) != "form_misconfigured" {
		t.Fatalf("submission into a newly guarded status: %d %s", r.Status, r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM items`); n != 1 {
		t.Fatalf("items after the refused submission: %d", n)
	}
}

// formUpload posts a multipart submission: a values part and files.
func (e *attachEnv) formUpload(token, path string, values map[string]any, files ...fileUpload) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	b, _ := json.Marshal(values)
	_ = mw.WriteField("values", string(b))
	for _, u := range files {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+u.Filename+`"`)
		hdr.Set("Content-Type", u.ContentType)
		pw, _ := mw.CreatePart(hdr)
		_, _ = pw.Write(u.Data)
	}
	_ = mw.Close()
	req := httptest.NewRequestWithContext(e.t.Context(), "POST", path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	r := resp{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.Body)
	return r
}

func TestFormAttachments(t *testing.T) {
	e := newAttachEnv(t, Attachments{MaxFileBytes: 1 << 10, MaxPerItem: 50})
	e.setup()
	e.must(201, "alice", "POST", formsPath, map[string]any{"slug": "receipt", "title": "Receipt", "item_type": "task",
		"title_template": "Receipt from {{requester}}", "max_files": 2,
		"fields": []map[string]any{{"key": "description", "label": "Notes"}}})
	pub := "/api/v1/forms/receipt/submissions"
	png := fileUpload{Filename: "receipt.png", ContentType: "image/png", Data: pngBytes}

	if got := e.must(200, "bob", "GET", "/api/v1/forms/receipt", nil).Body; got["form"].(map[string]any)["max_files"] != 2.0 || got["limits"] == nil {
		t.Fatalf("file limits: %v", got)
	}
	if r := e.formUpload("bob", pub, map[string]any{}, png, png, png); r.Status != 422 || errField(r) != "files" {
		t.Fatalf("three files: %d %s", r.Status, r.Raw)
	}
	big := fileUpload{Filename: "big.bin", ContentType: "application/octet-stream", Data: bytes.Repeat([]byte{1}, 2<<10)}
	if r := e.formUpload("bob", pub, map[string]any{}, png, big); r.Status != 413 {
		t.Fatalf("oversized file: %d %s", r.Status, r.Raw)
	}
	if r := e.formUpload("bob", pub, map[string]any{"bogus": 1}, png); r.Status != 422 {
		t.Fatalf("invalid values with a file: %d %s", r.Status, r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM items`); n != 0 {
		t.Fatalf("refused uploads created items: %d", n)
	}
	e.purge()
	if blobs, staging := e.blobCount(); blobs != 0 || staging != 0 {
		t.Fatalf("refused uploads left files: %d blobs, %d staging", blobs, staging)
	}

	s := e.formUpload("bob", pub, map[string]any{"description": "Taxi to the office"}, png)
	if s.Status != 201 || s.Body["title"] != "Receipt from Bob" {
		t.Fatalf("submission with a file: %d %s", s.Status, s.Raw)
	}
	atts := e.must(200, "carol", "GET", items+"/1/attachments", nil).Body["attachments"].([]any)
	if len(atts) != 1 || atts[0].(map[string]any)["uploader"].(map[string]any)["display_name"] != "Bob" ||
		atts[0].(map[string]any)["content_type"] != "image/png" {
		t.Fatalf("item attachments: %v", atts)
	}
	// The team's own files never show on the requester's page.
	e.upload("carol", items+"/1/attachments", fileUpload{Filename: "internal.png", ContentType: "image/png", Data: pngBytes})
	files := e.must(200, "bob", "GET", "/api/v1/me/requests/"+s.Body["id"].(string), nil).Body["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["filename"] != "receipt.png" {
		t.Fatalf("requester files: %v", files)
	}
	if n := e.countEvents(`action = 'attachment.added' AND source = 'form'`); n != 1 {
		t.Fatalf("attachment events from the form: %d", n)
	}
}
