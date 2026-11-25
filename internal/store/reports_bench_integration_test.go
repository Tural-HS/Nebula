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

// TestReportsLargeDataset is the reports benchmark of ADR-0041. Opt-in: one
// workspace with 100 000 items over two years in 5 projects (one private),
// ~40% of them completed through a status change, 300 000 events (status
// changes plus comment/field noise) and 10 000 closed approval requests;
// it reports p50/p95 of the whole ReportsFor batch for typical ranges and
// the plan of each query:
//
//	NEBULA_TEST_DATABASE_URL=... NEBULA_REPORTS_BENCH=1 go test -run TestReportsLargeDataset -v -timeout 30m ./internal/store/
func TestReportsLargeDataset(t *testing.T) {
	if os.Getenv("NEBULA_REPORTS_BENCH") == "" {
		t.Skip("set NEBULA_REPORTS_BENCH=1 to run the reports benchmark")
	}
	const n = 100_000
	f := newSearchFixture(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	bob, err := f.st.ResolveUser(ctx, auth.Claims{Issuer: "https://idp.test", Subject: "sub-bob", Email: "bob@example.com", Name: "Bob", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 1; i <= 4; i++ {
		vis := "workspace"
		if i == 4 {
			vis = "private"
		}
		if _, err := f.st.CreateProject(ctx, f.wsID, f.userID, fmt.Sprintf("R%d", i), fmt.Sprintf("Layihə %d", i), nil, vis, store.ProjectTemplate{}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'member')`, f.wsID, bob.UserID)
	// 20 synthetic assignees.
	exec(`INSERT INTO users (oidc_issuer, oidc_subject, email, display_name)
		SELECT 'https://idp.test', 'bench-' || g, 'bench' || g || '@example.com', 'İcraçı ' || g FROM generate_series(1, 20) g`)
	exec(`INSERT INTO workspace_members (workspace_id, user_id, role)
		SELECT $1, id, 'member' FROM users WHERE oidc_subject LIKE 'bench-%'`, f.wsID)
	// Items spread over the last 730 days; status cycles todo, in progress,
	// done, canceled; ~2/3 have a due date within ±90 days of creation.
	exec(`INSERT INTO items (workspace_id, project_id, item_type_id, status_id, number, title, reporter_id, assignee_id, due_date, created_at)
		SELECT $1, p.id, t.id, s.id, g / 5 + 1, 'Bench item ' || g, $2, u.id,
			CASE WHEN g % 3 <> 0 THEN (now() - (g % 730 || ' days')::interval)::date + (g % 181 - 90) END,
			now() - (g % 730 || ' days')::interval - (g % 1440 || ' minutes')::interval
		FROM generate_series(0, $3 - 1) g
		JOIN LATERAL (SELECT id FROM projects WHERE workspace_id = $1 ORDER BY key OFFSET g % 5 LIMIT 1) p ON true
		JOIN LATERAL (SELECT id, workflow_id FROM item_types WHERE workspace_id = $1 ORDER BY key LIMIT 1) t ON true
		JOIN LATERAL (SELECT id FROM statuses WHERE workflow_id = t.workflow_id ORDER BY position OFFSET (g / 7) % 4 LIMIT 1) s ON true
		LEFT JOIN LATERAL (SELECT id FROM users WHERE oidc_subject = 'bench-' || (g % 21) LIMIT 1) u ON true`,
		f.wsID, f.userID, n)
	exec(`UPDATE items SET deleted_at = now() WHERE workspace_id = $1 AND number % 97 = 0`, f.wsID)
	// One completion event per done item, a few days after creation, and
	// two noise events (a field change, a comment) per item.
	exec(`INSERT INTO events (workspace_id, actor_id, entity_type, entity_id, action, payload, occurred_at)
		SELECT i.workspace_id, $2, 'item', i.id, 'item.updated',
			jsonb_build_object('changes', jsonb_build_object('status_id', jsonb_build_object('from', null, 'to', s.id)),
				'snapshot', jsonb_build_object('status_category', s.category)),
			LEAST(now(), i.created_at + ((i.number % 20) || ' days')::interval)
		FROM items i JOIN statuses s ON s.id = i.status_id WHERE i.workspace_id = $1 AND s.category IN ('done', 'in_progress')`, f.wsID, f.userID)
	exec(`INSERT INTO events (workspace_id, actor_id, entity_type, entity_id, action, payload, occurred_at)
		SELECT i.workspace_id, $2, 'item', i.id, a, '{"changes": {"title": {"from": "a", "to": "b"}}}',
			LEAST(now(), i.created_at + ((i.number % 9) || ' hours')::interval)
		FROM items i CROSS JOIN unnest(ARRAY['item.updated', 'comment.created']) a WHERE i.workspace_id = $1`, f.wsID, f.userID)
	exec(`INSERT INTO approval_requests (id, workspace_id, item_id, requested_by, min_approvals, approver_roles, approver_user_ids,
			to_status_id, state, created_at, closed_at, due_on)
		SELECT gen_random_uuid(), i.workspace_id, i.id, $2, 1, '{member}', '{}', i.status_id,
			(ARRAY['approved', 'rejected', 'canceled'])[i.number % 3 + 1], i.created_at, i.created_at + ((i.number % 72) || ' hours')::interval,
			CASE WHEN i.number % 2 = 0 THEN i.created_at::date + 1 END
		FROM items i JOIN statuses s ON s.id = i.status_id WHERE i.workspace_id = $1 AND s.category = 'done' AND i.number % 4 = 0`, f.wsID, f.userID)
	exec(`ANALYZE`)
	var items, events, reqs int
	_ = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM items), (SELECT count(*) FROM events), (SELECT count(*) FROM approval_requests)`).Scan(&items, &events, &reqs)
	t.Logf("seeded %d items, %d events, %d approval requests in %s", items, events, reqs, time.Since(start).Round(time.Millisecond))

	today := time.Now().UTC()
	day := func(d int) string { return today.AddDate(0, 0, d).Format(time.DateOnly) }
	queries := store.ReportsSQLForTest()
	args := []any{f.wsID, f.userID, true, nil, nil, false, "", day(-89), day(0)}
	names := slices.Sorted(func(yield func(string) bool) {
		for k := range queries {
			if !yield(k) {
				return
			}
		}
	})
	for _, name := range names {
		a := args
		if name == "throughput" {
			a = append(slices.Clone(args), "week")
		}
		rows, err := f.owner.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS, COSTS OFF) `+queries[name], a...)
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
		t.Logf("EXPLAIN %s (owner, 90 days):\n%s", name, strings.Join(plan, "\n"))
	}
	report := []string{"| case | open / created / completed | p50 | p95 |", "|---|---|---|---|"}
	type benchCase struct {
		name string
		p    store.ReportsParams
	}
	owner := store.Viewer{UserID: f.userID, Role: store.RoleOwner}
	member := store.Viewer{UserID: bob.UserID, Role: store.RoleMember}
	var someone uuid.UUID
	_ = f.owner.QueryRow(ctx, `SELECT id FROM users WHERE oidc_subject = 'bench-3'`).Scan(&someone)
	for _, c := range []benchCase{
		{"owner, last 30 days", store.ReportsParams{Viewer: owner, From: day(-29), To: day(0)}},
		{"owner, last 90 days", store.ReportsParams{Viewer: owner, From: day(-89), To: day(0)}},
		{"owner, 2 years", store.ReportsParams{Viewer: owner, From: day(-730), To: day(0)}},
		{"member (no private), 2 years", store.ReportsParams{Viewer: member, From: day(-730), To: day(0)}},
		{"owner, 1 project, 1 assignee, 90 days", store.ReportsParams{Viewer: owner, From: day(-89), To: day(0), Projects: []string{"R2"}, AssigneeID: &someone}},
	} {
		c.p.WorkspaceID = f.wsID
		var durs []time.Duration
		var r store.Reports
		for i := range 25 {
			s := time.Now()
			var err error
			if r, err = f.st.ReportsFor(ctx, c.p); err != nil {
				t.Fatal(err)
			}
			if i >= 5 {
				durs = append(durs, time.Since(s))
			}
		}
		if c.p.Viewer.UserID == member.UserID && c.p.Viewer.Role == member.Role {
			for _, s := range r.Status {
				if s.Key == "R4" {
					t.Fatal("private project counted for a member")
				}
			}
		}
		report = append(report, fmt.Sprintf("| %s | %d / %d / %d | %s | %s |", c.name, r.KPIs.Open, r.KPIs.Created, r.KPIs.Completed, pct(durs, 50), pct(durs, 95)))
	}
	t.Log("\n" + strings.Join(report, "\n"))

}
