package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Event is a stored audit event as needed by dispatch and delivery.
type Event struct {
	ID            int64          `json:"id"`
	WorkspaceID   uuid.UUID      `json:"workspace_id"`
	WorkspaceSlug string         `json:"workspace_slug"`
	ActorID       *uuid.UUID     `json:"actor_id"`
	EntityType    string         `json:"entity_type"`
	EntityID      uuid.UUID      `json:"entity_id"`
	Action        string         `json:"action"`
	Payload       map[string]any `json:"payload"`
	Source        string         `json:"source"`
	RuleID        *uuid.UUID     `json:"automation_rule_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	// WorkspaceArchived: the workspace is archived or being deleted; its
	// events fire nothing (ADR-0039).
	WorkspaceArchived bool `json:"-"`
}

// GetEvent loads one event.
func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `SELECT e.id, e.workspace_id, w.slug, e.actor_id, e.entity_type, e.entity_id, e.action,
			e.payload, e.source, e.automation_rule_id, e.occurred_at, w.archived_at IS NOT NULL
		FROM events e JOIN workspaces w ON w.id = e.workspace_id WHERE e.id = $1`, id).
		Scan(&e.ID, &e.WorkspaceID, &e.WorkspaceSlug, &e.ActorID, &e.EntityType, &e.EntityID, &e.Action,
			&e.Payload, &e.Source, &e.RuleID, &e.OccurredAt, &e.WorkspaceArchived)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return e, err
}

// FanOut enqueues one delivery job per matching webhook, one run job per
// matching automation rule and, for events that can notify people, one
// notifications job. Dedupe keys make it safe to run more than once.
// Events caused by automation never trigger automation (loop prevention),
// but they are still delivered to webhooks. Events of an archived workspace
// or project fire nothing (ADR-0039).
func (s *Store) FanOut(ctx context.Context, e Event) (webhooks, rules int, err error) {
	if e.Action == WebhookPingAction || e.WorkspaceArchived {
		return 0, 0, nil // a ping is delivered directly to the pinged webhook only
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var scope *EventScope // resolved once, only if needed
		if projectScoped[e.EntityType] {
			sc, err := eventScope(ctx, tx, e)
			if err != nil {
				return err
			}
			if sc.Archived {
				return nil
			}
			scope = &sc
		}
		rows, err := tx.Query(ctx, `SELECT id, events, project_ids FROM webhooks WHERE workspace_id = $1 AND is_active`, e.WorkspaceID)
		if err != nil {
			return err
		}
		type hook struct {
			id       uuid.UUID
			events   []string
			projects []uuid.UUID
		}
		hooks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (hook, error) {
			var h hook
			return h, r.Scan(&h.id, &h.events, &h.projects)
		})
		if err != nil {
			return err
		}
		for _, h := range hooks {
			if !MatchEventOf(h.events, e) {
				continue
			}
			if scope == nil {
				sc, err := eventScope(ctx, tx, e)
				if err != nil {
					return err
				}
				scope = &sc
			}
			// Project filter (ADR-0013): private projects only reach webhooks
			// that list them. Checked again at delivery time.
			if !scope.Allows(h.projects) {
				continue
			}
			if err := enqueueJob(ctx, tx, JobDeliverWebhook, map[string]any{"webhook_id": h.id, "event_id": e.ID}, 12,
				fmt.Sprintf("wh:%s:%d", h.id, e.ID)); err != nil {
				return err
			}
			webhooks++
		}

		// In-app notifications: one job per event, deduped on the event ID.
		if notifiable(e) {
			if err := enqueueJob(ctx, tx, JobCreateNotifications, map[string]any{"event_id": e.ID}, 5,
				fmt.Sprintf("nt:%d", e.ID)); err != nil {
				return err
			}
		}

		if e.Source == "automation" || (e.Action != TriggerItemCreated && e.Action != TriggerItemUpdated && e.Action != TriggerItemOverdue) {
			return nil
		}
		rows, err = tx.Query(ctx, `SELECT id FROM automation_rules WHERE workspace_id = $1 AND trigger = $2 AND is_enabled
			ORDER BY created_at, id`,
			e.WorkspaceID, e.Action)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := enqueueJob(ctx, tx, JobRunAutomation, map[string]any{"rule_id": id, "event_id": e.ID}, 5,
				fmt.Sprintf("ar:%s:%d", id, e.ID)); err != nil {
				return err
			}
			rules++
		}
		return nil
	})
	return webhooks, rules, err
}

// EnqueueEmail queues an email job; dedupeKey prevents duplicates on retries.
// wsID selects the workspace's SMTP settings (ADR-0038); uuid.Nil: instance.
func (s *Store) EnqueueEmail(ctx context.Context, wsID uuid.UUID, to []string, subject, body, dedupeKey string) error {
	return enqueueJob(ctx, s.pool, JobSendEmail, emailPayload(wsID, to, subject, body), 8, dedupeKey)
}

// EmailJob is the payload of a JobSendEmail job. WorkspaceID is absent in
// jobs queued before ADR-0038 and for mail without a workspace context; both
// are sent through the instance configuration.
type EmailJob struct {
	WorkspaceID *uuid.UUID `json:"workspace_id,omitempty"`
	To          []string   `json:"to"`
	Subject     string     `json:"subject"`
	Body        string     `json:"body"`
}

func emailPayload(wsID uuid.UUID, to []string, subject, body string) EmailJob {
	j := EmailJob{To: to, Subject: subject, Body: body}
	if wsID != uuid.Nil {
		j.WorkspaceID = &wsID
	}
	return j
}

// ItemByID returns a live item by its ID within a workspace, with its project ID.
func (s *Store) ItemByID(ctx context.Context, wsID, id uuid.UUID) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, itemSelect+` WHERE i.workspace_id = $1 AND i.id = $2 AND i.deleted_at IS NULL`, wsID, id))
}

// MemberEmails returns active members' emails for the given user IDs.
func (s *Store) MemberEmails(ctx context.Context, wsID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.email FROM users u JOIN workspace_members m ON m.user_id = u.id
		WHERE m.workspace_id = $1 AND u.is_active AND u.id = ANY($2::uuid[])`, wsID, strs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		out[id] = email
	}
	return out, rows.Err()
}

