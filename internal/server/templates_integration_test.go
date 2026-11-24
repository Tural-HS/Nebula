package server

import (
	"sort"
	"strings"
	"testing"
)

// statusNames returns the status names of the workspace's first item type, in order.
func (e *env) statusNames(user, ws string) (names []string, types []string) {
	e.t.Helper()
	for i, it := range e.must(200, user, "GET", "/api/v1/workspaces/"+ws+"/item-types", nil).Body["item_types"].([]any) {
		m := it.(map[string]any)
		types = append(types, m["key"].(string)+"="+m["name"].(string))
		if i == 0 {
			for _, st := range m["statuses"].([]any) {
				names = append(names, st.(map[string]any)["name"].(string))
			}
		}
	}
	return names, types
}

func TestWorkspaceTemplates(t *testing.T) {
	e := newEnv(t)
	e.must(200, "alice", "GET", "/api/v1/me", nil)

	if r := e.call("alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "bad", "name": "x", "template": "fr"}); r.Status != 422 {
		t.Fatalf("unknown template: %d %s", r.Status, r.Raw)
	}

	az := e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fintez", "name": "FinTez", "template": "az"}).Body
	if az["timezone"] != "Asia/Baku" {
		t.Fatalf("az timezone: %v", az["timezone"])
	}
	names, types := e.statusNames("alice", "fintez")
	if strings.Join(names, ",") != "Görüləcək,İcrada,Yoxlamada,Tamamlandı,Ləğv edildi" {
		t.Fatalf("az statuses: %v", names)
	}
	if strings.Join(types, ",") != "task=Tapşırıq,bug=Xəta" && strings.Join(types, ",") != "bug=Xəta,task=Tapşırıq" {
		t.Fatalf("az item types (keys stay English): %v", types)
	}
	rules := e.must(200, "alice", "GET", "/api/v1/workspaces/fintez/automations", nil).Raw
	if !strings.Contains(rules, "Yaradılanda icraçıya bildir") || !strings.Contains(rules, "Sizə təyin edildi") {
		t.Fatalf("az rules: %s", rules)
	}

	// Turkish and Russian (ADR-0025): same layout, translated names and rules.
	for _, c := range []struct{ tpl, tz, statuses, types, rule string }{
		{"tr", "Europe/Istanbul", "Yapılacak,Devam ediyor,İncelemede,Tamamlandı,İptal edildi", "task=Görev,bug=Hata", "Size atandı"},
		{"ru", "UTC", "К выполнению,В работе,На проверке,Готово,Отменено", "task=Задача,bug=Ошибка", "Вам назначено"},
	} {
		slug := "tpl-" + c.tpl
		ws := e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": slug, "name": c.tpl, "template": c.tpl}).Body
		names, types := e.statusNames("alice", slug)
		sort.Strings(types)
		want := strings.Split(c.types, ",")
		sort.Strings(want)
		if ws["timezone"] != c.tz || strings.Join(names, ",") != c.statuses || strings.Join(types, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: %v %v %v", c.tpl, ws["timezone"], names, types)
		}
		if rules := e.must(200, "alice", "GET", "/api/v1/workspaces/"+slug+"/automations", nil).Raw; !strings.Contains(rules, c.rule) {
			t.Fatalf("%s rules: %s", c.tpl, rules)
		}
	}

	// Default and explicit English keep the historical template.
	for slug, body := range map[string]map[string]any{
		"plain": {"slug": "plain", "name": "Plain"},
		"eng":   {"slug": "eng", "name": "Eng", "template": "en"},
	} {
		ws := e.must(201, "alice", "POST", "/api/v1/workspaces", body).Body
		names, _ := e.statusNames("alice", slug)
		if ws["timezone"] != "UTC" || strings.Join(names, ",") != "To Do,In Progress,Done,Canceled" {
			t.Fatalf("%s: %v %v", slug, ws["timezone"], names)
		}
	}
}

