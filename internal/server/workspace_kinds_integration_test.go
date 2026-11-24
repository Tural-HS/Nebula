package server

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/store"
)

// kindWorkspace is what a kind installed, read back through the public API.
type kindWorkspace struct {
	statuses []string          // names, in order
	statusID map[string]string // name -> id
	types    []string          // key=name, sorted by key
	fields   map[string]map[string]any
	bound    map[string][]string // item type key -> field keys
	policies []any
	projects map[string]map[string]any
	views    map[string]map[string]any // name -> view (first project)
	forms    []map[string]any
	calendar []any
}

func (e *env) readKind(ws, project string) kindWorkspace {
	e.t.Helper()
	base := "/api/v1/workspaces/" + ws
	k := kindWorkspace{statusID: map[string]string{}, fields: map[string]map[string]any{}, bound: map[string][]string{},
		projects: map[string]map[string]any{}, views: map[string]map[string]any{}}
	for _, it := range e.must(200, "alice", "GET", base+"/item-types", nil).Body["item_types"].([]any) {
		m := it.(map[string]any)
		k.types = append(k.types, m["key"].(string)+"="+m["name"].(string))
		if k.statuses == nil {
			for _, st := range m["statuses"].([]any) {
				s := st.(map[string]any)
				k.statuses = append(k.statuses, s["name"].(string))
				k.statusID[s["name"].(string)] = s["id"].(string)
			}
		}
		for _, f := range m["fields"].([]any) {
			k.bound[m["key"].(string)] = append(k.bound[m["key"].(string)], f.(map[string]any)["key"].(string))
		}
	}
	for _, f := range e.must(200, "alice", "GET", base+"/fields", nil).Body["fields"].([]any) {
		m := f.(map[string]any)
		k.fields[m["key"].(string)] = m
	}
	for _, wf := range e.must(200, "alice", "GET", base+"/workflows", nil).Body["workflows"].([]any) {
		k.policies = append(k.policies, wf.(map[string]any)["approval_policies"].([]any)...)
	}
	for _, p := range e.must(200, "alice", "GET", base+"/projects", nil).Body["projects"].([]any) {
		m := p.(map[string]any)
		k.projects[m["key"].(string)] = m
	}
	if project != "" {
		pp := base + "/projects/" + project
		for _, v := range e.must(200, "alice", "GET", pp+"/views", nil).Body["views"].([]any) {
			m := v.(map[string]any)
			k.views[m["name"].(string)] = m
		}
		for _, f := range e.must(200, "alice", "GET", pp+"/forms", nil).Body["forms"].([]any) {
			k.forms = append(k.forms, f.(map[string]any))
		}
	}
	year := time.Now().In(time.FixedZone("Baku", 4*3600)).Year()
	k.calendar = e.must(200, "alice", "GET", fmt.Sprintf("%s/calendar?year=%d", base, year), nil).Body["days"].([]any)
	return k
}

func expectJoined(t *testing.T, what string, got []string, want string) {
	t.Helper()
	if strings.Join(got, ",") != want {
		t.Fatalf("%s: %q, want %q", what, strings.Join(got, ","), want)
	}
}

// wantAzCalendar is what the az preset adds for the current year: the full
// official calendar when shipped, else the fixed-date holidays.
func wantAzCalendar() int {
	year := time.Now().In(time.FixedZone("Baku", 4*3600)).Year()
	if slices.Contains(store.AzOfficialYears(), year) {
		return map[int]int{2026: 25}[year]
	}
	return 11
}

