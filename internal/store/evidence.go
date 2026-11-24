package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Audit evidence export (ADR-0029): everything an auditor needs to answer
// "who created, changed and approved this, and when", read in one consistent
// snapshot and limited to what the exporter may see (ADR-0013).

// EvidenceLimits bound one export. Every collection is counted while it is
// read; exceeding a limit fails the export instead of truncating it, because
// a silently incomplete evidence package is worse than none.
type EvidenceLimits struct {
	MaxDays        int // longest date range of a scope export
	MaxItems       int
	MaxEvents      int
	MaxComments    int
	MaxApprovals   int
	MaxAttachments int
	MaxLinks       int
}

// DefaultEvidenceLimits keep an export within a few seconds and well under
// 100 MB uncompressed.
var DefaultEvidenceLimits = EvidenceLimits{
	MaxDays: 366, MaxItems: 5000, MaxEvents: 50000, MaxComments: 20000,
	MaxApprovals: 5000, MaxAttachments: 20000, MaxLinks: 20000,
}

// EvidenceTooLargeError means a collection exceeds its limit: narrow the
// scope (fewer projects, a shorter range).
type EvidenceTooLargeError struct {
	What  string // items | events | comments | approvals | attachments | links
	Limit int
}

func (e *EvidenceTooLargeError) Error() string {
	return fmt.Sprintf("the export has more than %d %s; choose fewer projects or a shorter date range", e.Limit, e.What)
}

// EvidenceQuery selects what to export. Either an item (ProjectID + Number)
// or a scope (ProjectKeys, empty = every project the viewer can see, and a
// required From..To range).
type EvidenceQuery struct {
	WorkspaceID uuid.UUID
	Viewer      Viewer
	ProjectID   uuid.UUID
	Number      int64 // > 0: item export
	ProjectKeys []string
	// From and To are inclusive dates (YYYY-MM-DD) in the workspace time zone.
	From, To      string
	ApprovalsOnly bool
	Limits        EvidenceLimits
}

// IsItem reports whether q exports one item.
func (q EvidenceQuery) IsItem() bool { return q.Number > 0 }

// EvidenceProject is a project in the export.
type EvidenceProject struct {
	ID         uuid.UUID `json:"id"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	Visibility string    `json:"visibility"`
}

// EvidenceItem is an item as of export time.
type EvidenceItem struct {
	Item
	ProjectKey string     `json:"project_key"`
	DeletedAt  *time.Time `json:"deleted_at"`
}

// EvidenceEvent is one audit event, unabridged.
type EvidenceEvent struct {
	ID          int64          `json:"id"`
	WorkspaceID uuid.UUID      `json:"workspace_id"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Action      string         `json:"action"`
	EntityType  string         `json:"entity_type"`
	EntityID    uuid.UUID      `json:"entity_id"`
	ItemKey     string         `json:"item_key,omitempty"`
	ActorID     *uuid.UUID     `json:"actor_id"`
	Source      string         `json:"source"`
	RuleID      *uuid.UUID     `json:"automation_rule_id"`
	Payload     map[string]any `json:"payload"`
}

