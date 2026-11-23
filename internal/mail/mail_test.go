package mail

import (
	"bufio"
	"context"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuild(t *testing.T) {
	from, _ := mail.ParseAddress("Nebula <noreply@nebula.example>")
	msg, rcpts, err := Build(from, Message{
		To:      []string{"Əli Həsənov <ali@example.com>", "ops@example.com"},
		Subject: "OPS-12 statusu dəyişdi",
		Body:    "Salam,\nOPS-12 Done oldu.",
	}, time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	for _, want := range []string{
		"From: \"Nebula\" <noreply@nebula.example>\r\n",
		"Subject: =?utf-8?q?",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"Auto-Submitted: auto-generated\r\n",
		"Message-ID: <",
		"@nebula.example>\r\n",
		"\r\n\r\nSalam,\r\nOPS-12 Done oldu.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q:\n%s", want, s)
		}
	}
	if len(rcpts) != 2 || rcpts[0] != "ali@example.com" {
		t.Fatalf("rcpts: %v", rcpts)
	}
}

func TestBuildRejectsInjection(t *testing.T) {
	from, _ := mail.ParseAddress("noreply@nebula.example")
	bad := []Message{
		{To: []string{"a@example.com"}, Subject: "hi\r\nBcc: victim@example.com"},
		{To: []string{"a@example.com\r\nBcc: victim@example.com"}, Subject: "x"},
		{To: []string{"not an address"}, Subject: "x"},
		{To: nil, Subject: "x"},
		{To: []string{"a@example.com"}, Subject: strings.Repeat("x", 301)},
	}
	for i, m := range bad {
		if _, _, err := Build(from, m, time.Now()); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestNewSMTPSenderValidation(t *testing.T) {
	good := Config{Host: "smtp.example.com", Port: 587, From: "noreply@example.com", TLS: TLSStartTLS}
	if _, err := NewSMTPSender(good); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Config){
		"no host": func(c *Config) { c.Host = "" },
		"port":    func(c *Config) { c.Port = 0 },
		"tls":     func(c *Config) { c.TLS = "ssl3" },
		"from":    func(c *Config) { c.From = "nope" },
	} {
		c := good
		mut(&c)
		if _, err := NewSMTPSender(c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// fakeSMTP is a minimal SMTP server that records one message.
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	from     string
	rcpts    []string
	data     string
	startTLS bool // advertise STARTTLS (without supporting it)
}

func startFake(t *testing.T, advertiseStartTLS bool) *fakeSMTP {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, startTLS: advertiseStartTLS}
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if f.startTLS {
				say("250-fake")
				say("250 STARTTLS")
			} else {
				say("250 fake")
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			f.mu.Lock()
			f.from = strings.TrimSpace(line[10:])
			f.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			f.mu.Lock()
			f.rcpts = append(f.rcpts, strings.TrimSpace(line[8:]))
			f.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("502 unsupported")
		}
	}
}

func TestSendOverSMTP(t *testing.T) {
	f := startFake(t, false)
	s, err := NewSMTPSender(Config{Host: "127.0.0.1", Port: f.port(), From: "Nebula <noreply@nebula.example>", TLS: TLSNone, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), Message{To: []string{"ops@example.com"}, Subject: "Hi", Body: "Body"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.from != "<noreply@nebula.example>" || len(f.rcpts) != 1 || f.rcpts[0] != "<ops@example.com>" || !strings.Contains(f.data, "Subject: Hi") {
		t.Fatalf("from=%q rcpts=%v data=%q", f.from, f.rcpts, f.data)
	}
}

func TestStartTLSIsMandatory(t *testing.T) {
	f := startFake(t, false) // server does not offer STARTTLS
	s, _ := NewSMTPSender(Config{Host: "127.0.0.1", Port: f.port(), From: "noreply@nebula.example", TLS: TLSStartTLS, Timeout: 5 * time.Second})
	err := s.Send(context.Background(), Message{To: []string{"ops@example.com"}, Subject: "x", Body: "y"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("want downgrade refusal, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data != "" {
		t.Fatal("message was sent without TLS")
	}
}
