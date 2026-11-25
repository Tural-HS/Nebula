package store

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Workspace reports "Hesabatlar" (ADR-0041): KPI tiles, throughput, aging,
// approval cycle time, workload and status distribution for one workspace,
// in one round trip (a pgx batch). Every query starts from the same scope
// CTEs: the workspace's local calendar (ws), the projects the viewer can
// see (vis, visibleProjectFor, ADR-0013) narrowed by the project filter,
// and the live items of those projects matching the assignee and type
// filters (it). Nothing outside vis is ever counted.
//
// "Now" numbers (open, overdue, awaiting approval, aging, workload open /
// overdue, status distribution) describe the current state; "in period"
// numbers (created, completed, approval decisions) use [from, to] as local
// dates of the workspace time zone. The SQL below is the definition the
// ADR documents; keep the two in sync.

const (
	// MaxReportRangeDays caps from..to: two years (a leap day included).
	MaxReportRangeDays = 731
	// reportWeeklyDays: longer ranges are bucketed by month, so a chart
	// never has more than ~27 weekly or 25 monthly bars.
	reportWeeklyDays = 186
	// MaxReportProjects bounds the project filter.
	MaxReportProjects = 50
)

var reportTypeKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// ReportsParams selects what a report covers.
type ReportsParams struct {
	WorkspaceID uuid.UUID
	Viewer      Viewer
	From, To    string     // YYYY-MM-DD, inclusive, workspace-local dates
	Projects    []string   // project keys; empty = every visible project
	AssigneeID  *uuid.UUID // current assignee
	Unassigned  bool       // only items without an assignee
	TypeKey     string     // item type key; "" = every type
}

// ReportRange echoes the period and the calendar the numbers use.
type ReportRange struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Today    string `json:"today"`
	Timezone string `json:"timezone"`
	Bucket   string `json:"bucket"` // "week" (Monday start) or "month"
}

// ReportProject is a project in scope.
type ReportProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ReportKPIs are the tiles. Created and Completed are the sums of the
// throughput series.
type ReportKPIs struct {
	Open                 int `json:"open"`
	Created              int `json:"created"`
	Completed            int `json:"completed"`
	Overdue              int `json:"overdue"`
	AwaitingApproval     int `json:"awaiting_approval"`
	AwaitingApprovalLate int `json:"awaiting_approval_late"` // past the request's deadline (ADR-0034)
}

// ReportBucket is one bar pair of the throughput chart.
type ReportBucket struct {
	Start     string `json:"start"` // first day of the week/month (may precede From)
	Created   int    `json:"created"`
	Completed int    `json:"completed"`
}

// ReportCount is one age bucket.
type ReportCount struct {
	Bucket string `json:"bucket"` // "0-7", "8-30", "31-90", "91+" (late: "1-7" first)
	Count  int    `json:"count"`
}

// ReportAging holds open items by age (days since creation) and overdue items by
// days past the due date.
type ReportAging struct {
	Age  []ReportCount `json:"age"`
	Late []ReportCount `json:"late"`
}

// ReportStatusRef names an approval request's target status.
type ReportStatusRef struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Workflow string    `json:"workflow"`
}

// ReportApprovalRow counts requests closed in the period, for one target status
// (the transition an approval policy guards) or in total. Median and P90
// are seconds from request to decision over approved + rejected requests;
// nil when there were none.
type ReportApprovalRow struct {
	ToStatus      *ReportStatusRef `json:"to_status"` // nil: the total row, or a deleted status
	Approved      int              `json:"approved"`
	Rejected      int              `json:"rejected"`
	Canceled      int              `json:"canceled"`
	Late          int              `json:"late"` // decided after the request's deadline
	MedianSeconds *float64         `json:"median_seconds"`
	P90Seconds    *float64         `json:"p90_seconds"`
}

// ReportApprovals is the approval cycle widget.
type ReportApprovals struct {
	Total        ReportApprovalRow   `json:"total"`
	ByTransition []ReportApprovalRow `json:"by_transition"`
}

// ReportWorkCounts are open / overdue now and completed in the period.
type ReportWorkCounts struct {
	Open      int `json:"open"`
	Overdue   int `json:"overdue"`
	Completed int `json:"completed"`
}

// ReportAssigneeProject is one assignee's share of one project.
type ReportAssigneeProject struct {
	Key string `json:"key"`
	ReportWorkCounts
}