func TestWorkspaceKindFinance(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)
	e.must(200, "bob", "GET", "/api/v1/me", nil)

	ws := e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fin-team", "name": "Maliyyə komandası", "template": "az", "kind": "finance"}).Body
	if ws["timezone"] != "Asia/Baku" {
		t.Fatalf("timezone: %v", ws["timezone"])
	}
	k := e.readKind("fin-team", "FIN")
	expectJoined(t, "statuses", k.statuses, "Hazırlanır,Yoxlamada,Təsdiq gözləyir,Ödənildi,Ləğv edildi")
	expectJoined(t, "types", k.types, "invoice=Hesab-faktura,payment=Ödəniş,report=Hesabat")
	for key, want := range map[string]string{"amount": "number", "vat_rate": "number", "vat": "formula", "counterparty": "text", "payment_date": "date"} {
		if f := k.fields[key]; f == nil || f["type"] != want {
			t.Fatalf("field %s: %v", key, f)
		}
	}
	if k.fields["amount"]["name"] != "Məbləğ" || k.fields["vat"]["name"] != "ƏDV" || k.fields["counterparty"]["name"] != "Kontragent" {
		t.Fatalf("field names: %v %v", k.fields["amount"], k.fields["vat"])
	}
	expr := k.fields["vat"]["options"].(map[string]any)["formula"].(map[string]any)["expression"]
	if expr != "round(amount * coalesce(vat_rate, 18) / 100, 2)" {
		t.Fatalf("vat formula: %v", expr)
	}
	expectJoined(t, "payment fields", k.bound["payment"], "amount,vat_rate,vat,counterparty,payment_date")
	if len(k.bound["report"]) != 0 {
		t.Fatalf("report fields: %v", k.bound["report"])
	}

	if len(k.policies) != 1 {
		t.Fatalf("policies: %v", k.policies)
	}
	pol := k.policies[0].(map[string]any)
	roles := fmt.Sprint(pol["approver_roles"])
	if pol["status_id"] != k.statusID["Ödənildi"] || pol["min_approvals"] != float64(1) || roles != "[admin owner]" {
		t.Fatalf("policy: %v", pol)
	}

	if p := k.projects["FIN"]; p == nil || p["name"] != "Maliyyə" || p["visibility"] != "workspace" || len(k.projects) != 1 {
		t.Fatalf("projects: %v", k.projects)
	}
	overdue, awaiting := k.views["Gecikənlər"], k.views["Təsdiq gözləyir"]
	if overdue == nil || awaiting == nil || len(k.views) != 2 || overdue["shared"] != true || awaiting["shared"] != true {
		t.Fatalf("views: %v", k.views)
	}
	if f := overdue["config"].(map[string]any)["filters"].(map[string]any); f["due"] != "overdue" {
		t.Fatalf("overdue filters: %v", f)
	}
	if f := awaiting["config"].(map[string]any)["filters"].(map[string]any); fmt.Sprint(f["status"]) != "["+k.statusID["Təsdiq gözləyir"]+"]" {
		t.Fatalf("awaiting filters: %v", f)
	}

	if len(k.forms) != 1 {
		t.Fatalf("forms: %v", k.forms)
	}
	form := k.forms[0]
	if form["title"] != "Ödəniş tələbi" || form["slug"] != "fin-team-payment-request" || form["audience"] != "members" ||
		form["enabled"] != true || form["item_type"].(map[string]any)["key"] != "payment" {
		t.Fatalf("form: %v", form)
	}
	if n := len(k.calendar); n != wantAzCalendar() {
		t.Fatalf("calendar days: %d, want %d", n, wantAzCalendar())
	}

	// Each step left its own event, after workspace.created.
	audit := e.must(200, "alice", "GET", "/api/v1/workspaces/fin-team/audit-log?limit=100", nil).Raw
	for _, a := range []string{`"workspace.created"`, `"kind":"finance"`, `"field.created"`, `"item_type.field_bound"`,
		`"approval_policy.set"`, `"calendar.preset_applied"`, `"project.created"`, `"view.created"`, `"form.created"`} {
		if !strings.Contains(audit, a) {
			t.Errorf("audit lacks %s", a)
		}
	}

	// It works end to end: a member files a payment request, VAT is computed
	// at the default rate, and "Ödənildi" needs an approval.
	e.must(201, "alice", "POST", "/api/v1/workspaces/fin-team/members", map[string]any{"email": "bob@example.com", "role": "member"})
	sub := e.must(201, "bob", "POST", "/api/v1/forms/fin-team-payment-request/submissions", map[string]any{"values": map[string]any{
		"counterparty": "Test Vendor MMC", "amount": 1000, "payment_date": "2026-12-15",
	}}).Body
	if sub["key"] != "FIN-1" || sub["title"] != "Ödəniş: Test Vendor MMC" {
		t.Fatalf("submission: %v", sub)
	}
	item := e.must(200, "alice", "GET", "/api/v1/workspaces/fin-team/projects/FIN/items/1", nil).Body
	if item["fields"].(map[string]any)["vat"] != float64(180) {
		t.Fatalf("vat: %v", item["fields"])
	}
	if r := e.call("alice", "PATCH", "/api/v1/workspaces/fin-team/projects/FIN/items/1",
		map[string]any{"version": item["version"], "status_id": k.statusID["Ödənildi"]}); r.Status != 409 || errCode(r) != "approval_required" {
		t.Fatalf("paid without approval: %d %s", r.Status, r.Raw)
	}
}

