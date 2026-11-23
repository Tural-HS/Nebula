package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/secretbox"
)

const (
	maxWebhooksPerWorkspace = 20
	// WebhookDisableAfter consecutive failed attempts disables a webhook.
	WebhookDisableAfter = 50
)

// "*", "item.*" or an exact action like "item.updated".
var eventPatternRe = regexp.MustCompile(`^(\*|[a-z_]{1,50}\.\*|[a-z_]{1,50}\.[a-z_.]{1,50})$`)

// WithSecrets enables features that store encrypted secrets (webhooks).
func (s *Store) WithSecrets(box *secretbox.Box) *Store {
	s.box = box
	return s
}

// Webhook is an outgoing HTTP subscription. The signing secret is never returned
// after creation (or rotation).
type Webhook struct {
	ID             uuid.UUID `json:"id"`
	URL            string    `json:"url"`
	Events         []string  `json:"events"`
	Active         bool      `json:"active"`
	FailureCount   int       `json:"failure_count"`
	DisabledReason *string   `json:"disabled_reason"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WebhookDelivery is one delivery attempt, for debugging receivers.
type WebhookDelivery struct {
	ID          int64     `json:"id"`
	EventID     int64     `json:"event_id"`
	Attempt     int       `json:"attempt"`
	StatusCode  *int      `json:"status_code"`
	Error       *string   `json:"error"`
	DurationMS  int       `json:"duration_ms"`
	DeliveredAt time.Time `json:"delivered_at"`
}

// URLValidator checks a webhook URL (SSRF policy lives in the caller).
type URLValidator func(string) error

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 || len(events) > 50 {
		return nil, invalid("events", "must contain 1 to 50 event patterns")
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if !eventPatternRe.MatchString(e) {
			return nil, invalid("events", "invalid event pattern %q (use \"*\", \"item.*\" or \"item.updated\")", e)
		}
		out = append(out, e)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// MatchEvent reports whether action matches any subscription pattern.
func MatchEvent(patterns []string, action string) bool {
	for _, p := range patterns {
		if p == "*" || p == action {
			return true
		}
		if prefix, ok := strings.CutSuffix(p, ".*"); ok && strings.HasPrefix(action, prefix+".") {
			return true
		}
	}
	return false
}

func newWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b)
}

func (s *Store) sealSecret(id uuid.UUID, secret string) ([]byte, error) {
	if s.box == nil {
		return nil, errors.New("webhooks require NEBULA_ENCRYPTION_KEY")
	}
	return s.box.Seal([]byte(secret), id[:])
}

const webhookSelect = `SELECT id, url, events, is_active, failure_count, disabled_reason, created_at, updated_at FROM webhooks`

func scanWebhook(row pgx.Row) (Webhook, error) {
	var w Webhook
	err := row.Scan(&w.ID, &w.URL, &w.Events, &w.Active, &w.FailureCount, &w.DisabledReason, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	return w, err
}

// CreateWebhook registers a webhook and returns its signing secret (shown once).
func (s *Store) CreateWebhook(ctx context.Context, wsID, actorID uuid.UUID, url string, events []string, check URLValidator) (Webhook, string, error) {
	url = strings.TrimSpace(url)
	if err := check(url); err != nil {
		return Webhook{}, "", invalid("url", "%s", err.Error())
	}
	events, err := normalizeEvents(events)
	if err != nil {
		return Webhook{}, "", err
	}
	id := newID()
	secret := newWebhookSecret()
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return Webhook{}, "", err
	}
	var w Webhook
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, wsID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM webhooks WHERE workspace_id = $1`, wsID).Scan(&n); err != nil {
			return err
		}
		if n >= maxWebhooksPerWorkspace {
			return &ConflictError{Code: "webhook_limit_reached", Message: "a workspace can have at most 20 webhooks"}
		}
		w, err = scanWebhook(tx.QueryRow(ctx, `INSERT INTO webhooks (id, workspace_id, url, secret_encrypted, events, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, url, events, is_active, failure_count, disabled_reason, created_at, updated_at`,
			id, wsID, url, sealed, events, actorID))
		if err != nil {
			return err
		}
		// The secret is deliberately not part of the audit payload.
		return appendEvent(ctx, tx, wsID, actorID, "webhook", id, "webhook.created", map[string]any{"url": url, "events": events})
	})
	return w, secret, err
}

// ListWebhooks returns the workspace's webhooks.
func (s *Store) ListWebhooks(ctx context.Context, wsID uuid.UUID) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, webhookSelect+` WHERE workspace_id = $1 ORDER BY created_at, id`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Webhook, error) { return scanWebhook(r) })
}

// GetWebhook returns one webhook of the workspace.
func (s *Store) GetWebhook(ctx context.Context, wsID, id uuid.UUID) (Webhook, error) {
	return scanWebhook(s.pool.QueryRow(ctx, webhookSelect+` WHERE workspace_id = $1 AND id = $2`, wsID, id))
}

