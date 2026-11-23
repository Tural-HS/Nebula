// Package server wires HTTP routing, middleware and handlers.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nebula/nebula/internal/auth"
	"github.com/nebula/nebula/internal/config"
	"github.com/nebula/nebula/internal/observability"
	"github.com/nebula/nebula/internal/ratelimit"
	"github.com/nebula/nebula/internal/store"
)

// Pinger is the readiness dependency (satisfied by *pgxpool.Pool).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the dependencies of the API handler.
type Deps struct {
	Logger         *slog.Logger
	Metrics        *observability.Metrics
	DB             Pinger
	Store          *store.Store
	Verifier       auth.Verifier
	Version        string
	RequestTimeout time.Duration
	RateLimit      config.RateLimit // zero value disables limiting
}

// NewAPIHandler returns the public HTTP handler.
func NewAPIHandler(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.GetHead) // HEAD served by GET handlers (load balancer checks)
	r.Use(recoverer(d.Logger))
	r.Use(accessLog(d.Logger, d.Metrics))
	r.Use(securityHeaders)
	r.Use(middleware.Timeout(d.RequestTimeout))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.DB.Ping(ctx); err != nil {
			d.Logger.WarnContext(ctx, "readiness check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/version", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"version": d.Version})
		})
		// Everything else requires a valid access token. The IP limit runs first so
		// floods of bad tokens are cut off before signature checks and DB lookups.
		r.Group(func(r chi.Router) {
			r.Use(ratelimit.Middleware(ratelimit.New(d.RateLimit.IPRPS, d.RateLimit.IPBurst), ratelimit.ClientIP, writeError))
			r.Use(auth.Middleware(d.Verifier, d.Store, d.Logger, writeError))
			r.Use(ratelimit.Middleware(ratelimit.New(d.RateLimit.UserRPS, d.RateLimit.UserBurst), func(r *http.Request) string {
				return principal(r).UserID.String()
			}, writeError))
			(&api{store: d.Store, logger: d.Logger}).routes(r)
		})
	})

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})
	return r
}

// NewAdminHandler serves metrics on a separate, private listener.
func NewAdminHandler(m *observability.Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError emits the single error format every API endpoint uses.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
						panic(rec)
					}
					logger.ErrorContext(r.Context(), "panic recovered", "panic", rec,
						"request_id", middleware.GetReqID(r.Context()))
					writeError(w, http.StatusInternalServerError, "internal", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// accessLog logs each request and records metrics by route *pattern*
// (e.g. /api/v1/items/{id}) to keep Prometheus label cardinality bounded.
func accessLog(logger *slog.Logger, m *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			elapsed := time.Since(start)
			m.RequestsTotal.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()
			m.RequestDuration.WithLabelValues(route, r.Method).Observe(elapsed.Seconds())

			if route == "/healthz" || route == "/readyz" {
				return // probes would drown real traffic in the logs
			}
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method, "route", route, "status", status,
				"duration_ms", elapsed.Milliseconds(), "bytes", ww.BytesWritten(),
				"request_id", middleware.GetReqID(r.Context()))
		})
	}
}