// EvidenceComment is a comment; the body of a deleted comment is not
// exported (nobody can read it in the app either).
type EvidenceComment struct {
	ID        uuid.UUID  `json:"id"`
	ItemID    uuid.UUID  `json:"item_id"`
	ItemKey   string     `json:"item_key"`
	AuthorID  *uuid.UUID `json:"author_id"`
	Body      *string    `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// EvidenceDecision is one approver's vote.
type EvidenceDecision struct {
	UserID    uuid.UUID `json:"user_id"`
	Decision  string    `json:"decision"`
	Comment   *string   `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// EvidenceApproval is an approval request (ADR-0026) with its policy
// snapshot and every decision.
type EvidenceApproval struct {
	ID              uuid.UUID          `json:"id"`
	ItemID          uuid.UUID          `json:"item_id"`
	ItemKey         string             `json:"item_key"`
	State           string             `json:"state"`
	FromStatusID    *uuid.UUID         `json:"from_status_id"`
	ToStatusID      *uuid.UUID         `json:"to_status_id"`
	RequestedBy     uuid.UUID          `json:"requested_by"`
	Comment         *string            `json:"comment"`
	MinApprovals    int                `json:"min_approvals"`
	Approvals       int                `json:"approvals"`
	ApproverRoles   []string           `json:"approver_roles"`
	ApproverUserIDs []uuid.UUID        `json:"approver_user_ids"`
	CreatedAt       time.Time          `json:"created_at"`
	ClosedAt        *time.Time         `json:"closed_at"`
	ClosedBy        *uuid.UUID         `json:"closed_by"`
	Decisions       []EvidenceDecision `json:"decisions"`
}

// EvidenceLink is a link (ADR-0028) whose both sides the viewer can see.
type EvidenceLink struct {
	ID        uuid.UUID  `json:"id"`
	Type      string     `json:"type"` // stored direction: source <type> target
	SourceID  uuid.UUID  `json:"source_id"`
	SourceKey string     `json:"source_key"`
	TargetID  uuid.UUID  `json:"target_id"`
	TargetKey string     `json:"target_key"`
	CreatedBy *uuid.UUID `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// EvidenceAttachment is attachment metadata with the blob hash (ADR-0022);
// file contents are never part of an export.
type EvidenceAttachment struct {
	ID          uuid.UUID  `json:"id"`
	ItemID      uuid.UUID  `json:"item_id"`
	ItemKey     string     `json:"item_key"`
	CommentID   *uuid.UUID `json:"comment_id"`
	UploaderID  *uuid.UUID `json:"uploader_id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	CreatedAt   time.Time  `json:"created_at"`
}

// EvidencePerson is a user referenced anywhere in the export.
type EvidencePerson struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Role        *string   `json:"workspace_role"` // nil: no longer a member
}

// EvidenceStatus resolves status ids in events and approvals.
type EvidenceStatus struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Category string    `json:"category"`
}

// EvidenceSet is the content of one export.
type EvidenceSet struct {
	Workspace   Workspace
	AsOf        time.Time  // the snapshot's transaction time
	RangeStart  *time.Time // nil for an item export (its whole history)
	RangeEnd    *time.Time // exclusive
	Projects    []EvidenceProject
	Items       []EvidenceItem
	Events      []EvidenceEvent
	Comments    []EvidenceComment
	Approvals   []EvidenceApproval
	Links       []EvidenceLink
	Attachments []EvidenceAttachment
	People      []EvidencePerson
	Statuses    []EvidenceStatus
}

// CollectEvidence reads an export's content in one REPEATABLE READ, read-only
// transaction, so every file describes the same instant. Items, comments,
// approvals, links and attachments are their state at that instant; events
// are filtered by the date range (an item export has its whole history).
func (s *Store) CollectEvidence(ctx context.Context, q EvidenceQuery) (*EvidenceSet, error) {
	if q.Limits == (EvidenceLimits{}) {
		q.Limits = DefaultEvidenceLimits
	}
	var set *EvidenceSet
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		set, err = collectEvidence(ctx, tx, q)
		return err
	})
	return set, err
}

