package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Absences and approval delegation (ADR-0042). A member records an absence
// in a workspace: workspace-local dates (inclusive) and optionally a
// delegate. While it is active, the delegate may decide on the approval
// requests the absent member could decide on, on their behalf. The
// delegate borrows the absent member's approver authority (policy roles and
// user list), never their identity: everything about the person acting —
// active, not a guest, sees the project, not the requester, has not voted
// on the request yet in any capacity — is checked on the delegate. There
// are no chains: an absent delegate stands in for nobody.

const (
	maxAbsenceNote = 500
	maxAbsenceDays = 366 // ends_on - starts_on
	maxAbsenceLead = 365 // how far ahead an absence may start
	maxAbsenceList = 500
)

// Absence audit events (entity type "absence"). The note is never part of
// a payload: it may say why someone is away.
const (
	EventAbsenceCreated = "absence.created"
	EventAbsenceUpdated = "absence.updated"
	EventAbsenceEnded   = "absence.ended"
)

// Absence is a member's absence as seen by one viewer.
type Absence struct {
	ID       uuid.UUID      `json:"id"`
	User     CommentAuthor  `json:"user"`
	Delegate *CommentAuthor `json:"delegate"`
	StartsOn string         `json:"starts_on"` // YYYY-MM-DD, workspace-local, inclusive
	EndsOn   string         `json:"ends_on"`
	// Note: only for the absent member, the delegate and admins.
	Note *string `json:"note"`
	// Active: today (workspace-local) is within the absence.
	Active bool `json:"active"`
	// DelegateAbsent: the delegate has an absence overlapping this one; on
	// the overlapping days nobody stands in (no chains).
	DelegateAbsent bool           `json:"delegate_absent"`
	CreatedBy      *CommentAuthor `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	EndedAt        *time.Time     `json:"ended_at"`
	CanEdit        bool           `json:"can_edit"`
}

// AbsenceInput is the body of a create. UserID nil means the caller.
type AbsenceInput struct {
	UserID     *uuid.UUID `json:"user_id"`
	DelegateID *uuid.UUID `json:"delegate_id"`
	StartsOn   string     `json:"starts_on"`
	EndsOn     string     `json:"ends_on"`
	Note       *string    `json:"note"`
}

// AbsencePatch changes a current or upcoming absence; absent keys stay.
type AbsencePatch struct {
	DelegateID opt.Field[uuid.UUID] `json:"delegate_id"`
	StartsOn   opt.Field[string]    `json:"starts_on"`
	EndsOn     opt.Field[string]    `json:"ends_on"`
	Note       opt.Field[string]    `json:"note"`
}

// absenceActive is the predicate "absence row a is in effect today", with
// today the workspace-local date of the workspaces row w in scope.
func absenceActive(a string) string {
	return a + `.ended_at IS NULL AND ` + a + `.starts_on <= ` + localToday + ` AND ` + a + `.ends_on >= ` + localToday
}

// delegateUsable is the part of a delegated decision about the person
// acting: member vm (user vu) on request r of project p. Separation of
// duties applies to the human: active, not a guest, not the requester, sees
// the project (ADR-0013), has not decided on this request in any capacity,
// and is not absent themselves (no chains). w must be in scope.
func delegateUsable(vm, vu, r, p string) string {
	return vu + `.is_active AND ` + vm + `.role <> 'guest' AND ` + vm + `.user_id <> ` + r + `.requested_by
	  AND ` + visibleProjectFor(p, vm+".user_id", vm+".role IN ('admin', 'owner')") + `
	  AND NOT EXISTS (SELECT 1 FROM approval_decisions hd WHERE hd.request_id = ` + r + `.id AND hd.user_id = ` + vm + `.user_id)
	  AND NOT EXISTS (SELECT 1 FROM workspace_absences sa
	      WHERE sa.workspace_id = ` + r + `.workspace_id AND sa.user_id = ` + vm + `.user_id AND ` + absenceActive("sa") + `)`
}

// delegatorsFrom is the FROM/WHERE of "the absent members member vm stands
// in for on request r now": an active absence naming vm as delegate, and
// the absent member is an eligible approver (approverEligible: the same
// predicate as the decision itself) whose approval slot is still open (no
// decision by them or on their behalf). Selects from du (users).
func delegatorsFrom(vm, r, p string) string {
	return ` FROM workspace_absences da
	JOIN workspace_members dm ON dm.workspace_id = da.workspace_id AND dm.user_id = da.user_id
	JOIN users du ON du.id = da.user_id
	WHERE da.workspace_id = ` + r + `.workspace_id AND da.delegate_id = ` + vm + `.user_id AND ` + absenceActive("da") + `
	  AND ` + approverEligible("dm", "du", p, r+".requested_by", r+".approver_roles", r+".approver_user_ids") + `
	  AND NOT EXISTS (SELECT 1 FROM approval_decisions dd
	      WHERE dd.request_id = ` + r + `.id AND COALESCE(dd.on_behalf_of, dd.user_id) = da.user_id)`
}

// canActForSomeone: member vm may decide on request r on somebody's behalf now.
func canActForSomeone(vm, vu, r, p string) string {
	return `(` + delegateUsable(vm, vu, r, p) + ` AND EXISTS (SELECT 1` + delegatorsFrom(vm, r, p) + `))`
}

// principalsJSON is a JSON array of {id, display_name}: whom vm may decide
// for on request r now (empty when vm may not act as a delegate at all).
func principalsJSON(vm, vu, r, p string) string {
	return `CASE WHEN ` + delegateUsable(vm, vu, r, p) + ` THEN COALESCE((SELECT json_agg(json_build_object('id', du.id, 'display_name', du.display_name)
		ORDER BY du.display_name, du.id)` + delegatorsFrom(vm, r, p) + `), '[]'::json) ELSE '[]'::json END`
}

// requestScope joins what the delegation predicates need around request
// r ($1) and member vm ($2).
const requestScope = ` FROM approval_requests r
	JOIN workspaces w ON w.id = r.workspace_id
	JOIN items i ON i.id = r.item_id
	JOIN projects p ON p.id = i.project_id
	JOIN workspace_members vm ON vm.workspace_id = r.workspace_id AND vm.user_id = $2
	JOIN users vu ON vu.id = vm.user_id
	WHERE r.id = $1`

// delegationFor returns the absence through which actor may decide on
// request on principal's behalf now (nil if they may not).
func delegationFor(ctx context.Context, q querier, requestID, actor, principal uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT x.id FROM approval_requests r
		JOIN workspaces w ON w.id = r.workspace_id
		JOIN items i ON i.id = r.item_id
		JOIN projects p ON p.id = i.project_id
		JOIN workspace_members vm ON vm.workspace_id = r.workspace_id AND vm.user_id = $2
		JOIN users vu ON vu.id = vm.user_id
		JOIN LATERAL (SELECT da.id`+delegatorsFrom("vm", "r", "p")+` AND da.user_id = $3 LIMIT 1) x ON true
		WHERE r.id = $1 AND `+delegateUsable("vm", "vu", "r", "p"), requestID, actor, principal).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// principalsFor lists whom viewer may decide for on a pending request now.
