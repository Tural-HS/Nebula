// Package engine implements the background job handlers: event fan-out,
// webhook delivery (signed Nebula JSON or Slack/Mattermost chat messages),
// automation rules and email.
package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/blobstore"
	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/mailroute"
	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/worker"
)

// Engine holds the dependencies of all job handlers.
type Engine struct {
	Store    *store.Store
	HTTP     *http.Client
	HTTPOpts safehttp.Options
	// Mail is the instance sender (NEBULA_SMTP_*); nil: only workspaces with
	// their own SMTP settings send email (ADR-0038), other email is skipped.
	Mail mail.Sender
	// MailPolicy limits workspace SMTP servers (SSRF, ports, plaintext).
	MailPolicy store.EmailPolicy
	// MailTimeout bounds one session with a workspace SMTP server (0: 30s).
	MailTimeout time.Duration
	Logger      *slog.Logger
	Version     string
	// PublicURL is the web UI base URL for links in chat messages ("" = no links).
	PublicURL string
	// Blobs is the attachment store whose orphaned blobs Tick removes (nil: none).
	Blobs blobstore.Store
	// BlobPurgeDelay keeps a deleted attachment's blob this long (backups, ADR-0022).
	BlobPurgeDelay time.Duration
}

// Handlers maps job kinds to handlers.
func (e *Engine) Handlers() map[string]worker.Handler {
	return map[string]worker.Handler{
		store.JobDispatchEvent:       e.dispatch,
		store.JobDeliverWebhook:      e.deliverWebhook,
		store.JobRunAutomation:       e.runAutomation,
		store.JobSendEmail:           e.sendEmail,
		store.JobCreateNotifications: e.createNotifications,
	}
}

func decode[T any](j store.Job) (T, error) {
	var v T
	if err := json.Unmarshal(j.Payload, &v); err != nil {
		return v, worker.Permanent(fmt.Errorf("decode %s payload: %w", j.Kind, err))
	}
	return v, nil
}

func (e *Engine) dispatch(ctx context.Context, j store.Job) error {
	p, err := decode[struct {
		EventID int64 `json:"event_id"`
	}](j)
	if err != nil {
		return err
	}
	ev, err := e.Store.GetEvent(ctx, p.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // workspace deleted: nothing to deliver
	}
	if err != nil {
		return err
	}
	_, _, err = e.Store.FanOut(ctx, ev)
	return err
}

