package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nebula/nebula/internal/auth"
	"github.com/nebula/nebula/internal/observability"
)

type fakeDB struct{ err error }

type denyVerifier struct{}

func (denyVerifier) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{}, auth.ErrInvalidToken
}

func (f fakeDB) Ping(context.Context) error { return f.err }

func newTestHandler(db Pinger) (http.Handler, *observability.Metrics) {
	m := observability.NewMetrics()
	return NewAPIHandler(Deps{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:        m,
		DB:             db,
		Verifier:       denyVerifier{},
		Version:        "test",
		RequestTimeout: 5 * time.Second,
	}), m
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, path, nil))
	return rec
}

func TestHealthAndReadiness(t *testing.T) {
	tests := []struct {
		name       string
		db         Pinger
		path       string
		wantStatus int
		wantBody   string
	}{
		{"liveness ignores db", fakeDB{err: errors.New("down")}, "/healthz", 200, `"ok"`},
		{"ready when db up", fakeDB{}, "/readyz", 200, `"ready"`},
		{"not ready when db down", fakeDB{err: errors.New("down")}, "/readyz", 503, `"unavailable"`},
		{"version", fakeDB{}, "/api/v1/version", 200, `"test"`},
		{"json 404", fakeDB{}, "/nope", 404, `"not_found"`},
		{"api requires auth", fakeDB{}, "/api/v1/me", 401, `"unauthenticated"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestHandler(tt.db)
			rec := do(h, http.MethodGet, tt.path)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q", ct)
			}
		})
	}
}

func TestHeadIsServed(t *testing.T) {
	h, _ := newTestHandler(fakeDB{})
	if rec := do(h, http.MethodHead, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(fakeDB{})
	if rec := do(h, http.MethodPost, "/healthz"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h, _ := newTestHandler(fakeDB{})
	rec := do(h, http.MethodGet, "/healthz")
	for k, v := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY"} {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestMetricsUseRoutePatternAndAreServedOnAdmin(t *testing.T) {
	h, m := newTestHandler(fakeDB{})
	do(h, http.MethodGet, "/healthz")
	do(h, http.MethodGet, "/random-path-1")
	do(h, http.MethodGet, "/random-path-2")

	rec := do(NewAdminHandler(m), http.MethodGet, "/metrics")
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("metrics status = %d", rec.Code)
	}
	if !strings.Contains(body, `nebula_http_requests_total{code="200",method="GET",route="/healthz"} 1`) {
		t.Errorf("missing healthz counter in:\n%s", body)
	}
	if strings.Contains(body, "random-path") {
		t.Errorf("raw paths leaked into metric labels (cardinality risk)")
	}
}

func TestPanicIsRecovered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := do(h, http.MethodGet, "/")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
