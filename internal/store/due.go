package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Time-based events (ADR-0009). Both scans are idempotent: a marker row is
// inserted with ON CONFLICT DO NOTHING and only rows actually inserted get an
// event, so several worker replicas or a re-run never emit an event twice.

const (
	// ActionItemOverdue is emitted once per item and due date.
	ActionItemOverdue = TriggerItemOverdue
	// ActionOverdueDigest is emitted at most once per workspace and local day.
	ActionOverdueDigest = "workspace.overdue_digest"

	// A digest lists the oldest maxDigestPerProject overdue items of every
	// project (at most maxDigestItems in all), so a webhook filtered to one
	// project still gets that project's list (ADR-0013).
	maxDigestPerProject = 50
	maxDigestItems      = 1000
)

// withSystem marks events as caused by the scheduler: actor_id NULL,
// source 'system'. System events, unlike automation ones, may trigger rules.
func withSystem(ctx context.Context) context.Context {
	return context.WithValue(ctx, sourceKey{}, eventSource{source: "system"})
}

// localToday is "today" in the workspace's time zone; the alias w must be
// joined.
const localToday = "(now() AT TIME ZONE w.timezone)::date"

// openOverdue selects overdue items: past due and still todo/in progress.
const openOverdue = `i.deleted_at IS NULL AND i.due_date IS NOT NULL
	AND i.due_date < ` + localToday + `
	AND s.category IN ('todo', 'in_progress')`

