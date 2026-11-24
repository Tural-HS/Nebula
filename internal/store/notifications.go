package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Notification kinds (ADR-0017, watcher kinds ADR-0028).
const (
	NotifyMention         = "mention"
	NotifyAssigned        = "assigned"
	NotifyOverdue         = "overdue"
	NotifyComment         = "comment"
	NotifyStatusChanged   = "status_changed"
	NotifyAssigneeChanged = "assignee_changed"
	NotifyDueChanged      = "due_changed"
	NotifyLinkAdded       = "link_added"
)

const (
	maxNotificationPage = 100
	// UnreadCountCap bounds the unread count query; the UI shows "99+" above 99.
	UnreadCountCap = 100
	// Retention, enforced by the worker's hourly housekeeping.
	notificationReadRetention = 90 * 24 * time.Hour
	notificationMaxRetention  = 180 * 24 * time.Hour
	notificationPurgeBatch    = 10_000
)

// isBulk reports whether an event is part of a bulk edit (ADR-0023). Bulk
// edits notify the new assignee as before, but never watchers: one bulk
// status change of 500 items must not become 500 notifications per watcher.
func isBulk(e Event) bool {
	_, ok := e.Payload["bulk"]
	return ok
}

// watchedChanges are the item.updated changes watchers hear about, in
// priority order (one notification per person and event: the first wins).
var watchedChanges = []struct{ change, kind string }{
	{"status_id", NotifyStatusChanged},
	{"assignee_id", NotifyAssigneeChanged},
	{"due_date", NotifyDueChanged},
}

// notifiable reports whether an event can produce notifications, so FanOut
// enqueues a notifications job only when there may be something to do.
// Imported items never dispatch (ADR-0011), so they never notify either.
func notifiable(e Event) bool {
	if e.EntityType != "item" || e.Source == "import" {
		return false
	}
	switch e.Action {
	case TriggerItemCreated, TriggerItemOverdue, "comment.created", "comment.updated", "link.added":
		return true
	case TriggerItemUpdated:
		changes, _ := e.Payload["changes"].(map[string]any)
		if _, ok := changes["assignee_id"]; ok {
			return true
		}
		if isBulk(e) {
			return false
		}
		for _, w := range watchedChanges {
			if _, ok := changes[w.change]; ok {
				return true
			}
		}
	}
	return false
}

// recipient is one person to notify about an event.
type recipient struct {
	id   uuid.UUID
	kind string
}

