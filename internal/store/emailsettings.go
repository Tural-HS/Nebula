package store

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/safehttp"
)

// Audit actions of the workspace SMTP settings (ADR-0038). Their payloads
// carry field names and the host, never the password.
const (
	EmailSettingsUpdatedAction = "email_settings.updated"
	EmailSettingsDeletedAction = "email_settings.deleted"
	EmailSettingsTestedAction  = "email_settings.tested"
	emailSettingsEntity        = "email_settings"
)

// SMTP TLS modes (same values as internal/mail).
const (
	SMTPTLSStartTLS = mail.TLSStartTLS
	SMTPTLSImplicit = mail.TLSImplicit
	SMTPTLSNone     = mail.TLSNone
)

// DefaultSMTPPorts are the submission/relay ports a workspace may use unless
// the operator configures others: SMTP relay, implicit TLS, submission and
// the common alternative.
var DefaultSMTPPorts = []int{25, 465, 587, 2525}

const maxSMTPPasswordLen = 1024

// EmailSettings is a workspace's SMTP configuration as the API returns it:
// the password is never included, only whether one is stored.
type EmailSettings struct {
	Host          string     `json:"host"`
	Port          int        `json:"port"`
	TLSMode       string     `json:"tls_mode"`
	Username      string     `json:"username"`
	PasswordSet   bool       `json:"password_set"`
	FromAddress   string     `json:"from_address"`
	ReplyTo       *string    `json:"reply_to"`
	Enabled       bool       `json:"enabled"`
	UpdatedBy     *uuid.UUID `json:"updated_by"`
	UpdatedByName *string    `json:"updated_by_name"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// EmailTransport is a workspace's SMTP configuration with the decrypted
// password, for sending only. Never serialize it.
type EmailTransport struct {
	Host        string
	Port        int
	TLSMode     string
	Username    string
	Password    string `json:"-"`
	FromAddress string
	ReplyTo     string
	Enabled     bool
}

// EmailPolicy is the operator's limit on what workspace admins may configure.
type EmailPolicy struct {
	// AllowPrivate permits loopback/private/reserved hosts (development).
	AllowPrivate bool
	// AllowPlaintext permits tls_mode "none" (development, e.g. Mailpit).
	AllowPlaintext bool
	// Ports allowed for the server; empty means DefaultSMTPPorts.
	Ports []int
}

// AllowsPort reports whether port is on the policy's list.
func (p EmailPolicy) AllowsPort(port int) bool {
	ports := p.Ports
	if len(ports) == 0 {
		ports = DefaultSMTPPorts
	}
	return slices.Contains(ports, port)
}

// CheckTransport re-applies the policy at send time, so tightening the
// operator flags also stops settings saved under looser ones.
func (p EmailPolicy) CheckTransport(t EmailTransport) error {
	if !p.AllowsPort(t.Port) {
		return fmt.Errorf("port %d is not allowed by the instance policy", t.Port)
	}
	if t.TLSMode == SMTPTLSNone && !p.AllowPlaintext {
		return errors.New("tls_mode none is not allowed by the instance policy")
	}
	if !p.AllowPrivate {
		return safehttp.CheckHost(t.Host)
	}
	return nil
}

// SaveEmailSettingsParams replaces a workspace's settings. Password: absent
// keeps the stored one, null clears it, a value replaces it.
type SaveEmailSettingsParams struct {
	WorkspaceID, ActorID uuid.UUID
	Host                 string
	Port                 int
	TLSMode              string
	Username             string
	Password             opt.Field[string]
	FromAddress          string
	ReplyTo              *string
	Enabled              bool
	Policy               EmailPolicy
}

var hostnameRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func normalizeSMTPHost(raw string) (string, error) {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if h == "" || len(h) > 253 {
		return "", invalid("host", "is required (at most 253 characters)")
	}
	if a, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		if a.Zone() != "" {
			return "", invalid("host", "must not contain an IPv6 zone")
		}
		return a.String(), nil
	}
	if !hostnameRe.MatchString(h) {
		return "", invalid("host", "must be a host name like mail.example.com (no scheme, port or path)")
	}
	return h, nil
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// validate normalizes p in place and checks it against the policy.
func (p *SaveEmailSettingsParams) validate() error {
	host, err := normalizeSMTPHost(p.Host)
	if err != nil {
		return err
	}
	p.Host = host
	if !p.Policy.AllowPrivate && safehttp.CheckHost(host) != nil {
		return invalid("host", "points to a private, loopback or reserved address; use a public mail server")
	}
	if p.Port < 1 || p.Port > 65535 {
		return invalid("port", "must be between 1 and 65535")
	}
	if !p.Policy.AllowsPort(p.Port) {
		ports := p.Policy.Ports
		if len(ports) == 0 {
			ports = DefaultSMTPPorts
		}
		return invalid("port", "must be one of %v", ports)
	}
	switch p.TLSMode {
	case SMTPTLSStartTLS, SMTPTLSImplicit:
	case SMTPTLSNone:
		if !p.Policy.AllowPlaintext {
			return invalid("tls_mode", "none is not allowed on this server; use starttls or tls")
		}
	default:
		return invalid("tls_mode", "must be starttls, tls or none")
	}
	p.Username = strings.TrimSpace(p.Username)
	if len(p.Username) > 320 || hasControl(p.Username) {
		return invalid("username", "must be at most 320 characters without control characters")
	}
	if p.Password.Set && !p.Password.Null {
		pw := p.Password.Value
		if pw == "" || len(pw) > maxSMTPPasswordLen || strings.ContainsAny(pw, "\x00\r\n") {
			return invalid("password", "must be 1 to %d characters without line breaks (null clears it)", maxSMTPPasswordLen)
		}
	}
	if p.TLSMode == SMTPTLSNone && (p.Username != "" || (p.Password.Set && !p.Password.Null)) {
		return invalid("tls_mode", "credentials are never sent without TLS; use starttls or tls")
	}
	from, err := mail.ParseMailbox(strings.TrimSpace(p.FromAddress))
	if err != nil {
		return invalid("from_address", "must be an email address like \"Team <noreply@example.com>\"")
	}
	p.FromAddress = from.String()
	if p.ReplyTo != nil {
		rt := strings.TrimSpace(*p.ReplyTo)
		if rt == "" {
			p.ReplyTo = nil
		} else {
			a, err := mail.ParseMailbox(rt)
			if err != nil {
				return invalid("reply_to", "must be an email address")
			}
			s := a.String()
			p.ReplyTo = &s
		}
	}
	return nil
}

// smtpPasswordContext binds a sealed password to its workspace and purpose.
func smtpPasswordContext(wsID uuid.UUID) []byte {
	return append([]byte("smtp-password:"), wsID[:]...)
}

const emailSettingsSelect = `SELECT s.host, s.port, s.tls_mode, s.username, s.password_encrypted IS NOT NULL,
	s.from_address, s.reply_to, s.is_enabled, s.updated_by, u.display_name, s.updated_at
	FROM workspace_email_settings s LEFT JOIN users u ON u.id = s.updated_by`

func scanEmailSettings(row pgx.Row) (EmailSettings, error) {
	var e EmailSettings
	err := row.Scan(&e.Host, &e.Port, &e.TLSMode, &e.Username, &e.PasswordSet, &e.FromAddress, &e.ReplyTo,
		&e.Enabled, &e.UpdatedBy, &e.UpdatedByName, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// GetEmailSettings returns the workspace's SMTP settings, or ErrNotFound when
// the workspace uses the instance configuration.
func (s *Store) GetEmailSettings(ctx context.Context, wsID uuid.UUID) (EmailSettings, error) {
	return scanEmailSettings(s.pool.QueryRow(ctx, emailSettingsSelect+` WHERE s.workspace_id = $1`, wsID))
}

// SaveEmailSettings creates or replaces the workspace's SMTP settings and
// records which fields changed (never their secret values).
func (s *Store) SaveEmailSettings(ctx context.Context, p SaveEmailSettingsParams) (EmailSettings, error) {
	if err := p.validate(); err != nil {
		return EmailSettings{}, err
	}
	if s.box == nil {
		return EmailSettings{}, errors.New("email settings require NEBULA_ENCRYPTION_KEY")
	}
	var sealed []byte
	if p.Password.Set && !p.Password.Null {
		var err error
		if sealed, err = s.box.Seal([]byte(p.Password.Value), smtpPasswordContext(p.WorkspaceID)); err != nil {
			return EmailSettings{}, err
		}
	}
	var out EmailSettings
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanEmailSettings(tx.QueryRow(ctx, emailSettingsSelect+` WHERE s.workspace_id = $1 FOR UPDATE OF s`, p.WorkspaceID))
		exists := err == nil
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if p.TLSMode == SMTPTLSNone && !p.Password.Set && cur.PasswordSet {
			return invalid("tls_mode", "credentials are never sent without TLS; clear the password or use starttls or tls")
		}
		var changed []string
		mark := func(name string, differs bool) {
			if !exists || differs {
				changed = append(changed, name)
			}
		}
		mark("host", cur.Host != p.Host)
		mark("port", cur.Port != p.Port)
		mark("tls_mode", cur.TLSMode != p.TLSMode)
		mark("username", cur.Username != p.Username)
		mark("from_address", cur.FromAddress != p.FromAddress)
		mark("reply_to", !equalPtr(cur.ReplyTo, p.ReplyTo))
		mark("enabled", cur.Enabled != p.Enabled)
		// The password is reported as changed when it was replaced or
		// cleared, never with its value. Re-sending the same password counts
		// as a change: the stored ciphertext is not compared.
		switch {
		case p.Password.Set && !p.Password.Null:
			changed = append(changed, "password")
		case p.Password.Set && p.Password.Null && cur.PasswordSet:
			changed = append(changed, "password")
		}
		keep := !p.Password.Set // absent: keep the stored ciphertext
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_email_settings
				(workspace_id, host, port, tls_mode, username, password_encrypted, from_address, reply_to, is_enabled, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (workspace_id) DO UPDATE SET host = EXCLUDED.host, port = EXCLUDED.port,
				tls_mode = EXCLUDED.tls_mode, username = EXCLUDED.username,
				password_encrypted = CASE WHEN $11 THEN workspace_email_settings.password_encrypted ELSE EXCLUDED.password_encrypted END,
				from_address = EXCLUDED.from_address, reply_to = EXCLUDED.reply_to, is_enabled = EXCLUDED.is_enabled,
				updated_by = EXCLUDED.updated_by, updated_at = now()`,
			p.WorkspaceID, p.Host, p.Port, p.TLSMode, p.Username, sealed, p.FromAddress, p.ReplyTo, p.Enabled,
			actorOrNil(p.ActorID), keep); err != nil {
			return err
		}
		if out, err = scanEmailSettings(tx.QueryRow(ctx, emailSettingsSelect+` WHERE s.workspace_id = $1`, p.WorkspaceID)); err != nil {
			return err
		}
		if len(changed) == 0 {
			return nil
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, emailSettingsEntity, p.WorkspaceID, EmailSettingsUpdatedAction,
			map[string]any{"fields": changed, "host": p.Host, "created": !exists})
	})
	return out, err
}