func principalsFor(ctx context.Context, q querier, requestID, viewer uuid.UUID) ([]CommentAuthor, error) {
	var out []CommentAuthor
	err := q.QueryRow(ctx, `SELECT `+principalsJSON("vm", "vu", "r", "p")+requestScope, requestID, viewer).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return []CommentAuthor{}, nil
	}
	return nonNilSlice(out), err
}

// requestDelegate is someone who may decide on a request on others' behalf.
type requestDelegate struct {
	ID          uuid.UUID
	DisplayName string
	For         []CommentAuthor
}

// requestDelegates lists who may decide on a pending request as a
// delegate now (notifications and chat cards), at most limit of them.
func requestDelegates(ctx context.Context, q querier, requestID uuid.UUID, limit int) ([]requestDelegate, error) {
	rows, err := q.Query(ctx, `SELECT vu.id, vu.display_name, `+principalsJSON("vm", "vu", "r", "p")+`
		FROM approval_requests r
		JOIN workspaces w ON w.id = r.workspace_id
		JOIN items i ON i.id = r.item_id
		JOIN projects p ON p.id = i.project_id
		JOIN workspace_members vm ON vm.workspace_id = r.workspace_id AND vm.user_id IN (
			SELECT delegate_id FROM workspace_absences WHERE workspace_id = r.workspace_id AND ended_at IS NULL AND delegate_id IS NOT NULL)
		JOIN users vu ON vu.id = vm.user_id
		WHERE r.id = $1 AND r.state = 'pending' AND `+canActForSomeone("vm", "vu", "r", "p")+`
		ORDER BY vu.display_name, vu.id LIMIT $2`, requestID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (requestDelegate, error) {
		var d requestDelegate
		return d, row.Scan(&d.ID, &d.DisplayName, &d.For)
	})
}

