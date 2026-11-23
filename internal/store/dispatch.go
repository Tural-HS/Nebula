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
}

// GetEvent loads one event.
func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `SELECT e.id, e.workspace_id, w.slug, e.actor_id, e.entity_type, e.entity_id, e.action,
			e.payload, e.source, e.automation_rule_id, e.occurred_at
		FROM events e JOIN workspaces w ON w.id = e.workspace_id WHERE e.id = $1`, id).
		Scan(&e.ID, &e.WorkspaceID, &e.WorkspaceSlug, &e.ActorID, &e.EntityType, &e.EntityID, &e.Action,
			&e.Payload, &e.Source, &e.RuleID, &e.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	return e, err
}

// FanOut enqueues one delivery job per matching webhook and one run job per
// matching automation rule. Dedupe keys make it safe to run more than once.
// Events caused by automation never trigger automation (loop prevention),
// but they are still delivered to webhooks.
func (s *Store) FanOut(ctx context.Context, e Event) (webhooks, rules int, err error) {
	if e.Action == WebhookPingAction {
		return 0, 0, nil // delivered directly to the pinged webhook only
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, events FROM webhooks WHERE workspace_id = $1 AND is_active`, e.WorkspaceID)
		if err != nil {
			return err
		}
		type hook struct {
			id     uuid.UUID
			events []string
		}
		hooks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (hook, error) {
			var h hook
			return h, r.Scan(&h.id, &h.events)
		})
		if err != nil {
			return err
		}
		for _, h := range hooks {
			if !MatchEvent(h.events, e.Action) {
				continue
			}
			if err := enqueueJob(ctx, tx, JobDeliverWebhook, map[string]any{"webhook_id": h.id, "event_id": e.ID}, 12,
				fmt.Sprintf("wh:%s:%d", h.id, e.ID)); err != nil {
				return err
			}
			webhooks++
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
func (s *Store) EnqueueEmail(ctx context.Context, to []string, subject, body, dedupeKey string) error {
	return enqueueJob(ctx, s.pool, JobSendEmail, map[string]any{"to": to, "subject": subject, "body": body}, 8, dedupeKey)
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
