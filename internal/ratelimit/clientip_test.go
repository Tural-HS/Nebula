package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	good := map[string]int{
		"":                                    0,
		" , ":                                 0,
		"10.0.0.0/8":                          1,
		"10.0.0.0/8, 192.168.1.10":            2,
		"fd00::/8,2001:db8::1, 172.16.0.0/12": 3,
	}
	for in, n := range good {
		got, err := ParseTrustedProxies(in)
		if err != nil || len(got) != n {
			t.Errorf("%q: got %v, %v; want %d prefixes", in, got, err, n)
		}
	}
	for _, in := range []string{"10.0.0.0/33", "10.0.0.1/8", "nope", "10.0.0.0/8,,bad", "::ffff:10.0.0.0/104", "fe80::1%eth0", "10.0.0.0/-1"} {
		if _, err := ParseTrustedProxies(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestClientIPResolver(t *testing.T) {
	trusted, err := ParseTrustedProxies("10.0.0.0/8, 2001:db8:ffff::/48, 192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	withProxies := NewClientIPResolver(trusted)
	noProxies := NewClientIPResolver(nil)

	tests := []struct {
		name   string
		res    *ClientIPResolver
		remote string
		xff    []string
		want   string
	}{
		{"no proxies: peer only", noProxies, "203.0.113.7:1", nil, "203.0.113.7"},
		{"no proxies: XFF ignored", noProxies, "203.0.113.7:1", []string{"198.51.100.1"}, "203.0.113.7"},
		{"untrusted peer: spoofed XFF ignored", withProxies, "203.0.113.7:1", []string{"198.51.100.1"}, "203.0.113.7"},
		{"untrusted peer: spoofed trusted XFF ignored", withProxies, "203.0.113.7:1", []string{"10.1.1.1"}, "203.0.113.7"},
		{"trusted peer, single hop", withProxies, "10.0.0.5:443", []string{"198.51.100.1"}, "198.51.100.1"},
		{"trusted peer, client prepends fake", withProxies, "10.0.0.5:443", []string{"1.1.1.1, 198.51.100.1"}, "198.51.100.1"},
		{"chain of trusted proxies", withProxies, "10.0.0.5:443", []string{"198.51.100.1, 10.2.0.1, 192.0.2.1"}, "198.51.100.1"},
		{"multiple header lines", withProxies, "10.0.0.5:443", []string{"6.6.6.6", "198.51.100.1, 10.2.0.1"}, "198.51.100.1"},
		{"client spoofs a trusted address on the left", withProxies, "10.0.0.5:443", []string{"10.9.9.9, 198.51.100.1"}, "198.51.100.1"},
		{"hop with port", withProxies, "10.0.0.5:443", []string{"198.51.100.1:5555"}, "198.51.100.1"},
		{"ipv6 hop with port", withProxies, "[2001:db8:ffff::2]:443", []string{"[2001:db8::7]:443"}, "2001:db8::7"},
		{"ipv6 hop", withProxies, "[2001:db8:ffff::2]:443", []string{"2001:db8::7"}, "2001:db8::7"},
		{"ipv4-mapped peer is trusted", withProxies, "[::ffff:10.0.0.5]:443", []string{"198.51.100.1"}, "198.51.100.1"},
		{"ipv4-mapped hop unmapped", withProxies, "10.0.0.5:443", []string{"::ffff:198.51.100.1"}, "198.51.100.1"},
		{"trusted peer, no XFF", withProxies, "10.0.0.5:443", nil, "10.0.0.5"},
		{"trusted peer, empty XFF", withProxies, "10.0.0.5:443", []string{""}, "10.0.0.5"},
		{"all hops trusted", withProxies, "10.0.0.5:443", []string{"10.0.0.9, 192.0.2.1"}, "10.0.0.5"},
		{"malformed hop", withProxies, "10.0.0.5:443", []string{"198.51.100.1, garbage"}, "10.0.0.5"},
		{"malformed left of client is not reached", withProxies, "10.0.0.5:443", []string{"garbage, 198.51.100.1"}, "198.51.100.1"},
		{"empty element", withProxies, "10.0.0.5:443", []string{"198.51.100.1,,"}, "10.0.0.5"},
		{"zoned hop rejected", withProxies, "10.0.0.5:443", []string{"fe80::1%eth0"}, "10.0.0.5"},
		{"obfuscated hop rejected", withProxies, "10.0.0.5:443", []string{"unknown"}, "10.0.0.5"},
		{"weird remote addr", withProxies, "weird", []string{"198.51.100.1"}, "weird"},
		{"nil resolver", nil, "203.0.113.7:1", []string{"198.51.100.1"}, "203.0.113.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := tc.res.ClientIP(r); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClientIPResolverBoundsHeader(t *testing.T) {
	trusted, _ := ParseTrustedProxies("10.0.0.0/8")
	res := NewClientIPResolver(trusted)
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:443"
	long := "198.51.100.1"
	for range 200 {
		long += ", 10.0.0.1"
	}
	r.Header.Set("X-Forwarded-For", long)
	if got := res.ClientIP(r); got != "10.0.0.5" {
		t.Fatalf("oversized header must fall back to the peer, got %s", got)
	}
}