func collectEvidence(ctx context.Context, tx pgx.Tx, q EvidenceQuery) (*EvidenceSet, error) {
	set := &EvidenceSet{}
	admin := RoleAtLeast(q.Viewer.Role, RoleAdmin)
	err := tx.QueryRow(ctx, `SELECT id, slug, name, timezone, now() FROM workspaces WHERE id = $1`, q.WorkspaceID).
		Scan(&set.Workspace.ID, &set.Workspace.Slug, &set.Workspace.Name, &set.Workspace.Timezone, &set.AsOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	set.Workspace.Role = q.Viewer.Role

	var itemIDs []uuid.UUID
	if q.IsItem() {
		item, err := liveItem(ctx, tx, q.WorkspaceID, q.ProjectID, q.Number)
		if err != nil {
			return nil, err
		}
		itemIDs = []uuid.UUID{item.id}
		// The same visibility predicate as everywhere else: a hidden project is 404.
		if set.Projects, err = evidenceProjects(ctx, tx, q, admin, []uuid.UUID{q.ProjectID}, nil); err != nil {
			return nil, err
		}
		if len(set.Projects) == 0 {
			return nil, ErrNotFound
		}
	} else {
		if err := evidenceRange(ctx, tx, q, set); err != nil {
			return nil, err
		}
		if set.Projects, err = evidenceProjects(ctx, tx, q, admin, nil, q.ProjectKeys); err != nil {
			return nil, err
		}
		if itemIDs, err = evidenceScopeItems(ctx, tx, q, set); err != nil {
			return nil, err
		}
	}

	if set.Items, err = evidenceItems(ctx, tx, itemIDs); err != nil {
		return nil, err
	}
	keys := make(map[uuid.UUID]string, len(set.Items))
	for _, it := range set.Items {
		keys[it.ID] = it.Key
	}
	steps := []func() error{
		func() (err error) { set.Events, err = evidenceEvents(ctx, tx, q, set, itemIDs, keys); return err },
		func() (err error) { set.Comments, err = evidenceComments(ctx, tx, q, itemIDs, keys); return err },
		func() (err error) { set.Approvals, err = evidenceApprovals(ctx, tx, q, itemIDs, keys); return err },
		func() (err error) { set.Links, err = evidenceLinks(ctx, tx, q, admin, itemIDs); return err },
		func() (err error) { set.Attachments, err = evidenceAttachments(ctx, tx, q, itemIDs, keys); return err },
		func() (err error) { set.Statuses, err = evidenceStatuses(ctx, tx, q.WorkspaceID); return err },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	set.People, err = evidencePeople(ctx, tx, q.WorkspaceID, set)
	return set, err
}

// CheckEvidenceScope validates a scope export's input without touching the
// database, so the API can reject bad input before it is rate limited.
func CheckEvidenceScope(q EvidenceQuery) error {
	if len(q.ProjectKeys) > 50 {
		return invalid("project_keys", "at most 50 projects")
	}
	_, _, err := evidenceDates(q)
	return err
}

func evidenceDates(q EvidenceQuery) (string, string, error) {
	maxDays := q.Limits.MaxDays
	if maxDays == 0 {
		maxDays = DefaultEvidenceLimits.MaxDays
	}
	from, err := dueDateField("from", &q.From)
	if err != nil || from == nil {
		return "", "", invalid("from", "a start date (YYYY-MM-DD) is required")
	}
	to, err := dueDateField("to", &q.To)
	if err != nil || to == nil {
		return "", "", invalid("to", "an end date (YYYY-MM-DD) is required")
	}
	f, _ := time.Parse(time.DateOnly, *from)
	t, _ := time.Parse(time.DateOnly, *to)
	switch days := int(t.Sub(f).Hours()/24) + 1; {
	case days < 1:
		return "", "", invalid("to", "the end date is before the start date")
	case days > maxDays:
		return "", "", invalid("to", "the range may cover at most %d days", maxDays)
	}
	return *from, *to, nil
}

// evidenceRange converts the scope's dates to instants in the workspace
// time zone: [From 00:00, To+1 00:00).
func evidenceRange(ctx context.Context, tx pgx.Tx, q EvidenceQuery, set *EvidenceSet) error {
	from, to, err := evidenceDates(q)
	if err != nil {
		return err
	}
	var start, end time.Time
	if err := tx.QueryRow(ctx, `SELECT $1::date::timestamp AT TIME ZONE $3, ($2::date + 1)::timestamp AT TIME ZONE $3`,
		from, to, set.Workspace.Timezone).Scan(&start, &end); err != nil {
		return err
	}
	set.RangeStart, set.RangeEnd = &start, &end
	return nil
}

// evidenceProjects resolves the projects in scope, only among those the
// viewer can see. An unknown and a hidden key get the same error, so the
// existence of a private project does not leak.
func evidenceProjects(ctx context.Context, tx pgx.Tx, q EvidenceQuery, admin bool, ids []uuid.UUID, keys []string) ([]EvidenceProject, error) {
	if len(keys) > 50 {
		return nil, invalid("project_keys", "at most 50 projects")
	}
	rows, err := tx.Query(ctx, `SELECT p.id, p.key, p.name, p.visibility FROM projects p
		WHERE p.workspace_id = $1 AND `+visibleProjectFor("p", "$2", "$3")+`
		  AND ($4::uuid[] IS NULL OR p.id = ANY($4)) AND (cardinality($5::text[]) = 0 OR p.key = ANY($5))
		ORDER BY p.key`, q.WorkspaceID, q.Viewer.UserID, admin, ids, nonNilSlice(keys))
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceProject, error) {
		var p EvidenceProject
		return p, r.Scan(&p.ID, &p.Key, &p.Name, &p.Visibility)
	})
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if !slices.ContainsFunc(list, func(p EvidenceProject) bool { return p.Key == k }) {
			return nil, invalid("project_keys", "unknown project %q", k)
		}
	}
	return nonNilSlice(list), nil
}

