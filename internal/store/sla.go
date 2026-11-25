package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/recur"
)

// SLA targets (ADR-0045): per-project first response and resolution targets
// in working days of the workspace calendar (ADR-0027). The computed due
// timestamps live in item_sla; triggers stamp responded_at / resolved_at.

// SLA states, worst first. "met" means every target was reached in time.
const (
	SLABreached = "breached"
	SLAAtRisk   = "at_risk"
	SLAOnTrack  = "on_track"
	SLAMet      = "met"

	EventSLAAtRisk   = "item.sla_at_risk"
	EventSLABreached = "item.sla_breached"

	NotifySLAAtRisk   = "sla_at_risk"
	NotifySLABreached = "sla_breached"

	maxSLAResponseDays   = 60
	maxSLAResolutionDays = 120
	// slaSyncLimit bounds the items recomputed inside a policy change; the
	// worker recomputes the rest (computed_at < changed_at).
	slaSyncLimit = 2000
	// maxSLAAdminNotifyees bounds the admins told about one breach.
	maxSLAAdminNotifyees = 20
)

// SLAPolicy is a project's SLA policy; a nil target is not tracked.
type SLAPolicy struct {
	ResponseDays   *int       `json:"response_days"`
	ResolutionDays *int       `json:"resolution_days"`
	Since          *time.Time `json:"since"`
}

