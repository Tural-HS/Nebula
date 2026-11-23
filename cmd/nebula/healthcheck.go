package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// healthcheck probes this container's own listener, so distroless images can
// use a Docker HEALTHCHECK without a shell, curl or wget:
//
//	nebula healthcheck            GET /readyz on NEBULA_HTTP_ADDR (serve: API + database)
//	nebula healthcheck -admin     GET /healthz on NEBULA_ADMIN_ADDR (worker: process is up)
//
// It reads only the listen address, never secrets or the rest of the
// configuration, and exits 0 when the endpoint answers 200, otherwise 1.
func healthcheck(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	admin := fs.Bool("admin", false, "probe the admin listener (NEBULA_ADMIN_ADDR) instead of the API readiness endpoint")
	timeout := fs.Duration("timeout", 3*time.Second, "give up after this long")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	key, def, path := "NEBULA_HTTP_ADDR", ":8080", "/readyz"
	if *admin {
		key, def, path = "NEBULA_ADMIN_ADDR", ":9090", "/healthz"
	}
	addr := strings.TrimSpace(getenv(key))
	if addr == "" {
		addr = def
	}
	target, err := loopbackTarget(addr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %s: %v\n", key, err)
		return 2
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+target+path, nil)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	client := &http.Client{
		// Never through HTTP(S)_PROXY, never follow redirects: only this process counts.
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(stderr, "healthcheck: %s answered %d\n", path, resp.StatusCode)
		return 1
	}
	return 0
}

// loopbackTarget turns a listen address into a dialable one: wildcard and
// empty hosts become loopback, so the probe never leaves the container.
func loopbackTarget(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", err
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return net.JoinHostPort(host, port), nil
}
