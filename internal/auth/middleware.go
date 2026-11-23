package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// ErrorWriter writes an API error response (injected to avoid an import cycle).
type ErrorWriter func(w http.ResponseWriter, status int, code, message string)

// Middleware authenticates every request; unauthenticated requests never reach next.
// The user is resolved from the database on every request (one indexed lookup),
// so deactivating a user takes effect immediately, not after a cache TTL.
func Middleware(v Verifier, users UserResolver, logger *slog.Logger, writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			raw, ok := bearerToken(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="nebula"`)
				writeErr(w, http.StatusUnauthorized, "unauthenticated", "missing bearer token")
				return
			}
			claims, err := v.Verify(ctx, raw)
			if err != nil {
				if errors.Is(err, ErrUnavailable) {
					logger.ErrorContext(ctx, "identity provider unavailable", "error", err)
					writeErr(w, http.StatusServiceUnavailable, "auth_unavailable", "identity provider unavailable")
					return
				}
				// The error never contains the token itself.
				logger.InfoContext(ctx, "token rejected", "error", err)
				w.Header().Set("WWW-Authenticate", `Bearer realm="nebula", error="invalid_token"`)
				writeErr(w, http.StatusUnauthorized, "invalid_token", "invalid or expired token")
				return
			}

			p, err := users.ResolveUser(ctx, claims)
			switch {
			case errors.Is(err, ErrUserDisabled):
				writeErr(w, http.StatusForbidden, "user_disabled", "user is disabled")
				return
			case errors.Is(err, ErrIdentityConflict):
				logger.WarnContext(ctx, "identity conflict", "issuer", claims.Issuer, "subject", claims.Subject)
				writeErr(w, http.StatusForbidden, "identity_conflict", "email is linked to another identity")
				return
			case err != nil:
				logger.ErrorContext(ctx, "resolve user", "error", err)
				writeErr(w, http.StatusInternalServerError, "internal", "internal server error")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, p)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
