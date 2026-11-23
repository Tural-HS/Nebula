package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
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

// Webhook payload formats (ADR-0010).
const (
	// WebhookFormatNebula is the HMAC-signed Nebula JSON event.
	WebhookFormatNebula = "nebula"
	// WebhookFormatSlack is a Slack/Mattermost incoming-webhook message. Its URL
	// is the credential, so it is stored encrypted and only shown masked.
	WebhookFormatSlack = "slack"
)

// Webhook is an outgoing HTTP subscription. The signing secret is never returned
// after creation (or rotation). For the slack format URL is masked.
type Webhook struct {
	ID     uuid.UUID `json:"id"`
	URL    string    `json:"url"`
	Format string    `json:"format"`
	Events []string  `json:"events"`
	// ProjectIDs limits delivery to these projects; empty = every
	// workspace-visible project (private ones only when listed).
	ProjectIDs     []uuid.UUID `json:"project_ids"`
	Active         bool        `json:"active"`
	FailureCount   int         `json:"failure_count"`
	DisabledReason *string     `json:"disabled_reason"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
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

// urlContext binds an encrypted URL to its webhook and keeps it from being
// swapped with the signing secret ciphertext of the same row.
func urlContext(id uuid.UUID) []byte { return append([]byte("webhook-url:"), id[:]...) }

// stored returns the url column value and the ciphertext column for a format:
// a slack URL is a bearer credential, so only a masked form stays readable.
func (s *Store) storedURL(id uuid.UUID, format, raw string) (string, []byte, error) {
	if format != WebhookFormatSlack {
		return raw, nil, nil
	}
	if s.box == nil {
		return "", nil, errors.New("webhooks require NEBULA_ENCRYPTION_KEY")
	}
	sealed, err := s.box.Seal([]byte(raw), urlContext(id))
	return MaskWebhookURL(raw), sealed, err
}

// MaskWebhookURL keeps scheme, host and the last 4 characters of a URL, enough
// to tell webhooks apart without revealing the token in its path.
func MaskWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "https://invalid/…"
	}
	rest := strings.TrimPrefix(raw, u.Scheme+"://"+u.Host)
	tail := ""
	if r := []rune(rest); len(r) >= 16 {
		tail = string(r[len(r)-4:])
	}
	return u.Scheme + "://" + u.Host + "/…" + tail
}

func normalizeFormat(f string) (string, error) {
	switch f = strings.TrimSpace(f); f {
	case "":
		return WebhookFormatNebula, nil
	case WebhookFormatNebula, WebhookFormatSlack:
		return f, nil
	}
	return "", invalid("format", "must be %q or %q", WebhookFormatNebula, WebhookFormatSlack)
}

const webhookColumns = `id, url, format, events, project_ids, is_active, failure_count, disabled_reason, created_at, updated_at`

const webhookSelect = `SELECT ` + webhookColumns + ` FROM webhooks`

func scanWebhook(row pgx.Row) (Webhook, error) {
	var w Webhook
	err := row.Scan(&w.ID, &w.URL, &w.Format, &w.Events, &w.ProjectIDs, &w.Active, &w.FailureCount, &w.DisabledReason, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrNotFound
	}
	return w, err
}

// CreateWebhook registers a webhook and returns its signing secret (shown
// once). A slack-format webhook is not signed, so its secret is "".
func (s *Store) CreateWebhook(ctx context.Context, wsID, actorID uuid.UUID, rawURL, format string, events []string, projectIDs []uuid.UUID, check URLValidator) (Webhook, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	format, err := normalizeFormat(format)
	if err != nil {
		return Webhook{}, "", err
	}
	if err := check(rawURL); err != nil {
		return Webhook{}, "", invalid("url", "%s", err.Error())
	}
	events, err = normalizeEvents(events)
	if err != nil {
		return Webhook{}, "", err
	}
	id := newID()
	// Every row keeps a sealed secret (the column is NOT NULL); for slack it is
	// simply never used or shown.
	secret := newWebhookSecret()
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return Webhook{}, "", err
	}
	shownURL, sealedURL, err := s.storedURL(id, format, rawURL)
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
		projects, err := normalizeProjectFilter(ctx, tx, wsID, projectIDs)
		if err != nil {
			return err
		}
		w, err = scanWebhook(tx.QueryRow(ctx, `INSERT INTO webhooks (id, workspace_id, url, url_encrypted, format, secret_encrypted, events, project_ids, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+webhookColumns,
			id, wsID, shownURL, sealedURL, format, sealed, events, projects, actorID))
		if err != nil {
			return err
		}
		// Neither the secret nor a slack URL is part of the audit payload.
		return appendEvent(ctx, tx, wsID, actorID, "webhook", id, "webhook.created",
			map[string]any{"url": shownURL, "format": format, "events": events, "project_ids": w.ProjectIDs})
	})
	if format == WebhookFormatSlack {
		secret = ""
	}
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
	ProjectIDs               opt.Field[[]uuid.UUID]
	Active                   opt.Field[bool]
	Check                    URLValidator
}

