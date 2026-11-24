package mailroute

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/mail"
	"github.com/Tural-HS/Nebula/internal/store"
)

type fakeTransports map[uuid.UUID]store.EmailTransport

var errBroken = errors.New("cannot decrypt")

func (f fakeTransports) EmailTransport(_ context.Context, id uuid.UUID) (store.EmailTransport, error) {
	if id == brokenWS {
		return store.EmailTransport{}, errBroken
	}
	t, ok := f[id]
	if !ok {
		return t, store.ErrNotFound
	}
	return t, nil
}

type instance struct{}

func (instance) Send(context.Context, mail.Message) error { return nil }

var (
	enabledWS  = uuid.New()
	disabledWS = uuid.New()
	plainWS    = uuid.New()
	brokenWS   = uuid.New()
	emptyWS    = uuid.New()
)

func transports() fakeTransports {
	base := store.EmailTransport{Host: "mail.example.com", Port: 587, TLSMode: mail.TLSStartTLS, FromAddress: "team@example.com", Enabled: true}
	disabled := base
	disabled.Enabled = false
	plain := base
	plain.TLSMode, plain.Port = mail.TLSNone, 25
	return fakeTransports{enabledWS: base, disabledWS: disabled, plainWS: plain}
}

func TestFallbackOrder(t *testing.T) {
	r := &Router{Instance: instance{}, Store: transports()}
	cases := []struct {
		name    string
		ws      uuid.UUID
		test    bool
		source  string
		wantErr error
	}{
		{"no workspace context uses the instance", uuid.Nil, false, SourceInstance, nil},
		{"no settings fall back to the instance", emptyWS, false, SourceInstance, nil},
		{"enabled settings win", enabledWS, false, SourceWorkspace, nil},
		{"disabled settings fall back", disabledWS, false, SourceInstance, nil},
		{"a test send uses disabled settings", disabledWS, true, SourceWorkspace, nil},
		{"a broken row never falls back silently", brokenWS, false, SourceWorkspace, errBroken},
		{"policy tightened after saving", plainWS, false, SourceWorkspace, ErrPolicy},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var (
				got Route
				err error
			)
			if c.test {
				got, err = r.ForTest(t.Context(), c.ws)
			} else {
				got, err = r.For(t.Context(), c.ws)
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got.Source != c.source {
				t.Fatalf("source = %q, want %q", got.Source, c.source)
			}
			if c.wantErr == nil && got.Sender == nil {
				t.Fatal("no sender")
			}
			if c.source == SourceInstance && c.wantErr == nil {
				if _, ok := got.Sender.(instance); !ok {
					t.Fatalf("sender %T is not the instance sender", got.Sender)
				}
			}
		})
	}
}

func TestNoInstanceSender(t *testing.T) {
	r := &Router{Store: transports()}
	if _, err := r.For(t.Context(), emptyWS); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v, want ErrNotConfigured", err)
	}
	if got, err := r.For(t.Context(), enabledWS); err != nil || got.Source != SourceWorkspace {
		t.Fatalf("workspace settings must work without an instance sender: %v %v", got, err)
	}
}

func TestWorkspaceSenderIsGuarded(t *testing.T) {
	r := &Router{Policy: store.EmailPolicy{Ports: []int{587}}}
	if _, err := r.Workspace(store.EmailTransport{Host: "10.0.0.1", Port: 587, TLSMode: mail.TLSStartTLS, FromAddress: "a@example.com"}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("private host: %v", err)
	}
	if _, err := r.Workspace(store.EmailTransport{Host: "mail.example.com", Port: 2525, TLSMode: mail.TLSStartTLS, FromAddress: "a@example.com"}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("port outside the policy: %v", err)
	}
	if _, err := r.Workspace(store.EmailTransport{Host: "mail.example.com", Port: 587, TLSMode: mail.TLSStartTLS, FromAddress: "a@example.com"}); err != nil {
		t.Fatalf("valid settings: %v", err)
	}
}