// ReportAssignee is a workload row; AssigneeID nil = unassigned items.
type ReportAssignee struct {
	AssigneeID *uuid.UUID `json:"assignee_id"`
	Name       *string    `json:"name"`
	ReportWorkCounts
	Projects []ReportAssigneeProject `json:"projects"`
}

// ReportStatusCount is one status of a project's distribution.
type ReportStatusCount struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Category string    `json:"category"`
	Count    int       `json:"count"`
}

// ReportProjectStatus is one project's bar of the status distribution.
type ReportProjectStatus struct {
	Key      string              `json:"key"`
	Name     string              `json:"name"`
	Total    int                 `json:"total"`
	Statuses []ReportStatusCount `json:"statuses"`
}

// Reports is the whole page.
type Reports struct {
	Range      ReportRange           `json:"range"`
	Projects   []ReportProject       `json:"projects"`
	KPIs       ReportKPIs            `json:"kpis"`
	Throughput []ReportBucket        `json:"throughput"`
	Aging      ReportAging           `json:"aging"`
	Approvals  ReportApprovals       `json:"approvals"`
	Workload   []ReportAssignee      `json:"workload"`
	Status     []ReportProjectStatus `json:"status"`
}

// ReportDates validates a from..to range: real dates, from <= to, at most
// MaxReportRangeDays days.
func ReportDates(from, to string) (time.Time, time.Time, error) {
	f, err := time.Parse(time.DateOnly, from)
	if err != nil || f.Year() < 1900 || f.Year() > 2200 {
		return time.Time{}, time.Time{}, invalid("from", "must be a date YYYY-MM-DD")
	}
	t, err := time.Parse(time.DateOnly, to)
	if err != nil || t.Year() < 1900 || t.Year() > 2200 {
		return time.Time{}, time.Time{}, invalid("to", "must be a date YYYY-MM-DD")
	}
	if t.Before(f) {
		return time.Time{}, time.Time{}, invalid("to", "must not be before from")
	}
	if days := int(t.Sub(f).Hours()/24) + 1; days > MaxReportRangeDays {
		return time.Time{}, time.Time{}, invalid("to", "the range may cover at most %d days", MaxReportRangeDays)
	}
	return f, t, nil
}

// reportItemFilter: the items a report covers (alias i): live, in a
// project of vis, matching the assignee and type filters.
const reportItemFilter = `i.workspace_id = $1 AND i.deleted_at IS NULL
		  AND i.project_id IN (SELECT vis.id FROM vis)
		  AND ($5::uuid IS NULL OR i.assignee_id = $5::uuid)
		  AND (NOT $6::boolean OR i.assignee_id IS NULL)
		  AND ($7::text = '' OR i.item_type_id = (SELECT t.id FROM item_types t WHERE t.workspace_id = $1 AND t.key = $7::text))`

// reportScope. Parameters: $1 workspace, $2 viewer, $3 viewer is admin,
// $4 project keys (NULL = all), $5 assignee (NULL = any), $6 unassigned
// only, $7 type key (empty = any), $8 from, $9 to.
//
//	ws:   the workspace's local today and the period as [lo, hi) instants
//	vis:  active projects the viewer can see, narrowed by the filter
//	it:   live items of vis matching the filters (reportItemFilter);
//	      open = todo/in progress. Inlined, never materialized: each query
//	      scans or probes items as its own plan needs
//	done: per item in it, its last move into a done-category status within
//	      the period (item.updated events whose changes include status_id
//	      and whose snapshot says done). Items created directly in a done
//	      status (imports included) were not completed in Nebula: not counted.
var reportScope = `WITH ws AS MATERIALIZED (
		SELECT w.timezone, (now() AT TIME ZONE w.timezone)::date AS today,
			$8::date::timestamp AT TIME ZONE w.timezone AS lo,
			($9::date + 1)::timestamp AT TIME ZONE w.timezone AS hi
		FROM workspaces w WHERE w.id = $1),
	vis AS MATERIALIZED (
		SELECT p.id, p.key, p.name FROM projects p
		WHERE p.workspace_id = $1 AND p.archived_at IS NULL
		  AND ($4::text[] IS NULL OR p.key = ANY ($4::text[]))
		  AND ` + visibleProjectFor("p", "$2", "$3") + `),
	it AS NOT MATERIALIZED (
		SELECT i.id, i.project_id, i.assignee_id, i.status_id, i.due_date, i.created_at,
			(i.created_at AT TIME ZONE ws.timezone)::date AS created_on,
			s.category IN ('todo', 'in_progress') AS open
		FROM items i
		JOIN statuses s ON s.id = i.status_id
		CROSS JOIN ws
		WHERE ` + reportItemFilter + `),
	done AS MATERIALIZED (
		SELECT DISTINCT ON (e.entity_id) e.entity_id AS item_id, e.occurred_at AS at
		FROM events e
		CROSS JOIN ws
		JOIN items i ON i.id = e.entity_id
		WHERE e.workspace_id = $1 AND e.entity_type = 'item' AND e.action = 'item.updated'
		  AND e.occurred_at >= ws.lo AND e.occurred_at < ws.hi
		  AND e.payload->'changes' ? 'status_id'
		  AND e.payload->'snapshot'->>'status_category' = 'done'
		  AND ` + reportItemFilter + `
		ORDER BY e.entity_id, e.occurred_at DESC, e.id DESC)`

