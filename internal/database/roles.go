package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GrantRuntime gives role the privileges the running application needs and
// nothing more: data manipulation (SELECT, INSERT, UPDATE, DELETE) on every
// table and use of every sequence in schema public. It grants no DDL, no
// TRUNCATE, REFERENCES or TRIGGER, and only read access to the migration
// version table.
//
// It must run as the role that owns the schema objects (the migration role).
// Existing objects are granted directly; objects that later migrations create
// as this role are covered by default privileges. Every "migrate up" runs it
// again, so it also repairs drift (an object created by another role, a
// manual REVOKE). The statements are idempotent and run in one transaction.
func GrantRuntime(ctx context.Context, pool *pgxpool.Pool, role string, logger *slog.Logger) error {
	var exists, isSuper, isCurrent bool
	err := pool.QueryRow(ctx, `SELECT true, rolsuper, rolname = current_user FROM pg_roles WHERE rolname = $1`, role).
		Scan(&exists, &isSuper, &isCurrent)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("runtime role %q does not exist: create it first (LOGIN, no other privileges)", role)
	case err != nil:
		return fmt.Errorf("look up runtime role: %w", err)
	case isCurrent:
		return fmt.Errorf("runtime role %q is the migration role itself: use a separate role for serve and worker", role)
	case isSuper:
		return fmt.Errorf("runtime role %q is a superuser: that defeats least privilege", role)
	}

	r := pgx.Identifier{role}.Sanitize()
	stmts := []string{
		`GRANT USAGE ON SCHEMA public TO ` + r,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ` + r,
		`GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO ` + r,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ` + r,
		`ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO ` + r,
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s); err != nil {
			return fmt.Errorf("grant runtime privileges to %s: %w", role, err)
		}
	}
	// The application reads the schema version at most; it must never be able
	// to fake or erase migration history.
	var hasVersionTable bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&hasVersionTable); err != nil {
		return err
	}
	if hasVersionTable {
		for _, s := range []string{
			`REVOKE INSERT, UPDATE, DELETE ON goose_db_version FROM ` + r,
			`GRANT SELECT ON goose_db_version TO ` + r,
		} {
			if _, err := tx.Exec(ctx, s); err != nil {
				return fmt.Errorf("restrict migration table for %s: %w", role, err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logger.Info("runtime role privileges granted", "role", role)
	return nil
}

// CheckLeastPrivilege reports whether the connected role can change the
// schema (create objects in public or owns a table there). It is advisory: a
// single-role setup (development) is allowed, production should not be one.
func CheckLeastPrivilege(ctx context.Context, pool *pgxpool.Pool) (canDDL bool, err error) {
	err = pool.QueryRow(ctx, `SELECT has_schema_privilege('public', 'CREATE')
		OR EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tableowner = current_user)
		OR (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)`).Scan(&canDDL)
	return canDDL, err
}
