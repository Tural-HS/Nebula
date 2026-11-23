package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/config"
	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/worker"
)

// startQueueStats exports the job-queue gauges on the metrics listener. Every
// replica reports the same (database-wide) values: aggregate with max().
func (rt *runtime) startQueueStats(ctx context.Context, cfg config.Config, logger *slog.Logger) {
	qs := worker.NewQueueStats(rt.store, cfg.JobMetricsInterval, logger)
	rt.metrics.Registry.MustRegister(qs)
	go qs.Run(ctx)
}

// warnIfSchemaOwner flags a production process that could change the schema:
// serve and worker should connect as a DML-only role (ADR-0018).
func warnIfSchemaOwner(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) {
	if cfg.Env != "production" {
		return
	}
	canDDL, err := database.CheckLeastPrivilege(ctx, pool)
	if err != nil {
		logger.Warn("could not check database privileges", "error", err)
		return
	}
	if canDDL {
		logger.Warn("the runtime database role can change the schema; use a DML-only role " +
			"(NEBULA_DATABASE_URL) and keep the owner for NEBULA_MIGRATE_DATABASE_URL")
	}
}
