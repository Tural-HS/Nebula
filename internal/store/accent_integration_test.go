package store_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Migration 00028: the CHECK backs up the store's validation, and the column
// goes down and up again cleanly (only the accent choice is lost).
func TestWorkspaceAccentMigration(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if _, err := f.owner.Exec(ctx, `UPDATE workspaces SET accent = 'teal' WHERE id = $1`, f.wsID); err == nil {
		t.Fatal("CHECK accepted 'teal' (the default must be stored as NULL)")
	}
	if _, err := f.owner.Exec(ctx, `UPDATE workspaces SET accent = 'red' WHERE id = $1`, f.wsID); err == nil {
		t.Fatal("CHECK accepted an unknown accent")
	}
	if err := f.st.UpdateWorkspace(ctx, f.wsID, f.userID, store.UpdateWorkspaceParams{Accent: opt.Of("plum")}); err != nil {
		t.Fatal(err)
	}

	version := func() int {
		var v int
		if err := f.owner.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	column := func() int {
		var n int
		if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'workspaces' AND column_name = 'accent'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Later migrations (other features) are undone first.
	for version() >= 28 {
		if err := database.Migrate(ctx, f.owner, "down", logger); err != nil {
			t.Fatal(err)
		}
	}
	if n := column(); n != 0 {
		t.Fatal("accent column survives the down migration")
	}
	if err := database.Migrate(ctx, f.owner, "up", logger); err != nil {
		t.Fatal(err)
	}
	if n := column(); n != 1 {
		t.Fatal("accent column missing after up")
	}
	ws, err := f.st.WorkspaceForMember(ctx, "acme", f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Accent != nil {
		t.Fatalf("accent after down/up = %q, want default", *ws.Accent)
	}
}