// IsMemberEmail reports whether email belongs to an active workspace member.
func (s *Store) IsMemberEmail(ctx context.Context, wsID uuid.UUID, email string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users u JOIN workspace_members m ON m.user_id = u.id
		WHERE m.workspace_id = $1 AND u.email = $2 AND u.is_active)`, wsID, email).Scan(&ok)
	return ok, err
}

// ItemLabel is how notifications refer to an item.
type ItemLabel struct {
	Key        string // e.g. QA-12
	ProjectKey string
	Number     int64
	Title      string
}

// ItemLabelByID returns an item's key and title, including deleted items
// (an item.deleted notification still needs to say which item it was).
func (s *Store) ItemLabelByID(ctx context.Context, wsID, id uuid.UUID) (ItemLabel, error) {
	var l ItemLabel
	err := s.pool.QueryRow(ctx, `SELECT p.key, i.number, i.title FROM items i JOIN projects p ON p.id = i.project_id
		WHERE i.workspace_id = $1 AND i.id = $2`, wsID, id).Scan(&l.ProjectKey, &l.Number, &l.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	l.Key = fmt.Sprintf("%s-%d", l.ProjectKey, l.Number)
	return l, err
}

// DisplayNames returns display names of the given users (former members
// included: an event's actor may have left since).
func (s *Store) DisplayNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return s.namesByID(ctx, `SELECT id, display_name FROM users WHERE id = ANY($1::uuid[])`, ids)
}

// StatusCategories maps status IDs to their category (todo, in_progress, done, canceled).
func (s *Store) StatusCategories(ctx context.Context, wsID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return s.namesByID(ctx, `SELECT id, category FROM statuses WHERE id = ANY($1::uuid[]) AND workspace_id = $2`, ids, wsID)
}

// StatusNames returns the names of the workspace's statuses with the given IDs.
func (s *Store) StatusNames(ctx context.Context, wsID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return s.namesByID(ctx, `SELECT id, name FROM statuses WHERE id = ANY($1::uuid[]) AND workspace_id = $2`, ids, wsID)
}

func (s *Store) namesByID(ctx context.Context, query string, ids []uuid.UUID, args ...any) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	rows, err := s.pool.Query(ctx, query, append([]any{strs}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}