// createNotifications writes the in-app notifications of one event (ADR-0017).
// The store dedupes per (event, recipient), so retries are safe.
func (e *Engine) createNotifications(ctx context.Context, j store.Job) error {
	p, err := decode[struct {
		EventID int64 `json:"event_id"`
	}](j)
	if err != nil {
		return err
	}
	ev, err := e.Store.GetEvent(ctx, p.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = e.Store.CreateNotifications(ctx, ev)
	return err
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

// Sign returns the Nebula-Signature header value: t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>.
// Receivers must recompute it, compare in constant time and reject old timestamps.
func Sign(secret string, ts time.Time, body []byte) string {
	t := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(t))
	mac.Write([]byte("."))
	mac.Write(body)
	return "t=" + t + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// WebhookPayload is the JSON body sent to receivers.
type WebhookPayload struct {
	EventID    int64          `json:"event_id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Workspace  map[string]any `json:"workspace"`
	ActorID    *uuid.UUID     `json:"actor_id"`
	Source     string         `json:"source"`
	Entity     map[string]any `json:"entity"`
	Data       map[string]any `json:"data"`
	// Item is the item's current state at delivery time (item events only).
	Item *store.Item `json:"item,omitempty"`
}

func (e *Engine) deliverWebhook(ctx context.Context, j store.Job) error {
	p, err := decode[struct {
		WebhookID uuid.UUID `json:"webhook_id"`
		EventID   int64     `json:"event_id"`
		Force     bool      `json:"force"` // test ping: ignore active flag and event filter
	}](j)
	if err != nil {
		return err
	}
	target, err := e.Store.WebhookTarget(ctx, p.WebhookID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // webhook deleted
	}
	if err != nil {
		return err
	}
	ev, err := e.Store.GetEvent(ctx, p.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if target.WorkspaceID != ev.WorkspaceID {
		return worker.Permanent(errors.New("webhook and event belong to different workspaces"))
	}
	if !p.Force && (!target.Active || !store.MatchEvent(target.Events, ev.Action)) {
		return nil // disabled or unsubscribed since the event happened
	}
	if !p.Force {
		// Project filter, checked again at delivery: the project may have
		// become private, or the filter changed, since the event (ADR-0013).
		scope, err := e.Store.EventScopeOf(ctx, ev)
		if err != nil {
			return err
		}
		if !scope.Allows(target.ProjectIDs) {
			return nil
		}
		if ev.Action == store.ActionOverdueDigest {
			keys, err := e.Store.WebhookProjectKeys(ctx, ev.WorkspaceID, target.ProjectIDs)
			if err != nil {
				return err
			}
			filtered, ok := FilterDigest(ev.Payload, keys)
			if !ok {
				return nil // nothing overdue in this webhook's projects
			}
			ev.Payload = filtered
		}
	}

	body, err := e.webhookBody(ctx, target, ev)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return worker.Permanent(errors.New("invalid webhook URL")) // never echo it: it may be a credential
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nebula-Webhooks/"+e.Version)
	if target.Format != store.WebhookFormatSlack {
		// Chat servers authenticate by the URL and ignore these; only Nebula
		// receivers verify the signature and dedupe on the event ID.
		req.Header.Set("Nebula-Event-Id", strconv.FormatInt(ev.ID, 10))
		req.Header.Set("Nebula-Event-Type", ev.Action)
		req.Header.Set("Nebula-Delivery-Attempt", strconv.Itoa(j.Attempts))
		req.Header.Set("Nebula-Signature", Sign(target.Secret, time.Now(), body))
	}

	start := time.Now()
	resp, err := safehttp.Do(ctx, e.HTTP, e.HTTPOpts, req)
	d := store.WebhookDelivery{EventID: ev.ID, Attempt: j.Attempts, DurationMS: int(time.Since(start).Milliseconds())}
	var result error
	switch {
	case err != nil:
		err = redactURL(err)
		msg := truncate(err.Error(), 500)
		d.Error = &msg
		result = fmt.Errorf("deliver: %w", err)
		if errors.Is(err, safehttp.ErrBlockedAddress) {
			result = worker.Permanent(result) // policy violation: retrying won't help
		}
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		code := resp.StatusCode
		d.StatusCode = &code
		switch {
		case code >= 200 && code < 300:
		case code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500:
			result = fmt.Errorf("receiver returned %d", code)
		default:
			result = worker.Permanent(fmt.Errorf("receiver returned %d", code))
		}
	}
	if rerr := e.Store.RecordDelivery(context.WithoutCancel(ctx), d, target.ID, j.ID, result == nil); rerr != nil {
		e.Logger.Error("record webhook delivery", "error", rerr)
	}
	return result
}

// redactURL drops the request URL from net/http errors ("Post \"https://…\": …"):
// they end up in the delivery log, job errors and logs, and a chat webhook
// URL is a credential. Only the operation and the cause remain.
func redactURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &redactedError{msg: ue.Op + ": " + ue.Err.Error(), err: ue.Err}
	}
	return err
}

type redactedError struct {
	msg string
	err error
}

func (r *redactedError) Error() string { return r.msg }
func (r *redactedError) Unwrap() error { return r.err }

// webhookBody renders the request body for the webhook's format.
func (e *Engine) webhookBody(ctx context.Context, target store.DeliveryTarget, ev store.Event) ([]byte, error) {
	if target.Format == store.WebhookFormatSlack {
		c, err := e.chatContext(ctx, ev)
		if err != nil {
			return nil, err
		}
		msg := RenderChatMessage(ev, c)
		if e.PublicURL != "" {
			msg.IconURL = e.PublicURL + "/logo-mark.png"
		}
		body, err := json.Marshal(msg)
		if err != nil {
			return nil, worker.Permanent(err)
		}
		return body, nil
	}

	payload := WebhookPayload{
		EventID: ev.ID, Type: ev.Action, OccurredAt: ev.OccurredAt, ActorID: ev.ActorID, Source: ev.Source,
		Workspace: map[string]any{"id": ev.WorkspaceID, "slug": ev.WorkspaceSlug},
		Entity:    map[string]any{"type": ev.EntityType, "id": ev.EntityID},
		Data:      ev.Payload,
	}
	if ev.EntityType == "item" {
		if it, err := e.Store.ItemByID(ctx, ev.WorkspaceID, ev.EntityID); err == nil {
			payload.Item = &it
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, worker.Permanent(err)
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Automation
// ---------------------------------------------------------------------------

// Snapshot is the view of an item that conditions are evaluated against.
type Snapshot struct {
	Values  map[string]any  // field name -> value (see itemValues)
	Changed map[string]bool // for item.updated: names of changed fields
}

func projectKey(itemKey string) string {
	if i := strings.LastIndexByte(itemKey, '-'); i > 0 {
		return itemKey[:i]
	}
	return itemKey
}

// Evaluate reports whether all conditions hold for s.
func Evaluate(conds []store.Condition, s Snapshot) bool {
	for _, c := range conds {
		if !evalOne(c, s) {
			return false
		}
	}
	return true
}

func evalOne(c store.Condition, s Snapshot) bool {
	var want any
	if len(c.Value) > 0 {
		if json.Unmarshal(c.Value, &want) != nil {
			return false
		}
	}
	if c.Field == "changed" {
		name, _ := want.(string)
		return s.Changed[name]
	}
	got := s.Values[c.Field]
	switch c.Op {
	case "eq":
		return equal(got, want)
	case "neq":
		return !equal(got, want)
	case "in", "not_in":
		list, _ := want.([]any)
		found := false
		for _, w := range list {
			if equal(got, w) {
				found = true
				break
			}
		}
		return found == (c.Op == "in")
	case "contains":
		switch g := got.(type) {
		case []any:
			for _, x := range g {
				if equal(x, want) {
					return true
				}
			}
			return false
		case string:
			w, ok := want.(string)
			return ok && strings.Contains(strings.ToLower(g), strings.ToLower(w))
		}
		return false
	case "is_empty":
		return isEmpty(got)
	case "is_not_empty":
		return !isEmpty(got)
	}
	return false
}

func equal(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// Render fills {{placeholders}} in an email template. Only a fixed set of
// values is available; there is no template language to inject into.
func Render(tmpl string, it store.Item, ruleName, workspace string) string {
	return strings.NewReplacer(
		"{{item.key}}", it.Key,
		"{{item.title}}", it.Title,
		"{{item.type}}", it.Type.Name,
		"{{item.status}}", it.Status.Name,
		"{{item.project}}", projectKey(it.Key),
		"{{rule.name}}", ruleName,
		"{{workspace}}", workspace,
	).Replace(tmpl)
}

func (e *Engine) runAutomation(ctx context.Context, j store.Job) error {
	p, err := decode[struct {
		RuleID  uuid.UUID `json:"rule_id"`
		EventID int64     `json:"event_id"`
	}](j)
	if err != nil {
		return err
	}
	rule, wsID, err := e.Store.RuleByID(ctx, p.RuleID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ev, err := e.Store.GetEvent(ctx, p.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !rule.Enabled || ev.WorkspaceID != wsID || ev.Action != rule.Trigger || ev.EntityType != "item" {
		return nil
	}
	item, err := e.Store.ItemByID(ctx, wsID, ev.EntityID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // deleted since
	}
	if err != nil {
		return err
	}

	// Conditions see the item as it was at the event, not as it is now:
	// another rule may already have changed it.
	values, ok := ev.Payload["snapshot"].(map[string]any)
	if !ok {
		values = store.Snapshot(item) // events written before snapshots existed
	}
	snap := Snapshot{Values: values, Changed: map[string]bool{}}
	if changes, ok := ev.Payload["changes"].(map[string]any); ok {
		for k := range changes {
			snap.Changed[k] = true
		}
	}
	if !Evaluate(rule.Conditions, snap) {
		return nil
	}
	log := e.Logger.With("rule_id", rule.ID, "event_id", ev.ID, "item", item.Key)

	actx := store.WithAutomation(ctx, rule.ID)
	if item, err = e.applyMutations(actx, wsID, item, rule.Actions); err != nil {
		var ve *store.ValidationError
		if errors.As(err, &ve) {
			log.Warn("automation action rejected", "field", ve.Field, "reason", ve.Message)
			return worker.Permanent(err)
		}
		// Approvals (ADR-0026): a rule can neither enter a guarded status
		// nor edit an item locked by a pending request, nor change approved
		// content that needs re-approval (ADR-0034). Retrying won't help.
		var ce *store.ConflictError
		if errors.As(err, &ce) && (ce.Code == "approval_required" || ce.Code == "approval_pending" || ce.Code == "reapproval_required") {
			log.Warn("automation action rejected", "code", ce.Code)
			return worker.Permanent(err)
		}
		return err
	}

	for i, a := range rule.Actions {
		if a.Type != store.ActionSendEmail {
			continue
		}
		to, err := e.recipients(ctx, wsID, item, a.To, ev.ActorID)
		if err != nil {
			return err
		}
		if len(to) == 0 {
			continue
		}
		subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(Render(a.Subject, item, rule.Name, ev.WorkspaceSlug))
		body := Render(a.Body, item, rule.Name, ev.WorkspaceSlug)
		if err := e.Store.EnqueueEmail(ctx, wsID, to, truncate(subject, 300), body,
			fmt.Sprintf("em:%s:%d:%d", rule.ID, ev.ID, i)); err != nil {
			return err
		}
	}
	log.Info("automation rule applied")
	return nil
}

// applyMutations performs all set_* actions as one update, retrying on
// version conflicts with concurrent human edits.
func (e *Engine) applyMutations(ctx context.Context, wsID uuid.UUID, item store.Item, actions []store.Action) (store.Item, error) {
	p := store.UpdateItemParams{WorkspaceID: wsID}
	fields := map[string]json.RawMessage{}
	mutate := false
	for _, a := range actions {
		switch a.Type {
		case store.ActionSetStatus:
			p.StatusID = opt.Of(*a.StatusID)
			mutate = true
		case store.ActionSetAssignee:
			if a.UserID == nil {
				p.AssigneeID = opt.Null[uuid.UUID]()
			} else {
				p.AssigneeID = opt.Of(*a.UserID)
			}
			mutate = true
		case store.ActionSetField:
			fields[a.Key] = a.Value
			mutate = true
		}
	}
	if !mutate {
		return item, nil
	}
	if len(fields) > 0 {
		p.Fields = opt.Of(fields)
	}
	for attempt := 0; ; attempt++ {
		p.ProjectID, p.Number, p.Version = item.ProjectID, item.Number, item.Version
		updated, err := e.Store.UpdateItem(ctx, p)
		var ce *store.ConflictError
		if errors.As(err, &ce) && ce.Code == "version_conflict" && attempt < 3 {
			if item, err = e.Store.ItemByID(ctx, wsID, item.ID); err != nil {
				return item, err
			}
			continue
		}
		return updated, err
	}
}

func (e *Engine) recipients(ctx context.Context, wsID uuid.UUID, it store.Item, to []string, actor *uuid.UUID) ([]string, error) {
	var ids []uuid.UUID
	var emails []string
	for _, r := range to {
		switch r {
		case "assignee":
			if it.AssigneeID != nil {
				ids = append(ids, *it.AssigneeID)
			}
		case "reporter":
			if it.ReporterID != nil {
				ids = append(ids, *it.ReporterID)
			}
		default:
			// Membership is re-checked at send time: the person may have left.
			ok, err := e.Store.IsMemberEmail(ctx, wsID, r)
			if err != nil {
				return nil, err
			}
			if ok {
				emails = append(emails, r)
			}
		}
	}
	byID, err := e.Store.MemberEmails(ctx, wsID, append(ids, uuidOrNil(actor)...))
	if err != nil {
		return nil, err
	}
	actorEmail := ""
	if actor != nil {
		actorEmail = byID[*actor]
	}
	seen := map[string]bool{}
	var out []string
	add := func(email string) {
		// People don't need an email about their own change.
		if email == "" || seen[strings.ToLower(email)] || strings.EqualFold(email, actorEmail) {
			return
		}
		seen[strings.ToLower(email)] = true
		out = append(out, email)
	}
	for _, id := range ids {
		add(byID[id])
	}
	for _, em := range emails {
		add(em)
	}
	return out, nil
}

func uuidOrNil(id *uuid.UUID) []uuid.UUID {
	if id == nil {
		return nil
	}
	return []uuid.UUID{*id}
}

// ---------------------------------------------------------------------------
// Email
// ---------------------------------------------------------------------------

func (e *Engine) sendEmail(ctx context.Context, j store.Job) error {
	m, err := decode[store.EmailJob](j)
	if err != nil {
		return err
	}
	var wsID uuid.UUID
	if m.WorkspaceID != nil {
		wsID = *m.WorkspaceID
	}
	route, err := e.mailRouter().For(ctx, wsID)
	switch {
	case errors.Is(err, mailroute.ErrNotConfigured):
		e.Logger.Debug("email disabled (no workspace or instance SMTP configuration); skipping", "job_id", j.ID)
		return nil
	case errors.Is(err, mailroute.ErrPolicy):
		return worker.Permanent(err)
	case err != nil:
		return err
	}
	if err := route.Sender.Send(ctx, mail.Message{To: m.To, Subject: m.Subject, Body: m.Body}); err != nil {
		permanent := errors.Is(err, mail.ErrInvalidMessage) || errors.Is(err, safehttp.ErrBlockedAddress)
		if route.Source == mailroute.SourceWorkspace {
			// The admin-controlled server's reply is reduced to a safe text
			// (credentials masked, never the dialed IP) before it reaches
			// jobs.last_error and the logs.
			err = fmt.Errorf("workspace SMTP: %s", route.Describe(err))
		}
		if permanent {
			return worker.Permanent(err)
		}
		return err
	}
	return nil
}

// mailRouter selects the workspace's SMTP settings or the instance sender (ADR-0038).
func (e *Engine) mailRouter() *mailroute.Router {
	return &mailroute.Router{Instance: e.Mail, Store: e.Store, Policy: e.MailPolicy, Timeout: e.MailTimeout}
}

// truncate cuts s to at most n bytes without splitting a UTF-8 character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
