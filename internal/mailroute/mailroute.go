// Package mailroute chooses the SMTP server for a piece of mail (ADR-0038):
// the workspace's own settings when it has enabled ones, otherwise the
// instance configuration (NEBULA_SMTP_*). Mail without a workspace context
// always uses the instance configuration.
//
// The choice is made per message from the database, so there is no mutable
// global state and a settings change applies to the next email.
package mailroute

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Where a message is sent from.
const (
	SourceWorkspace = "workspace"
	SourceInstance  = "instance"
)

// ErrNotConfigured means there is neither a usable workspace configuration
// nor an instance one: the message cannot be sent.
var ErrNotConfigured = errors.New("email is not configured")

// ErrPolicy wraps a stored configuration that the current instance policy no
// longer permits (e.g. a port removed from the allowed list). Retrying cannot
// fix it; an admin must change the settings.
var ErrPolicy = errors.New("email settings rejected by the instance policy")

// Transports reads workspace SMTP settings (implemented by *store.Store).
type Transports interface {
	EmailTransport(ctx context.Context, wsID uuid.UUID, includeDisabled bool) (store.EmailTransport, error)
}

// Router selects a sender. The zero value sends nothing.
type Router struct {
	// Instance is the operator-configured sender; nil when NEBULA_SMTP_HOST is unset.
	Instance mail.Sender
	// InstanceHost names the instance server in audit events ("" if unknown).
	InstanceHost string
	Store        Transports
	Policy       store.EmailPolicy
	// Timeout bounds one SMTP session of a workspace server (default 30s).
	Timeout time.Duration
}

// Route is the chosen sender and where it came from.
type Route struct {
	Sender mail.Sender
	Source string
	Host   string
	// secrets are the workspace server's credentials, masked out of error
	// texts (Describe). Empty for the instance server.
	secrets []string
}

// Describe reduces a send error to a text that is safe to store or show:
// network details are categorized and the credentials are masked. For the
// instance server only the category and the SMTP reply code are given — its
// replies are the operator's business, not the workspace admin's.
func (r Route) Describe(err error) string {
	if r.Source == SourceInstance {
		return mail.DescribeBrief(err)
	}
	return mail.Describe(err, r.secrets...)
}

// For returns the sender for mail of workspace wsID (uuid.Nil: no workspace
// context). Workspace settings win when present and enabled.
func (r *Router) For(ctx context.Context, wsID uuid.UUID) (Route, error) {
	return r.route(ctx, wsID, false)
}

// ForTest is For, except that stored settings are used even when disabled, so
// an admin can try them before switching them on.
func (r *Router) ForTest(ctx context.Context, wsID uuid.UUID) (Route, error) {
	return r.route(ctx, wsID, true)
}

func (r *Router) route(ctx context.Context, wsID uuid.UUID, includeDisabled bool) (Route, error) {
	if wsID != uuid.Nil && r.Store != nil {
		t, err := r.Store.EmailTransport(ctx, wsID, includeDisabled)
		switch {
		case err == nil && (t.Enabled || includeDisabled):
			s, err := r.Workspace(t)
			if err != nil {
				return Route{Source: SourceWorkspace, Host: t.Host}, err
			}
			return Route{Sender: s, Source: SourceWorkspace, Host: t.Host, secrets: []string{t.Password, t.Username}}, nil
		case err != nil && !errors.Is(err, store.ErrNotFound):
			// Never fall back silently on a broken row (e.g. undecryptable
			// password): the mail would leave from another address.
			return Route{Source: SourceWorkspace}, err
		}
	}
	if r.Instance == nil {
		return Route{Source: SourceInstance}, ErrNotConfigured
	}
	return Route{Sender: r.Instance, Source: SourceInstance, Host: r.InstanceHost}, nil
}

// Workspace builds a guarded sender for workspace settings: the policy is
// checked again (it may have been tightened since the save) and every
// resolved destination IP is checked while dialing.
func (r *Router) Workspace(t store.EmailTransport) (*mail.SMTPSender, error) {
	if err := r.Policy.CheckTransport(t); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPolicy, err)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	s, err := mail.NewSMTPSender(mail.Config{
		Host: t.Host, Port: t.Port, Username: t.Username, Password: t.Password,
		From: t.FromAddress, ReplyTo: t.ReplyTo, TLS: t.TLSMode, Timeout: timeout,
		Guard: &mail.Guard{AllowPrivate: r.Policy.AllowPrivate},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPolicy, err)
	}
	return s, nil
}
