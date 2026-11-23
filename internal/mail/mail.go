// Package mail sends plain-text email over SMTP.
//
// Security properties:
//   - No header injection: subjects and addresses containing CR/LF are rejected,
//     non-ASCII subjects are RFC 2047 encoded.
//   - No TLS downgrade: in "starttls" mode a server without STARTTLS is an error.
//   - Credentials are never sent in clear text (net/smtp PlainAuth refuses
//     unencrypted connections except to localhost).
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidMessage wraps deterministic message errors (bad address, subject,
// size): retrying cannot fix them.
var ErrInvalidMessage = errors.New("mail: invalid message")

// TLS modes.
const (
	TLSStartTLS = "starttls" // port 587: plain connect, then mandatory STARTTLS
	TLSImplicit = "tls"      // port 465: TLS from the first byte
	TLSNone     = "none"     // development only (e.g. Mailpit)
)

const (
	maxRecipients = 50
	maxSubjectLen = 300
	maxBodyLen    = 512 << 10
)

// Config holds SMTP settings.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // "Nebula <noreply@example.com>" or a bare address
	TLS      string
	Timeout  time.Duration
}

// Message is a plain-text email.
type Message struct {
	To      []string
	Subject string
	Body    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTPSender sends through one SMTP server.
type SMTPSender struct {
	cfg  Config
	from *mail.Address
}

// NewSMTPSender validates cfg and returns a sender.
func NewSMTPSender(cfg Config) (*SMTPSender, error) {
	if cfg.Host == "" {
		return nil, errors.New("mail: host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, errors.New("mail: invalid port")
	}
	switch cfg.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return nil, fmt.Errorf("mail: unknown TLS mode %q", cfg.TLS)
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, errors.New("mail: invalid From address")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &SMTPSender{cfg: cfg, from: from}, nil
}

// Build renders an RFC 5322 message. Exposed for tests and previews.
func Build(from *mail.Address, m Message, now time.Time) ([]byte, []string, error) {
	if len(m.To) == 0 || len(m.To) > maxRecipients {
		return nil, nil, fmt.Errorf("mail: 1 to %d recipients required", maxRecipients)
	}
	rcpts := make([]string, 0, len(m.To))
	headers := make([]string, 0, len(m.To))
	for _, to := range m.To {
		if strings.ContainsAny(to, "\r\n") {
			return nil, nil, errors.New("mail: invalid recipient")
		}
		a, err := mail.ParseAddress(to)
		if err != nil {
			return nil, nil, fmt.Errorf("mail: invalid recipient %q", to)
		}
		rcpts = append(rcpts, a.Address)
		headers = append(headers, a.String())
	}
	if strings.ContainsAny(m.Subject, "\r\n") || len(m.Subject) > maxSubjectLen {
		return nil, nil, errors.New("mail: invalid subject")
	}
	if len(m.Body) > maxBodyLen {
		return nil, nil, errors.New("mail: body too large")
	}

	domain := "nebula.local"
	if _, d, ok := strings.Cut(from.Address, "@"); ok {
		domain = d
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)

	var b bytes.Buffer
	w := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	w("From", from.String())
	w("To", strings.Join(headers, ", "))
	w("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	w("Date", now.UTC().Format(time.RFC1123Z))
	w("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	w("MIME-Version", "1.0")
	w("Content-Type", "text/plain; charset=utf-8")
	w("Content-Transfer-Encoding", "quoted-printable")
	w("Auto-Submitted", "auto-generated") // RFC 3834: tells autoresponders not to reply
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(m.Body, "\n", "\r\n"))); err != nil {
		return nil, nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, nil, err
	}
	return b.Bytes(), rcpts, nil
}

// Send delivers m, honoring ctx's deadline.
func (s *SMTPSender) Send(ctx context.Context, m Message) error {
	msg, rcpts, err := Build(s.from, m, time.Now())
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	deadline := time.Now().Add(s.cfg.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}

	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	if s.cfg.TLS == TLSImplicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if s.cfg.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not support STARTTLS (refusing to send in clear text)")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("mail: RCPT TO: %w", err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("mail: end data: %w", err)
	}
	return c.Quit()
}
