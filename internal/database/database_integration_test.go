package database_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// Integration tests run against a real PostgreSQL in a throwaway database.
// They are skipped unless NEBULA_TEST_DATABASE_URL is set.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestMigrationsUpDownUp(t *testing.T) {
	pool := testdb.NewEmpty(t)
	ctx := context.Background()

	for _, step := range []string{"up", "down", "up", "status"} {
		if err := database.Migrate(ctx, pool, step, quiet); err != nil {
			t.Fatalf("migrate %s: %v", step, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = ANY($1)`,
		[]string{"workspaces", "users", "projects", "items", "comments", "events"}).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6 core tables, found %d", n)
	}
}

// seed creates a workspace with a workflow, status, item type and project,
// and returns their ids.
func seed(t *testing.T, pool *pgxpool.Pool, slug, key string) (ws, status, itemType, project string) {
	t.Helper()
	ctx := context.Background()
	err := pool.QueryRow(ctx, `
		WITH w AS (INSERT INTO workspaces (slug, name) VALUES ($1::text, $1::text) RETURNING id),
		     wf AS (INSERT INTO workflows (workspace_id, name) SELECT id, 'Default' FROM w RETURNING id, workspace_id),
		     st AS (INSERT INTO statuses (workspace_id, workflow_id, name, category, is_initial)
		            SELECT workspace_id, id, 'Open', 'todo', true FROM wf RETURNING id),
		     it AS (INSERT INTO item_types (workspace_id, workflow_id, key, name)
		            SELECT workspace_id, id, 'task', 'Task' FROM wf RETURNING id),
		     p  AS (INSERT INTO projects (workspace_id, key, name) SELECT id, $2::text, $2::text FROM w RETURNING id)
		SELECT (SELECT id FROM w), (SELECT id FROM st), (SELECT id FROM it), (SELECT id FROM p)`,
		slug, key).Scan(&ws, &status, &itemType, &project)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return
}

func TestSchemaInvariants(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	wsA, statusA, typeA, projA := seed(t, pool, "acme", "ACME")
	_, statusB, _, projB := seed(t, pool, "globex", "GLX")

	insertItem := `INSERT INTO items (workspace_id, project_id, item_type_id, status_id, number, title)
	               VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`

	var itemID string
	if err := pool.QueryRow(ctx, insertItem, wsA, projA, typeA, statusA, 1, "Fix login").Scan(&itemID); err != nil {
		t.Fatalf("valid insert failed: %v", err)
	}

	t.Run("rejects cross-tenant project", func(t *testing.T) {
		_, err := pool.Exec(ctx, insertItem, wsA, projB, typeA, statusA, 2, "x")
		wantErr(t, err, "foreign key")
	})
	t.Run("rejects cross-tenant status", func(t *testing.T) {
		_, err := pool.Exec(ctx, insertItem, wsA, projA, typeA, statusB, 3, "x")
		wantErr(t, err, "foreign key")
	})
	t.Run("rejects duplicate item number", func(t *testing.T) {
		_, err := pool.Exec(ctx, insertItem, wsA, projA, typeA, statusA, 1, "dup")
		wantErr(t, err, "duplicate key")
	})
	t.Run("rejects empty title", func(t *testing.T) {
		_, err := pool.Exec(ctx, insertItem, wsA, projA, typeA, statusA, 4, "")
		wantErr(t, err, "check constraint")
	})
	t.Run("rejects non-object fields", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE items SET fields = '[1,2]' WHERE id = $1`, itemID)
		wantErr(t, err, "check constraint")
	})
	t.Run("rejects invalid project key", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO projects (workspace_id, key, name) VALUES ($1, 'bad-key', 'x')`, wsA)
		wantErr(t, err, "check constraint")
	})
	t.Run("only one initial status per workflow", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO statuses (workspace_id, workflow_id, name, category, is_initial)
			SELECT workspace_id, workflow_id, 'Also initial', 'todo', true FROM statuses WHERE id = $1`, statusA)
		wantErr(t, err, "duplicate key")
	})
	t.Run("updated_at trigger fires", func(t *testing.T) {
		var before, after time.Time
		_ = pool.QueryRow(ctx, `SELECT updated_at FROM items WHERE id = $1`, itemID).Scan(&before)
		time.Sleep(10 * time.Millisecond)
		if err := pool.QueryRow(ctx, `UPDATE items SET title = 'Fix login page', version = version + 1
			WHERE id = $1 RETURNING updated_at`, itemID).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if !after.After(before) {
			t.Fatalf("updated_at not advanced: %v -> %v", before, after)
		}
	})
	t.Run("full text search finds item", func(t *testing.T) {
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM items WHERE search @@ plainto_tsquery('simple', 'login')`).Scan(&n)
		if n != 1 {
			t.Fatalf("search hits = %d, want 1", n)
		}
	})
	t.Run("events are append-only", func(t *testing.T) {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO events (workspace_id, entity_type, entity_id, action)
			VALUES ($1, 'item', $2, 'item.created') RETURNING id`, wsA, itemID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(ctx, `UPDATE events SET action = 'item.hacked' WHERE id = $1`, id)
		wantErr(t, err, "append-only")
	})
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("want error containing %q, got %v", substr, err)
	}
}
