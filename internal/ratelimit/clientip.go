package ratelimit

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedHops bounds the X-Forwarded-For walk so a huge header cannot
// cost more than a few microseconds.
const maxForwardedHops = 32

// ParseTrustedProxies parses a comma-separated list of CIDRs (or single IPs)
// of reverse proxies whose X-Forwarded-For header may be believed. Prefixes
// with host bits set (10.0.0.1/8) are rejected as likely typos.
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var p netip.Prefix
		if strings.Contains(part, "/") {
			var err error
			if p, err = netip.ParsePrefix(part); err != nil {
				return nil, fmt.Errorf("invalid CIDR %q", part)
			}
			if p.Addr().Is4In6() {
				return nil, fmt.Errorf("invalid CIDR %q: use the plain IPv4 form", part)
			}
			if p.Masked() != p {
				return nil, fmt.Errorf("invalid CIDR %q: host bits set (did you mean %s?)", part, p.Masked())
			}
		} else {
			a, err := netip.ParseAddr(part)
			if err != nil || a.Zone() != "" {
				return nil, fmt.Errorf("invalid IP %q", part)
			}
			a = a.Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		}
		out = append(out, p)
	}
	return out, nil
}

// ClientIPResolver determines the client address of a request. Without
// trusted proxies it returns the TCP peer and ignores X-Forwarded-For, which
// any client can forge. With trusted proxies it believes X-Forwarded-For only
// when the TCP peer is one of them, and then only as far back as the chain of
// trusted hops goes: the right-most address that is not a trusted proxy is the
// client. Everything left of it was supplied by the client and is ignored.
type ClientIPResolver struct {
	trusted []netip.Prefix
}

// NewClientIPResolver returns a resolver trusting the given proxy networks.
func NewClientIPResolver(trusted []netip.Prefix) *ClientIPResolver {
	return &ClientIPResolver{trusted: trusted}
}

func (c *ClientIPResolver) isTrusted(a netip.Addr) bool {
	for _, p := range c.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

var errMalformed = errors.New("malformed X-Forwarded-For")

// ClientIP returns the resolved client address as a string (the rate-limit key).
func (c *ClientIPResolver) ClientIP(r *http.Request) string {
	peer, err := parseHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // unusual transports (tests, unix sockets): still a stable key
	}
	if c == nil || len(c.trusted) == 0 || !c.isTrusted(peer) {
		return peer.String()
	}
	client, err := c.fromForwarded(r.Header.Values("X-Forwarded-For"))
	if err != nil || !client.IsValid() {
		return peer.String()
	}
	return client.String()
}

// fromForwarded walks the X-Forwarded-For chain right to left. Multiple
// header lines are one list in order (RFC 9110 field combination).
func (c *ClientIPResolver) fromForwarded(values []string) (netip.Addr, error) {
	var hops []string
	for _, v := range values {
		for h := range strings.SplitSeq(v, ",") {
			hops = append(hops, strings.TrimSpace(h))
			if len(hops) > 4*maxForwardedHops {
				return netip.Addr{}, errMalformed
			}
		}
	}
	for i, n := len(hops)-1, 0; i >= 0 && n < maxForwardedHops; i, n = i-1, n+1 {
		a, err := parseHop(hops[i])
		if err != nil {
			return netip.Addr{}, errMalformed
		}
		if !c.isTrusted(a) {
			return a, nil
		}
	}
	// Every hop we looked at is a trusted proxy (or there was no header):
	// there is no client address to believe, so the caller uses the peer.
	return netip.Addr{}, nil
}

// parseHop accepts "1.2.3.4", "2001:db8::1", "1.2.3.4:5678" and "[2001:db8::1]:443".
func parseHop(s string) (netip.Addr, error) {
	if a, err := netip.ParseAddr(s); err == nil && a.Zone() == "" {
		return a.Unmap(), nil
	}
	if ap, err := netip.ParseAddrPort(s); err == nil && ap.Addr().Zone() == "" {
		return ap.Addr().Unmap(), nil
	}
	return netip.Addr{}, errMalformed
}

func parseHostPort(s string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return netip.Addr{}, err
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Unmap(), nil
}

// ClientIP returns the TCP peer address, ignoring X-Forwarded-For. It is the
// behaviour of a resolver without trusted proxies.
func ClientIP(r *http.Request) string {
	return (*ClientIPResolver)(nil).ClientIP(r)
}
