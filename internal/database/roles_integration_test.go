package database_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// TestRuntimeRoleHasNoDDL proves the runtime role used by serve/worker can
// read and write data but cannot change the schema or migration history, and
// that tables created by later migrations become usable without extra grants.
func TestRuntimeRoleHasNoDDL(t *testing.T) {
	rt, owner := testdb.NewWithOwner(t)
	ctx := context.Background()

	var user string
	if err := rt.QueryRow(ctx, `SELECT current_user`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(user, "_rt") {
		t.Fatalf("runtime pool runs as %q, not the runtime role", user)
	}

	// DML works.
	var ws string
	if err := rt.QueryRow(ctx, `INSERT INTO workspaces (slug, name) VALUES ('rt', 'rt') RETURNING id`).Scan(&ws); err != nil {
		t.Fatalf("insert: %v", err)
	}
	mustExec(t, rt, `UPDATE workspaces SET name = 'rt2' WHERE id = $1`, ws)
	mustExec(t, rt, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := rt.Exec(ctx, `SELECT count(*) FROM goose_db_version`); err != nil {
		t.Fatalf("schema version must stay readable: %v", err)
	}

	denied := map[string]string{
		"create table":     `CREATE TABLE evil (id int)`,
		"create function":  `CREATE FUNCTION evil() RETURNS int LANGUAGE sql AS 'SELECT 1'`,
		"alter table":      `ALTER TABLE items ADD COLUMN evil int`,
		"drop table":       `DROP TABLE comments`,
		"create index":     `CREATE INDEX evil_idx ON items (created_at)`,
		"truncate":         `TRUNCATE events`,
		"create trigger":   `CREATE TRIGGER evil BEFORE INSERT ON items FOR EACH ROW EXECUTE FUNCTION suppress_redundant_updates_trigger()`,
		"fake migration":   `INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, true)`,
		"erase migrations": `DELETE FROM goose_db_version`,
		"create schema":    `CREATE SCHEMA evil`,
	}
	for name, sql := range denied {
		t.Run(name, func(t *testing.T) {
			_, err := rt.Exec(ctx, sql)
			var pgErr *pgconn.PgError
			// 42501 insufficient_privilege. Ownership checks also report 42501.
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Fatalf("%s: want insufficient_privilege, got %v", sql, err)
			}
		})
	}

	// GRANT without grant option only warns ("no privileges were granted").
	mustExec(t, rt, `GRANT SELECT ON items TO PUBLIC`)
	var leaked bool
	if err := owner.QueryRow(ctx, `SELECT has_table_privilege('public', 'items', 'SELECT')`).Scan(&leaked); err != nil || leaked {
		t.Fatalf("runtime role could re-grant table access: %v %v", leaked, err)
	}

	canDDL, err := database.CheckLeastPrivilege(ctx, rt)
	if err != nil || canDDL {
		t.Fatalf("runtime role reported as DDL-capable: %v %v", canDDL, err)
	}
	if canDDL, err := database.CheckLeastPrivilege(ctx, owner); err != nil || !canDDL {
		t.Fatalf("owner must be reported as DDL-capable: %v %v", canDDL, err)
	}

	// A future migration: the owner creates a table with an identity column.
	// Default privileges make it usable at once, sequence included.
	mustExec(t, owner, `CREATE TABLE future_things (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, v text)`)
	mustExec(t, owner, `CREATE SEQUENCE future_seq`)
	mustExec(t, rt, `INSERT INTO future_things (v) VALUES ('x')`)
	mustExec(t, rt, `SELECT nextval('future_seq')`)
	if _, err := rt.Exec(ctx, `TRUNCATE future_things`); err == nil {
		t.Fatal("default privileges must not include TRUNCATE")
	}
}

func TestGrantRuntimeRejectsUnsafeRoles(t *testing.T) {
	_, owner := testdb.NewWithOwner(t)
	ctx := context.Background()
	var self string
	if err := owner.QueryRow(ctx, `SELECT current_user`).Scan(&self); err != nil {
		t.Fatal(err)
	}
	for role, want := range map[string]string{
		"nebula_no_such_role": "does not exist",
		self:                  "migration role itself",
	} {
		if err := database.GrantRuntime(ctx, owner, role, quiet); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("GrantRuntime(%q): want %q, got %v", role, want, err)
		}
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}