// reportKPISQL: the "now" tiles and the aging buckets in one row.
var reportKPISQL = reportScope + `
	SELECT
		count(*) FILTER (WHERE it.open),
		count(*) FILTER (WHERE it.open AND it.due_date < ws.today),
		count(*) FILTER (WHERE it.open AND ws.today - it.created_on <= 7),
		count(*) FILTER (WHERE it.open AND ws.today - it.created_on BETWEEN 8 AND 30),
		count(*) FILTER (WHERE it.open AND ws.today - it.created_on BETWEEN 31 AND 90),
		count(*) FILTER (WHERE it.open AND ws.today - it.created_on > 90),
		count(*) FILTER (WHERE it.open AND ws.today - it.due_date BETWEEN 1 AND 7),
		count(*) FILTER (WHERE it.open AND ws.today - it.due_date BETWEEN 8 AND 30),
		count(*) FILTER (WHERE it.open AND ws.today - it.due_date BETWEEN 31 AND 90),
		count(*) FILTER (WHERE it.open AND ws.today - it.due_date > 90),
		(SELECT count(*) FROM approval_requests r JOIN it a ON a.id = r.item_id
			WHERE r.workspace_id = $1 AND r.state = 'pending'),
		(SELECT count(*) FROM approval_requests r JOIN it a ON a.id = r.item_id
			WHERE r.workspace_id = $1 AND r.state = 'pending' AND r.due_on < ws.today),
		ws.today::text, ws.timezone
	FROM ws LEFT JOIN it ON true
	GROUP BY ws.today, ws.timezone`

// reportThroughputSQL: $10 is the bucket ('week' or 'month'). created =
// items of it created in the period; completed = rows of done. Buckets
// cover the period only (a partial first or last week counts its days in
// the period).
var reportThroughputSQL = reportScope + `,
	b AS (SELECT g::date AS start FROM generate_series(date_trunc($10::text, $8::date::timestamp),
			$9::date::timestamp, ('1 ' || $10::text)::interval) g),
	c AS (SELECT date_trunc($10::text, it.created_on::timestamp)::date AS start, count(*) AS n
		FROM it CROSS JOIN ws WHERE it.created_at >= ws.lo AND it.created_at < ws.hi GROUP BY 1),
	d AS (SELECT date_trunc($10::text, (done.at AT TIME ZONE ws.timezone))::date AS start, count(*) AS n
		FROM done CROSS JOIN ws GROUP BY 1)
	SELECT b.start::text, COALESCE(c.n, 0), COALESCE(d.n, 0)
	FROM b LEFT JOIN c USING (start) LEFT JOIN d USING (start)
	ORDER BY b.start`

// reportApprovalsSQL: requests on items of it closed in the period, per
// target status and in total (GROUPING SETS; grouping() = 1 is the total).
// Cycle time = closed_at - created_at of approved and rejected requests;
// canceled requests have no decision and no cycle time. late = decided on a
// local date after the request's deadline (due_on).
var reportApprovalsSQL = reportScope + `
	SELECT grouping(r.to_status_id), r.to_status_id, s.name, wf.name,
		count(*) FILTER (WHERE r.state = 'approved'),
		count(*) FILTER (WHERE r.state = 'rejected'),
		count(*) FILTER (WHERE r.state = 'canceled'),
		count(*) FILTER (WHERE r.state IN ('approved', 'rejected') AND r.due_on IS NOT NULL
			AND (r.closed_at AT TIME ZONE ws.timezone)::date > r.due_on),
		percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM r.closed_at - r.created_at))
			FILTER (WHERE r.state IN ('approved', 'rejected')),
		percentile_cont(0.9) WITHIN GROUP (ORDER BY extract(epoch FROM r.closed_at - r.created_at))
			FILTER (WHERE r.state IN ('approved', 'rejected'))
	FROM approval_requests r
	JOIN it ON it.id = r.item_id
	CROSS JOIN ws
	LEFT JOIN statuses s ON s.id = r.to_status_id
	LEFT JOIN workflows wf ON wf.id = s.workflow_id
	WHERE r.workspace_id = $1 AND r.state <> 'pending' AND r.closed_at >= ws.lo AND r.closed_at < ws.hi
	GROUP BY GROUPING SETS ((r.to_status_id, s.name, wf.name), ())
	ORDER BY grouping(r.to_status_id), wf.name, s.name, r.to_status_id`