// ItemSLA is an item's SLA clock (nil on items of projects without policy).
type ItemSLA struct {
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"started_at"`
	ResponseDue   *time.Time `json:"response_due"`
	ResolutionDue *time.Time `json:"resolution_due"`
	RespondedAt   *time.Time `json:"responded_at"`
	ResolvedAt    *time.Time `json:"resolved_at"`
}

// SLADue is the end (local midnight) of the days-th working day after the
// local date of start: a request that arrives on Friday with one working day
// is due at the end of Monday. Arrival on a non-working day counts from the
// next working day, like AddWorkdays.
func SLADue(start time.Time, loc *time.Location, cal *recur.Calendar, days int) (time.Time, bool) {
	d, ok := cal.AddWorkdays(recur.Date(start.In(loc)), days)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, loc), true
}

// slaTargetState is the state of one target (empty when it is not tracked).
// At risk: less than a quarter of the target's time is left.
func slaTargetState(start, due, done *time.Time, now time.Time) string {
	if due == nil {
		return ""
	}
	if done != nil {
		if done.After(*due) {
			return SLABreached
		}
		return SLAMet
	}
	if !now.Before(*due) {
		return SLABreached
	}
	if start != nil && now.After(due.Add(-due.Sub(*start)/4)) {
		return SLAAtRisk
	}
	return SLAOnTrack
}

// state combines the targets: the worst one wins.
func (s *ItemSLA) state(now time.Time) string {
	rank := map[string]int{"": 0, SLAMet: 1, SLAOnTrack: 2, SLAAtRisk: 3, SLABreached: 4}
	worst := ""
	for _, st := range []string{
		slaTargetState(s.StartedAt, s.ResponseDue, s.RespondedAt, now),
		slaTargetState(s.StartedAt, s.ResolutionDue, s.ResolvedAt, now),
	} {
		if rank[st] > rank[worst] {
			worst = st
		}
	}
	return worst
}

// slaFilter returns the condition of the "breached" and "at_risk" queues
// on the alias i: open targets only (a met or late-resolved item is history).
func slaFilter(kind string) (string, error) {
	const open = `FROM item_sla fq WHERE fq.item_id = i.id AND fq.resolved_at IS NULL`
	const breached = `COALESCE((fq.responded_at IS NULL AND fq.response_due <= now()) OR fq.resolution_due <= now(), false)`
	risk := func(due string) string {
		return `(` + due + ` > now() AND now() > ` + due + ` - (` + due + ` - fq.started_at) / 4)`
	}
	switch kind {
	case SLABreached:
		return `EXISTS (SELECT 1 ` + open + ` AND ` + breached + `)`, nil
	case SLAAtRisk:
		return `EXISTS (SELECT 1 ` + open + ` AND NOT ` + breached + ` AND COALESCE((fq.responded_at IS NULL AND ` +
			risk("fq.response_due") + `) OR ` + risk("fq.resolution_due") + `, false))`, nil
	}
	return "", invalid("sla", "must be breached or at_risk")
}

// GetSLAPolicy returns the project's policy (both targets nil when none).
func (s *Store) GetSLAPolicy(ctx context.Context, projectID uuid.UUID) (SLAPolicy, error) {
	var p SLAPolicy
	var resp, res *int16
	err := s.pool.QueryRow(ctx, `SELECT response_days, resolution_days, since FROM project_sla_policies WHERE project_id = $1`,
		projectID).Scan(&resp, &res, &p.Since)
	if errors.Is(err, pgx.ErrNoRows) {
		return SLAPolicy{}, nil
	}
	p.ResponseDays, p.ResolutionDays = int16Ptr(resp), int16Ptr(res)
	return p, err
}

func int16Ptr(v *int16) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

// SLAPolicyInput sets a policy; both targets null removes it.
type SLAPolicyInput struct {
	ResponseDays   opt.Field[int] `json:"response_days"`
	ResolutionDays opt.Field[int] `json:"resolution_days"`
}

func slaDays(field string, f opt.Field[int], limit int) (*int, error) {
	if !f.Set || f.Null {
		return nil, nil
	}
	if f.Value < 1 || f.Value > limit {
		return nil, invalid(field, "must be between 1 and %d working days", limit)
	}
	v := f.Value
	return &v, nil
}

// SetSLAPolicy replaces the project's policy, records project.sla_updated
// and recomputes the project's items (up to slaSyncLimit now, the rest in
// the worker).
func (s *Store) SetSLAPolicy(ctx context.Context, wsID, projectID, actorID uuid.UUID, in SLAPolicyInput) (SLAPolicy, error) {
	resp, err := slaDays("response_days", in.ResponseDays, maxSLAResponseDays)
	if err != nil {
		return SLAPolicy{}, err
	}
	res, err := slaDays("resolution_days", in.ResolutionDays, maxSLAResolutionDays)
	if err != nil {
		return SLAPolicy{}, err
	}
	if resp != nil && res != nil && *resp > *res {
		return SLAPolicy{}, invalid("response_days", "must not be longer than the resolution target")
	}
	before, err := s.GetSLAPolicy(ctx, projectID)
	if err != nil {
		return SLAPolicy{}, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		// Workspace first, like calendar writers (lockCalendar, then the
		// policy rows): the opposite order could deadlock with them.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR SHARE`, wsID); err != nil {
			return err
		}
		if resp == nil && res == nil {
			tag, err := tx.Exec(ctx, `DELETE FROM project_sla_policies WHERE project_id = $1`, projectID)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM item_sla q USING items i
				WHERE q.item_id = i.id AND i.project_id = $1`, projectID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `INSERT INTO project_sla_policies (project_id, workspace_id, response_days, resolution_days)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (project_id) DO UPDATE SET response_days = EXCLUDED.response_days,
					resolution_days = EXCLUDED.resolution_days, changed_at = now()`,
				projectID, wsID, resp, res); err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT id FROM items WHERE project_id = $1 AND deleted_at IS NULL
				ORDER BY created_at DESC LIMIT $2`, projectID, slaSyncLimit)
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil {
				return err
			}
			if err := computeSLA(ctx, tx, wsID, ids); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "project.sla_updated", map[string]any{
			"changes": map[string]any{
				"response_days":   map[string]any{"from": before.ResponseDays, "to": resp},
				"resolution_days": map[string]any{"from": before.ResolutionDays, "to": res},
			},
		})
	})
	if err != nil {
		return SLAPolicy{}, err
	}
	return s.GetSLAPolicy(ctx, projectID)
}

