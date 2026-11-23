// Package auth authenticates API requests with OIDC bearer tokens issued by an
// external identity provider (Keycloak, Authentik, Entra ID, ...). Nebula never
// sees or stores passwords.
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Claims are the identity facts Nebula takes from a verified access token.
type Claims struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
	// EmailVerified is the IdP's email_verified claim. Only a verified
	// address may accept an invitation sent to it (ADR-0016).
	EmailVerified bool
}

// Principal is the authenticated Nebula user attached to a request context.
type Principal struct {
	UserID      uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	// EmailVerified comes from the current token, never from storage.
	EmailVerified bool `json:"email_verified"`
}

// Verifier validates a raw bearer token and returns its claims.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (Claims, error)
}

// UserResolver maps verified claims to a Nebula user, creating it on first sign-in.
type UserResolver interface {
	ResolveUser(ctx context.Context, c Claims) (Principal, error)
}

var (
	// ErrInvalidToken means signature, issuer, audience, expiry or required claims failed.
	ErrInvalidToken = errors.New("invalid token")
	// ErrUnavailable means the identity provider's metadata or keys could not be fetched.
	ErrUnavailable = errors.New("identity provider unavailable")
	// ErrUserDisabled means the user exists but was deactivated in Nebula.
	ErrUserDisabled = errors.New("user disabled")
	// ErrIdentityConflict means the token's email belongs to a different identity.
	ErrIdentityConflict = errors.New("identity conflict")
)

type principalKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the authenticated user, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
