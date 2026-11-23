// Package safehttp provides an HTTP client for calling user-supplied URLs
// (webhooks) without exposing internal networks (SSRF protection).
//
// The destination IP is checked after DNS resolution, inside the dialer, so a
// hostname that resolves to a public IP at validation time and a private IP at
// connect time (DNS rebinding) is still blocked. Redirects are never followed.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a URL resolves to a non-public address.
var ErrBlockedAddress = errors.New("destination address is not allowed")

// Options configure the client.
type Options struct {
	// AllowPrivate permits loopback/private destinations. Development only.
	AllowPrivate bool
	// RequireHTTPS rejects http:// URLs.
	RequireHTTPS bool
	Timeout      time.Duration
}

// Extra ranges not covered by netip's IsPrivate/IsLoopback/... helpers.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 (could map to private IPv4)
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// IsPublic reports whether addr is a globally routable unicast address.
func IsPublic(addr netip.Addr) bool {
	addr = addr.Unmap() // ::ffff:10.0.0.1 is 10.0.0.1
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// ValidateURL checks scheme, host and credentials before any network I/O.
func ValidateURL(raw string, o Options) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid URL")
	}
	switch {
	case u.Scheme != "https" && u.Scheme != "http":
		return nil, errors.New("URL must use http or https")
	case o.RequireHTTPS && u.Scheme != "https":
		return nil, errors.New("URL must use https")
	case u.Hostname() == "":
		return nil, errors.New("URL must have a host")
	case u.User != nil:
		return nil, errors.New("URL must not contain credentials")
	case u.Fragment != "":
		return nil, errors.New("URL must not contain a fragment")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" && !o.AllowPrivate {
		// Non-standard ports are a common way to reach internal admin services.
		if !strings.HasPrefix(p, "8") || len(p) != 4 { // allow 8000-8999 (common for apps)
			return nil, errors.New("URL port must be 80, 443 or 8000-8999")
		}
	}
	if !o.AllowPrivate {
		if addr, err := netip.ParseAddr(strings.Trim(u.Hostname(), "[]")); err == nil && !IsPublic(addr) {
			return nil, ErrBlockedAddress
		}
		if isObfuscatedIP(u.Hostname()) {
			return nil, ErrBlockedAddress // 0x7f000001, 2130706433, 0177.0.0.1 ...
		}
		if h := strings.ToLower(u.Hostname()); h == "localhost" || strings.HasSuffix(h, ".localhost") ||
			strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".local") {
			return nil, ErrBlockedAddress
		}
	}
	return u, nil
}

var numericLabel = regexp.MustCompile(`^(0[xX][0-9a-fA-F]*|[0-9]+)$`)

// isObfuscatedIP catches hex, octal and integer IPv4 spellings that some
// resolvers and libraries accept but netip does not parse.
func isObfuscatedIP(host string) bool {
	for label := range strings.SplitSeq(host, ".") {
		if !numericLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// NewClient returns a client that enforces Options on every connection.
func NewClient(o Options) *http.Client {
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control runs after DNS resolution, for the exact IP being dialed.
		Control: func(_, address string, _ syscall.RawConn) error {
			if o.AllowPrivate {
				return nil
			}
			ap, err := netip.ParseAddrPort(address)
			if err != nil || !IsPublic(ap.Addr()) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, address)
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                  nil, // an env proxy would bypass the IP check
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        60 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  o.Timeout,
		MaxResponseHeaderBytes: 64 << 10,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   o.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // a redirect could point inside the network
		},
	}
}

// Do validates the request URL and sends it with client.
func Do(ctx context.Context, client *http.Client, o Options, req *http.Request) (*http.Response, error) {
	if _, err := ValidateURL(req.URL.String(), o); err != nil {
		return nil, err
	}
	// URL validated above and every dialed IP is checked in the dialer.
	return client.Do(req.WithContext(ctx)) // #nosec G704 -- this function is the SSRF guard
}
