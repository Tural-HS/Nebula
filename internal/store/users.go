package store

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/auth"
)

// ResolveUser finds the user for (issuer, subject), creating it on first
// sign-in and syncing email/display name when the IdP changes them.
func (s *Store) ResolveUser(ctx context.Context, c auth.Claims) (auth.Principal, error) {
	name := truncateRunes(strings.TrimSpace(c.Name), 200)
	if name == "" {
		name = c.Email
	}
	var p auth.Principal
	var active bool
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, is_active FROM users WHERE oidc_issuer = $1 AND oidc_subject = $2`,
		c.Issuer, c.Subject).Scan(&p.UserID, &p.Email, &p.DisplayName, &active)

	switch {
	case err == nil:
		if !active {
			return auth.Principal{}, auth.ErrUserDisabled
		}
		if !strings.EqualFold(p.Email, c.Email) || p.DisplayName != name {
			_, err := s.pool.Exec(ctx, `UPDATE users SET email = $1, display_name = $2 WHERE id = $3`, c.Email, name, p.UserID)
			if isUniqueViolation(err, "users_email_key") {
				return auth.Principal{}, auth.ErrIdentityConflict
			}
			if err != nil {
				return auth.Principal{}, err
			}
			p.Email, p.DisplayName = c.Email, name
		}
		return p, nil

	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT covers two first requests from the same user racing each other.
		err = s.pool.QueryRow(ctx, `
			INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (oidc_issuer, oidc_subject) DO UPDATE SET email = EXCLUDED.email
			RETURNING id, is_active`,
			newID(), c.Issuer, c.Subject, c.Email, name).Scan(&p.UserID, &active)
		if isUniqueViolation(err, "users_email_key") {
			return auth.Principal{}, auth.ErrIdentityConflict
		}
		if err != nil {
			return auth.Principal{}, err
		}
		if !active {
			return auth.Principal{}, auth.ErrUserDisabled
		}
		p.Email, p.DisplayName = c.Email, name
		return p, nil

	default:
		return auth.Principal{}, err
	}
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
