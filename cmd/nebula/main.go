// Command nebula is the single binary for the API server and operational tasks.
//
//	nebula serve            run the API (default)
//	nebula migrate up       apply all pending migrations
//	nebula migrate down     roll back the latest migration
//	nebula migrate status   show migration state
//	nebula version          print the build version
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nebula/nebula/internal/auth"
	"github.com/nebula/nebula/internal/config"
	"github.com/nebula/nebula/internal/database"
	"github.com/nebula/nebula/internal/observability"
	"github.com/nebula/nebula/internal/server"
	"github.com/nebula/nebula/internal/store"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	if cmd == "version" {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return 2
	}
	logger := observability.NewLogger(stdout, cfg.LogFormat, cfg.LogLevel, version)

	switch cmd {
	case "serve":
		if err := cfg.ValidateAuth(); err != nil {
			_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
			return 2
		}
		err = serve(ctx, cfg, logger)
	case "migrate":
		if len(args) < 2 {
			_, _ = fmt.Fprintln(stderr, "usage: nebula migrate up|down|status")
			return 2
		}
		err = migrate(ctx, cfg, logger, args[1])
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q (use serve, migrate, version)\n", cmd)
		return 2
	}
	if err != nil {
		logger.Error("fatal", "command", cmd, "error", err)
		return 1
	}
	return 0
}

func migrate(ctx context.Context, cfg config.Config, logger *slog.Logger, sub string) error {
	pool, err := database.Connect(ctx, cfg.DatabaseURL, 2)
	if err != nil {
		return err
	}
	defer pool.Close()
	return database.Migrate(ctx, pool, sub, logger)
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	metrics := observability.NewMetrics()
	api := newHTTPServer(cfg.HTTPAddr, server.NewAPIHandler(server.Deps{
		Logger:  logger,
		Metrics: metrics,
		DB:      pool,
		Store:   store.New(pool),
		Verifier: auth.NewOIDCVerifier(auth.OIDCConfig{
			Issuer: cfg.OIDCIssuer, Audience: cfg.OIDCAudience, JWKSURL: cfg.OIDCJWKSURL,
		}),
		Version:        version,
		RequestTimeout: cfg.RequestTimeout,
		RateLimit:      cfg.RateLimit,
	}), cfg.RequestTimeout)
	admin := newHTTPServer(cfg.AdminAddr, server.NewAdminHandler(metrics), 10*time.Second)

	errCh := make(chan error, 2)
	for _, srv := range []*http.Server{api, admin} {
		go func() {
			logger.Info("listening", "addr", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("listen %s: %w", srv.Addr, err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	// Graceful shutdown: stop accepting, drain in-flight requests, then close the DB.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	err = errors.Join(api.Shutdown(shutdownCtx), admin.Shutdown(shutdownCtx))
	logger.Info("shutdown complete", "error", err)
	return err
}

func newHTTPServer(addr string, h http.Handler, requestTimeout time.Duration) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second, // slowloris protection
		ReadTimeout:       requestTimeout,
		WriteTimeout:      requestTimeout + 5*time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
	}
}
