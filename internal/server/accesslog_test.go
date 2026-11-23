package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/observability"
)

// accessLogLine sends one request through the API handler and returns the
// decoded "http request" log record plus the raw log output.
func accessLogLine(t *testing.T, trusted []netip.Prefix, req *http.Request) (map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	h := NewAPIHandler(Deps{
		Logger:         slog.New(slog.NewJSONHandler(&buf, nil)),
		Metrics:        observability.NewMetrics(),
		DB:             fakeDB{},
		Verifier:       denyVerifier{},
		Version:        "test",
		RequestTimeout: 5 * time.Second,
		TrustedProxies: trusted,
	})
	h.ServeHTTP(httptest.NewRecorder(), req)
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		if rec["msg"] == "http request" {
			return rec, buf.String()
		}
	}
	t.Fatalf("no access log record in:\n%s", buf.String())
	return nil, ""
}

func TestAccessLogClientIP(t *testing.T) {
	proxy := []netip.Prefix{netip.MustParsePrefix("10.0.0.10/32")}
	tests := []struct {
		name    string
		trusted []netip.Prefix
		peer    string
		xff     string
		want    string
	}{
		{"no proxies: peer, forged header ignored", nil, "203.0.113.7:4711", "198.51.100.1", "203.0.113.7"},
		{"trusted proxy: forwarded client", proxy, "10.0.0.10:5000", "198.51.100.1", "198.51.100.1"},
		{"trusted proxy: client-supplied prefix ignored", proxy, "10.0.0.10:5000", "1.1.1.1, 198.51.100.1", "198.51.100.1"},
		{"untrusted peer: header ignored", proxy, "203.0.113.9:5000", "198.51.100.1", "203.0.113.9"},
		{"ipv6 peer", nil, "[2001:db8::5]:443", "", "2001:db8::5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/version", nil)
			req.RemoteAddr = tt.peer
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			rec, _ := accessLogLine(t, tt.trusted, req)
			if rec["client_ip"] != tt.want {
				t.Fatalf("client_ip = %v, want %s", rec["client_ip"], tt.want)
			}
		})
	}
}

func TestAccessLogHasNoSecrets(t *testing.T) {
	const secret = "s3cr3t-token-value"
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/items/42?access_token="+secret+"&code="+secret, nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Cookie", "session="+secret)
	rec, raw := accessLogLine(t, nil, req)
	if strings.Contains(raw, secret) {
		t.Fatalf("log output contains a secret:\n%s", raw)
	}
	if strings.Contains(raw, "/items/42") {
		t.Fatalf("log output contains the raw path (IDs, cardinality):\n%s", raw)
	}
	if rec["route"] == nil || rec["status"] == nil || rec["client_ip"] == nil {
		t.Fatalf("missing fields: %v", rec)
	}
}

func TestAdminHealthz(t *testing.T) {
	rec := do(NewAdminHandler(observability.NewMetrics()), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
