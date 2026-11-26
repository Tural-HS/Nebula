package store

import "testing"

func TestMaskWebhookURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://chat.example.com/hooks/abcdefghijklmnopqrstuvwxyz": "https://chat.example.com/…wxyz",
		"https://chat.example.com:8443/hooks/abcdefghijklmn1234":    "https://chat.example.com:8443/…1234",
		"https://chat.example.com/short":                            "https://chat.example.com/…",
		"::not a url":                                               "https://invalid/…",
	} {
		if got := MaskWebhookURL(in); got != want {
			t.Errorf("MaskWebhookURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeFormat(t *testing.T) {
	for in, want := range map[string]string{"": "nebula", "nebula": "nebula", " slack ": "slack"} {
		if got, err := normalizeFormat(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if _, err := normalizeFormat("mattermost"); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestMatchEventOfStatusChanged(t *testing.T) {
	status := Event{Action: "item.updated", Payload: map[string]any{"changes": map[string]any{"status_id": map[string]any{}}}}
	field := Event{Action: "item.updated", Payload: map[string]any{"changes": map[string]any{"fields.phone": map[string]any{}}}}
	only := []string{EventStatusChanged, "workspace.overdue_digest"}
	if !MatchEventOf(only, status) {
		t.Error("status change not matched")
	}
	if MatchEventOf(only, field) {
		t.Error("field edit matched item.status_changed")
	}
	if !MatchEventOf([]string{"item.*"}, field) {
		t.Error("item.* lost field edits")
	}
	if !MatchEventOf(only, Event{Action: "workspace.overdue_digest"}) {
		t.Error("digest not matched")
	}
	if _, err := normalizeEvents([]string{EventStatusChanged}); err != nil {
		t.Errorf("pattern rejected: %v", err)
	}
}
