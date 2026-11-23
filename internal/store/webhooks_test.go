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