// EmitOverdue emits item.overdue for up to limit newly overdue items and
// returns how many it emitted. Call it periodically; it catches up in batches.
func (s *Store) EmitOverdue(ctx context.Context, limit int) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			WITH cand AS (
				SELECT i.workspace_id, i.id, i.due_date
				FROM items i
				JOIN workspaces w ON w.id = i.workspace_id AND w.archived_at IS NULL
				JOIN projects   p ON p.id = i.project_id AND p.archived_at IS NULL
				JOIN statuses   s ON s.id = i.status_id
				WHERE `+openOverdue+`
				  AND NOT EXISTS (SELECT 1 FROM item_overdue_marks m WHERE m.item_id = i.id AND m.due_date = i.due_date)
				ORDER BY i.due_date, i.id
				LIMIT $1)
			INSERT INTO item_overdue_marks (workspace_id, item_id, due_date)
			SELECT workspace_id, id, due_date FROM cand
			ON CONFLICT DO NOTHING
			RETURNING workspace_id, item_id`, limit)
		if err != nil {
			return err
		}
		type mark struct{ ws, item uuid.UUID }
		marks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (mark, error) {
			var m mark
			return m, r.Scan(&m.ws, &m.item)
		})
		if err != nil {
			return err
		}
		for _, m := range marks {
			it, err := scanItem(tx.QueryRow(ctx, itemSelect+` WHERE i.id = $1`, m.item))
			if err != nil {
				return err
			}
			entry, err := overdueEntry(ctx, tx, m.item)
			if err != nil {
				return err
			}
			payload := map[string]any{"snapshot": Snapshot(it)}
			for k, v := range entry {
				payload[k] = v
			}
			if err := appendEvent(ctx, tx, m.ws, uuid.Nil, "item", m.item, ActionItemOverdue, payload); err != nil {
				return err
			}
		}
		n = len(marks)
		return nil
	})
	return n, err
}

// openDueToday selects open items due today in the workspace's time zone.
const openDueToday = `i.deleted_at IS NULL AND i.due_date = ` + localToday + `
	AND s.category IN ('todo', 'in_progress')`

// digestList returns the entries (oldest maxDigestPerProject per project, at
// most maxDigestItems), the total and the per-project counts of the open
// items of a workspace that match cond.
func digestList(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, cond string) ([]map[string]any, int, map[string]int, error) {
	counts, err := tx.Query(ctx, `SELECT p.key, count(*) FROM items i
		JOIN workspaces w ON w.id = i.workspace_id JOIN statuses s ON s.id = i.status_id
		JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
		WHERE i.workspace_id = $1 AND `+cond+` GROUP BY p.key`, wsID)
	if err != nil {
		return nil, 0, nil, err
	}
	byProject := map[string]int{}
	total := 0
	var key string
	var cnt int
	if _, err := pgx.ForEachRow(counts, []any{&key, &cnt}, func() error {
		byProject[key] = cnt
		total += cnt
		return nil
	}); err != nil {
		return nil, 0, nil, err
	}
	items := []map[string]any{}
	if total == 0 {
		return items, 0, byProject, nil
	}
	rows, err := tx.Query(ctx, `SELECT id FROM (
			SELECT i.id, i.due_date, i.number,
				row_number() OVER (PARTITION BY i.project_id ORDER BY i.due_date, i.number) AS rn
			FROM items i
			JOIN workspaces w ON w.id = i.workspace_id JOIN statuses s ON s.id = i.status_id
			JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
			WHERE i.workspace_id = $1 AND `+cond+`) x
		WHERE rn <= $3
		ORDER BY due_date, number LIMIT $2`, wsID, maxDigestItems, maxDigestPerProject)
	if err != nil {
		return nil, 0, nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, 0, nil, err
	}
	for _, id := range ids {
		e, err := overdueEntry(ctx, tx, id)
		if err != nil {
			return nil, 0, nil, err
		}
		items = append(items, e)
	}
	return items, total, byProject, nil
}

// overdueEntry describes one overdue item for events and digests.
func overdueEntry(ctx context.Context, q querier, itemID uuid.UUID) (map[string]any, error) {
	var (
		key, title, project, due string
		days                     int
		assignee                 *uuid.UUID
		assigneeName             *string
	)
	err := q.QueryRow(ctx, `
		SELECT p.key || '-' || i.number, i.title, p.key, i.due_date::text,
			`+localToday+` - i.due_date, i.assignee_id, u.display_name
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		JOIN projects   p ON p.id = i.project_id
		LEFT JOIN users u ON u.id = i.assignee_id
		WHERE i.id = $1`, itemID).Scan(&key, &title, &project, &due, &days, &assignee, &assigneeName)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"key": key, "title": title, "project": project, "due_date": due, "days_overdue": days,
		"assignee_id": assignee, "assignee_name": assigneeName,
	}, nil
}

// EmitOverdueDigests emits workspace.overdue_digest for every workspace whose
// digest hour has come today (local time) and that has not had one today.
// Workspaces without overdue items are marked as done for the day but get no
// event. Returns the number of digests emitted.
func (s *Store) EmitOverdueDigests(ctx context.Context) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO workspace_digest_runs (workspace_id, local_date)
			SELECT w.id, `+localToday+` FROM workspaces w
			WHERE w.overdue_digest_hour IS NOT NULL AND w.archived_at IS NULL
			  AND extract(hour FROM now() AT TIME ZONE w.timezone) >= w.overdue_digest_hour
			ON CONFLICT DO NOTHING
			RETURNING workspace_id, local_date::text, (SELECT timezone FROM workspaces WHERE id = workspace_id)`)
		if err != nil {
			return err
		}
		type run struct {
			ws       uuid.UUID
			date, tz string
		}
		runs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (run, error) {
			var x run
			return x, r.Scan(&x.ws, &x.date, &x.tz)
		})
		if err != nil {
			return err
		}
		for _, r := range runs {
			// No digest on the workspace's non-working days (ADR-0027): the run
			// row above marks the day as done, and the next working day's
			// digest lists everything that is still overdue.
			if working, err := isWorkday(ctx, tx, r.ws, r.date); err != nil {
				return err
			} else if !working {
				continue
			}
			counts, err := tx.Query(ctx, `SELECT p.key, count(*) FROM items i
				JOIN workspaces w ON w.id = i.workspace_id JOIN statuses s ON s.id = i.status_id
				JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
				WHERE i.workspace_id = $1 AND `+openOverdue+` GROUP BY p.key`, r.ws)
			if err != nil {
				return err
			}
			byProject := map[string]int{}
			total := 0
			var key string
			var cnt int
			if _, err := pgx.ForEachRow(counts, []any{&key, &cnt}, func() error {
				byProject[key] = cnt
				total += cnt
				return nil
			}); err != nil {
				return err
			}
			// Due today (open items): the morning reminder of what is due now.
			today, todayTotal, todayByProject, err := digestList(ctx, tx, r.ws, openDueToday)
			if err != nil {
				return err
			}
			if total == 0 && todayTotal == 0 {
				continue
			}
			ids, err := tx.Query(ctx, `SELECT id FROM (
					SELECT i.id, i.due_date, i.number,
						row_number() OVER (PARTITION BY i.project_id ORDER BY i.due_date, i.number) AS rn
					FROM items i
					JOIN workspaces w ON w.id = i.workspace_id JOIN statuses s ON s.id = i.status_id
					JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
					WHERE i.workspace_id = $1 AND `+openOverdue+`) x
				WHERE rn <= $3
				ORDER BY due_date, number LIMIT $2`, r.ws, maxDigestItems, maxDigestPerProject)
			if err != nil {
				return err
			}
			itemIDs, err := pgx.CollectRows(ids, pgx.RowTo[uuid.UUID])
			if err != nil {
				return err
			}
			items := make([]map[string]any, 0, len(itemIDs))
			for _, id := range itemIDs {
				e, err := overdueEntry(ctx, tx, id)
				if err != nil {
					return err
				}
				items = append(items, e)
			}
			payload := map[string]any{
				"date": r.date, "timezone": r.tz, "total": total, "truncated": total > len(items), "items": items,
				"by_project": byProject,
				"today":      today, "today_total": todayTotal, "today_by_project": todayByProject,
			}
			if err := appendEvent(ctx, tx, r.ws, uuid.Nil, "workspace", r.ws, ActionOverdueDigest, payload); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
