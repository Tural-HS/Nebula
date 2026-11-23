package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCConfig configures token verification.
type OIDCConfig struct {
	// Issuer must exactly match the token's "iss" claim.
	Issuer string
	// Audience must be present in the token's "aud" claim.
	Audience string
	// JWKSURL is optional. When set, discovery is skipped and keys are fetched
	// from this URL (useful when the API reaches the IdP on an internal address).
	JWKSURL string
	// HTTPClient is used for discovery and key fetches. Defaults to a 10s-timeout client.
	HTTPClient *http.Client
}

// OIDCVerifier verifies JWT access tokens against the IdP's published keys.
// The IdP is contacted lazily, so Nebula starts even while the IdP is down.
type OIDCVerifier struct {
	cfg OIDCConfig

	mu          sync.Mutex
	verifier    *oidc.IDTokenVerifier
	lastAttempt time.Time
}

const discoveryRetryInterval = 5 * time.Second

// Only asymmetric algorithms: HS* would let anyone holding the shared secret mint tokens.
var supportedAlgs = []string{
	oidc.RS256, oidc.RS384, oidc.RS512,
	oidc.ES256, oidc.ES384, oidc.ES512,
	oidc.PS256, oidc.PS384, oidc.PS512, oidc.EdDSA,
}

// NewOIDCVerifier creates a verifier. It performs no network I/O.
func NewOIDCVerifier(cfg OIDCConfig) *OIDCVerifier {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	v := &OIDCVerifier{cfg: cfg}
	if cfg.JWKSURL != "" {
		v.verifier = v.newVerifier(cfg.JWKSURL)
	}
	return v
}

func (v *OIDCVerifier) newVerifier(jwksURL string) *oidc.IDTokenVerifier {
	// The key set keeps this context for background refreshes, so it must
	// never be a request context (which is cancelled when the request ends).
	keyCtx := oidc.ClientContext(context.Background(), v.cfg.HTTPClient)
	keys := oidc.NewRemoteKeySet(keyCtx, jwksURL)
	return oidc.NewVerifier(v.cfg.Issuer, keys, &oidc.Config{
		ClientID:             v.cfg.Audience,
		SupportedSigningAlgs: supportedAlgs,
	})
}

func (v *OIDCVerifier) get(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.verifier != nil {
		return v.verifier, nil
	}
	if !v.lastAttempt.IsZero() && time.Since(v.lastAttempt) < discoveryRetryInterval {
		return nil, fmt.Errorf("%w: discovery recently failed", ErrUnavailable)
	}
	v.lastAttempt = time.Now()

	jwksURL, err := v.discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	v.verifier = v.newVerifier(jwksURL)
	return v.verifier, nil
}

func (v *OIDCVerifier) discover(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := strings.TrimSuffix(v.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	// The URL comes from operator configuration (validated, https in production), never from requests.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) // #nosec G704 -- trusted config
	if err != nil {
		return "", err
	}
	resp, err := v.cfg.HTTPClient.Do(req) // #nosec G704 -- trusted config
	if err != nil {
		return "", fmt.Errorf("fetch discovery document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discovery document: unexpected status %d", resp.StatusCode)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return "", fmt.Errorf("decode discovery document: %w", err)
	}
	// Prevents an attacker-controlled document from redirecting key lookups.
	if doc.Issuer != v.cfg.Issuer {
		return "", fmt.Errorf("discovery issuer %q does not match configured issuer", doc.Issuer)
	}
	if doc.JWKSURI == "" {
		return "", fmt.Errorf("discovery document has no jwks_uri")
	}
	return doc.JWKSURI, nil
}

// Verify checks signature, issuer, audience and expiry, then extracts claims.
func (v *OIDCVerifier) Verify(ctx context.Context, raw string) (Claims, error) {
	ver, err := v.get(ctx)
	if err != nil {
		return Claims{}, err
	}
	tok, err := ver.Verify(ctx, raw)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	var c struct {
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Typ               string `json:"typ"`
	}
	if err := tok.Claims(&c); err != nil {
		return Claims{}, fmt.Errorf("%w: decode claims: %w", ErrInvalidToken, err)
	}
	// Keycloak marks ID tokens typ=ID; only access tokens may call the API.
	if c.Typ != "" && !strings.EqualFold(c.Typ, "Bearer") {
		return Claims{}, fmt.Errorf("%w: token type %q is not an access token", ErrInvalidToken, c.Typ)
	}
	if tok.Subject == "" {
		return Claims{}, fmt.Errorf("%w: missing sub claim", ErrInvalidToken)
	}
	if c.Email == "" {
		return Claims{}, fmt.Errorf("%w: missing email claim", ErrInvalidToken)
	}
	name := c.Name
	if name == "" {
		name = c.PreferredUsername
	}
	if name == "" {
		name = c.Email
	}
	return Claims{Issuer: tok.Issuer, Subject: tok.Subject, Email: c.Email, Name: name, EmailVerified: c.EmailVerified}, nil
}