// ApproverAbsence is an approver of a pending request who is away today,
// with the delegate who can stand in for them on this request (nil: nobody
// can, e.g. no delegate, the delegate is away too or cannot see the project).
type ApproverAbsence struct {
	User     CommentAuthor  `json:"user"`
	Delegate *CommentAuthor `json:"delegate"`
	EndsOn   string         `json:"ends_on"`
}

// absentApprovers lists the request's eligible approvers with an open slot
// who are absent today.
func absentApprovers(ctx context.Context, q querier, wsID, projectID uuid.UUID, r approvalRow) ([]ApproverAbsence, error) {
	rows, err := q.Query(ctx, `SELECT e.id, e.display_name, a.ends_on::text, dl.id, dl.display_name,
			COALESCE(dm.user_id IS NOT NULL AND `+delegateUsable("dm", "dl", "rq", "pr")+`, false)
		FROM (`+eligibleApproversSQL+`
			AND NOT EXISTS (SELECT 1 FROM approval_decisions d WHERE d.request_id = $7
			    AND (d.user_id = m.user_id OR d.on_behalf_of = m.user_id))) e
		JOIN workspaces w ON w.id = $1
		JOIN approval_requests rq ON rq.id = $7
		JOIN projects pr ON pr.id = $2
		JOIN workspace_absences a ON a.workspace_id = $1 AND a.user_id = e.id AND `+absenceActive("a")+`
		LEFT JOIN users dl ON dl.id = a.delegate_id
		LEFT JOIN workspace_members dm ON dm.workspace_id = $1 AND dm.user_id = a.delegate_id
		ORDER BY e.display_name, e.id LIMIT 50`,
		wsID, projectID, r.requestedBy, r.roles, r.users, nil, r.id)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApproverAbsence, error) {
		var a ApproverAbsence
		var dID *uuid.UUID
		var dName *string
		var usable bool
		err := row.Scan(&a.User.ID, &a.User.DisplayName, &a.EndsOn, &dID, &dName, &usable)
		if usable && dID != nil && dName != nil {
			a.Delegate = &CommentAuthor{ID: *dID, DisplayName: *dName}
		}
		return a, err
	})
	return nonNilSlice(list), err
}