// UpdateWebhook changes URL, events or active state. Re-activating clears the failure counter.
func (s *Store) UpdateWebhook(ctx context.Context, p UpdateWebhookParams) (Webhook, error) {
	if p.URL.Null || p.Events.Null || p.Active.Null || p.ProjectIDs.Null {
		return Webhook{}, invalid("body", "fields cannot be null")
	}
	var w Webhook
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, p.WorkspaceID, p.ID))
		if err != nil {
			return err
		}
		next := cur
		var sealedURL []byte
		if p.URL.Set {
			raw := strings.TrimSpace(p.URL.Value)
			if err := p.Check(raw); err != nil {
				return invalid("url", "%s", err.Error())
			}
			if next.URL, sealedURL, err = s.storedURL(cur.ID, cur.Format, raw); err != nil {
				return err
			}
		}
		if p.Events.Set {
			if next.Events, err = normalizeEvents(p.Events.Value); err != nil {
				return err
			}
		}
		if p.ProjectIDs.Set {
			if next.ProjectIDs, err = normalizeProjectFilter(ctx, tx, p.WorkspaceID, p.ProjectIDs.Value); err != nil {
				return err
			}
		}
		if p.Active.Set {
			next.Active = p.Active.Value
		}
		reactivate := next.Active && !cur.Active
		w, err = scanWebhook(tx.QueryRow(ctx, `UPDATE webhooks SET url = $1, events = $2, is_active = $3,
				failure_count = CASE WHEN $4 THEN 0 ELSE failure_count END,
				disabled_reason = CASE WHEN $4 THEN NULL ELSE disabled_reason END,
				url_encrypted = COALESCE($6, url_encrypted), project_ids = $7
			WHERE id = $5
			RETURNING `+webhookColumns,
			next.URL, next.Events, next.Active, reactivate, cur.ID, sealedURL, next.ProjectIDs))
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "webhook", cur.ID, "webhook.updated",
			map[string]any{"url": w.URL, "events": w.Events, "active": w.Active, "project_ids": w.ProjectIDs})
	})
	return w, err
}

