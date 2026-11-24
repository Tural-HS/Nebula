package store_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// searchFixture is a workspace with one project, created through the store.
type searchFixture struct {
	st      *store.Store
	owner   *pgxpool.Pool
	userID  uuid.UUID
	wsID    uuid.UUID
	project uuid.UUID
}

func newSearchFixture(t testing.TB) searchFixture {
	t.Helper()
	runtime, owner := testdb.NewWithOwner(t)
	st := store.New(runtime)
	ctx := context.Background()
	u, err := st.ResolveUser(ctx, auth.Claims{Issuer: "https://idp.test", Subject: "sub-alice", Email: "alice@example.com", Name: "Alice", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := st.CreateWorkspace(ctx, u.UserID, "acme", "Acme", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProject(ctx, ws.ID, u.UserID, "QA", "QA", nil, "workspace", store.ProjectTemplate{})
	if err != nil {
		t.Fatal(err)
	}
	return searchFixture{st: st, owner: owner, userID: u.UserID, wsID: ws.ID, project: p.ID}
}

func (f searchFixture) search(t testing.TB, q string) []store.SearchHit {
	t.Helper()
	res, err := f.st.Search(context.Background(), store.SearchParams{
		WorkspaceID: f.wsID, Viewer: store.Viewer{UserID: f.userID, Role: store.RoleOwner}, Query: q, Limit: 50,
	})
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	return res.Hits
}

// The Go copy of search_fold (used for highlighting) must agree with the SQL
// function that builds the index.
func TestSearchFoldMatchesSQL(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	for _, s := range []string{
		"ƏəİIıÖöÜüĞğŞşÇç", "ÁáÀàÂâÄäÉéÈèÊêËëÍíÌìÎîÏïÓóÒòÔôÕõÚúÙùÛûÑñ",
		"ödèniş", "Ödəniş sistemi — 2026!", "Привет", "日本語",
	} {
		var sqlFold string
		if err := f.owner.QueryRow(ctx, `SELECT lower(search_fold($1))`, s).Scan(&sqlFold); err != nil {
			t.Fatal(err)
		}
		if got := store.SearchFoldForTest(s); got != sqlFold {
			t.Errorf("fold(%q): Go %q, SQL %q", s, got, sqlFold)
		}
	}
}

// The migration's backfill indexes existing data: run 00018 down, write data
// without the triggers, run it up again.
func TestSearchBackfill(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Later migrations are undone first, one step at a time.
	var version int
	for range 10 {
		if err := database.Migrate(ctx, f.owner, "down", logger); err != nil {
			t.Fatal(err)
		}
		if err := f.owner.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version < 18 {
			break
		}
	}
	if version >= 18 {
		t.Fatalf("down did not undo 00018 (now %d)", version)
	}
	for i, title := range []string{"Ödəniş gecikir", "Silinmiş ödəniş", "Başqa iş"} {
		it, err := f.st.CreateItem(ctx, store.CreateItemParams{WorkspaceID: f.wsID, ProjectID: f.project, ActorID: f.userID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if _, err := f.st.CreateComment(ctx, f.wsID, f.project, it.Number, f.userID, "ödənişlə bağlı qeyd"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := f.st.DeleteItem(ctx, f.wsID, f.project, 2, f.userID); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, f.owner, "up", logger); err != nil {
		t.Fatal(err)
	}
	var docs int
	if err := f.owner.QueryRow(ctx, `SELECT count(*) FROM search_docs`).Scan(&docs); err != nil || docs != 3 {
		t.Fatalf("backfilled %d docs (want 2 items + 1 comment): %v", docs, err)
	}
	hits := f.search(t, "odenis")
	if len(hits) != 2 || hits[0].ItemKey != "QA-1" || hits[1].Kind != "comment" || hits[1].ItemKey != "QA-3" {
		t.Fatalf("after backfill: %+v", hits)
	}
}