// reportWorkloadSQL: per current assignee and project, open and overdue
// items now and items of done.
var reportWorkloadSQL = reportScope + `
	SELECT it.assignee_id, u.display_name, vis.key,
		count(*) FILTER (WHERE it.open),
		count(*) FILTER (WHERE it.open AND it.due_date < ws.today),
		count(done.item_id)
	FROM it
	JOIN vis ON vis.id = it.project_id
	CROSS JOIN ws
	LEFT JOIN done ON done.item_id = it.id
	LEFT JOIN users u ON u.id = it.assignee_id
	WHERE it.open OR done.item_id IS NOT NULL
	GROUP BY it.assignee_id, u.display_name, vis.key
	ORDER BY u.display_name NULLS LAST, it.assignee_id, vis.key`

// reportStatusSQL: every item of it by project and current status.
var reportStatusSQL = reportScope + `
	SELECT vis.key, vis.name, s.id, s.name, s.category, count(*)
	FROM it
	JOIN vis ON vis.id = it.project_id
	JOIN statuses s ON s.id = it.status_id
	GROUP BY vis.key, vis.name, s.id, s.name, s.category, s.position
	ORDER BY vis.key, array_position(ARRAY['todo', 'in_progress', 'done', 'canceled'], s.category), s.position, s.name`

// reportProjectsSQL: the projects in scope.
var reportProjectsSQL = reportScope + `
	SELECT key, name FROM vis ORDER BY key`