// payloadUUID reads a UUID string from a decoded JSON value (nil if absent).
func payloadUUID(v any) *uuid.UUID {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func payloadUUIDs(v any) []uuid.UUID {
	list, _ := v.([]any)
	var out []uuid.UUID
	for _, x := range list {
		if id := payloadUUID(x); id != nil {
			out = append(out, *id)
		}
	}
	return out
}

// CreateNotifications writes the in-app notifications for one event. It is
// idempotent: (event_id, recipient_id) is unique, so a retried job inserts
// nothing new. The actor is never notified, nobody gets two notifications
// for one event, and every recipient must be an active workspace member who
// can see the item's project right now (for link.added: both items).
// It returns the number of notifications created.
func (s *Store) CreateNotifications(ctx context.Context, e Event) (int, error) {
	if !notifiable(e) {
		return 0, nil
	}
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var projectID uuid.UUID
		var assignee, reporter *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT project_id, assignee_id, reporter_id FROM items
			WHERE workspace_id = $1 AND id = $2 AND deleted_at IS NULL`, e.WorkspaceID, e.EntityID).
			Scan(&projectID, &assignee, &reporter)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // deleted since: nothing to point at
		}
		if err != nil {
			return err
		}

		var want []recipient
		add := func(id *uuid.UUID, kind string) {
			if id == nil || (e.ActorID != nil && *id == *e.ActorID) {
				return
			}
			if slices.ContainsFunc(want, func(r recipient) bool { return r.id == *id }) {
				return // one notification per person and event; the first kind wins
			}
			want = append(want, recipient{id: *id, kind: kind})
		}
		var watchers []uuid.UUID
		loadWatchers := func() error {
			rows, err := tx.Query(ctx, `SELECT user_id FROM item_watchers WHERE item_id = $1 ORDER BY created_at, user_id`, e.EntityID)
			if err != nil {
				return err
			}
			watchers, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			return err
		}
		addWatchers := func(kind string) {
			for i := range watchers {
				add(&watchers[i], kind)
			}
		}
		var other *uuid.UUID // link.added: recipients must see the other item too
		switch e.Action {
		case TriggerItemCreated:
			snap, _ := e.Payload["snapshot"].(map[string]any)
			add(payloadUUID(snap["assignee"]), NotifyAssigned)
		case TriggerItemUpdated:
			changes, _ := e.Payload["changes"].(map[string]any)
			change, _ := changes["assignee_id"].(map[string]any)
			add(payloadUUID(change["to"]), NotifyAssigned)
			if !isBulk(e) {
				if err := loadWatchers(); err != nil {
					return err
				}
				for _, w := range watchedChanges {
					if _, ok := changes[w.change]; ok {
						addWatchers(w.kind)
					}
				}
			}
		case TriggerItemOverdue:
			add(payloadUUID(e.Payload["assignee_id"]), NotifyOverdue)
		case "comment.created":
			// Mentions first: someone mentioned on their own item gets one
			// notification, the mention.
			for _, id := range payloadUUIDs(e.Payload["mentions"]) {
				add(&id, NotifyMention)
			}
			add(assignee, NotifyComment)
			add(reporter, NotifyComment)
			if err := loadWatchers(); err != nil {
				return err
			}
			addWatchers(NotifyComment)
		case "comment.updated":
			for _, id := range payloadUUIDs(e.Payload["mentions_added"]) {
				add(&id, NotifyMention)
			}
		case "link.added":
			if other = payloadUUID(e.Payload["other_item_id"]); other == nil {
				return nil
			}
			if err := loadWatchers(); err != nil {
				return err
			}
			if src, _ := e.Payload["source"].(bool); !src {
				// Watchers of both items hear of the link once: on the source.
				rows, err := tx.Query(ctx, `SELECT user_id FROM item_watchers WHERE item_id = $1`, *other)
				if err != nil {
					return err
				}
				both, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
				if err != nil {
					return err
				}
				watchers = slices.DeleteFunc(watchers, func(id uuid.UUID) bool { return slices.Contains(both, id) })
			}
			addWatchers(NotifyLinkAdded)
		}
		if len(want) == 0 {
			return nil
		}

		ids := make([]string, len(want))
		kinds := make([]string, len(want))
		for i, r := range want {
			ids[i], kinds[i] = r.id.String(), r.kind
		}
		// One statement for every recipient: active member, sees the project
		// (and, for a link, the other item's project) right now.
		tag, err := tx.Exec(ctx, `INSERT INTO notifications (workspace_id, recipient_id, event_id, kind, project_id, item_id, actor_id)
			SELECT $1, r.id, $2, r.kind, $3, $4, $5
			FROM unnest($6::uuid[], $7::text[]) AS r (id, kind)
			JOIN users u ON u.id = r.id AND u.is_active
			JOIN workspace_members m ON m.workspace_id = $1 AND m.user_id = r.id
			JOIN projects p ON p.workspace_id = $1 AND p.id = $3
			WHERE `+visibleProjectFor("p", "r.id", "m.role IN ('admin', 'owner')")+`
			  AND ($8::uuid IS NULL OR EXISTS (SELECT 1 FROM items o JOIN projects op ON op.id = o.project_id
			       WHERE o.workspace_id = $1 AND o.id = $8 AND o.deleted_at IS NULL
			         AND `+visibleProjectFor("op", "r.id", "m.role IN ('admin', 'owner')")+`))
			ON CONFLICT (event_id, recipient_id) DO NOTHING`,
			e.WorkspaceID, e.ID, projectID, e.EntityID, e.ActorID, ids, kinds, other)
		if err != nil {
			return err
		}
		n = int(tag.RowsAffected())
		return nil
	})
	return n, err
}

// Notification is one entry of a user's notification list, with everything
// the UI needs to render it. Item key and title are current, not as of the event.
type Notification struct {
	ID            int64      `json:"id"`
	Kind          string     `json:"kind"`
	Read          bool       `json:"read"`
	CreatedAt     time.Time  `json:"created_at"`
	WorkspaceSlug string     `json:"workspace_slug"`
	WorkspaceName string     `json:"workspace_name"`
	ProjectKey    string     `json:"project_key"`
	ItemNumber    int64      `json:"item_number"`
	ItemKey       string     `json:"item_key"`
	ItemTitle     string     `json:"item_title"`
	ActorID       *uuid.UUID `json:"actor_id"`
	ActorName     *string    `json:"actor_name"`
}

// visibleNotification is the access predicate on a notification row n joined
// with its project p and the recipient's membership m: the recipient must
// still see the project (ADR-0013). Keep in sync with canSeeProject.
const visibleNotification = `(p.visibility = 'workspace' OR m.role IN ('admin', 'owner')
	OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = n.recipient_id))`

// notificationFrom joins what a visible notification needs. Rows of deleted
// items and of projects the recipient lost access to are filtered out.
const notificationFrom = ` FROM notifications n
	JOIN workspaces w ON w.id = n.workspace_id
	LEFT JOIN users a ON a.id = n.actor_id
	JOIN workspace_members m ON m.workspace_id = n.workspace_id AND m.user_id = n.recipient_id
	JOIN projects p ON p.workspace_id = n.workspace_id AND p.id = n.project_id
	JOIN items i ON i.workspace_id = n.workspace_id AND i.id = n.item_id AND i.deleted_at IS NULL
	WHERE n.recipient_id = $1 AND ` + visibleNotification

// NotificationFilter selects a page of a user's notifications.
type NotificationFilter struct {
	UnreadOnly bool
	Before     int64 // keyset cursor: only ids below this (0 = from the newest)
	Limit      int
}

// ListNotifications returns a user's visible notifications across all
// workspaces, newest first, and the cursor for the next page ("" = last page).
func (s *Store) ListNotifications(ctx context.Context, userID uuid.UUID, f NotificationFilter) ([]Notification, string, error) {
	limit := min(max(f.Limit, 1), maxNotificationPage)
	cond := ""
	if f.UnreadOnly {
		cond += ` AND n.read_at IS NULL`
	}
	if f.Before > 0 {
		cond += ` AND n.id < $3`
	}
	args := []any{userID, limit + 1}
	if f.Before > 0 {
		args = append(args, f.Before)
	}
	rows, err := s.pool.Query(ctx, `SELECT n.id, n.kind, n.read_at IS NOT NULL, n.created_at, w.slug, w.name,
			p.key, i.number, i.title, n.actor_id, a.display_name`+notificationFrom+cond+`
		ORDER BY n.id DESC LIMIT $2`, args...)
	if err != nil {
		return nil, "", err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Notification, error) {
		var n Notification
		err := r.Scan(&n.ID, &n.Kind, &n.Read, &n.CreatedAt, &n.WorkspaceSlug, &n.WorkspaceName,
			&n.ProjectKey, &n.ItemNumber, &n.ItemTitle, &n.ActorID, &n.ActorName)
		n.ItemKey = fmt.Sprintf("%s-%d", n.ProjectKey, n.ItemNumber)
		return n, err
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(list) > limit {
		list = list[:limit]
		next = strconv.FormatInt(list[len(list)-1].ID, 10)
	}
	if list == nil {
		list = []Notification{}
	}
	return list, next, nil
}

// UnreadNotifications counts a user's visible unread notifications, capped at
// UnreadCountCap: the badge is polled often, so the query walks at most that
// many rows of the partial unread index.
func (s *Store) UnreadNotifications(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1`+notificationFrom+` AND n.read_at IS NULL
		ORDER BY n.id DESC LIMIT $2) t`, userID, UnreadCountCap).Scan(&n)
	return n, err
}