func actorOrNil(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// DeleteEmailSettings removes the workspace's settings; its email goes
// through the instance configuration again.
func (s *Store) DeleteEmailSettings(ctx context.Context, wsID, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var host string
		err := tx.QueryRow(ctx, `DELETE FROM workspace_email_settings WHERE workspace_id = $1 RETURNING host`, wsID).Scan(&host)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, emailSettingsEntity, wsID, EmailSettingsDeletedAction,
			map[string]any{"host": host})
	})
}

// EmailTransport returns the workspace's settings with the decrypted password
// (ErrNotFound: none stored). The caller decides what a disabled row means.
func (s *Store) EmailTransport(ctx context.Context, wsID uuid.UUID) (EmailTransport, error) {
	var (
		t       EmailTransport
		sealed  []byte
		replyTo *string
	)
	err := s.pool.QueryRow(ctx, `SELECT host, port, tls_mode, username, password_encrypted, from_address, reply_to, is_enabled
		FROM workspace_email_settings WHERE workspace_id = $1`, wsID).
		Scan(&t.Host, &t.Port, &t.TLSMode, &t.Username, &sealed, &t.FromAddress, &replyTo, &t.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if replyTo != nil {
		t.ReplyTo = *replyTo
	}
	if sealed != nil {
		if s.box == nil {
			return t, errors.New("email settings require NEBULA_ENCRYPTION_KEY")
		}
		pw, err := s.box.Open(sealed, smtpPasswordContext(wsID))
		if err != nil {
			return t, fmt.Errorf("workspace %s: SMTP password cannot be decrypted with the configured keys", wsID)
		}
		t.Password = string(pw)
	}
	return t, nil
}

// RecordEmailTest audits a test send: who sent it, through which server
// ("workspace" or "instance"), to whom, and whether it was accepted.
func (s *Store) RecordEmailTest(ctx context.Context, wsID, actorID uuid.UUID, source, host, to string, ok bool) error {
	data := map[string]any{"source": source, "to": to, "ok": ok}
	if host != "" {
		data["host"] = host
	}
	return appendEvent(ctx, s.pool, wsID, actorID, emailSettingsEntity, wsID, EmailSettingsTestedAction, data)
}

// MemberEmail returns the stored address of the active member whose email
// matches (case-insensitively), or ErrNotFound: test emails go only to people
// already in the workspace, so the endpoint is never an open relay.
func (s *Store) MemberEmail(ctx context.Context, wsID uuid.UUID, email string) (string, error) {
	var out string
	err := s.pool.QueryRow(ctx, `SELECT u.email FROM users u JOIN workspace_members m ON m.user_id = u.id
		WHERE m.workspace_id = $1 AND u.is_active AND u.email = $2`, wsID, email).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return out, err
}

// reencryptEmailPasswords re-seals SMTP passwords sealed with a retired key.
func (s *Store) reencryptEmailPasswords(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, `SELECT workspace_id, password_encrypted FROM workspace_email_settings
		WHERE password_encrypted IS NOT NULL ORDER BY workspace_id FOR UPDATE`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id     uuid.UUID
		sealed []byte
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		return x, r.Scan(&x.id, &x.sealed)
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, x := range all {
		aad := smtpPasswordContext(x.id)
		pt, old, err := s.box.OpenWithInfo(x.sealed, aad)
		if err != nil {
			return 0, fmt.Errorf("workspace %s: SMTP password cannot be decrypted with the configured keys", x.id)
		}
		if !old {
			continue
		}
		fresh, err := s.box.Seal(pt, aad)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE workspace_email_settings SET password_encrypted = $1 WHERE workspace_id = $2`,
			fresh, x.id); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}