// evidenceScopeItems are the items of the scope's projects with any event in
// the range (and, with ApprovalsOnly, an approval request opened or closed in
// it). Deleted items are included: their history is evidence too.
func evidenceScopeItems(ctx context.Context, tx pgx.Tx, q EvidenceQuery, set *EvidenceSet) ([]uuid.UUID, error) {
	projects := make([]uuid.UUID, len(set.Projects))
	for i, p := range set.Projects {
		projects[i] = p.ID
	}
	rows, err := tx.Query(ctx, `SELECT i.id FROM items i JOIN projects p ON p.id = i.project_id
		WHERE i.workspace_id = $1 AND i.project_id = ANY($2)
		  AND EXISTS (SELECT 1 FROM events e WHERE e.entity_type = 'item' AND e.entity_id = i.id
		              AND e.workspace_id = $1 AND e.occurred_at >= $3 AND e.occurred_at < $4)
		  AND (NOT $5 OR EXISTS (SELECT 1 FROM approval_requests r WHERE r.item_id = i.id
		              AND ((r.created_at >= $3 AND r.created_at < $4) OR (r.closed_at >= $3 AND r.closed_at < $4))))
		ORDER BY p.key, i.number LIMIT $6`,
		q.WorkspaceID, projects, *set.RangeStart, *set.RangeEnd, q.ApprovalsOnly, q.Limits.MaxItems+1)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	if len(ids) > q.Limits.MaxItems {
		return nil, &EvidenceTooLargeError{What: "items", Limit: q.Limits.MaxItems}
	}
	return ids, nil
}

func evidenceItems(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) ([]EvidenceItem, error) {
	rows, err := tx.Query(ctx, itemSelect+` WHERE i.id = ANY($1) ORDER BY p.key, i.number`, ids)
	if err != nil {
		return nil, err
	}
	out := []EvidenceItem{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		key, _, _ := strings.Cut(it.Key, "-")
		out = append(out, EvidenceItem{Item: it, ProjectKey: key})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	deleted := map[uuid.UUID]time.Time{}
	rows, err = tx.Query(ctx, `SELECT id, deleted_at FROM items WHERE id = ANY($1) AND deleted_at IS NOT NULL`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uuid.UUID
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			rows.Close()
			return nil, err
		}
		deleted[id] = at
	}
	rows.Close()
	for i := range out {
		if at, ok := deleted[out[i].ID]; ok {
			out[i].DeletedAt = &at
		}
	}
	return out, rows.Err()
}

// evidenceEvents are the items' events (link events only when the viewer can
// see the other item, as in the activity feed) and, for a scope export, the
// projects' own events and approval policy changes.
func evidenceEvents(ctx context.Context, tx pgx.Tx, q EvidenceQuery, set *EvidenceSet, itemIDs []uuid.UUID, keys map[uuid.UUID]string) ([]EvidenceEvent, error) {
	var projects []uuid.UUID
	if !q.IsItem() {
		for _, p := range set.Projects {
			projects = append(projects, p.ID)
		}
	}
	rows, err := tx.Query(ctx, `SELECT e.id, e.occurred_at, e.action, e.entity_type, e.entity_id, e.actor_id, e.source,
			e.automation_rule_id, e.payload
		FROM events e
		WHERE e.workspace_id = $1
		  AND ((e.entity_type = 'item' AND e.entity_id = ANY($2))
		       OR (e.entity_type = 'project' AND e.entity_id = ANY($3))
		       OR (NOT $4 AND e.entity_type = 'workflow' AND e.action LIKE 'approval\_policy.%'))
		  AND ($5::timestamptz IS NULL OR (e.occurred_at >= $5 AND e.occurred_at < $6))
		  AND (e.action NOT IN ('link.added', 'link.removed') OR EXISTS (
		       SELECT 1 FROM items o JOIN projects op ON op.id = o.project_id
		       WHERE o.workspace_id = $1 AND o.id = (e.payload->>'other_item_id')::uuid
		         AND `+visibleProjectFor("op", "$7", "$8")+`))
		ORDER BY e.id LIMIT $9`,
		q.WorkspaceID, itemIDs, nonNilSlice(projects), q.IsItem(), set.RangeStart, set.RangeEnd,
		q.Viewer.UserID, RoleAtLeast(q.Viewer.Role, RoleAdmin), q.Limits.MaxEvents+1)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceEvent, error) {
		e := EvidenceEvent{WorkspaceID: q.WorkspaceID}
		err := r.Scan(&e.ID, &e.OccurredAt, &e.Action, &e.EntityType, &e.EntityID, &e.ActorID, &e.Source, &e.RuleID, &e.Payload)
		if e.EntityType == "item" {
			e.ItemKey = keys[e.EntityID]
		}
		e.OccurredAt = e.OccurredAt.UTC()
		return e, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) > q.Limits.MaxEvents {
		return nil, &EvidenceTooLargeError{What: "events", Limit: q.Limits.MaxEvents}
	}
	return nonNilSlice(list), nil
}

