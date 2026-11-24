package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"unicode"

	"github.com/Tural-HS/Nebula/internal/safehttp"
)

const maxErrorText = 200

// Describe turns a send error into a short text that is safe to show to a
// workspace admin: network errors become a category (never the resolved IP,
// which could map the internal network), SMTP replies keep their code and a
// truncated text, and every given secret is masked. Control characters are
// dropped.
func Describe(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	text, _ := classify(err)
	return clean(text, secrets)
}

// DescribeBrief is Describe without any text from the server: an SMTP reply
// becomes its code only and an unclassified error a generic text. For servers
// whose replies the reader should not see (the operator's instance server).
func DescribeBrief(err error) string {
	if err == nil {
		return ""
	}
	var tp *textproto.Error
	if errors.As(err, &tp) {
		return "the mail server replied " + strconv.Itoa(tp.Code) + " (details are in the server log)"
	}
	text, known := classify(err)
	if !known {
		text = "sending failed (details are in the server log)"
	}
	return clean(text, nil)
}

// classify maps err to a safe text; known is false for the fallback that
// keeps the error's own text.
func classify(err error) (text string, known bool) {
	var (
		tp      *textproto.Error
		dnsErr  *net.DNSError
		netErr  net.Error
		certErr *tls.CertificateVerificationError
		hostErr x509.HostnameError
		opErr   *net.OpError
	)
	known = true
	switch {
	case errors.Is(err, safehttp.ErrBlockedAddress):
		text = "the server address is not allowed (private, loopback or reserved network)"
	case errors.Is(err, ErrInvalidMessage):
		text = "the message was rejected before sending (invalid address or content)"
	case errors.As(err, &tp):
		text = "server replied " + strconv.Itoa(tp.Code) + ": " + tp.Msg
	case errors.As(err, &dnsErr):
		text = "the server name could not be resolved"
	case errors.As(err, &certErr), errors.As(err, &hostErr):
		text = "the server's TLS certificate is not valid for this host"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		text = "the connection timed out"
	case errors.As(err, &opErr) && opErr.Op == "dial":
		text = "the connection was refused or the server is unreachable"
	default:
		// Our own sentinel texts ("mail: server does not support STARTTLS ...",
		// "mail: auth: unencrypted connection") are safe; anything else is
		// reduced to its first line after masking.
		known = false
		text = strings.TrimPrefix(err.Error(), "mail: ")
		if strings.Contains(text, "dial tcp") {
			text, known = "the connection was refused or the server is unreachable", true
		}
	}
	return text, known
}

// clean masks secrets, drops control characters and truncates.
func clean(text string, secrets []string) string {
	for _, s := range secrets {
		if len(s) >= 3 {
			text = strings.ReplaceAll(text, s, "•••")
		}
	}
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > maxErrorText {
		cut := maxErrorText
		for cut > 0 && (text[cut]&0xC0) == 0x80 {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}
