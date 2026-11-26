package store

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The home page "Mənim işlərim" (ADR-0037): the signed-in user's work across
// every workspace they belong to, in one round trip (a pgx batch). Every
// query starts from the same two CTEs: the viewer's memberships and the
// projects the viewer can see (visibleProjectFor, ADR-0013), so an item of a
// project the viewer cannot see is never counted, listed or dated.

// Home list kinds: what GET /me/home?show= lists.
const (
	HomeAttention = "attention" // overdue, then awaiting my approval, then due this week
	HomeOverdue   = "overdue"
	HomeApproval  = "approval"
	HomeWeek      = "week"
	HomeAssigned  = "assigned" // every open item assigned to me
)

// IsHomeShow reports whether s is a known list kind.
func IsHomeShow(s string) bool {
	switch s {
	case HomeAttention, HomeOverdue, HomeApproval, HomeWeek, HomeAssigned:
		return true
	}
	return false
}

const (
	// MaxHomeList bounds one list page; the home page shows ~15 rows and
	// "show all" asks for this many.
	MaxHomeList  = 200
	homeWatching = 5
	homeRecent   = 6
	// homeRecentScan is how many of a workspace's latest items are looked at
	// to find its recent projects and last activity (an index scan).
	homeRecentScan = 50
)

// HomeCounters are the four cards; each is the total of the list it opens.
type HomeCounters struct {
	Overdue   int `json:"overdue"`
	Approvals int `json:"approvals"`
	Week      int `json:"week"`
	Assigned  int `json:"assigned"`
}