func evidenceComments(ctx context.Context, tx pgx.Tx, q EvidenceQuery, itemIDs []uuid.UUID, keys map[uuid.UUID]string) ([]EvidenceComment, error) {
	rows, err := tx.Query(ctx, `SELECT id, item_id, author_id, CASE WHEN deleted_at IS NULL THEN body END, created_at, updated_at, deleted_at
		FROM comments WHERE workspace_id = $1 AND item_id = ANY($2) ORDER BY created_at, id LIMIT $3`,
		q.WorkspaceID, itemIDs, q.Limits.MaxComments+1)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceComment, error) {
		var c EvidenceComment
		err := r.Scan(&c.ID, &c.ItemID, &c.AuthorID, &c.Body, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt)
		c.ItemKey = keys[c.ItemID]
		return c, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) > q.Limits.MaxComments {
		return nil, &EvidenceTooLargeError{What: "comments", Limit: q.Limits.MaxComments}
	}
	return nonNilSlice(list), nil
}

func evidenceApprovals(ctx context.Context, tx pgx.Tx, q EvidenceQuery, itemIDs []uuid.UUID, keys map[uuid.UUID]string) ([]EvidenceApproval, error) {
	rows, err := tx.Query(ctx, `SELECT id, item_id, state, from_status_id, to_status_id, requested_by, comment, min_approvals,
			approvals, approver_roles, approver_user_ids, created_at, closed_at, closed_by
		FROM approval_requests WHERE workspace_id = $1 AND item_id = ANY($2) ORDER BY created_at, id LIMIT $3`,
		q.WorkspaceID, itemIDs, q.Limits.MaxApprovals+1)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceApproval, error) {
		var a EvidenceApproval
		err := r.Scan(&a.ID, &a.ItemID, &a.State, &a.FromStatusID, &a.ToStatusID, &a.RequestedBy, &a.Comment, &a.MinApprovals,
			&a.Approvals, &a.ApproverRoles, &a.ApproverUserIDs, &a.CreatedAt, &a.ClosedAt, &a.ClosedBy)
		a.ItemKey, a.Decisions = keys[a.ItemID], []EvidenceDecision{}
		return a, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) > q.Limits.MaxApprovals {
		return nil, &EvidenceTooLargeError{What: "approvals", Limit: q.Limits.MaxApprovals}
	}
	if len(list) == 0 {
		return []EvidenceApproval{}, nil
	}
	ids := make([]uuid.UUID, len(list))
	index := make(map[uuid.UUID]int, len(list))
	for i, a := range list {
		ids[i], index[a.ID] = a.ID, i
	}
	rows, err = tx.Query(ctx, `SELECT request_id, user_id, decision, comment, created_at FROM approval_decisions
		WHERE request_id = ANY($1) ORDER BY created_at, user_id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var reqID uuid.UUID
		var d EvidenceDecision
		if err := rows.Scan(&reqID, &d.UserID, &d.Decision, &d.Comment, &d.CreatedAt); err != nil {
			return nil, err
		}
		a := &list[index[reqID]]
		a.Decisions = append(a.Decisions, d)
	}
	return list, rows.Err()
}

func evidenceLinks(ctx context.Context, tx pgx.Tx, q EvidenceQuery, admin bool, itemIDs []uuid.UUID) ([]EvidenceLink, error) {
	rows, err := tx.Query(ctx, `SELECT l.id, l.type, l.source_id, sp.key, s.number, l.target_id, tp.key, t.number, l.created_by, l.created_at
		FROM item_links l
		JOIN items s ON s.id = l.source_id AND s.deleted_at IS NULL JOIN projects sp ON sp.id = s.project_id
		JOIN items t ON t.id = l.target_id AND t.deleted_at IS NULL JOIN projects tp ON tp.id = t.project_id
		WHERE l.workspace_id = $1 AND (l.source_id = ANY($2) OR l.target_id = ANY($2))
		  AND `+visibleProjectFor("sp", "$3", "$4")+` AND `+visibleProjectFor("tp", "$3", "$4")+`
		ORDER BY l.created_at, l.id LIMIT $5`, q.WorkspaceID, itemIDs, q.Viewer.UserID, admin, q.Limits.MaxLinks+1)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceLink, error) {
		var l EvidenceLink
		var sk, tk string
		var sn, tn int64
		err := r.Scan(&l.ID, &l.Type, &l.SourceID, &sk, &sn, &l.TargetID, &tk, &tn, &l.CreatedBy, &l.CreatedAt)
		l.SourceKey, l.TargetKey = itemKey(sk, sn), itemKey(tk, tn)
		return l, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) > q.Limits.MaxLinks {
		return nil, &EvidenceTooLargeError{What: "links", Limit: q.Limits.MaxLinks}
	}
	return nonNilSlice(list), nil
}

func evidenceAttachments(ctx context.Context, tx pgx.Tx, q EvidenceQuery, itemIDs []uuid.UUID, keys map[uuid.UUID]string) ([]EvidenceAttachment, error) {
	rows, err := tx.Query(ctx, `SELECT a.id, a.item_id, a.comment_id, a.uploader_id, a.filename, a.content_type, a.size_bytes, a.sha256, a.created_at
		FROM attachments a LEFT JOIN comments c ON c.id = a.comment_id
		WHERE a.workspace_id = $1 AND a.item_id = ANY($2) AND c.deleted_at IS NULL
		ORDER BY a.created_at, a.id LIMIT $3`, q.WorkspaceID, itemIDs, q.Limits.MaxAttachments+1)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceAttachment, error) {
		var a EvidenceAttachment
		var sum []byte
		err := r.Scan(&a.ID, &a.ItemID, &a.CommentID, &a.UploaderID, &a.Filename, &a.ContentType, &a.Size, &sum, &a.CreatedAt)
		a.SHA256, a.ItemKey = hex.EncodeToString(sum), keys[a.ItemID]
		return a, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) > q.Limits.MaxAttachments {
		return nil, &EvidenceTooLargeError{What: "attachments", Limit: q.Limits.MaxAttachments}
	}
	return nonNilSlice(list), nil
}

func evidenceStatuses(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) ([]EvidenceStatus, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, category FROM statuses WHERE workspace_id = $1 ORDER BY workflow_id, position, id`, wsID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidenceStatus, error) {
		var s EvidenceStatus
		return s, r.Scan(&s.ID, &s.Name, &s.Category)
	})
	return nonNilSlice(list), err
}