// MarkNotificationRead marks one of the user's notifications read. Marking a
// read one again is a no-op; someone else's notification is ErrNotFound.
func (s *Store) MarkNotificationRead(ctx context.Context, userID uuid.UUID, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND recipient_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllNotificationsRead marks all of the user's unread notifications read,
// or only those of one workspace (by slug; the user must be a member).
// It returns how many were marked.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID, workspaceSlug string) (int64, error) {
	if workspaceSlug == "" {
		tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE recipient_id = $1 AND read_at IS NULL`, userID)
		return tag.RowsAffected(), err
	}
	var wsID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT w.id FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
		WHERE w.slug = $1 AND m.user_id = $2`, workspaceSlug, userID).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now()
		WHERE recipient_id = $1 AND workspace_id = $2 AND read_at IS NULL`, userID, wsID)
	return tag.RowsAffected(), err
}

// PurgeNotifications deletes read notifications older than readOlderThan and
// all notifications older than allOlderThan, at most batch rows of each per call.
func (s *Store) PurgeNotifications(ctx context.Context, readOlderThan, allOlderThan time.Duration, batch int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notifications WHERE id IN (
		SELECT id FROM notifications WHERE read_at IS NOT NULL AND created_at < now() - $1::interval LIMIT $2)`,
		readOlderThan.String(), batch)
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	tag, err = s.pool.Exec(ctx, `DELETE FROM notifications WHERE id IN (
		SELECT id FROM notifications WHERE created_at < now() - $1::interval LIMIT $2)`,
		allOlderThan.String(), batch)
	if err != nil {
		return n, err
	}
	return n + tag.RowsAffected(), nil
}