func TestWorkspaceKindIT(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "it-team", "name": "IT", "template": "en", "kind": "it"})
	k := e.readKind("it-team", "IT")
	expectJoined(t, "statuses", k.statuses, "Backlog,In Progress,Review,Done,Canceled")
	expectJoined(t, "types", k.types, "bug=Bug,request=Request,task=Task")
	prio := k.fields["priority"]
	if prio == nil || prio["type"] != "select" || k.fields["component"]["type"] != "text" {
		t.Fatalf("fields: %v", k.fields)
	}
	var labels []string
	for _, c := range prio["options"].(map[string]any)["choices"].([]any) {
		labels = append(labels, c.(map[string]any)["label"].(string))
	}
	expectJoined(t, "priority", labels, "Low,Medium,High,Critical")
	critical := prio["options"].(map[string]any)["choices"].([]any)[3].(map[string]any)["id"]
	for _, typ := range []string{"bug", "task", "request"} {
		expectJoined(t, typ+" fields", k.bound[typ], "priority,component")
	}
	if len(k.policies) != 0 {
		t.Fatalf("it has no approval policy: %v", k.policies)
	}
	mine, crit := k.views["My work"], k.views["Critical"]
	if mine == nil || crit == nil {
		t.Fatalf("views: %v", k.views)
	}
	if f := mine["config"].(map[string]any)["filters"].(map[string]any); f["assignee"] != "me" {
		t.Fatalf("my work: %v", f)
	}
	if f := crit["config"].(map[string]any)["filters"].(map[string]any)["fields"].(map[string]any); f["priority"] != critical {
		t.Fatalf("critical: %v (want %v)", f, critical)
	}
	if len(k.forms) != 1 || k.forms[0]["title"] != "IT access request" || k.forms[0]["item_type"].(map[string]any)["key"] != "request" {
		t.Fatalf("forms: %v", k.forms)
	}
	// Not an az workspace: the calendar stays empty.
	if len(k.calendar) != 0 {
		t.Fatalf("calendar: %v", k.calendar)
	}
}

func TestWorkspaceKindHR(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "hr-team", "name": "Kadrlar", "template": "az", "kind": "hr"})
	k := e.readKind("hr-team", "HR")
	expectJoined(t, "statuses", k.statuses, "Yeni,İcrada,Tamamlandı,Ləğv edildi")
	expectJoined(t, "types", k.types, "leave=Məzuniyyət,onboarding=İşə qəbul,task=Tapşırıq")
	expectJoined(t, "leave fields", k.bound["leave"], "employee,leave_type,start_date,end_date")
	// Personnel data stays in a private project (ADR-0013).
	if p := k.projects["HR"]; p == nil || p["visibility"] != "private" || p["name"] != "Kadrlar" {
		t.Fatalf("project: %v", k.projects)
	}
	var titles []string
	for _, f := range k.forms {
		titles = append(titles, f["title"].(string))
	}
	expectJoined(t, "forms", titles, "Məzuniyyət sorğusu,Yeni əməkdaş")
	if k.views["Mənim işlərim"] == nil || k.views["Gecikənlər"] == nil {
		t.Fatalf("views: %v", k.views)
	}
	if len(k.policies) != 0 || len(k.calendar) != wantAzCalendar() {
		t.Fatalf("policies %v, calendar %d", k.policies, len(k.calendar))
	}
}