// ReportsFor computes the reports page. Validation errors are
// *ValidationError.
func (s *Store) ReportsFor(ctx context.Context, p ReportsParams) (Reports, error) {
	from, to, err := ReportDates(p.From, p.To)
	if err != nil {
		return Reports{}, err
	}
	if len(p.Projects) > MaxReportProjects {
		return Reports{}, invalid("projects", "at most %d projects", MaxReportProjects)
	}
	for _, k := range p.Projects {
		if !projectKeyRe.MatchString(k) {
			return Reports{}, invalid("projects", "must be a list of project keys")
		}
	}
	if p.TypeKey != "" && !reportTypeKeyRe.MatchString(p.TypeKey) {
		return Reports{}, invalid("type", "must be an item type key")
	}
	if p.Unassigned && p.AssigneeID != nil {
		return Reports{}, invalid("assignee", "choose a user or unassigned, not both")
	}
	bucket := "week"
	if int(to.Sub(from).Hours()/24)+1 > reportWeeklyDays {
		bucket = "month"
	}
	var keys []string // nil = every project
	if len(p.Projects) > 0 {
		keys = slices.Compact(slices.Sorted(slices.Values(p.Projects)))
	}
	args := []any{p.WorkspaceID, p.Viewer.UserID, RoleAtLeast(p.Viewer.Role, RoleAdmin), keys,
		p.AssigneeID, p.Unassigned, p.TypeKey, p.From, p.To}

	out := Reports{
		Range:      ReportRange{From: p.From, To: p.To, Bucket: bucket},
		Projects:   []ReportProject{},
		Throughput: []ReportBucket{},
		Approvals:  ReportApprovals{ByTransition: []ReportApprovalRow{}},
		Workload:   []ReportAssignee{},
		Status:     []ReportProjectStatus{},
	}
	var age, late [4]int
	batch := &pgx.Batch{}
	batch.Queue(`SET LOCAL plan_cache_mode = force_custom_plan`)
	batch.Queue(reportKPISQL, args...).QueryRow(func(r pgx.Row) error {
		k := &out.KPIs
		return r.Scan(&k.Open, &k.Overdue, &age[0], &age[1], &age[2], &age[3], &late[0], &late[1], &late[2], &late[3],
			&k.AwaitingApproval, &k.AwaitingApprovalLate, &out.Range.Today, &out.Range.Timezone)
	})
	batch.Queue(reportThroughputSQL, append(slices.Clone(args), bucket)...).Query(func(rows pgx.Rows) error {
		var err error
		out.Throughput, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (ReportBucket, error) {
			var b ReportBucket
			return b, r.Scan(&b.Start, &b.Created, &b.Completed)
		})
		return err
	})
	batch.Queue(reportApprovalsSQL, args...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var total int
			var id *uuid.UUID
			var name, wf *string
			var row ReportApprovalRow
			if err := rows.Scan(&total, &id, &name, &wf, &row.Approved, &row.Rejected, &row.Canceled, &row.Late,
				&row.MedianSeconds, &row.P90Seconds); err != nil {
				return err
			}
			if total == 1 {
				out.Approvals.Total = row
				continue
			}
			if id != nil && name != nil {
				row.ToStatus = &ReportStatusRef{ID: *id, Name: *name, Workflow: deref(wf)}
			}
			out.Approvals.ByTransition = append(out.Approvals.ByTransition, row)
		}
		return rows.Err()
	})
	batch.Queue(reportWorkloadSQL, args...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var id *uuid.UUID
			var name *string
			var pr ReportAssigneeProject
			if err := rows.Scan(&id, &name, &pr.Key, &pr.Open, &pr.Overdue, &pr.Completed); err != nil {
				return err
			}
			n := len(out.Workload)
			if n == 0 || !equalPtr(out.Workload[n-1].AssigneeID, id) {
				out.Workload = append(out.Workload, ReportAssignee{AssigneeID: id, Name: name, Projects: []ReportAssigneeProject{}})
				n++
			}
			a := &out.Workload[n-1]
			a.Open += pr.Open
			a.Overdue += pr.Overdue
			a.Completed += pr.Completed
			a.Projects = append(a.Projects, pr)
		}
		return rows.Err()
	})
	batch.Queue(reportStatusSQL, args...).Query(func(rows pgx.Rows) error {
		for rows.Next() {
			var key, pname string
			var st ReportStatusCount
			if err := rows.Scan(&key, &pname, &st.ID, &st.Name, &st.Category, &st.Count); err != nil {
				return err
			}
			n := len(out.Status)
			if n == 0 || out.Status[n-1].Key != key {
				out.Status = append(out.Status, ReportProjectStatus{Key: key, Name: pname, Statuses: []ReportStatusCount{}})
				n++
			}
			ps := &out.Status[n-1]
			ps.Total += st.Count
			ps.Statuses = append(ps.Statuses, st)
		}
		return rows.Err()
	})
	batch.Queue(reportProjectsSQL, args...).Query(func(rows pgx.Rows) error {
		var err error
		out.Projects, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (ReportProject, error) {
			var rp ReportProject
			return rp, r.Scan(&rp.Key, &rp.Name)
		})
		return err
	})
	// One read-only REPEATABLE READ snapshot: every widget counts the same
	// state. Custom plans only: the filters are "param IS NULL OR ..."
	// switches, and a generic plan built for all of them at once scanned
	// the event log per item (13 s instead of 60 ms on the benchmark).
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Reports{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return Reports{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reports{}, err
	}

	for _, b := range out.Throughput {
		out.KPIs.Created += b.Created
		out.KPIs.Completed += b.Completed
	}
	out.Aging = ReportAging{
		Age:  []ReportCount{{"0-7", age[0]}, {"8-30", age[1]}, {"31-90", age[2]}, {"91+", age[3]}},
		Late: []ReportCount{{"1-7", late[0]}, {"8-30", late[1]}, {"31-90", late[2]}, {"91+", late[3]}},
	}
	// Most loaded first: overdue, then open; ties by name (unassigned last).
	slices.SortStableFunc(out.Workload, func(a, b ReportAssignee) int {
		if a.Overdue != b.Overdue {
			return b.Overdue - a.Overdue
		}
		if a.Open != b.Open {
			return b.Open - a.Open
		}
		if (a.Name == nil) != (b.Name == nil) {
			if a.Name == nil {
				return 1
			}
			return -1
		}
		return strings.Compare(deref(a.Name), deref(b.Name))
	})
	out.Projects = nonNilSlice(out.Projects)
	out.Throughput = nonNilSlice(out.Throughput)
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// RecordReportExport audits a reports CSV download (ADR-0041): a
// workspace event with the filter summary only, never the numbers.
func (s *Store) RecordReportExport(ctx context.Context, wsID, actorID uuid.UUID, payload map[string]any) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return appendEvent(ctx, tx, wsID, actorID, "workspace", wsID, "reports.exported", payload)
	})
}
