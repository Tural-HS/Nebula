package server

import "testing"

func TestMatchTokenRoute(t *testing.T) {
	ws := "/workspaces/acme"
	it := ws + "/projects/ACME/items"
	for _, c := range []struct {
		method, path, scope string
		ok                  bool
	}{
		{"GET", "/me", "", true},
		{"HEAD", ws, "", true},
		{"GET", ws + "/", "", true},
		{"GET", it, "items:read", true},
		{"GET", it + "/12/comments", "items:read", true},
		{"POST", it, "items:write", true},
		{"PATCH", it + "/12", "items:write", true},
		{"POST", it + "/12/comments", "comments:write", true},
		{"GET", it + "/export", "export:read", true},
		{"GET", ws + "/reports/workload.csv", "reports:read", true},
		// Default deny.
		{"DELETE", it + "/12", "", false},
		{"PATCH", it + "/12/comments/1", "", false},
		{"GET", ws + "/service-tokens", "", false},
		{"POST", ws + "/archive", "", false},
		{"GET", ws + "/members", "", false},
		{"GET", ws + "/email-settings", "", false},
		{"GET", ws + "/projects/ACME/items/12/attachments", "", false},
		{"GET", "/workspaces", "", false},
		{"GET", it + "/12/../../../../service-tokens", "", false},
		{"PUT", it, "", false},
	} {
		rt, ok := matchTokenRoute(c.method, c.path)
		if ok != c.ok || (ok && rt.scope != c.scope) {
			t.Errorf("%s %s: ok=%v scope=%q", c.method, c.path, ok, rt.scope)
		}
	}
}
