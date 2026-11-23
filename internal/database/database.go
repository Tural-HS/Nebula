// Package database owns the PostgreSQL connection pool and schema migrations.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/Tural-HS/Nebula/migrations"
)

// Connect opens a pool and verifies connectivity before returning.
func Connect(ctx context.Context, databaseURL string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err) // pgx redacts the password
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	// Guard against runaway queries holding connections forever.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000"
	cfg.ConnConfig.RuntimeParams["application_name"] = "nebula"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate runs a goose command ("up", "down", "status") using the embedded SQL files.
func Migrate(ctx context.Context, pool *pgxpool.Pool, command string, logger *slog.Logger) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	// Advisory lock: safe when several replicas run migrations at the same time.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	switch command {
	case "up":
		results, err := provider.Up(ctx)
		logResults(logger, results)
		return err
	case "down":
		result, err := provider.Down(ctx)
		if result != nil {
			logResults(logger, []*goose.MigrationResult{result})
		}
		return err
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			logger.Info("migration", "migration_version", s.Source.Version, "file", s.Source.Path, "state", s.State)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q (use up, down or status)", command)
	}
}

func logResults(logger *slog.Logger, results []*goose.MigrationResult) {
	if len(results) == 0 {
		logger.Info("no migrations to apply")
	}
	for _, r := range results {
		logger.Info("migration applied", "migration_version", r.Source.Version, "file", r.Source.Path,
			"direction", r.Direction, "duration", r.Duration.String())
	}
}
