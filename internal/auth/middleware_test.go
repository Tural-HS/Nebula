package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

type stubVerifier struct {
	claims Claims
	err    error
}

func (s stubVerifier) Verify(context.Context, string) (Claims, error) { return s.claims, s.err }

type stubResolver struct {
	calls atomic.Int32
	p     Principal
	err   error
}

func (s *stubResolver) ResolveUser(context.Context, Claims) (Principal, error) {
	s.calls.Add(1)
	return s.p, s.err
}

func writeErr(w http.ResponseWriter, status int, code, _ string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(code))
}

func serve(t *testing.T, v Verifier, r UserResolver, authz string) *httptest.ResponseRecorder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := Middleware(v, r, logger, writeErr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			t.Fatal("principal missing in context")
		}
		_, _ = w.Write([]byte(p.Email))
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMiddleware(t *testing.T) {
	okClaims := Claims{Issuer: "i", Subject: "s", Email: "a@x.io", Name: "A"}
	okPrincipal := Principal{UserID: uuid.New(), Email: "a@x.io"}

	tests := []struct {
		name       string
		v          Verifier
		r          *stubResolver
		authz      string
		wantStatus int
		wantBody   string
	}{
		{"no header", stubVerifier{claims: okClaims}, &stubResolver{p: okPrincipal}, "", 401, "unauthenticated"},
		{"wrong scheme", stubVerifier{claims: okClaims}, &stubResolver{p: okPrincipal}, "Basic abc", 401, "unauthenticated"},
		{"empty token", stubVerifier{claims: okClaims}, &stubResolver{p: okPrincipal}, "Bearer   ", 401, "unauthenticated"},
		{"invalid token", stubVerifier{err: ErrInvalidToken}, &stubResolver{p: okPrincipal}, "Bearer t", 401, "invalid_token"},
		{"idp down", stubVerifier{err: ErrUnavailable}, &stubResolver{p: okPrincipal}, "Bearer t", 503, "auth_unavailable"},
		{"disabled", stubVerifier{claims: okClaims}, &stubResolver{err: ErrUserDisabled}, "Bearer t", 403, "user_disabled"},
		{"conflict", stubVerifier{claims: okClaims}, &stubResolver{err: ErrIdentityConflict}, "Bearer t", 403, "identity_conflict"},
		{"db error", stubVerifier{claims: okClaims}, &stubResolver{err: errors.New("db down")}, "Bearer t", 500, "internal"},
		{"ok", stubVerifier{claims: okClaims}, &stubResolver{p: okPrincipal}, "bearer t", 200, "a@x.io"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.v, tt.r, tt.authz)
			if rec.Code != tt.wantStatus || rec.Body.String() != tt.wantBody {
				t.Fatalf("got %d %q, want %d %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
			if rec.Code == 401 && rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 must carry WWW-Authenticate")
			}
		})
	}
}