// HomeWorkspace is one workspace card. All counts are the viewer's own and
// only cover projects the viewer can see.
type HomeWorkspace struct {
	ID           uuid.UUID  `json:"id"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	Accent       *string    `json:"accent"`
	Timezone     string     `json:"timezone"`
	Today        string     `json:"today"` // the workspace's local date
	Projects     int        `json:"projects"`
	Assigned     int        `json:"assigned"`
	Overdue      int        `json:"overdue"`
	Week         int        `json:"week"`
	Approvals    int        `json:"approvals"`
	LastActivity *time.Time `json:"last_activity"`
}

// HomeRef names the workspace and project of a row.
type HomeRef struct {
	Slug   string  `json:"slug"`
	Name   string  `json:"name"`
	Accent *string `json:"accent"`
}

// HomeProject is a project in a list row.
type HomeProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// HomeApprovalInfo is the pending request waiting for the viewer's decision.
type HomeApprovalInfo struct {
	RequestedAt time.Time `json:"requested_at"`
	DueOn       *string   `json:"due_on"`
	Overdue     bool      `json:"overdue"`
}

// HomeItem is one row of the attention (or a filtered) list.
type HomeItem struct {
	// Reason is the most urgent of overdue, approval, week, assigned.
	Reason    string            `json:"reason"`
	Workspace HomeRef           `json:"workspace"`
	Project   HomeProject       `json:"project"`
	Key       string            `json:"key"`
	Number    int64             `json:"number"`
	Title     string            `json:"title"`
	Status    Status            `json:"status"`
	DueDate   *string           `json:"due_date"`
	Assigned  bool              `json:"assigned"` // assigned to the viewer
	Approval  *HomeApprovalInfo `json:"approval"` // waiting for the viewer's decision
}

// HomeWatched is a recently changed item the viewer watches (ADR-0028).
type HomeWatched struct {
	Workspace HomeRef     `json:"workspace"`
	Project   HomeProject `json:"project"`
	Key       string      `json:"key"`
	Number    int64       `json:"number"`
	Title     string      `json:"title"`
	Status    Status      `json:"status"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// HomeRecentProject is a project with recent item activity.
type HomeRecentProject struct {
	Workspace    HomeRef     `json:"workspace"`
	Project      HomeProject `json:"project"`
	LastActivity time.Time   `json:"last_activity"`
}

// HomePinned is a project an admin put on the home page (ADR-0053).
type HomePinned struct {
	Workspace HomeRef     `json:"workspace"`
	Project   HomeProject `json:"project"`
}

// Home is the whole page.
type Home struct {
	Counters       HomeCounters        `json:"counters"`
	Show           string              `json:"show"`
	Items          []HomeItem          `json:"items"`
	Total          int                 `json:"total"` // rows of the list without the limit
	Workspaces     []HomeWorkspace     `json:"workspaces"`
	Watching       []HomeWatched       `json:"watching"`
	RecentProjects []HomeRecentProject `json:"recent_projects"`
	// Pinned: the workspaces' home links the viewer can open, in the
	// admin's order (private projects of others are left out).
	Pinned []HomePinned `json:"pinned"`
}

// homeScope: $1 is the viewer. my = memberships with the workspace's local
// date; vis = active projects the viewer can see; mine = open items assigned
// to the viewer in them; appr = pending requests the viewer may decide on
// now, on their own account or as a delegate (inboxCanDecide, the same
// predicate as the decision itself).
var homeScope = `WITH my AS MATERIALIZED (
		SELECT w.id, w.slug, w.name, w.accent, w.timezone, m.role, m.role IN ('admin', 'owner') AS adm,
			(now() AT TIME ZONE w.timezone)::date AS today
		FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.archived_at IS NULL),
	vis AS MATERIALIZED (
		SELECT p.id, p.workspace_id, p.key, p.name FROM projects p JOIN my ON my.id = p.workspace_id
		WHERE p.archived_at IS NULL AND ` + visibleProjectFor("p", "$1", "my.adm") + `),
	mine AS MATERIALIZED (
		SELECT i.id, i.workspace_id, i.due_date, my.today
		FROM items i
		JOIN my ON my.id = i.workspace_id
		JOIN vis ON vis.id = i.project_id
		JOIN statuses s ON s.id = i.status_id
		WHERE i.assignee_id = $1 AND i.deleted_at IS NULL AND s.category IN ('todo', 'in_progress')),
	appr AS MATERIALIZED (
		SELECT r.item_id, r.workspace_id, r.created_at, r.due_on, COALESCE(r.due_on < my.today, false) AS late
		FROM approval_requests r
		JOIN my ON my.id = r.workspace_id
		JOIN workspaces w ON w.id = r.workspace_id -- delegation dates are local (ADR-0042)
		JOIN items i ON i.id = r.item_id AND i.deleted_at IS NULL
		JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
		JOIN workspace_members vm ON vm.workspace_id = r.workspace_id AND vm.user_id = $1
		JOIN users vu ON vu.id = vm.user_id
		WHERE r.state = 'pending' AND ` + inboxCanDecide + `)`

// homeCards: one row per workspace with the viewer's counts.
var homeCards = homeScope + `,
	mc AS (SELECT workspace_id, count(*) AS open,
			count(*) FILTER (WHERE due_date < today) AS overdue,
			count(*) FILTER (WHERE due_date BETWEEN today AND today + 6) AS week
		FROM mine GROUP BY workspace_id),
	ac AS (SELECT workspace_id, count(*) AS n FROM appr GROUP BY workspace_id),
	pc AS (SELECT workspace_id, count(*) AS n FROM vis GROUP BY workspace_id)
	SELECT my.id, my.slug, my.name, my.role, my.accent, my.timezone, my.today::text,
		COALESCE(pc.n, 0), COALESCE(mc.open, 0), COALESCE(mc.overdue, 0), COALESCE(mc.week, 0), COALESCE(ac.n, 0)
	FROM my
	LEFT JOIN mc ON mc.workspace_id = my.id
	LEFT JOIN ac ON ac.workspace_id = my.id
	LEFT JOIN pc ON pc.workspace_id = my.id
	ORDER BY my.name, my.id`

// homeList: $2 the list kind, $3 the limit. cand has one row per reason an
// item is listed (rank 1 overdue, 2 approval, 3 due this week, 4 other
// open); the list has one row per item, under its most urgent reason.
// Ordering: overdue by due date (oldest first), approvals by request age
// (oldest first), then by due date (soonest first).
var homeList = homeScope + `,
	cand AS (
		SELECT m.id AS item_id, CASE WHEN m.due_date < m.today THEN 1
				WHEN m.due_date <= m.today + 6 THEN 3 ELSE 4 END AS rank,
			true AS assigned, NULL::timestamptz AS requested_at, NULL::date AS req_due, false AS late
		FROM mine m
		UNION ALL
		SELECT a.item_id, 2, false, a.created_at, a.due_on, a.late FROM appr a),
	pick AS (
		SELECT * FROM cand WHERE CASE $2::text
			WHEN 'attention' THEN rank <= 3
			WHEN 'overdue' THEN rank = 1
			WHEN 'approval' THEN rank = 2
			WHEN 'week' THEN rank = 3
			WHEN 'assigned' THEN assigned
			ELSE false END),
	per AS (
		SELECT item_id, min(rank) AS rank, bool_or(assigned) AS assigned, max(requested_at) AS requested_at,
			max(req_due) AS req_due, bool_or(late) AS late, count(*) OVER () AS total
		FROM pick GROUP BY item_id)
	SELECT per.rank, my.slug, my.name, my.accent, vis.key, vis.name, i.number, i.title,
		s.id, s.name, s.category, i.due_date::text, per.assigned OR i.assignee_id IS NOT DISTINCT FROM $1,
		per.requested_at, per.req_due::text, per.late, per.total
	FROM per
	JOIN items i ON i.id = per.item_id
	JOIN vis ON vis.id = i.project_id
	JOIN my ON my.id = i.workspace_id
	JOIN statuses s ON s.id = i.status_id
	ORDER BY per.rank, CASE WHEN per.rank = 2 THEN per.requested_at END, i.due_date NULLS LAST,
		my.name, vis.key, i.number
	LIMIT $3`

// homeWatchedSQL: the viewer's watched items, most recently changed first.
// Assignees watch their items automatically (ADR-0028); those are already
// counted and listed as "assigned", so they are left out here.
var homeWatchedSQL = homeScope + `
	SELECT my.slug, my.name, my.accent, vis.key, vis.name, i.number, i.title, s.id, s.name, s.category, i.updated_at
	FROM my
	JOIN item_watchers wt ON wt.workspace_id = my.id AND wt.user_id = $1
	JOIN items i ON i.id = wt.item_id AND i.deleted_at IS NULL
	JOIN vis ON vis.id = i.project_id
	JOIN statuses s ON s.id = i.status_id
	WHERE i.assignee_id IS DISTINCT FROM $1
	ORDER BY i.updated_at DESC, i.id
	LIMIT $2`

// homeRecentSQL: per workspace, the projects among its latest live items
// (visible ones only), with their last change: the workspace card's "last
// activity" and the empty state's recent projects. The LIMIT inside the
// lateral keeps it an index scan of items_workspace_recent_idx.
// homePinnedSQL: the admin-chosen home links (workspaces.home_projects), only
// the visible ones, in the stored order.
var homePinnedSQL = homeScope + `
	SELECT my.slug, my.name, my.accent, vis.key, vis.name
	FROM my
	JOIN workspaces w ON w.id = my.id
	CROSS JOIN LATERAL unnest(w.home_projects) WITH ORDINALITY AS h(project_id, pos)
	JOIN vis ON vis.id = h.project_id AND vis.workspace_id = my.id
	ORDER BY my.name, my.id, h.pos`

var homeRecentSQL = homeScope + `
	SELECT my.id, my.slug, my.name, my.accent, vis.key, vis.name, r.at
	FROM my CROSS JOIN LATERAL (
		SELECT x.project_id, max(x.updated_at) AS at FROM (
			SELECT i.project_id, i.updated_at FROM items i
			WHERE i.workspace_id = my.id AND i.deleted_at IS NULL
			  AND i.project_id IN (SELECT id FROM vis WHERE vis.workspace_id = my.id)
			ORDER BY i.updated_at DESC LIMIT $2) x
		GROUP BY x.project_id) r
	JOIN vis ON vis.id = r.project_id
	ORDER BY r.at DESC, vis.id`

var homeReasons = map[int]string{1: HomeOverdue, 2: HomeApproval, 3: HomeWeek, 4: HomeAssigned}

// HomeFor builds the home page of a user. show is one of the Home* kinds
// (IsHomeShow), limit 1..MaxHomeList.
func (s *Store) HomeFor(ctx context.Context, userID uuid.UUID, show string, limit int) (Home, error) {
	if !IsHomeShow(show) {
		return Home{}, invalid("show", "must be attention, overdue, approval, week or assigned")
	}
	limit = min(max(limit, 1), MaxHomeList)
	out := Home{Show: show, Items: []HomeItem{}, Workspaces: []HomeWorkspace{}, Watching: []HomeWatched{}, RecentProjects: []HomeRecentProject{}, Pinned: []HomePinned{}}

	batch := &pgx.Batch{}
	batch.Queue(homeCards, userID).Query(func(rows pgx.Rows) error {
		var err error
		out.Workspaces, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HomeWorkspace, error) {
			var w HomeWorkspace
			return w, r.Scan(&w.ID, &w.Slug, &w.Name, &w.Role, &w.Accent, &w.Timezone, &w.Today,
				&w.Projects, &w.Assigned, &w.Overdue, &w.Week, &w.Approvals)
		})
		return err
	})
	batch.Queue(homeList, userID, show, limit).Query(func(rows pgx.Rows) error {
		var err error
		out.Items, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HomeItem, error) {
			var it HomeItem
			var rank int
			var requestedAt *time.Time
			var reqDue *string
			var late bool
			err := r.Scan(&rank, &it.Workspace.Slug, &it.Workspace.Name, &it.Workspace.Accent, &it.Project.Key, &it.Project.Name,
				&it.Number, &it.Title, &it.Status.ID, &it.Status.Name, &it.Status.Category, &it.DueDate, &it.Assigned,
				&requestedAt, &reqDue, &late, &out.Total)
			it.Reason = homeReasons[rank]
			it.Key = itemKey(it.Project.Key, it.Number)
			if requestedAt != nil {
				it.Approval = &HomeApprovalInfo{RequestedAt: *requestedAt, DueOn: reqDue, Overdue: late}
			}
			return it, err
		})
		return err
	})
	batch.Queue(homeWatchedSQL, userID, homeWatching).Query(func(rows pgx.Rows) error {
		var err error
		out.Watching, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HomeWatched, error) {
			var w HomeWatched
			err := r.Scan(&w.Workspace.Slug, &w.Workspace.Name, &w.Workspace.Accent, &w.Project.Key, &w.Project.Name,
				&w.Number, &w.Title, &w.Status.ID, &w.Status.Name, &w.Status.Category, &w.UpdatedAt)
			w.Key = itemKey(w.Project.Key, w.Number)
			return w, err
		})
		return err
	})
	batch.Queue(homePinnedSQL, userID).Query(func(rows pgx.Rows) error {
		var err error
		out.Pinned, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HomePinned, error) {
			var p HomePinned
			return p, r.Scan(&p.Workspace.Slug, &p.Workspace.Name, &p.Workspace.Accent, &p.Project.Key, &p.Project.Name)
		})
		return err
	})
	last := map[uuid.UUID]time.Time{}
	batch.Queue(homeRecentSQL, userID, homeRecentScan).Query(func(rows pgx.Rows) error {
		var err error
		out.RecentProjects, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HomeRecentProject, error) {
			var p HomeRecentProject
			var wsID uuid.UUID
			err := r.Scan(&wsID, &p.Workspace.Slug, &p.Workspace.Name, &p.Workspace.Accent, &p.Project.Key, &p.Project.Name, &p.LastActivity)
			if t, ok := last[wsID]; !ok || p.LastActivity.After(t) {
				last[wsID] = p.LastActivity
			}
			return p, err
		})
		return err
	})
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return Home{}, err
	}

	for i := range out.Workspaces {
		w := &out.Workspaces[i]
		if t, ok := last[w.ID]; ok {
			w.LastActivity = &t
		}
		out.Counters.Overdue += w.Overdue
		out.Counters.Approvals += w.Approvals
		out.Counters.Week += w.Week
		out.Counters.Assigned += w.Assigned
	}
	// Already newest first; keep the few the page shows.
	out.RecentProjects = slices.Clip(out.RecentProjects[:min(len(out.RecentProjects), homeRecent)])
	out.Items = nonNilSlice(out.Items)
	out.Workspaces = nonNilSlice(out.Workspaces)
	out.Watching = nonNilSlice(out.Watching)
	out.RecentProjects = nonNilSlice(out.RecentProjects)
	out.Pinned = nonNilSlice(out.Pinned)
	return out, nil
}