// absentToday returns the members of a workspace who are absent today.
func absentToday(ctx context.Context, q querier, wsID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT a.user_id FROM workspace_absences a JOIN workspaces w ON w.id = a.workspace_id
		WHERE a.workspace_id = $1 AND `+absenceActive("a"), wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// ---------------------------------------------------------------------------
// Managing absences
// ---------------------------------------------------------------------------

const absenceSelect = `SELECT a.id, a.user_id, u.display_name, a.delegate_id, d.display_name, a.starts_on::text, a.ends_on::text, a.note,
		a.ended_at IS NULL AND a.starts_on <= t.today AND a.ends_on >= t.today,
		a.delegate_id IS NOT NULL AND EXISTS (SELECT 1 FROM workspace_absences o
			WHERE o.workspace_id = a.workspace_id AND o.user_id = a.delegate_id AND o.ended_at IS NULL
			  AND o.starts_on <= a.ends_on AND o.ends_on >= a.starts_on),
		a.created_by, cu.display_name, a.created_at, a.updated_at, a.ended_at, a.ends_on >= t.today
	FROM workspace_absences a
	CROSS JOIN (SELECT (now() AT TIME ZONE timezone)::date AS today FROM workspaces WHERE id = $1) t
	JOIN workspace_members m ON m.workspace_id = a.workspace_id AND m.user_id = a.user_id
	JOIN users u ON u.id = a.user_id
	LEFT JOIN users d ON d.id = a.delegate_id
	LEFT JOIN users cu ON cu.id = a.created_by
	WHERE a.workspace_id = $1`

func scanAbsence(row pgx.CollectableRow, v Viewer) (Absence, error) {
	var a Absence
	var dID, cID *uuid.UUID
	var dName, cName *string
	var current bool
	err := row.Scan(&a.ID, &a.User.ID, &a.User.DisplayName, &dID, &dName, &a.StartsOn, &a.EndsOn, &a.Note,
		&a.Active, &a.DelegateAbsent, &cID, &cName, &a.CreatedAt, &a.UpdatedAt, &a.EndedAt, &current)
	if err != nil {
		return a, err
	}
	if dID != nil && dName != nil {
		a.Delegate = &CommentAuthor{ID: *dID, DisplayName: *dName}
	}
	if cID != nil && cName != nil {
		a.CreatedBy = &CommentAuthor{ID: *cID, DisplayName: *cName}
	}
	admin := RoleAtLeast(v.Role, RoleAdmin)
	delegate := dID != nil && *dID == v.UserID
	if !admin && v.UserID != a.User.ID && !delegate {
		a.Note = nil
	}
	a.CanEdit = (admin || v.UserID == a.User.ID) && a.EndedAt == nil && current
	return a, nil
}

// ListAbsences returns the workspace's current and upcoming absences
// (not ended, ending today or later), of current members, by start date.
// userID narrows to one member.
func (s *Store) ListAbsences(ctx context.Context, wsID uuid.UUID, v Viewer, userID *uuid.UUID) ([]Absence, error) {
	rows, err := s.pool.Query(ctx, absenceSelect+` AND a.ended_at IS NULL AND a.ends_on >= t.today
		AND ($2::uuid IS NULL OR a.user_id = $2)
		ORDER BY a.starts_on, u.display_name, a.id LIMIT $3`, wsID, userID, maxAbsenceList)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Absence, error) { return scanAbsence(r, v) })
	return nonNilSlice(list), err
}

func getAbsence(ctx context.Context, q querier, wsID, id uuid.UUID, v Viewer) (Absence, error) {
	rows, err := q.Query(ctx, absenceSelect+` AND a.id = $2`, wsID, id)
	if err != nil {
		return Absence{}, err
	}
	a, err := pgx.CollectExactlyOneRow(rows, func(r pgx.CollectableRow) (Absence, error) { return scanAbsence(r, v) })
	if errors.Is(err, pgx.ErrNoRows) {
		return Absence{}, ErrNotFound
	}
	return a, err
}

func absenceDate(field, v string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return time.Time{}, invalid(field, "must be a date (YYYY-MM-DD)")
	}
	return d, nil
}

func days(from, to time.Time) int { return int(to.Sub(from).Hours() / 24) }

// absenceRules are the checks shared by create and update.
type absenceRules struct {
	ws, user   uuid.UUID
	delegate   *uuid.UUID
	start, end time.Time
	today      time.Time
	newStart   bool // the start date is set or changed: it must not be in the past
	self       *uuid.UUID
}

func (c absenceRules) check(ctx context.Context, tx pgx.Tx) error {
	if c.end.Before(c.start) {
		return invalid("ends_on", "must not be before starts_on")
	}
	if days(c.start, c.end) > maxAbsenceDays {
		return invalid("ends_on", "an absence lasts at most %d days", maxAbsenceDays+1)
	}
	if c.end.Before(c.today) {
		return invalid("ends_on", "must be today or later (workspace time zone)")
	}
	if c.newStart && c.start.Before(c.today) {
		return invalid("starts_on", "must be today or later (workspace time zone)")
	}
	if days(c.today, c.start) > maxAbsenceLead {
		return invalid("starts_on", "must be within %d days from today", maxAbsenceLead)
	}
	if c.delegate != nil {
		if *c.delegate == c.user {
			return invalid("delegate_id", "the delegate must be someone else")
		}
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN users u ON u.id = m.user_id
			WHERE m.workspace_id = $1 AND m.user_id = $2 AND u.is_active AND m.role <> 'guest')`, c.ws, *c.delegate).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return invalid("delegate_id", "the delegate must be an active member of this workspace with at least the member role")
		}
	}
	var overlap bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_absences
		WHERE workspace_id = $1 AND user_id = $2 AND ended_at IS NULL AND starts_on <= $4 AND ends_on >= $3
		  AND ($5::uuid IS NULL OR id <> $5))`, c.ws, c.user, c.start, c.end, c.self).Scan(&overlap); err != nil {
		return err
	}
	if overlap {
		return &ConflictError{Code: "absence_overlap", Message: "this member already has an absence in these dates; change or end it instead"}
	}
	return nil
}

// lockAbsentMember locks the member's workspace_members row, so absence
// changes of one member serialize (the overlap check cannot race), and
// checks that they may have an absence: an active member, not a guest
// (guests never approve, so there is nothing to hand over).
func lockAbsentMember(ctx context.Context, tx pgx.Tx, wsID, userID uuid.UUID) error {
	var role string
	var active bool
	err := tx.QueryRow(ctx, `SELECT m.role, u.is_active FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 AND m.user_id = $2 FOR UPDATE OF m`, wsID, userID).Scan(&role, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return invalid("user_id", "not a member of this workspace")
	}
	if err != nil {
		return err
	}
	if !active || role == RoleGuest {
		return invalid("user_id", "only active members with at least the member role can record an absence")
	}
	return nil
}

func absencePayload(user uuid.UUID, delegate *uuid.UUID, start, end string, note *string, actor uuid.UUID) map[string]any {
	return map[string]any{"user_id": user, "delegate_id": delegate, "starts_on": start, "ends_on": end,
		"has_note": note != nil, "set_by_admin": actor != user}
}

// CreateAbsence records an absence. Members record their own; admins may
// record one for any member (sudden sick leave).
func (s *Store) CreateAbsence(ctx context.Context, wsID uuid.UUID, actor Viewer, in AbsenceInput) (Absence, error) {
	user := actor.UserID
	if in.UserID != nil {
		user = *in.UserID
	}
	if user != actor.UserID && !RoleAtLeast(actor.Role, RoleAdmin) {
		return Absence{}, &ForbiddenError{Code: "forbidden", Message: "only admins can record an absence for someone else"}
	}
	start, err := absenceDate("starts_on", in.StartsOn)
	if err != nil {
		return Absence{}, err
	}
	end, err := absenceDate("ends_on", in.EndsOn)
	if err != nil {
		return Absence{}, err
	}
	note, err := optionalText("note", in.Note, maxAbsenceNote)
	if err != nil {
		return Absence{}, err
	}
	id := newID()
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockAbsentMember(ctx, tx, wsID, user); err != nil {
			return err
		}
		today, err := workspaceToday(ctx, tx, wsID)
		if err != nil {
			return err
		}
		rules := absenceRules{ws: wsID, user: user, delegate: in.DelegateID, start: start, end: end, today: today, newStart: true}
		if err := rules.check(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_absences (id, workspace_id, user_id, delegate_id, starts_on, ends_on, note, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, wsID, user, in.DelegateID, start, end, note, actor.UserID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actor.UserID, "absence", id, EventAbsenceCreated,
			absencePayload(user, in.DelegateID, in.StartsOn, in.EndsOn, note, actor.UserID))
	})
	if err != nil {
		return Absence{}, err
	}
	return getAbsence(ctx, s.pool, wsID, id, actor)
}

// absenceRow is an absence as stored, read under its member's lock.
type absenceRow struct {
	user       uuid.UUID
	delegate   *uuid.UUID
	start, end time.Time
	note       *string
	ended      bool
}

// lockAbsence reads an absence for a change by actor: the member's row is
// locked first (same order as create), then the absence is read. Only the
// member and admins may change it, and only while it is current or upcoming.
func lockAbsence(ctx context.Context, tx pgx.Tx, wsID, id uuid.UUID, actor Viewer) (absenceRow, time.Time, error) {
	var r absenceRow
	err := tx.QueryRow(ctx, `SELECT user_id FROM workspace_absences WHERE workspace_id = $1 AND id = $2`, wsID, id).Scan(&r.user)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, time.Time{}, ErrNotFound
	}
	if err != nil {
		return r, time.Time{}, err
	}
	if r.user != actor.UserID && !RoleAtLeast(actor.Role, RoleAdmin) {
		return r, time.Time{}, &ForbiddenError{Code: "forbidden", Message: "only the absent member or an admin can change this absence"}
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2 FOR UPDATE`, wsID, r.user); err != nil {
		return r, time.Time{}, err
	}
	err = tx.QueryRow(ctx, `SELECT delegate_id, starts_on, ends_on, note, ended_at IS NOT NULL
		FROM workspace_absences WHERE id = $1 FOR UPDATE`, id).Scan(&r.delegate, &r.start, &r.end, &r.note, &r.ended)
	if err != nil {
		return r, time.Time{}, err
	}
	today, err := workspaceToday(ctx, tx, wsID)
	if err != nil {
		return r, time.Time{}, err
	}
	if r.ended || r.end.Before(today) {
		return r, today, &ConflictError{Code: "absence_closed", Message: "this absence is over or was ended; record a new one instead"}
	}
	return r, today, nil
}

