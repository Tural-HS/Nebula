package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// fakeIDP is a minimal OIDC provider: discovery document + JWKS + token signing.
type fakeIDP struct {
	srv        *httptest.Server
	key        *rsa.PrivateKey
	issuer     string // what discovery advertises (defaults to srv.URL)
	discovered atomic.Int32
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIDP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		idp.discovered.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": idp.issuer, "jwks_uri": idp.srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	idp.srv = httptest.NewServer(mux)
	idp.issuer = idp.srv.URL
	t.Cleanup(idp.srv.Close)
	return idp
}

func (idp *fakeIDP) sign(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	obj, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (idp *fakeIDP) claims(overrides map[string]any) map[string]any {
	c := map[string]any{
		"iss": idp.srv.URL, "sub": "user-1", "aud": []string{"nebula-api", "account"},
		"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(),
		"typ": "Bearer", "email": "alice@example.com", "name": "Alice",
	}
	for k, v := range overrides {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	return c
}

func TestOIDCVerifier(t *testing.T) {
	idp := newFakeIDP(t)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	v := NewOIDCVerifier(OIDCConfig{Issuer: idp.srv.URL, Audience: "nebula-api"})
	ctx := context.Background()

	got, err := v.Verify(ctx, idp.sign(t, idp.key, idp.claims(nil)))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if got.Subject != "user-1" || got.Email != "alice@example.com" || got.Name != "Alice" || got.Issuer != idp.srv.URL {
		t.Fatalf("unexpected claims: %+v", got)
	}

	// Keycloak sends aud as a plain string when there is a single audience.
	if _, err := v.Verify(ctx, idp.sign(t, idp.key, idp.claims(map[string]any{"aud": "nebula-api"}))); err != nil {
		t.Fatalf("string aud rejected: %v", err)
	}

	rejected := map[string]string{
		"expired":          idp.sign(t, idp.key, idp.claims(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})),
		"wrong audience":   idp.sign(t, idp.key, idp.claims(map[string]any{"aud": "other-api"})),
		"wrong issuer":     idp.sign(t, idp.key, idp.claims(map[string]any{"iss": "https://evil.example"})),
		"foreign key":      idp.sign(t, otherKey, idp.claims(nil)),
		"id token":         idp.sign(t, idp.key, idp.claims(map[string]any{"typ": "ID"})),
		"no email":         idp.sign(t, idp.key, idp.claims(map[string]any{"email": nil})),
		"no subject":       idp.sign(t, idp.key, idp.claims(map[string]any{"sub": nil})),
		"garbage":          "not-a-jwt",
		"alg none":         "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + strings.Split(idp.sign(t, idp.key, idp.claims(nil)), ".")[1] + ".",
		"tampered payload": tamper(idp.sign(t, idp.key, idp.claims(nil))),
	}
	for name, tok := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}
	if n := idp.discovered.Load(); n != 1 {
		t.Errorf("discovery fetched %d times, want exactly 1 (cached)", n)
	}
}

// tamper swaps the payload for one with a different subject, keeping the signature.
func tamper(tok string) string {
	parts := strings.Split(tok, ".")
	evil, _ := json.Marshal(map[string]any{"sub": "admin"})
	parts[1] = base64.RawURLEncoding.EncodeToString(evil)
	return strings.Join(parts, ".")
}

func TestOIDCVerifierWithExplicitJWKSSkipsDiscovery(t *testing.T) {
	idp := newFakeIDP(t)
	v := NewOIDCVerifier(OIDCConfig{Issuer: idp.srv.URL, Audience: "nebula-api", JWKSURL: idp.srv.URL + "/jwks"})
	if _, err := v.Verify(context.Background(), idp.sign(t, idp.key, idp.claims(nil))); err != nil {
		t.Fatal(err)
	}
	if idp.discovered.Load() != 0 {
		t.Fatal("discovery must not be called when JWKS URL is configured")
	}
}

func TestOIDCVerifierDiscoveryFailures(t *testing.T) {
	t.Run("issuer mismatch", func(t *testing.T) {
		idp := newFakeIDP(t)
		idp.issuer = "https://evil.example"
		v := NewOIDCVerifier(OIDCConfig{Issuer: idp.srv.URL, Audience: "nebula-api"})
		if _, err := v.Verify(context.Background(), idp.sign(t, idp.key, idp.claims(nil))); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("want ErrUnavailable, got %v", err)
		}
	})
	t.Run("idp down, retry is rate limited", func(t *testing.T) {
		idp := newFakeIDP(t)
		url := idp.srv.URL
		idp.srv.Close()
		v := NewOIDCVerifier(OIDCConfig{Issuer: url, Audience: "nebula-api"})
		for range 3 {
			if _, err := v.Verify(context.Background(), "x.y.z"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("want ErrUnavailable, got %v", err)
			}
		}
	})
}

func TestNewVerifierDoesNoIO(_ *testing.T) {
	// Startup must not depend on the IdP being reachable.
	_ = NewOIDCVerifier(OIDCConfig{Issuer: "http://127.0.0.1:1", Audience: "x"})
}
