package safehttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestIsPublic(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", // cloud metadata
		"0.0.0.0", "100.64.0.1", "::1", "fe80::1", "fc00::1", "::ffff:10.0.0.1", "::ffff:127.0.0.1",
		"224.0.0.1", "255.255.255.255", "64:ff9b::a00:1", "198.51.100.7", "2001:db8::1",
	}
	for _, s := range blocked {
		if IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "94.20.59.81"} {
		if !IsPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
}

func TestValidateURL(t *testing.T) {
	strict := Options{RequireHTTPS: true}
	bad := []string{
		"ftp://example.com", "javascript:alert(1)", "http://example.com/hook", "https:///nohost",
		"https://user:pass@example.com/", "https://127.0.0.1/", "https://[::1]/", "https://169.254.169.254/latest/meta-data",
		"https://localhost/", "https://api.localhost/", "https://db.internal/", "https://printer.local/",
		"https://example.com:22/", "https://example.com:6379/", "https://example.com/#frag", "https://0x7f000001/", "https://2130706433/", "https://0177.0.0.1/", "https://127.1/",
	}
	for _, u := range bad {
		if _, err := ValidateURL(u, strict); err == nil {
			t.Errorf("%s must be rejected", u)
		}
	}
	for _, u := range []string{"https://hooks.example.com/nebula", "https://example.com:8443/x", "https://example.com:443/"} {
		if _, err := ValidateURL(u, strict); err != nil {
			t.Errorf("%s rejected: %v", u, err)
		}
	}
	if _, err := ValidateURL("http://127.0.0.1:9999/", Options{AllowPrivate: true}); err != nil {
		t.Errorf("dev mode must allow private: %v", err)
	}
}

func TestDialerBlocksPrivateAfterResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()

	// The dialer check must hold even if URL validation is bypassed.
	client := NewClient(Options{})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback dial must be blocked, got %v", err)
	}

	client = NewClient(Options{AllowPrivate: true})
	req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err = Do(t.Context(), client, Options{AllowPrivate: true}, req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("redirect target was called")
	}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()
	client := NewClient(Options{AllowPrivate: true})
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