func TestProjectTemplate(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member; project ACME
	carol := e.userID("carol")

	// A private department project with a member, a recurring item and views.
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "HR", "name": "HR şöbəsi", "visibility": "private"})
	hr := projectsPath + "/HR"
	e.must(204, "alice", "PUT", hr+"/members/"+carol, nil)
	e.must(201, "alice", "POST", hr+"/recurrences", map[string]any{
		"title": "Aylıq əmək haqqı hesabatı", "freq": "monthly", "every": 1, "month_day": 25,
		"start_date": "2026-11-01", "assignee_id": carol,
	})
	e.must(201, "alice", "POST", hr+"/views", map[string]any{"name": "Gecikənlər", "kind": "table", "shared": true,
		"config": map[string]any{"filters": map[string]any{"due": "overdue"}}})
	e.must(201, "alice", "POST", hr+"/views", map[string]any{"name": "Alicenin şəxsi", "kind": "table", "config": map[string]any{}})
	e.must(201, "alice", "POST", hr+"/items", map[string]any{"title": "Köhnə iş"})

	if r := e.call("carol", "POST", projectsPath, map[string]any{"key": "FIN", "name": "x", "template": map[string]any{"from": "HR"}}); r.Status != 403 {
		t.Fatalf("member uses a template: %d", r.Status)
	}
	for _, tpl := range []map[string]any{{"from": ""}, {"from": "NOPE"}} {
		if r := e.call("alice", "POST", projectsPath, map[string]any{"key": "FIN", "name": "x", "template": tpl}); r.Status != 422 {
			t.Fatalf("template %v: %d %s", tpl, r.Status, r.Raw)
		}
	}
	// A failed copy leaves no half-created project behind.
	if r := e.call("alice", "GET", projectsPath+"/FIN", nil); r.Status != 404 {
		t.Fatalf("project left after failed template: %d", r.Status)
	}

	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "FIN", "name": "Maliyyə şöbəsi", "visibility": "private",
		"template": map[string]any{"from": "hr", "members": true, "recurrences": true, "views": true}})
	fin := projectsPath + "/FIN"

	if !e.visibleKeys("carol")["FIN"] {
		t.Fatal("copied member cannot see the new project")
	}
	recs := e.must(200, "alice", "GET", fin+"/recurrences", nil).Body["recurrences"].([]any)
	if len(recs) != 1 {
		t.Fatalf("recurrences: %v", recs)
	}
	rec := recs[0].(map[string]any)
	if rec["title"] != "Aylıq əmək haqqı hesabatı" || rec["assignee_id"] != carol || rec["next_due"] == nil {
		t.Fatalf("copied recurrence: %v", rec)
	}
	views := e.must(200, "alice", "GET", fin+"/views", nil).Raw
	if !strings.Contains(views, "Gecikənlər") || strings.Contains(views, "Alicenin şəxsi") {
		t.Fatalf("only shared views are copied: %s", views)
	}
	if n := len(e.must(200, "alice", "GET", fin+"/items", nil).Body["items"].([]any)); n != 0 {
		t.Fatalf("items must not be copied: %d", n)
	}
	// The source is untouched and the copy is independent.
	if n := len(e.must(200, "alice", "GET", hr+"/recurrences", nil).Body["recurrences"].([]any)); n != 1 {
		t.Fatalf("source recurrences: %d", n)
	}
	audit := e.must(200, "alice", "GET", wsBase+"/audit-log", nil).Raw
	if !strings.Contains(audit, `"from":"HR"`) {
		t.Fatalf("project.created lacks the template: %s", audit)
	}

	// Without options nothing but the project itself is created.
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "OPS", "name": "Əməliyyat", "visibility": "private",
		"template": map[string]any{"from": "HR"}})
	if e.visibleKeys("carol")["OPS"] {
		t.Fatal("members copied without being asked")
	}
}