// UpdateAbsence changes the delegate, dates or note of a current or
// upcoming absence. Once it has begun its start date is history and cannot
// move (decisions may already have been made through it).
func (s *Store) UpdateAbsence(ctx context.Context, wsID, id uuid.UUID, actor Viewer, p AbsencePatch) (Absence, error) {
	if p.StartsOn.Null || p.EndsOn.Null {
		return Absence{}, invalid("starts_on", "dates cannot be cleared")
	}
	var note *string
	if p.Note.Set && !p.Note.Null {
		var err error
		if note, err = optionalText("note", &p.Note.Value, maxAbsenceNote); err != nil {
			return Absence{}, err
		}
	}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, today, err := lockAbsence(ctx, tx, wsID, id, actor)
		if err != nil {
			return err
		}
		next := cur
		var changed []string
		if p.StartsOn.Set {
			d, err := absenceDate("starts_on", p.StartsOn.Value)
			if err != nil {
				return err
			}
			if !d.Equal(cur.start) {
				if !cur.start.After(today) {
					return invalid("starts_on", "the absence has already begun: its start date cannot change")
				}
				next.start = d
				changed = append(changed, "starts_on")
			}
		}
		if p.EndsOn.Set {
			d, err := absenceDate("ends_on", p.EndsOn.Value)
			if err != nil {
				return err
			}
			if !d.Equal(cur.end) {
				next.end = d
				changed = append(changed, "ends_on")
			}
		}
		if p.DelegateID.Set {
			next.delegate = p.DelegateID.Ptr()
			if !uuidPtrEqual(next.delegate, cur.delegate) {
				changed = append(changed, "delegate_id")
			}
		}
		if p.Note.Set {
			next.note = note
			if !strPtrEqual(note, cur.note) {
				changed = append(changed, "note")
			}
		}
		if len(changed) == 0 {
			return nil
		}
		rules := absenceRules{ws: wsID, user: cur.user, delegate: next.delegate, start: next.start, end: next.end, today: today,
			newStart: slices.Contains(changed, "starts_on"), self: &id}
		if err := rules.check(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_absences SET delegate_id = $2, starts_on = $3, ends_on = $4, note = $5, updated_at = now()
			WHERE id = $1`, id, next.delegate, next.start, next.end, next.note); err != nil {
			return err
		}
		payload := absencePayload(cur.user, next.delegate, next.start.Format(time.DateOnly), next.end.Format(time.DateOnly), next.note, actor.UserID)
		payload["changes"] = changed
		if slices.Contains(changed, "delegate_id") {
			payload["previous_delegate_id"] = cur.delegate
		}
		return appendEvent(ctx, tx, wsID, actor.UserID, "absence", id, EventAbsenceUpdated, payload)
	})
	if err != nil {
		return Absence{}, err
	}
	return getAbsence(ctx, s.pool, wsID, id, actor)
}

// EndAbsence ends a current absence early, or cancels an upcoming one.
// From that moment the delegate can no longer act on the member's behalf.
func (s *Store) EndAbsence(ctx context.Context, wsID, id uuid.UUID, actor Viewer) (Absence, error) {
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, today, err := lockAbsence(ctx, tx, wsID, id, actor)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_absences SET ended_at = now(), ended_by = $2, updated_at = now() WHERE id = $1`,
			id, actor.UserID); err != nil {
			return err
		}
		payload := absencePayload(cur.user, cur.delegate, cur.start.Format(time.DateOnly), cur.end.Format(time.DateOnly), cur.note, actor.UserID)
		payload["began"] = !cur.start.After(today)
		return appendEvent(ctx, tx, wsID, actor.UserID, "absence", id, EventAbsenceEnded, payload)
	})
	if err != nil {
		return Absence{}, err
	}
	return getAbsence(ctx, s.pool, wsID, id, actor)
}

func uuidPtrEqual(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func strPtrEqual(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