// TestWorkspaceKindLanguages: language × kind, general unchanged, bad input,
// and form slugs that stay free across workspaces.
func TestWorkspaceKindLanguages(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)

	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "ru-fin", "name": "Финансы", "template": "ru", "kind": "finance"})
	k := e.readKind("ru-fin", "FIN")
	expectJoined(t, "ru statuses", k.statuses, "Подготовка,На проверке,Ожидает утверждения,Оплачено,Отменено")
	expectJoined(t, "ru types", k.types, "invoice=Счёт-фактура,payment=Платёж,report=Отчёт")
	if k.fields["amount"]["name"] != "Сумма" || k.projects["FIN"]["name"] != "Финансы" || k.views["Просроченные"] == nil ||
		len(k.forms) != 1 || k.forms[0]["title"] != "Заявка на оплату" {
		t.Fatalf("ru names: %v %v %v %v", k.fields["amount"], k.projects, k.views, k.forms)
	}
	// Outside az the rate is not assumed, and no Azerbaijani calendar.
	if expr := k.fields["vat"]["options"].(map[string]any)["formula"].(map[string]any)["expression"]; expr != "round(amount * vat_rate / 100, 2)" {
		t.Fatalf("ru vat formula: %v", expr)
	}
	if len(k.calendar) != 0 {
		t.Fatalf("ru calendar: %d", len(k.calendar))
	}

	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "tr-it", "name": "BT", "template": "tr", "kind": "it"})
	expectJoined(t, "tr statuses", e.readKind("tr-it", "").statuses, "Bekleyen,Devam ediyor,İncelemede,Tamamlandı,İptal edildi")

	// general (explicit or omitted) is exactly the language template.
	for slug, body := range map[string]map[string]any{
		"gen-a": {"slug": "gen-a", "name": "A", "template": "az", "kind": "general"},
		"gen-b": {"slug": "gen-b", "name": "B", "template": "az"},
	} {
		e.must(201, "alice", "POST", "/api/v1/workspaces", body)
		k := e.readKind(slug, "")
		expectJoined(t, slug+" statuses", k.statuses, "Görüləcək,İcrada,Yoxlamada,Tamamlandı,Ləğv edildi")
		if len(k.fields) != 0 || len(k.projects) != 0 || len(k.policies) != 0 || len(k.calendar) != 0 {
			t.Fatalf("%s is not plain: %v %v %v %d", slug, k.fields, k.projects, k.policies, len(k.calendar))
		}
	}

	if r := e.call("alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "bad-kind", "name": "x", "kind": "sales"}); r.Status != 422 || errField(r) != "kind" {
		t.Fatalf("unknown kind: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "GET", "/api/v1/workspaces/bad-kind", nil); r.Status != 404 {
		t.Fatalf("refused workspace exists: %d", r.Status)
	}

	// A form slug taken by another workspace gets a random tail instead of
	// failing the creation.
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fin", "name": "F1", "kind": "finance"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/fin/projects/FIN/forms", map[string]any{
		"slug": "fin2-payment-request", "title": "Squatter", "item_type": "report", "title_template": "x", "fields": []any{},
	})
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fin2", "name": "F2", "kind": "finance"})
	forms := e.readKind("fin2", "FIN").forms
	if len(forms) != 1 || !strings.HasPrefix(forms[0]["slug"].(string), "fin2-payment-request-") {
		t.Fatalf("fallback slug: %v", forms)
	}
	long := strings.Repeat("a", 63)
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": long, "name": "Long", "kind": "hr"})
	for _, f := range e.readKind(long, "HR").forms {
		if s := f["slug"].(string); len(s) > 63 {
			t.Fatalf("slug too long: %s", s)
		}
	}
}
