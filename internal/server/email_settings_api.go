package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/mailroute"
	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Test sends per workspace: 5 per 10 minutes (per replica), so the button
// cannot be used to flood a mailbox or burn the server's sending reputation.
const (
	emailTestBurst   = 5
	emailTestWindow  = 10 * time.Minute
	emailTestTimeout = 20 * time.Second // below the request timeout
)

func newEmailTestLimiter() *ratelimit.Limiter {
	return ratelimit.New(float64(emailTestBurst)/emailTestWindow.Seconds(), emailTestBurst)
}

// emailSettingsRoutes: per-workspace SMTP settings (ADR-0038). Admin-only,
// reads included: host and username describe the team's mail account.
func (a *api) emailSettingsRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(requireRole(store.RoleAdmin))
		r.Get("/email-settings", a.getEmailSettings)
		r.Put("/email-settings", a.putEmailSettings)
		r.Delete("/email-settings", a.deleteEmailSettings)
		r.Post("/email-settings/test", a.testEmailSettings)
	})
}

type emailPolicyView struct {
	AllowPlaintext bool  `json:"allow_plaintext"`
	AllowPrivate   bool  `json:"allow_private"`
	Ports          []int `json:"ports"`
}

// emailSettingsView never contains the password: only password_set.
type emailSettingsView struct {
	// Settings is null when the workspace uses the instance configuration.
	Settings *store.EmailSettings `json:"settings"`
	// InstanceConfigured: the server has NEBULA_SMTP_* (the fallback).
	InstanceConfigured bool            `json:"instance_configured"`
	Policy             emailPolicyView `json:"policy"`
}

func (a *api) emailView(s *store.EmailSettings) emailSettingsView {
	p := a.mail.Policy
	ports := p.Ports
	if len(ports) == 0 {
		ports = store.DefaultSMTPPorts
	}
	return emailSettingsView{Settings: s, InstanceConfigured: a.mail.Instance != nil,
		Policy: emailPolicyView{AllowPlaintext: p.AllowPlaintext, AllowPrivate: p.AllowPrivate, Ports: ports}}
}

func (a *api) getEmailSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.GetEmailSettings(r.Context(), workspaceFrom(r).ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, a.emailView(nil))
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.emailView(&s))
}

func (a *api) putEmailSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host        string            `json:"host"`
		Port        int               `json:"port"`
		TLSMode     string            `json:"tls_mode"`
		Username    string            `json:"username"`
		Password    opt.Field[string] `json:"password"` // absent: keep, null: clear
		FromAddress string            `json:"from_address"`
		ReplyTo     *string           `json:"reply_to"`
		Enabled     *bool             `json:"enabled"` // absent: true
	}
	if !a.decode(w, r, &body) {
		return
	}
	enabled := body.Enabled == nil || *body.Enabled
	s, err := a.store.SaveEmailSettings(r.Context(), store.SaveEmailSettingsParams{
		WorkspaceID: workspaceFrom(r).ID, ActorID: principal(r).UserID,
		Host: body.Host, Port: body.Port, TLSMode: body.TLSMode, Username: body.Username, Password: body.Password,
		FromAddress: body.FromAddress, ReplyTo: body.ReplyTo, Enabled: enabled, Policy: a.mail.Policy,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.emailView(&s))
}

func (a *api) deleteEmailSettings(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteEmailSettings(r.Context(), workspaceFrom(r).ID, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testEmailSettings sends one test message through the settings the
// workspace's mail would use (stored settings even while disabled, else the
// instance configuration). Recipients are limited to the requester's own
// verified address or another member's, so this is never an open relay.
func (a *api) testEmailSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To *string `json:"to"`
	}
	if r.ContentLength != 0 && !a.decode(w, r, &body) {
		return
	}
	ws, me := workspaceFrom(r), principal(r)
	ctx := r.Context()

	var to string
	if body.To == nil || strings.TrimSpace(*body.To) == "" || strings.EqualFold(strings.TrimSpace(*body.To), me.Email) {
		if !me.EmailVerified {
			writeError(w, http.StatusUnprocessableEntity, "email_unverified", "your email address is not verified; verify it or choose another member")
			return
		}
		to = me.Email
	} else {
		addr, err := mail.ParseMailbox(strings.TrimSpace(*body.To))
		if err != nil {
			a.fail(w, r, &store.ValidationError{Field: "to", Message: "must be an email address"})
			return
		}
		member, err := a.store.MemberEmail(ctx, ws.ID, addr.Address)
		if errors.Is(err, store.ErrNotFound) {
			a.fail(w, r, &store.ValidationError{Field: "to", Message: "must be the email of a member of this workspace"})
			return
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		to = member
	}

	route, err := a.mail.ForTest(ctx, ws.ID)
	switch {
	case errors.Is(err, mailroute.ErrNotConfigured):
		writeError(w, http.StatusConflict, "email_not_configured", "no SMTP server is configured for this workspace or the server")
		return
	case errors.Is(err, mailroute.ErrPolicy):
		a.fail(w, r, &store.ValidationError{Field: "host", Message: mail.Describe(err)})
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}

	if ok, wait := a.emailTestLimiter.Allow(ws.ID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many test emails; try again later")
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, emailTestTimeout)
	defer cancel()
	sendErr := route.Sender.Send(sendCtx, mail.Message{
		To:      []string{to},
		Subject: "Nebula: test email / sınaq məktubu",
		Body: fmt.Sprintf("This is a test email from the Nebula workspace %q, sent by %s.\n"+
			"Bu, Nebula-nın %q workspace-indən %s tərəfindən göndərilmiş sınaq məktubudur.\n",
			ws.Name, me.DisplayName, ws.Name, me.DisplayName),
	})
	// Audit with a background context: the attempt happened even if the client left.
	host := "" // the instance server is the operator's business, not the workspace's
	if route.Source == mailroute.SourceWorkspace {
		host = route.Host
	}
	if err := a.store.RecordEmailTest(context.WithoutCancel(ctx), ws.ID, me.UserID, route.Source, host, to, sendErr == nil); err != nil {
		a.fail(w, r, err)
		return
	}
	resp := map[string]any{"ok": sendErr == nil, "source": route.Source, "to": to}
	if sendErr != nil {
		var secrets []string
		if t, err := a.store.EmailTransport(ctx, ws.ID); err == nil && route.Source == mailroute.SourceWorkspace {
			secrets = []string{t.Password, t.Username}
		}
		resp["error"] = mail.Describe(sendErr, secrets...)
		a.logger.InfoContext(ctx, "test email failed", "workspace_id", ws.ID, "source", route.Source, "reason", resp["error"])
	}
	writeJSON(w, http.StatusOK, resp)
}
