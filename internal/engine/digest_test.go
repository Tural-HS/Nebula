package engine

import (
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/store"
)

func TestFilterDigest(t *testing.T) {
	payload := map[string]any{
		"date": "2026-11-23", "total": 120.0, "truncated": true,
		"by_project": map[string]any{"HR": 3.0, "KR": 117.0},
		"items": []any{
			map[string]any{"key": "HR-1", "project": "HR"},
			map[string]any{"key": "KR-9", "project": "KR"},
			map[string]any{"key": "HR-4", "project": "HR"},
		},
	}
	out, ok := FilterDigest(payload, map[string]bool{"HR": true})
	if !ok {
		t.Fatal("HR has overdue items")
	}
	items := out["items"].([]any)
	if len(items) != 2 || out["total"] != 3 || out["truncated"] != true {
		t.Fatalf("filtered: %v", out)
	}
	if bp := out["by_project"].(map[string]any); len(bp) != 1 || bp["HR"] != 3.0 {
		t.Fatalf("by_project leaks other projects: %v", bp)
	}
	if payload["total"] != 120.0 || len(payload["items"].([]any)) != 3 {
		t.Fatal("input payload was modified")
	}
	if _, ok := FilterDigest(payload, map[string]bool{"SEC": true}); ok {
		t.Fatal("a webhook without overdue items in its projects gets no digest")
	}
}

func TestFilterDigestDueToday(t *testing.T) {
	payload := map[string]any{
		"total": 0.0, "by_project": map[string]any{}, "items": []any{},
		"today_total": 2.0, "today_by_project": map[string]any{"YIG": 1.0, "HR": 1.0},
		"today": []any{
			map[string]any{"key": "YIG-7", "project": "YIG", "title": "Əli", "assignee_name": "Elmar"},
			map[string]any{"key": "HR-2", "project": "HR", "title": "Məzuniyyət"},
		},
	}
	out, ok := FilterDigest(payload, map[string]bool{"YIG": true})
	if !ok {
		t.Fatal("YIG has an item due today")
	}
	if today := out["today"].([]any); len(today) != 1 || out["today_total"] != 1 {
		t.Fatalf("today filtered: %v", out)
	}
	text := ChatContext{}.overdueDigest(store.Event{Action: store.ActionOverdueDigest, Payload: out})
	if !strings.Contains(text, "📅 Bu gün son tarix (1):") || !strings.Contains(text, "YIG-7 Əli — Elmar") || strings.Contains(text, "Gecikmədə") {
		t.Fatalf("rendered: %q", text)
	}
	if _, ok := FilterDigest(payload, map[string]bool{"SEC": true}); ok {
		t.Fatal("nothing due or overdue in SEC: no digest")
	}
	// Both lists: today first, then overdue.
	payload["total"], payload["by_project"] = 1.0, map[string]any{"YIG": 1.0}
	payload["items"] = []any{map[string]any{"key": "YIG-1", "project": "YIG", "title": "Köhnə", "days_overdue": 3.0, "due_date": "2026-11-23"}}
	out, _ = FilterDigest(payload, map[string]bool{"YIG": true})
	text = ChatContext{}.overdueDigest(store.Event{Action: store.ActionOverdueDigest, Payload: out})
	if i, j := strings.Index(text, "Bu gün"), strings.Index(text, "Gecikmədə olanlar (1)"); i < 0 || j < i || !strings.Contains(text, "3 gün gecikir") {
		t.Fatalf("both lists: %q", text)
	}
}

func TestRenderPeriodClosed(t *testing.T) {
	ev := store.Event{Action: "period.closed", Payload: map[string]any{
		"project_name": "Müştəri *Aylıq* Hesabat", "period": "2026-11", "items": 48.0, "changed": 45.0, "failed": 1.0,
	}}
	got := RenderChat(ev, ChatContext{})
	want := "📅 Müştəri \\*Aylıq\\* Hesabat: Noyabr 2026 bağlandı — 48 tapşırıq tarixçəyə yazıldı, 45 yeniləndi; 1 tapşırıq yenilənmədi, əl ilə yoxlayın — Sistem"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := azMonth("bad"); got != "bad" {
		t.Fatalf("azMonth(bad) = %q", got)
	}
}

func TestIsCycleReset(t *testing.T) {
	if !isCycleReset(store.Event{Action: "item.updated", Payload: map[string]any{"cycle": map[string]any{"period": "2026-11"}}}) {
		t.Fatal("a monthly reset is not recognised")
	}
	if isCycleReset(store.Event{Action: "item.updated", Payload: map[string]any{}}) || isCycleReset(store.Event{Action: "period.closed", Payload: map[string]any{"cycle": 1}}) {
		t.Fatal("other events are skipped too")
	}
}