// markSLAStale makes the worker recompute a workspace's SLA dues after a
// calendar change (called inside the calendar transaction).
func markSLAStale(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE project_sla_policies SET changed_at = now() WHERE workspace_id = $1`, wsID)
	return err
}

// computeSLA (re)computes the item_sla rows of items of one workspace whose
// project has a policy. The workspace row is share-locked (loadCalendar), so
// a concurrent calendar change is waited for and never leaves a stale due.
// responded_at / resolved_at of an existing row are kept (the triggers own
// them); a new row derives them from the current status and comments.
func computeSLA(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	cal, err := loadCalendar(ctx, tx, wsID, true)
	if err != nil {
		return err
	}
	var tz string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM workspaces WHERE id = $1`, wsID).Scan(&tz); err != nil {
		return err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	rows, err := tx.Query(ctx, `SELECT i.id, greatest(i.created_at, sp.since), sp.response_days, sp.resolution_days,
			CASE WHEN s.category <> 'todo' THEN i.updated_at END,
			(SELECT min(c.created_at) FROM comments c WHERE c.item_id = i.id
			   AND c.author_id IS NOT NULL AND c.author_id IS DISTINCT FROM i.reporter_id),
			CASE WHEN s.category IN ('done', 'canceled') THEN i.updated_at END
		FROM items i
		JOIN project_sla_policies sp ON sp.project_id = i.project_id
		JOIN statuses s ON s.id = i.status_id
		WHERE i.workspace_id = $1 AND i.id = ANY($2) AND i.deleted_at IS NULL`, wsID, ids)
	if err != nil {
		return err
	}
	type row struct {
		id                         uuid.UUID
		start                      time.Time
		resp, res                  *int16
		movedAt, commentAt, doneAt *time.Time
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		return x, r.Scan(&x.id, &x.start, &x.resp, &x.res, &x.movedAt, &x.commentAt, &x.doneAt)
	})
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, x := range list {
		due := func(days *int16) *time.Time {
			if days == nil {
				return nil
			}
			d, ok := SLADue(x.start, loc, cal, int(*days))
			if !ok {
				return nil
			}
			return &d
		}
		responded := x.movedAt
		if x.commentAt != nil && (responded == nil || x.commentAt.Before(*responded)) {
			responded = x.commentAt
		}
		batch.Queue(`INSERT INTO item_sla (item_id, workspace_id, started_at, response_due, resolution_due,
				responded_at, resolved_at, computed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			ON CONFLICT (item_id) DO UPDATE SET started_at = EXCLUDED.started_at,
				response_due = EXCLUDED.response_due, resolution_due = EXCLUDED.resolution_due, computed_at = now()`,
			x.id, wsID, x.start, due(x.resp), due(x.res), responded, x.doneAt)
	}
	return tx.SendBatch(ctx, batch).Close()
}

