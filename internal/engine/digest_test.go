package engine

import "testing"

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
