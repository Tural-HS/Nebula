// Command nebula is the single binary for the API server and operational tasks.
//
//	nebula serve            run the API and background worker (default)
//	nebula worker           run only the background worker
//	nebula rotate-secrets   re-encrypt stored secrets with the current key
//	nebula migrate up       apply all pending migrations
//	nebula migrate down     roll back the latest migration
//	nebula migrate status   show migration state
//	nebula healthcheck      probe the local readiness endpoint (Docker HEALTHCHECK)
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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/engine"
	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/secretbox"
	"github.com/Tural-HS/Nebula/internal/server"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/worker"
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
	switch cmd {
	case "version":
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	case "healthcheck": // needs only the listen address, never secrets
		return healthcheck(ctx, args[1:], getenv, stderr)
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return 2
	}
	logger := observability.NewLogger(stdout, cfg.LogFormat, cfg.LogLevel, version)

	switch cmd {
	case "rotate-secrets":
		if verr := cfg.ValidateRuntime(); verr != nil {
			_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", verr)
			return 2
		}
		err = rotateSecrets(ctx, cfg, logger)
	case "serve", "worker":
		verr := cfg.ValidateRuntime()
		if cmd == "serve" {
			verr = errors.Join(verr, cfg.ValidateAuth())
		}
		if verr != nil {
			_, _ = fmt.Fprintf(stderr, "invalid configuration:\n%v\n", verr)
			return 2
		}
		if cmd == "serve" {
			err = serve(ctx, cfg, logger)
		} else {
			err = runWorkerOnly(ctx, cfg, logger)
		}
	case "migrate":
		if len(args) < 2 {
			_, _ = fmt.Fprintln(stderr, "usage: nebula migrate up|down|status")
			return 2
		}
		err = migrate(ctx, cfg, logger, args[1])
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q (use serve, worker, migrate, rotate-secrets, healthcheck, version)\n", cmd)
		return 2
	}
	if err != nil {
		logger.Error("fatal", "command", cmd, "error", err)
		return 1
	}
	return 0
}

// migrate runs as the schema owner (NEBULA_MIGRATE_DATABASE_URL, falling back
// to NEBULA_DATABASE_URL). With NEBULA_DB_RUNTIME_ROLE, "up" then grants that
// role DML on every table and sequence, including future ones.
func migrate(ctx context.Context, cfg config.Config, logger *slog.Logger, sub string) error {
	pool, err := database.Connect(ctx, cfg.MigrateDatabaseURL, 2)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool, sub, logger); err != nil {
		return err
	}
	if sub == "up" && cfg.DBRuntimeRole != "" {
		return database.GrantRuntime(ctx, pool, cfg.DBRuntimeRole, logger)
	}
	return nil
}

// runtime holds what serve and worker share.
type runtime struct {
	pool    *pgxpool.Pool
	store   *store.Store
	metrics *observability.Metrics
	webhook safehttp.Options
}

func newRuntime(ctx context.Context, cfg config.Config, logger *slog.Logger) (*runtime, error) {
	box, err := secretbox.NewFromBase64(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	if box, err = box.WithOldKeys(cfg.OldEncryptionKeys...); err != nil {
		return nil, err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return nil, err
	}
	warnIfSchemaOwner(ctx, pool, cfg, logger)
	return &runtime{
		pool:    pool,
		store:   store.New(pool).WithSecrets(box),
		metrics: observability.NewMetrics(),
		webhook: safehttp.Options{
			AllowPrivate: cfg.Webhooks.AllowPrivate,
			RequireHTTPS: cfg.Env == "production",
			Timeout:      10 * time.Second,
		},
	}, nil
}

func (rt *runtime) newWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) (*worker.Worker, error) {
	var sender mail.Sender
	if cfg.SMTP.Host != "" {
		s, err := mail.NewSMTPSender(mail.Config{
			Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username, Password: cfg.SMTP.Password,
			From: cfg.SMTP.From, TLS: cfg.SMTP.TLS,
		})
		if err != nil {
			return nil, err
		}
		sender = s
	} else {
		logger.Warn("NEBULA_SMTP_HOST not set: email notifications are disabled")
	}
	eng := &engine.Engine{
		Store: rt.store, HTTP: safehttp.NewClient(rt.webhook), HTTPOpts: rt.webhook,
		Mail: sender, Logger: logger, Version: version, PublicURL: cfg.PublicURL,
	}
	return worker.New(rt.store, eng.Handlers(), logger, rt.metrics, worker.Options{
		Concurrency:   max(cfg.WorkerConcurrency, 1),
		ShutdownGrace: cfg.ShutdownTimeout - 5*time.Second,
		Wake:          worker.Listen(ctx, rt.pool, logger),
		Tick:          eng.Tick,
	}), nil
}

// rotateSecrets re-encrypts secrets sealed with NEBULA_ENCRYPTION_KEYS_OLD.
// Rotation: 1) set the new key as NEBULA_ENCRYPTION_KEY and the previous one in
// NEBULA_ENCRYPTION_KEYS_OLD, deploy; 2) run this; 3) remove the old key.
func rotateSecrets(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	rt, err := newRuntime(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer rt.pool.Close()
	n, err := rt.store.ReencryptSecrets(ctx)
	if err != nil {
		return err
	}
	logger.Info("secrets re-encrypted with the current key", "updated", n,
		"next_step", "remove NEBULA_ENCRYPTION_KEYS_OLD once every replica runs with the new key")
	return nil
}

func runWorkerOnly(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	rt, err := newRuntime(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer rt.pool.Close()
	w, err := rt.newWorker(ctx, cfg, logger)
	if err != nil {
		return err
	}
	rt.startQueueStats(ctx, cfg, logger)
	admin := newHTTPServer(cfg.AdminAddr, server.NewAdminHandler(rt.metrics), 10*time.Second)
	go func() {
		if err := admin.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("admin listener", "error", err)
		}
	}()
	w.Run(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return admin.Shutdown(shutdownCtx)
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	rt, err := newRuntime(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer rt.pool.Close()
	pool, metrics := rt.pool, rt.metrics
	rt.startQueueStats(ctx, cfg, logger)

	workerDone := make(chan struct{})
	if cfg.WorkerConcurrency > 0 {
		w, err := rt.newWorker(ctx, cfg, logger)
		if err != nil {
			return err
		}
		go func() { w.Run(ctx); close(workerDone) }()
	} else {
		close(workerDone)
	}

	api := newHTTPServer(cfg.HTTPAddr, server.NewAPIHandler(server.Deps{
		Logger:    logger,
		Metrics:   metrics,
		DB:        pool,
		Store:     rt.store,
		Webhooks:  rt.webhook,
		PublicURL: cfg.PublicURL,
		Verifier: auth.NewOIDCVerifier(auth.OIDCConfig{
			Issuer: cfg.OIDCIssuer, Audience: cfg.OIDCAudience, JWKSURL: cfg.OIDCJWKSURL,
		}),
		Version:        version,
		RequestTimeout: cfg.RequestTimeout,
		RateLimit:      cfg.RateLimit,
		TrustedProxies: cfg.TrustedProxies,
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
	<-workerDone // the worker stops on ctx and drains within ShutdownGrace
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