// RecomputeSLA brings up to limit stale item_sla rows up to date: items of
// projects with a policy that have no row yet (created by imports, forms,
// recurrences, older items), or were computed before the last policy or
// calendar change; and drops rows of items whose project has no policy any
// more (an item moved). Returns the number of items handled.
func (s *Store) RecomputeSLA(ctx context.Context, limit int) (int, error) {
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM item_sla WHERE item_id IN (
				SELECT q.item_id FROM item_sla q JOIN items i ON i.id = q.item_id
				WHERE NOT EXISTS (SELECT 1 FROM project_sla_policies sp WHERE sp.project_id = i.project_id)
				LIMIT $1)`, limit); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT i.workspace_id, i.id FROM project_sla_policies sp
			JOIN projects p ON p.id = sp.project_id AND p.archived_at IS NULL
			JOIN items i ON i.project_id = sp.project_id AND i.deleted_at IS NULL
			LEFT JOIN item_sla q ON q.item_id = i.id
			WHERE q.item_id IS NULL OR q.computed_at IS NULL OR q.computed_at < sp.changed_at
			ORDER BY i.workspace_id LIMIT $1`, limit)
		if err != nil {
			return err
		}
		byWS := map[uuid.UUID][]uuid.UUID{}
		var ws, id uuid.UUID
		if _, err := pgx.ForEachRow(rows, []any{&ws, &id}, func() error {
			byWS[ws] = append(byWS[ws], id)
			n++
			return nil
		}); err != nil {
			return err
		}
		for ws, ids := range byWS {
			if err := computeSLA(ctx, tx, ws, ids); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

// slaScans are the four reminder scans: target x (at risk | breached). The
// column names are constants, never input.
var slaScans = []struct {
	target, action, due, marker, done string
	breach                            bool
}{
	{"response", EventSLABreached, "response_due", "response_breach_for", "c.responded_at IS NULL AND ", true},
	{"resolution", EventSLABreached, "resolution_due", "resolution_breach_for", "", true},
	{"response", EventSLAAtRisk, "response_due", "response_risk_for", "c.responded_at IS NULL AND ", false},
	{"resolution", EventSLAAtRisk, "resolution_due", "resolution_risk_for", "", false},
}

// EmitSLA emits item.sla_at_risk / item.sla_breached once per item, target
// and due timestamp (the *_for markers; a recomputed due is reminded of
// again), for up to limit items per scan. Archived workspaces and projects
// are skipped. Like EmitApprovalOverdue, rows a writer holds are skipped
// (SKIP LOCKED) and caught on the next tick.
func (s *Store) EmitSLA(ctx context.Context, limit int) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		for _, sc := range slaScans {
			due := "c." + sc.due
			cond := due + " <= now()"
			if !sc.breach {
				cond = due + " > now() AND now() > " + due + " - (" + due + " - c.started_at) / 4"
			}
			rows, err := tx.Query(ctx, `UPDATE item_sla q SET `+sc.marker+` = q.`+sc.due+`
				WHERE q.item_id IN (
					SELECT c.item_id FROM item_sla c
					JOIN workspaces w ON w.id = c.workspace_id AND w.archived_at IS NULL
					JOIN items i ON i.id = c.item_id AND i.deleted_at IS NULL
					JOIN projects p ON p.id = i.project_id AND p.archived_at IS NULL
					JOIN project_sla_policies sp ON sp.project_id = i.project_id
					WHERE c.resolved_at IS NULL AND `+sc.done+due+` IS NOT NULL
					  AND c.`+sc.marker+` IS DISTINCT FROM `+due+` AND `+cond+`
					ORDER BY `+due+`, c.item_id LIMIT $1
					FOR UPDATE OF c SKIP LOCKED)
				RETURNING q.workspace_id, q.item_id, q.`+sc.due, limit)
			if err != nil {
				return err
			}
			type hit struct {
				ws, item uuid.UUID
				due      time.Time
			}
			hits, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (hit, error) {
				var h hit
				return h, r.Scan(&h.ws, &h.item, &h.due)
			})
			if err != nil {
				return err
			}
			for _, h := range hits {
				var key, title string
				var assignee *uuid.UUID
				if err := tx.QueryRow(ctx, `SELECT p.key || '-' || i.number, i.title, i.assignee_id
					FROM items i JOIN projects p ON p.id = i.project_id WHERE i.id = $1`, h.item).
					Scan(&key, &title, &assignee); err != nil {
					return err
				}
				if err := appendEvent(ctx, tx, h.ws, uuid.Nil, "item", h.item, sc.action, map[string]any{
					"target": sc.target, "due": h.due, "key": key, "title": title, "assignee_id": assignee,
				}); err != nil {
					return err
				}
			}
			n += len(hits)
		}
		return nil
	})
	return n, err
}

// addSLARecipients: the assignee; on a breach also the workspace admins
// and owners (who see every project), at most maxSLAAdminNotifyees.
func addSLARecipients(ctx context.Context, tx pgx.Tx, e Event, add func(*uuid.UUID, string)) error {
	if e.Action == EventSLAAtRisk {
		add(payloadUUID(e.Payload["assignee_id"]), NotifySLAAtRisk)
		return nil
	}
	add(payloadUUID(e.Payload["assignee_id"]), NotifySLABreached)
	rows, err := tx.Query(ctx, `SELECT m.user_id FROM workspace_members m JOIN users u ON u.id = m.user_id AND u.is_active
		WHERE m.workspace_id = $1 AND m.role IN ('admin', 'owner') ORDER BY m.user_id LIMIT $2`,
		e.WorkspaceID, maxSLAAdminNotifyees)
	if err != nil {
		return err
	}
	admins, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for i := range admins {
		add(&admins[i], NotifySLABreached)
	}
	return nil
}
