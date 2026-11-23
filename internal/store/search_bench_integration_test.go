package store_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/database"
	"github.com/Tural-HS/Nebula/internal/store"
)

// TestSearchLargeDataset is the search benchmark of ADR-0024. Opt-in, it
// seeds a throwaway database (dropped afterwards) with
// NEBULA_SEARCH_BENCH_ITEMS items (default 100 000) and twice as many
// comments in 20 projects (2 private), measures the 00018 backfill, the
// trigger cost on writes and the latency of typical queries as a member who
// cannot see the private projects:
//
//	NEBULA_TEST_DATABASE_URL=... NEBULA_SEARCH_BENCH=1 go test -run TestSearchLargeDataset -v -timeout 30m ./internal/store/
func TestSearchLargeDataset(t *testing.T) {
	if os.Getenv("NEBULA_SEARCH_BENCH") == "" {
		t.Skip("set NEBULA_SEARCH_BENCH=1 to run the search benchmark")
	}
	n := 100_000
	if v := os.Getenv("NEBULA_SEARCH_BENCH_ITEMS"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1000 {
			t.Fatalf("NEBULA_SEARCH_BENCH_ITEMS=%q", v)
		}
	}
	f := newSearchFixture(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}

	// A member (not admin) who is not in the private projects.
	bob, err := f.st.ResolveUser(ctx, auth.Claims{Issuer: "https://idp.test", Subject: "sub-bob", Email: "bob@example.com", Name: "Bob", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, f.wsID, bob.UserID)
	for i := 2; i <= 20; i++ {
		vis := "workspace"
		if i > 18 {
			vis = "private"
		}
		if _, err := f.st.CreateProject(ctx, f.wsID, f.userID, fmt.Sprintf("P%d", i), fmt.Sprintf("Layihə %d", i), nil, vis); err != nil {
			t.Fatal(err)
		}
	}

	// Seed without the search triggers, as a database that predates 00018.
	if err := database.Migrate(ctx, f.owner, "down", logger); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	// Zipf-like vocabulary: early words are common, "zümrüd" (rare) is in
	// about 0.1% of titles.
	exec(`CREATE TABLE bench_vocab AS SELECT array[
		'ödəniş','hesabat','müştəri','kredit','faiz','müqavilə','server','giriş','xəta','sənəd',
		'anbar','sifariş','çatdırılma','büdcə','işçi','məzuniyyət','layihə','görüş','yoxlama','təhlükəsizlik',
		'şöbə','qəbul','imza','arxiv','proqram','yeniləmə','miqrasiya','verilənlər','bazası','şəbəkə',
		'sığorta','lizinq','girov','qrafik','komissiya','balans','köçürmə','kart','terminal','filial'] AS w`)
	exec(`CREATE FUNCTION bench_word() RETURNS text LANGUAGE sql VOLATILE
		AS $$ SELECT w[1 + floor(power(random(), 2) * 40)::int] FROM bench_vocab $$`)
	word := `bench_word()`
	exec(fmt.Sprintf(`INSERT INTO items (workspace_id, project_id, item_type_id, status_id, number, title, description, reporter_id)
		SELECT $1, p.id, t.id, s.id, g / 20 + 1,
			(SELECT string_agg(%[1]s, ' ') FROM generate_series(1, 3 + g %% 4) WHERE g >= 0)
				|| CASE WHEN g %% 1000 = 0 THEN ' zümrüd' ELSE '' END,
			(SELECT string_agg(%[1]s, ' ') FROM generate_series(1, 20 + g %% 40) WHERE g >= 0),
			$2
		FROM generate_series(0, $3 - 1) g
		JOIN LATERAL (SELECT id FROM projects WHERE workspace_id = $1 ORDER BY key OFFSET g %% 20 LIMIT 1) p ON true
		JOIN LATERAL (SELECT id, workflow_id FROM item_types WHERE workspace_id = $1 ORDER BY key LIMIT 1) t ON true
		JOIN LATERAL (SELECT id FROM statuses WHERE workflow_id = t.workflow_id ORDER BY position LIMIT 1) s ON true`, word),
		f.wsID, f.userID, n)
	exec(`UPDATE projects p SET next_item_number = (SELECT coalesce(max(number), 0) + 1 FROM items WHERE project_id = p.id) WHERE workspace_id = $1`, f.wsID)
	exec(fmt.Sprintf(`INSERT INTO comments (workspace_id, item_id, author_id, body)
		SELECT i.workspace_id, i.id, $2, (SELECT string_agg(%s, ' ') FROM generate_series(1, 10 + k * 7) WHERE k > 0)
		FROM items i CROSS JOIN generate_series(1, 2) k WHERE i.workspace_id = $1`, word), f.wsID, f.userID)
	exec(`UPDATE items SET deleted_at = now() WHERE workspace_id = $1 AND number % 50 = 0`, f.wsID) // 2% deleted
	exec(`ANALYZE`)
	t.Logf("seeded %d items, %d comments in %s", n, 2*n, time.Since(start).Round(time.Millisecond))

	createItems := func(label string) {
		t.Helper()
		start := time.Now()
		for i := range 1000 {
			desc := "Yeni müştəri ödəniş sənədi " + strconv.Itoa(i)
			if _, err := f.st.CreateItem(ctx, store.CreateItemParams{WorkspaceID: f.wsID, ProjectID: f.project, ActorID: f.userID,
				Title: "Bench ödəniş " + strconv.Itoa(i), Description: &desc}); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("CreateItem %s: %s per item (1000 sequential)", label, (time.Since(start) / 1000).Round(time.Microsecond))
	}
	createItems("without search triggers")

	start = time.Now()
	if err := database.Migrate(ctx, f.owner, "up", logger); err != nil {
		t.Fatal(err)
	}
	var docs int
	var size string
	if err := f.owner.QueryRow(ctx, `SELECT count(*), pg_size_pretty(pg_total_relation_size('search_docs')) FROM search_docs`).Scan(&docs, &size); err != nil {
		t.Fatal(err)
	}
	t.Logf("migration 00018 (backfill) took %s: %d documents, %s with indexes", time.Since(start).Round(time.Millisecond), docs, size)

	createItems("with search triggers")
	exec(`ANALYZE search_docs`)

	v := store.Viewer{UserID: bob.UserID, Role: store.RoleMember}
	queries := []struct{ name, q string }{
		{"rare word", "zümrüd"},
		{"common word", "ödəniş"},
		{"folded ascii", "odenis"},
		{"two words", "müştəri kredit"},
		{"prefix 3", "kom"},
		{"prefix 2 (broadest)", "ba"},
		{"exact key", "P7-120"},
		{"typo (fuzzy)", "zumrut"},
		{"no match", "qwertyuiop"},
	}
	var all []time.Duration
	report := []string{"| query | matches (items/comments) | p50 | p95 |", "|---|---|---|---|"}
	for _, qc := range queries {
		var durs []time.Duration
		var counts store.SearchCounts
		for i := range 40 {
			s := time.Now()
			res, err := f.st.Search(ctx, store.SearchParams{WorkspaceID: f.wsID, Viewer: v, Query: qc.q, Limit: 20})
			d := time.Since(s)
			if err != nil {
				t.Fatalf("%s: %v", qc.name, err)
			}
			for _, h := range res.Hits {
				if strings.HasPrefix(h.ItemKey, "P19-") || strings.HasPrefix(h.ItemKey, "P20-") {
					t.Fatalf("%s: private hit %s", qc.name, h.ItemKey)
				}
			}
			if i == 0 && res.Counts != nil {
				counts = *res.Counts
			}
			if i >= 5 { // warm-up
				durs = append(durs, d)
			}
		}
		all = append(all, durs...)
		report = append(report, fmt.Sprintf("| %s `%s` | %d / %d | %s | %s |", qc.name, qc.q, counts.Items, counts.Comments,
			pct(durs, 50), pct(durs, 95)))
	}
	// Paging deep into the broadest common query.
	var durs []time.Duration
	var cur *store.SearchCursor
	for range 10 {
		s := time.Now()
		res, err := f.st.Search(ctx, store.SearchParams{WorkspaceID: f.wsID, Viewer: v, Query: "ödəniş", Limit: 20, Cursor: cur})
		durs = append(durs, time.Since(s))
		if err != nil || res.Next == nil {
			t.Fatalf("paging: %v", err)
		}
		cur = res.Next
	}
	report = append(report, fmt.Sprintf("| next pages of `ödəniş` (10 pages) | | %s | %s |", pct(durs, 50), pct(durs, 95)))
	report = append(report, fmt.Sprintf("| **all queries** | | %s | %s |", pct(all, 50), pct(all, 95)))
	t.Log("\n" + strings.Join(report, "\n"))

	var plan []string
	rows, err := f.owner.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS, COSTS OFF)
		SELECT d.id FROM search_docs d, to_tsquery('simple', search_fold('zumrud:*')) q
		WHERE d.workspace_id = $1 AND d.doc @@ q ORDER BY ts_rank(d.doc, q, 1) DESC LIMIT 21`, f.wsID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var line string
		_ = rows.Scan(&line)
		plan = append(plan, line)
	}
	rows.Close()
	if !strings.Contains(strings.Join(plan, "\n"), "search_docs_doc_idx") {
		t.Errorf("rare-word query does not use the GIN index:\n%s", strings.Join(plan, "\n"))
	}
}

func pct(d []time.Duration, p int) string {
	s := slices.Clone(d)
	slices.Sort(s)
	i := min(len(s)-1, (len(s)*p+99)/100-1)
	return s[max(i, 0)].Round(100 * time.Microsecond).String()
}
