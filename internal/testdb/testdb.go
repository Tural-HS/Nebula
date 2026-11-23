// Package testdb gives each integration test its own throwaway PostgreSQL
// database, so test packages can run in parallel without interfering.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/database"
)

// EnvVar points at a Postgres server where the test user may CREATE DATABASE
// and CREATE ROLE (the postgres image's POSTGRES_USER can).
const EnvVar = "NEBULA_TEST_DATABASE_URL"

// New returns a pool to a fresh, fully migrated database (dropped after the
// test). The pool runs as a least-privilege runtime role, exactly like
// production serve/worker, so a store query that needs more than DML fails
// in tests instead of in production.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	runtime, _ := NewWithOwner(t)
	return runtime
}

// NewWithOwner is New plus a pool connected as the schema owner (the role
// "migrate" uses), for tests that change the schema.
func NewWithOwner(t testing.TB) (runtime, owner *pgxpool.Pool) {
	t.Helper()
	db := create(t)
	owner = db.connect(t, "")
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, owner, "up", logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Roles are cluster-wide: unique name, dropped after the database is gone.
	role := db.name + "_rt"
	ident := pgx.Identifier{role}.Sanitize()
	if _, err := db.admin.Exec(ctx, "CREATE ROLE "+ident+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"); err != nil {
		t.Fatalf("create runtime role (the %s user needs CREATEROLE): %v", EnvVar, err)
	}
	db.roles = append(db.roles, ident)
	if _, err := db.admin.Exec(ctx, "GRANT CONNECT ON DATABASE "+pgx.Identifier{db.name}.Sanitize()+" TO "+ident); err != nil {
		t.Fatalf("grant connect: %v", err)
	}
	if err := database.GrantRuntime(ctx, owner, role, logger); err != nil {
		t.Fatalf("grant runtime: %v", err)
	}
	// The test user logs in and switches to the runtime role at connection
	// start, so the test needs no password for it; privileges are the role's.
	return db.connect(t, role), owner
}

// NewEmpty returns a pool to a fresh database with no schema, connected as
// its owner.
func NewEmpty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	db := create(t)
	return db.connect(t, "")
}

type testDB struct {
	name  string
	base  string
	admin *pgxpool.Pool
	roles []string // quoted; dropped after the database
}

func create(t testing.TB) *testDB {
	t.Helper()
	base := os.Getenv(EnvVar)
	if base == "" {
		t.Skip(EnvVar + " not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	db := &testDB{name: "nebula_t_" + hex.EncodeToString(suffix), base: base, admin: admin}
	ident := pgx.Identifier{db.name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	// Registered first, so it runs last: after every pool of the test is closed.
	t.Cleanup(func() {
		defer admin.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
			return
		}
		for _, r := range db.roles {
			if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+r); err != nil {
				t.Errorf("drop test role: %v", err)
			}
		}
	})
	return db
}

// connect opens a pool to the test database; a non-empty role is assumed at
// connection start (like SET ROLE).
func (db *testDB) connect(t testing.TB, role string) *pgxpool.Pool {
	t.Helper()
	u, err := url.Parse(db.base)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvVar, err)
	}
	u.Path = "/" + db.name
	if role != "" {
		q := u.Query()
		q.Set("role", role)
		u.RawQuery = q.Encode()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Connect(ctx, u.String(), 10)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