// UpdateWebhookParams is a partial update.
type UpdateWebhookParams struct {
	WorkspaceID, ActorID, ID uuid.UUID
	URL                      opt.Field[string]
	Events                   opt.Field[[]string]
	Active                   opt.Field[bool]
	Check                    URLValidator
}

// UpdateWebhook changes URL, events or active state. Re-activating clears the failure counter.
func (s *Store) UpdateWebhook(ctx context.Context, p UpdateWebhookParams) (Webhook, error) {
	if p.URL.Null || p.Events.Null || p.Active.Null {
		return Webhook{}, invalid("body", "fields cannot be null")
	}
	var w Webhook
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, p.WorkspaceID, p.ID))
		if err != nil {
			return err
		}
		next := cur
		if p.URL.Set {
			next.URL = strings.TrimSpace(p.URL.Value)
			if err := p.Check(next.URL); err != nil {
				return invalid("url", "%s", err.Error())
			}
		}
		if p.Events.Set {
			if next.Events, err = normalizeEvents(p.Events.Value); err != nil {
				return err
			}
		}
		if p.Active.Set {
			next.Active = p.Active.Value
		}
		reactivate := next.Active && !cur.Active
		w, err = scanWebhook(tx.QueryRow(ctx, `UPDATE webhooks SET url = $1, events = $2, is_active = $3,
				failure_count = CASE WHEN $4 THEN 0 ELSE failure_count END,
				disabled_reason = CASE WHEN $4 THEN NULL ELSE disabled_reason END
			WHERE id = $5
			RETURNING id, url, events, is_active, failure_count, disabled_reason, created_at, updated_at`,
			next.URL, next.Events, next.Active, reactivate, cur.ID))
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "webhook", cur.ID, "webhook.updated",
			map[string]any{"url": w.URL, "events": w.Events, "active": w.Active})
	})
	return w, err
}

// RotateWebhookSecret replaces the signing secret and returns the new one (shown once).
func (s *Store) RotateWebhookSecret(ctx context.Context, wsID, actorID, id uuid.UUID) (string, error) {
	secret := newWebhookSecret()
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return "", err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE webhooks SET secret_encrypted = $1 WHERE workspace_id = $2 AND id = $3`, sealed, wsID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return appendEvent(ctx, tx, wsID, actorID, "webhook", id, "webhook.secret_rotated", nil)
	})
	return secret, err
}

// DeleteWebhook removes a webhook and its delivery log.
func (s *Store) DeleteWebhook(ctx context.Context, wsID, actorID, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM webhooks WHERE workspace_id = $1 AND id = $2`, wsID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return appendEvent(ctx, tx, wsID, actorID, "webhook", id, "webhook.deleted", nil)
	})
}

// ListDeliveries returns the latest delivery attempts of a webhook.
func (s *Store) ListDeliveries(ctx context.Context, wsID, id uuid.UUID, limit int) ([]WebhookDelivery, error) {
	if _, err := s.GetWebhook(ctx, wsID, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, event_id, attempt, status_code, error, duration_ms, delivered_at
		FROM webhook_deliveries WHERE webhook_id = $1 ORDER BY id DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[WebhookDelivery])
}

// DeliveryTarget is what the worker needs to send one delivery.
type DeliveryTarget struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	URL         string
	Secret      string
	Active      bool
	Events      []string
}

// WebhookTarget loads and decrypts a webhook for delivery.
func (s *Store) WebhookTarget(ctx context.Context, id uuid.UUID) (DeliveryTarget, error) {
	var t DeliveryTarget
	var sealed []byte
	err := s.pool.QueryRow(ctx, `SELECT id, workspace_id, url, secret_encrypted, is_active, events FROM webhooks WHERE id = $1`, id).
		Scan(&t.ID, &t.WorkspaceID, &t.URL, &sealed, &t.Active, &t.Events)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if s.box == nil {
		return t, errors.New("webhooks require NEBULA_ENCRYPTION_KEY")
	}
	secret, err := s.box.Open(sealed, id[:])
	if err != nil {
		return t, err
	}
	t.Secret = string(secret)
	return t, nil
}

// RecordDelivery logs an attempt and updates the webhook's health. After
// WebhookDisableAfter consecutive failures the webhook is disabled.
func (s *Store) RecordDelivery(ctx context.Context, d WebhookDelivery, webhookID uuid.UUID, jobID int64, success bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO webhook_deliveries (webhook_id, event_id, job_id, attempt, status_code, error, duration_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, webhookID, d.EventID, jobID, d.Attempt, d.StatusCode, d.Error, d.DurationMS); err != nil {
			return err
		}
		if success {
			_, err := tx.Exec(ctx, `UPDATE webhooks SET failure_count = 0 WHERE id = $1 AND failure_count <> 0`, webhookID)
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE webhooks SET failure_count = failure_count + 1,
				is_active = CASE WHEN failure_count + 1 >= $2 THEN false ELSE is_active END,
				disabled_reason = CASE WHEN failure_count + 1 >= $2 THEN 'disabled after repeated delivery failures' ELSE disabled_reason END
			WHERE id = $1`, webhookID, WebhookDisableAfter)
		return err
	})
}
