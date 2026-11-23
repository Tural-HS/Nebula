package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func probeServer(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestHealthcheck(t *testing.T) {
	// A proxy in the environment must be ignored: the probe is container-local.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	ready := probeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	down := probeServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	redirect := probeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+ready+"/readyz", http.StatusFound)
	})
	admin := probeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
		}
	})
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().String()
	_ = l.Close()

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want int
	}{
		{"ready", nil, map[string]string{"NEBULA_HTTP_ADDR": ready}, 0},
		{"not ready", nil, map[string]string{"NEBULA_HTTP_ADDR": down}, 1},
		{"redirect is not followed", nil, map[string]string{"NEBULA_HTTP_ADDR": redirect}, 1},
		{"nothing listening", nil, map[string]string{"NEBULA_HTTP_ADDR": closed}, 1},
		{"admin listener", []string{"-admin"}, map[string]string{"NEBULA_ADMIN_ADDR": admin, "NEBULA_HTTP_ADDR": closed}, 0},
		{"bad address", nil, map[string]string{"NEBULA_HTTP_ADDR": "8080"}, 2},
		{"bad flag", []string{"-nope"}, nil, 2},
		// Secrets and the rest of the configuration are irrelevant (and never read).
		{"ignores invalid config", nil, map[string]string{"NEBULA_HTTP_ADDR": ready, "NEBULA_ENV": "bogus",
			"NEBULA_ENCRYPTION_KEY_FILE": "/nonexistent"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			getenv := func(k string) string { return tt.env[k] }
			code := run(context.Background(), append([]string{"healthcheck"}, tt.args...), getenv, &out, &errOut)
			if code != tt.want {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tt.want, errOut.String())
			}
		})
	}
}

func TestLoopbackTarget(t *testing.T) {
	for in, want := range map[string]string{
		":8080":         "127.0.0.1:8080",
		"0.0.0.0:8080":  "127.0.0.1:8080",
		"[::]:8080":     "[::1]:8080",
		"10.1.2.3:8080": "10.1.2.3:8080",
	} {
		if got, err := loopbackTarget(in); err != nil || got != want {
			t.Errorf("loopbackTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
