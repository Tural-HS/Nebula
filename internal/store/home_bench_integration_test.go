package store_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/store"
)

// TestHomeLargeDataset is the home page benchmark of ADR-0037. Opt-in: it
// seeds 5 workspaces × 20 000 items (4 projects each, one private), with a
// heavy user (bob: 5% of all items assigned, 200 pending requests he may
// decide per workspace, 300 watched items per workspace) and a light one
// (carol: 20 items), then reports p50/p95 of the whole HomeFor batch and
// the plan of each query:
//
//	NEBULA_TEST_DATABASE_URL=... NEBULA_HOME_BENCH=1 go test -run TestHomeLargeDataset -v -timeout 30m ./internal/store/
func TestHomeLargeDataset(t *testing.T) {
	if os.Getenv("NEBULA_HOME_BENCH") == "" {
		t.Skip("set NEBULA_HOME_BENCH=1 to run the home page benchmark")
	}
	const perWS = 20_000
	f := newSearchFixture(t) // alice owns "acme"; it gets the same data as the others
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	user := func(name string) uuid.UUID {
		p, err := f.st.ResolveUser(ctx, auth.Claims{Issuer: "https://idp.test", Subject: "sub-" + name, Email: name + "@example.com", Name: name, EmailVerified: true})
		if err != nil {
			t.Fatal(err)
		}
		return p.UserID
	}
	bob, carol := user("bob"), user("carol")

	start := time.Now()
	wsIDs := []uuid.UUID{f.wsID}
	for i := 2; i <= 5; i++ {
		ws, err := f.st.CreateWorkspace(ctx, f.userID, fmt.Sprintf("ws%d", i), fmt.Sprintf("Workspace %d", i), "")
		if err != nil {
			t.Fatal(err)
		}
		wsIDs = append(wsIDs, ws.ID)
	}
	for _, ws := range wsIDs {
		for i := 1; i <= 4; i++ {
			vis := "workspace"
			if i == 4 {
				vis = "private"
			}
			if _, err := f.st.CreateProject(ctx, ws, f.userID, fmt.Sprintf("B%d", i), fmt.Sprintf("Layihə %d", i), nil, vis, store.ProjectTemplate{}); err != nil {
				t.Fatal(err)
			}
		}
		exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'member'), ($1, $3, 'member')`, ws, bob, carol)
		// Statuses cycle through the workflow (todo, in progress, done, canceled);
		// due dates spread over ±60 days; ~5% assigned to bob, 0.1% to carol.
		exec(`INSERT INTO items (workspace_id, project_id, item_type_id, status_id, number, title, reporter_id, assignee_id, due_date)
			SELECT $1, p.id, t.id, s.id, g / 4 + 1, 'Bench item ' || g, $2,
				CASE WHEN g % 19 = 0 THEN $3::uuid WHEN g % 997 = 1 THEN $4::uuid END,
				CASE WHEN g % 3 <> 0 THEN current_date + (g % 121 - 60) END
			FROM generate_series(0, $5 - 1) g
			JOIN LATERAL (SELECT id FROM projects WHERE workspace_id = $1 AND key LIKE 'B%' ORDER BY key OFFSET g % 4 LIMIT 1) p ON true
			JOIN LATERAL (SELECT id, workflow_id FROM item_types WHERE workspace_id = $1 ORDER BY key LIMIT 1) t ON true
			JOIN LATERAL (SELECT id FROM statuses WHERE workflow_id = t.workflow_id ORDER BY position OFFSET (g / 20) % 4 LIMIT 1) s ON true`,
			ws, f.userID, bob, carol, perWS)
		exec(`UPDATE items SET deleted_at = now() WHERE workspace_id = $1 AND number % 50 = 0`, ws)
		exec(`INSERT INTO approval_requests (id, workspace_id, item_id, requested_by, min_approvals, approver_roles, approver_user_ids, created_at, due_on)
			SELECT gen_random_uuid(), i.workspace_id, i.id, $2, 1, '{member}', '{}', now() - (i.number || ' minutes')::interval,
				CASE WHEN i.number % 2 = 0 THEN current_date + (i.number % 7 - 3)::int END
			FROM items i WHERE i.workspace_id = $1 AND i.deleted_at IS NULL AND i.number % 25 = 1 AND i.assignee_id IS NULL
			ORDER BY i.number LIMIT 200`, ws, f.userID)
		exec(`INSERT INTO item_watchers (workspace_id, item_id, user_id)
			SELECT i.workspace_id, i.id, $2 FROM items i WHERE i.workspace_id = $1 AND i.number % 13 = 0 LIMIT 300`, ws, bob)
	}
	exec(`ANALYZE`)
	var total, mine int
	_ = f.owner.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE assignee_id = $1) FROM items`, bob).Scan(&total, &mine)
	t.Logf("seeded %d items (%d assigned to bob) in 5 workspaces in %s", total, mine, time.Since(start).Round(time.Millisecond))

	report := []string{"| user | overdue / approvals / week / assigned | p50 | p95 |", "|---|---|---|---|"}
	for _, c := range []struct {
		name string
		id   uuid.UUID
		show string
	}{
		{"bob (heavy) attention", bob, store.HomeAttention},
		{"bob (heavy) show=assigned limit 200", bob, store.HomeAssigned},
		{"carol (light) attention", carol, store.HomeAttention},
		{"alice (owner, sees private) attention", f.userID, store.HomeAttention},
	} {
		limit := 15
		if c.show != store.HomeAttention {
			limit = store.MaxHomeList
		}
		var durs []time.Duration
		var h store.Home
		for i := range 45 {
			s := time.Now()
			var err error
			h, err = f.st.HomeFor(ctx, c.id, c.show, limit)
			if err != nil {
				t.Fatal(err)
			}
			if i >= 5 {
				durs = append(durs, time.Since(s))
			}
		}
		if c.id == bob {
			for _, it := range h.Items {
				if it.Project.Key == "B4" {
					t.Fatalf("private project listed for bob: %s", it.Key)
				}
			}
		}
		k := h.Counters
		report = append(report, fmt.Sprintf("| %s | %d / %d / %d / %d | %s | %s |", c.name, k.Overdue, k.Approvals, k.Week, k.Assigned, pct(durs, 50), pct(durs, 95)))
	}
	t.Log("\n" + strings.Join(report, "\n"))

	queries := store.HomeSQLForTest()
	args := map[string][]any{
		"cards": {bob}, "list": {bob, store.HomeAttention, 15}, "watching": {bob, 5}, "recent": {bob, 50},
	}
	names := make([]string, 0, len(queries))
	for k := range queries {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, name := range names {
		rows, err := f.owner.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) `+queries[name], args[name]...)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var line string
			_ = rows.Scan(&line)
			plan = append(plan, line)
		}
		rows.Close()
		t.Logf("EXPLAIN %s:\n%s", name, strings.Join(plan, "\n"))
	}
}