// RotateWebhookSecret replaces the signing secret and returns the new one (shown once).
// Slack-format webhooks are not signed; to rotate their credential, change the URL.
func (s *Store) RotateWebhookSecret(ctx context.Context, wsID, actorID, id uuid.UUID) (string, error) {
	secret := newWebhookSecret()
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return "", err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		var format string
		err := tx.QueryRow(ctx, `UPDATE webhooks SET secret_encrypted = $1 WHERE workspace_id = $2 AND id = $3 RETURNING format`,
			sealed, wsID, id).Scan(&format)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if format == WebhookFormatSlack {
			return &ConflictError{Code: "not_signed", Message: "slack-format webhooks are not signed; change the URL to rotate the credential"}
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
	URL         string // plain URL; never log it (for slack it is the credential)
	Format      string
	Secret      string
	Active      bool
	Events      []string
	ProjectIDs  []uuid.UUID
}

// WebhookTarget loads and decrypts a webhook for delivery.
func (s *Store) WebhookTarget(ctx context.Context, id uuid.UUID) (DeliveryTarget, error) {
	var t DeliveryTarget
	var sealed, sealedURL []byte
	err := s.pool.QueryRow(ctx, `SELECT id, workspace_id, url, url_encrypted, format, secret_encrypted, is_active, events, project_ids FROM webhooks WHERE id = $1`, id).
		Scan(&t.ID, &t.WorkspaceID, &t.URL, &sealedURL, &t.Format, &sealed, &t.Active, &t.Events, &t.ProjectIDs)
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
	if sealedURL != nil {
		raw, err := s.box.Open(sealedURL, urlContext(id))
		if err != nil {
			return t, err
		}
		t.URL = string(raw)
	}
	return t, nil
}

// PingWebhook sends a test delivery to one webhook, regardless of its event
// filter, and returns the event ID to look for in the delivery log.
func (s *Store) PingWebhook(ctx context.Context, wsID, actorID, id uuid.UUID) (int64, error) {
	var eventID int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM webhooks WHERE workspace_id = $1 AND id = $2)`, wsID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		var err error
		eventID, err = appendEventID(ctx, tx, wsID, actorID, "webhook", id, WebhookPingAction, map[string]any{"webhook_id": id})
		if err != nil {
			return err
		}
		return enqueueJob(ctx, tx, JobDeliverWebhook, map[string]any{"webhook_id": id, "event_id": eventID, "force": true}, 1,
			fmt.Sprintf("whp:%s:%d", id, eventID))
	})
	return eventID, err
}

// WebhookPingAction is delivered only to the pinged webhook, never fanned out.
const WebhookPingAction = "webhook.ping"

// ReencryptSecrets re-seals every webhook secret and encrypted webhook URL
// still encrypted with a retired key, so the old key can be removed. Safe to
// run repeatedly; updated counts webhooks with at least one re-sealed value.
func (s *Store) ReencryptSecrets(ctx context.Context) (updated int, err error) {
	if s.box == nil {
		return 0, errors.New("NEBULA_ENCRYPTION_KEY is not configured")
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, secret_encrypted, url_encrypted FROM webhooks ORDER BY id FOR UPDATE`)
		if err != nil {
			return err
		}
		type row struct {
			id          uuid.UUID
			sealed, url []byte
		}
		all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var x row
			return x, r.Scan(&x.id, &x.sealed, &x.url)
		})
		if err != nil {
			return err
		}
		// reseal returns new ciphertext when ct was sealed with a retired key, else nil.
		reseal := func(id uuid.UUID, what string, ct, aad []byte) ([]byte, error) {
			pt, old, err := s.box.OpenWithInfo(ct, aad)
			if err != nil {
				return nil, fmt.Errorf("webhook %s: %s cannot be decrypted with the configured keys", id, what)
			}
			if !old {
				return nil, nil
			}
			return s.box.Seal(pt, aad)
		}
		for _, x := range all {
			secret, err := reseal(x.id, "secret", x.sealed, x.id[:])
			if err != nil {
				return err
			}
			var sealedURL []byte
			if x.url != nil {
				if sealedURL, err = reseal(x.id, "URL", x.url, urlContext(x.id)); err != nil {
					return err
				}
			}
			if secret == nil && sealedURL == nil {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE webhooks SET secret_encrypted = COALESCE($1, secret_encrypted),
					url_encrypted = COALESCE($2, url_encrypted) WHERE id = $3`, secret, sealedURL, x.id); err != nil {
				return err
			}
			updated++
		}
		return nil
	})
	return updated, err
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