// evidencePeople resolves every user id the export refers to, with their
// current workspace role.
func evidencePeople(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, set *EvidenceSet) ([]EvidencePerson, error) {
	seen := map[uuid.UUID]bool{}
	add := func(id *uuid.UUID) {
		if id != nil && *id != uuid.Nil {
			seen[*id] = true
		}
	}
	for _, it := range set.Items {
		add(it.AssigneeID)
		add(it.ReporterID)
	}
	for _, e := range set.Events {
		add(e.ActorID)
	}
	for _, c := range set.Comments {
		add(c.AuthorID)
	}
	for _, a := range set.Approvals {
		add(&a.RequestedBy)
		add(a.ClosedBy)
		for _, u := range a.ApproverUserIDs {
			add(&u)
		}
		for _, d := range a.Decisions {
			add(&d.UserID)
		}
	}
	for _, l := range set.Links {
		add(l.CreatedBy)
	}
	for _, a := range set.Attachments {
		add(a.UploaderID)
	}
	ids := make([]uuid.UUID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	rows, err := tx.Query(ctx, `SELECT u.id, u.display_name, u.email, m.role FROM users u
		LEFT JOIN workspace_members m ON m.workspace_id = $1 AND m.user_id = u.id
		WHERE u.id = ANY($2) ORDER BY u.display_name, u.id`, wsID, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (EvidencePerson, error) {
		var p EvidencePerson
		return p, r.Scan(&p.ID, &p.DisplayName, &p.Email, &p.Role)
	})
	return nonNilSlice(list), err
}

// RecordEvidenceExport appends the evidence.exported event: on the item for
// an item export (it shows in the item's history), on the workspace for a
// scope export.
func (s *Store) RecordEvidenceExport(ctx context.Context, wsID, actorID uuid.UUID, itemID *uuid.UUID, payload map[string]any) error {
	entityType, entityID := "workspace", wsID
	if itemID != nil {
		entityType, entityID = "item", *itemID
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		return appendEvent(ctx, tx, wsID, actorID, entityType, entityID, "evidence.exported", payload)
	})
}
