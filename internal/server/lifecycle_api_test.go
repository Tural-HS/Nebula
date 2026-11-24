package server

import "testing"

func TestWriteAllowedWhileArchived(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/projects/ACME/items", true},
		{"HEAD", "/", true},
		{"POST", "/restore", true},
		{"DELETE", "/", true},
		{"DELETE", "", true},
		{"POST", "/evidence/exports", true},
		{"POST", "/projects/ACME/items/12/evidence", true},
		{"POST", "/archive", false},
		{"PATCH", "/", false},
		{"POST", "/projects/ACME/items", false},
		{"POST", "/projects/ACME/restore", false}, // the workspace must be restored first
		{"DELETE", "/projects/ACME", false},
		{"DELETE", "/members/x", false},
		{"POST", "/restore/x", false},
		{"POST", "/projects/ACME/items/12/evidence/x", false},
		{"POST", "/projects/A/B/items/1/evidence", false},
	} {
		if got := writeAllowedWhileArchived(c.method, c.path); got != c.want {
			t.Errorf("%s %q: %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
