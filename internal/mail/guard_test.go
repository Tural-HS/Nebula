package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/safehttp"
)

func TestGuardRejectsPrivateHostUpFront(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.5", "169.254.169.254", "localhost", "metadata.google.internal", "0x7f000001", "::1"} {
		_, err := NewSMTPSender(Config{Host: host, Port: 587, From: "a@example.com", TLS: TLSStartTLS, Guard: &Guard{}})
		if !errors.Is(err, safehttp.ErrBlockedAddress) {
			t.Errorf("%s: got %v, want ErrBlockedAddress", host, err)
		}
	}
	if _, err := NewSMTPSender(Config{Host: "127.0.0.1", Port: 587, From: "a@example.com", TLS: TLSStartTLS,
		Guard: &Guard{AllowPrivate: true}}); err != nil {
		t.Errorf("AllowPrivate must permit loopback: %v", err)
	}
}

// The dial-time check covers names that resolve to private addresses (DNS
// rebinding): bypass the up-front host check and dial a loopback server.
func TestGuardBlocksPrivateIPAtDialTime(t *testing.T) {
	f := startFake(t, false)
	from, _ := ParseMailbox("a@example.com")
	s := &SMTPSender{cfg: Config{Host: "127.0.0.1", Port: f.port(), TLS: TLSNone, Timeout: 5 * time.Second, Guard: &Guard{}}, from: from}
	err := s.Send(context.Background(), Message{To: []string{"ops@example.com"}, Subject: "x", Body: "y"})
	if !errors.Is(err, safehttp.ErrBlockedAddress) {
		t.Fatalf("got %v, want ErrBlockedAddress", err)
	}
	if d := Describe(err); strings.Contains(d, "127.0.0.1") || !strings.Contains(d, "not allowed") {
		t.Errorf("describe: %q", d)
	}
}

func TestReplyToHeader(t *testing.T) {
	f := startFake(t, false)
	s, err := NewSMTPSender(Config{Host: "127.0.0.1", Port: f.port(), From: "Team <noreply@example.com>",
		ReplyTo: "Help <help@example.com>", TLS: TLSNone, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), Message{To: []string{"ops@example.com"}, Subject: "Hi", Body: "Body"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(f.data, "Reply-To: \"Help\" <help@example.com>\r\n") {
		t.Fatalf("no Reply-To header: %q", f.data)
	}
	if _, err := NewSMTPSender(Config{Host: "h", Port: 25, From: "a@example.com", ReplyTo: "x\r\nBcc: evil@example.com", TLS: TLSNone}); err == nil {
		t.Fatal("header injection through Reply-To accepted")
	}
}

func TestDescribeSanitizes(t *testing.T) {
	long := strings.Repeat("é", 300)
	cases := []struct {
		name string
		err  error
		want string // substring
		not  []string
	}{
		{"smtp reply masks secrets", fmt.Errorf("mail: auth: %w", &textproto.Error{Code: 535, Msg: "5.7.8 bad credentials for bob / hunter2-pass"}),
			"server replied 535", []string{"hunter2-pass", "bob@"}},
		{"dial error hides the IP", fmt.Errorf("mail: connect: %w", &net.OpError{Op: "dial", Net: "tcp",
			Addr: &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 25}, Err: errors.New("connection refused")}),
			"refused", []string{"10.1.2.3"}},
		{"dns", &net.DNSError{Err: "no such host", Name: "mail.example.com"}, "could not be resolved", nil},
		{"timeout", context.DeadlineExceeded, "timed out", nil},
		{"control characters", errors.New("bad\r\nInjected: header"), "bad Injected", []string{"\r", "\n"}},
		{"truncated", &textproto.Error{Code: 554, Msg: long}, "…", nil},
	}
	for _, c := range cases {
		got := Describe(c.err, "hunter2-pass")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: %q does not contain %q", c.name, got, c.want)
		}
		for _, n := range c.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: %q leaks %q", c.name, got, n)
			}
		}
		if len(got) > maxErrorText+len("…") {
			t.Errorf("%s: %d bytes", c.name, len(got))
		}
	}
}

func TestDescribeBrief(t *testing.T) {
	reply := fmt.Errorf("mail: auth: %w", &textproto.Error{Code: 535, Msg: "5.7.8 auth failed for ops@corp.example"})
	if got := DescribeBrief(reply); !strings.Contains(got, "535") || strings.Contains(got, "ops@corp") || strings.Contains(got, "auth failed") {
		t.Errorf("SMTP reply: %q", got)
	}
	if got := DescribeBrief(errors.New("mail: something internal ops@corp.example")); strings.Contains(got, "ops@corp") {
		t.Errorf("unclassified error leaks its text: %q", got)
	}
	dial := fmt.Errorf("mail: connect: %w", &net.OpError{Op: "dial", Net: "tcp",
		Addr: &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 25}, Err: errors.New("connection refused")})
	if got := DescribeBrief(dial); !strings.Contains(got, "refused") || strings.Contains(got, "10.1.2.3") {
		t.Errorf("dial error: %q", got)
	}
	if DescribeBrief(nil) != "" {
		t.Error("nil error")
	}
}
