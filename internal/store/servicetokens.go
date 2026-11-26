package store

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/apitoken"
)

// Service token scopes (ADR-0043): a small, fixed set. A route a token may
// call names exactly one of them; every other route is closed to tokens.
const (
	ScopeItemsRead     = "items:read"
	ScopeItemsWrite    = "items:write"
	ScopeCommentsWrite = "comments:write"
	ScopeReportsRead   = "reports:read"
	ScopeExportRead    = "export:read"
)

// ServiceTokenScopes lists every scope in display order.
var ServiceTokenScopes = []string{ScopeItemsRead, ScopeItemsWrite, ScopeCommentsWrite, ScopeReportsRead, ScopeExportRead}

var writeScopes = map[string]bool{ScopeItemsWrite: true, ScopeCommentsWrite: true}

// Service token limits.
const (
	ServiceTokenMaxDays        = 365
	maxServiceTokensPerWS      = 50
	maxServiceTokenProjects    = 50
	serviceTokenIssuer         = "nebula:service-token"
	serviceTokenDisplayPrefix  = "🔑 "
	serviceTokenNameMaxRunes   = 80
	serviceTokenEmailDomain    = "@service-token.invalid"
	ServiceTokenCreatedAction  = "service_token.created"
	ServiceTokenRevokedAction  = "service_token.revoked"
	ServiceTokenDeniedAction   = "service_token.denied"
	serviceTokenEntity         = "service_token"
	serviceTokenRejectedAction = "service_token.rejected"
)

// Errors of ResolveServiceToken. Only a caller holding the right secret
// learns that a token is expired, revoked or archived; everything else is
// ErrTokenInvalid.
var (
	ErrTokenInvalid           = errors.New("invalid service token")
	ErrTokenExpired           = errors.New("service token expired")
	ErrTokenRevoked           = errors.New("service token revoked")
	ErrTokenWorkspaceArchived = errors.New("service token workspace archived")
)

// ServiceToken is a token as admins see it; the secret is never included.
type ServiceToken struct {
	ID          uuid.UUID        `json:"id"`
	Name        string           `json:"name"`
	DisplayName string           `json:"display_name"`
	Role        string           `json:"role"`
	Scopes      []string         `json:"scopes"`
	Projects    []ServiceProject `json:"projects"` // empty and not restricted: every project the role can see
	// ProjectRestricted: the token was created with an allow-list; with
	// Projects empty (all deleted) it reaches no project.
	ProjectRestricted bool       `json:"project_restricted"`
	CreatedBy         *uuid.UUID `json:"created_by"`
	CreatedByName     *string    `json:"created_by_name"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	RevokedAt         *time.Time `json:"revoked_at"`
	LastUsedAt        *time.Time `json:"last_used_at"`
	LastUsedIP        *string    `json:"last_used_ip"`
}

// ServiceProject is one entry of a token's project allow-list.
type ServiceProject struct {
	ID   uuid.UUID `json:"id"`
	Key  string    `json:"key"`
	Name string    `json:"name"`
}

// TokenPrincipal is a resolved, usable service token.
type TokenPrincipal struct {
	TokenID       uuid.UUID
	UserID        uuid.UUID
	WorkspaceID   uuid.UUID
	WorkspaceSlug string
	DisplayName   string
	Role          string
	Scopes        []string
	// ProjectIDs is the allow-list; nil means every project the role sees.
	// A restricted token always has a non-nil list, empty when its last
	// project was deleted: it then reaches no project (ADR-0043).
	ProjectIDs []uuid.UUID
}

// HasScope reports whether the token carries scope.
func (t TokenPrincipal) HasScope(scope string) bool { return slices.Contains(t.Scopes, scope) }

// AllowsProject reports whether the allow-list (if any) contains id.
func (t TokenPrincipal) AllowsProject(id uuid.UUID) bool {
	return t.ProjectIDs == nil || slices.Contains(t.ProjectIDs, id)
}

// IsServicePrincipalEmail reports whether email is a token principal's
// placeholder address (never a person, never a member).
func IsServicePrincipalEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), serviceTokenEmailDomain)
}

// CreateServiceTokenParams creates a token.
type CreateServiceTokenParams struct {
	WorkspaceID uuid.UUID
	ActorID     uuid.UUID
	ActorRole   string
	Name        string
	Role        string
	Scopes      []string
	ProjectIDs  []uuid.UUID
	ExpiresDays int
}

// CreatedServiceToken carries the secret token string, returned once.
type CreatedServiceToken struct {
	ServiceToken
	Token string `json:"token"`
}

func normalizeScopes(in []string, role string) ([]string, error) {
	if len(in) == 0 {
		return nil, invalid("scopes", "choose at least one scope")
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !slices.Contains(ServiceTokenScopes, s) {
			return nil, invalid("scopes", "unknown scope %q (allowed: %s)", s, strings.Join(ServiceTokenScopes, ", "))
		}
		if role == RoleGuest && writeScopes[s] {
			return nil, invalid("scopes", "a guest token can only have read scopes")
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	// Display order, independent of the request.
	slices.SortFunc(out, func(a, b string) int {
		return slices.Index(ServiceTokenScopes, a) - slices.Index(ServiceTokenScopes, b)
	})
	return out, nil
}

// CreateServiceToken validates p, creates the token and its principal, and
// returns the token string (the only time it exists outside the caller).
func (s *Store) CreateServiceToken(ctx context.Context, p CreateServiceTokenParams) (CreatedServiceToken, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" || utf8.RuneCountInString(name) > serviceTokenNameMaxRunes {
		return CreatedServiceToken{}, invalid("name", "must be 1 to %d characters", serviceTokenNameMaxRunes)
	}
	switch p.Role {
	case RoleAdmin, RoleMember, RoleGuest:
	default:
		return CreatedServiceToken{}, invalid("role", "must be guest, member or admin")
	}
	if !RoleAtLeast(p.ActorRole, p.Role) {
		return CreatedServiceToken{}, ErrForbidden
	}
	scopes, err := normalizeScopes(p.Scopes, p.Role)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	if p.ExpiresDays < 1 || p.ExpiresDays > ServiceTokenMaxDays {
		return CreatedServiceToken{}, invalid("expires_in_days", "must be between 1 and %d", ServiceTokenMaxDays)
	}
	projects := slices.Compact(slices.SortedFunc(slices.Values(p.ProjectIDs), func(a, b uuid.UUID) int {
		return strings.Compare(a.String(), b.String())
	}))
	if len(projects) > maxServiceTokenProjects {
		return CreatedServiceToken{}, invalid("project_ids", "at most %d projects", maxServiceTokenProjects)
	}

	id := newID()
	token, hash, err := apitoken.Generate(id)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		// Serialize creations per workspace so the cap holds.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR UPDATE`, p.WorkspaceID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM service_tokens
			WHERE workspace_id = $1 AND revoked_at IS NULL AND expires_at > now()`, p.WorkspaceID).Scan(&n); err != nil {
			return err
		}
		if n >= maxServiceTokensPerWS {
			return invalid("name", "a workspace can have at most %d active tokens; revoke unused ones first", maxServiceTokensPerWS)
		}
		if len(projects) > 0 {
			// Admins see every project (ADR-0013); archived ones are refused.
			var found int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM projects
				WHERE workspace_id = $1 AND id = ANY($2::uuid[]) AND archived_at IS NULL`, p.WorkspaceID, projects).Scan(&found); err != nil {
				return err
			}
			if found != len(projects) {
				return invalid("project_ids", "unknown or archived project")
			}
		}
		userID := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name)
			VALUES ($1, $2, $3, $4, $5)`,
			userID, serviceTokenIssuer, id.String(), id.String()+serviceTokenEmailDomain, serviceTokenDisplayPrefix+name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO service_tokens (id, workspace_id, user_id, name, role, scopes, secret_hash, created_by, expires_at, project_restricted)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now() + make_interval(days => $9), $10)`,
			id, p.WorkspaceID, userID, name, p.Role, scopes, hash, p.ActorID, p.ExpiresDays, len(projects) > 0); err != nil {
			return err
		}
		if len(projects) > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO service_token_projects (token_id, workspace_id, project_id)
				SELECT $1, $2, unnest($3::uuid[])`, id, p.WorkspaceID, projects); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, serviceTokenEntity, id, ServiceTokenCreatedAction, map[string]any{
			"name": name, "role": p.Role, "scopes": scopes, "project_ids": projects, "expires_in_days": p.ExpiresDays,
		})
	})
	if err != nil {
		return CreatedServiceToken{}, err
	}
	t, err := s.getServiceToken(ctx, p.WorkspaceID, id)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	return CreatedServiceToken{ServiceToken: t, Token: token}, nil
}

const integrationSelect = `SELECT t.id, t.name, u.display_name, t.role, t.scopes,
		COALESCE((SELECT json_agg(json_build_object('id', p.id, 'key', p.key, 'name', p.name) ORDER BY p.key)
		          FROM service_token_projects sp JOIN projects p ON p.id = sp.project_id
		          WHERE sp.token_id = t.id), '[]'::json), t.project_restricted,
		t.created_by, c.display_name, t.created_at, t.expires_at, t.revoked_at, t.last_used_at, host(t.last_used_ip)
	FROM service_tokens t JOIN users u ON u.id = t.user_id LEFT JOIN users c ON c.id = t.created_by`

func scanServiceToken(r pgx.CollectableRow) (ServiceToken, error) {
	var t ServiceToken
	err := r.Scan(&t.ID, &t.Name, &t.DisplayName, &t.Role, &t.Scopes, &t.Projects, &t.ProjectRestricted, &t.CreatedBy, &t.CreatedByName,
		&t.CreatedAt, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.LastUsedIP)
	return t, err
}

// ListServiceTokens returns the workspace's tokens, newest first, revoked
// ones included (they stay as a record of what had access).
func (s *Store) ListServiceTokens(ctx context.Context, wsID uuid.UUID) ([]ServiceToken, error) {
	rows, err := s.pool.Query(ctx, integrationSelect+` WHERE t.workspace_id = $1 ORDER BY t.created_at DESC, t.id LIMIT 200`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanServiceToken)
}

func (s *Store) getServiceToken(ctx context.Context, wsID, id uuid.UUID) (ServiceToken, error) {
	rows, err := s.pool.Query(ctx, integrationSelect+` WHERE t.workspace_id = $1 AND t.id = $2`, wsID, id)
	if err != nil {
		return ServiceToken{}, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, scanServiceToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServiceToken{}, ErrNotFound
	}
	return t, err
}

// RevokeServiceToken revokes a token at once (the next request fails) and
// deactivates its principal. Revoking a revoked token changes nothing.
func (s *Store) RevokeServiceToken(ctx context.Context, wsID, id, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var name string
		var userID uuid.UUID
		var revoked bool
		err := tx.QueryRow(ctx, `SELECT name, user_id, revoked_at IS NOT NULL FROM service_tokens
			WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wsID, id).Scan(&name, &userID, &revoked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil || revoked {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE service_tokens SET revoked_at = now(), revoked_by = $2 WHERE id = $1`, id, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, userID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, serviceTokenEntity, id, ServiceTokenRevokedAction, map[string]any{"name": name})
	})
}

// dummyHash keeps the work of an unknown token id close to a known one.
var dummyHash = apitoken.Hash("nebula-unknown-token")

// ResolveServiceToken checks a raw "nbl_…" bearer token and, when it may be
// used, returns its principal and records the use (at most once a minute
// per token, so busy integrations do not write on every request).
func (s *Store) ResolveServiceToken(ctx context.Context, raw, clientIP string) (TokenPrincipal, error) {
	id, secret, err := apitoken.Parse(raw)
	if err != nil {
		return TokenPrincipal{}, ErrTokenInvalid
	}
	var t TokenPrincipal
	var hash []byte
	var expired, revoked, archived, touch, restricted bool
	err = s.pool.QueryRow(ctx, `SELECT t.user_id, t.workspace_id, w.slug, u.display_name, t.role, t.scopes, t.secret_hash,
			t.project_restricted, t.expires_at <= now(), t.revoked_at IS NOT NULL, w.archived_at IS NOT NULL,
			t.last_used_at IS NULL OR t.last_used_at < now() - interval '1 minute',
			ARRAY(SELECT project_id FROM service_token_projects WHERE token_id = t.id ORDER BY project_id)
		FROM service_tokens t JOIN workspaces w ON w.id = t.workspace_id JOIN users u ON u.id = t.user_id
		WHERE t.id = $1`, id).
		Scan(&t.UserID, &t.WorkspaceID, &t.WorkspaceSlug, &t.DisplayName, &t.Role, &t.Scopes, &hash,
			&restricted, &expired, &revoked, &archived, &touch, &t.ProjectIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		apitoken.Verify(secret, dummyHash)
		return TokenPrincipal{}, ErrTokenInvalid
	}
	if err != nil {
		return TokenPrincipal{}, err
	}
	if !apitoken.Verify(secret, hash) {
		return TokenPrincipal{}, ErrTokenInvalid
	}
	t.TokenID = id
	switch {
	case restricted && len(t.ProjectIDs) == 0:
		// Its last project was deleted: deny everything rather than widen
		// to the whole workspace (ADR-0043).
		t.ProjectIDs = []uuid.UUID{}
	case !restricted:
		t.ProjectIDs = nil
	}
	switch {
	case revoked:
		return t, ErrTokenRevoked
	case expired:
		return t, ErrTokenExpired
	case archived:
		return t, ErrTokenWorkspaceArchived
	}
	if touch {
		var ip any
		if a, err := netip.ParseAddr(clientIP); err == nil {
			ip = a.String()
		}
		// The condition repeats the check so concurrent requests write once.
		if _, err := s.pool.Exec(ctx, `UPDATE service_tokens SET last_used_at = now(), last_used_ip = $2::inet
			WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`,
			id, ip); err != nil {
			return TokenPrincipal{}, err
		}
	}
	return t, nil
}

// RecordServiceTokenDenied audits a refused request of a token whose
// secret was valid: a route outside its scopes or role, a project outside
// its allow-list, another workspace, or an expired/revoked token. The
// payload carries the reason, method and route, never the token. Callers
// throttle it.
func (s *Store) RecordServiceTokenDenied(ctx context.Context, t TokenPrincipal, reason, method, route string) error {
	action := ServiceTokenDeniedAction
	if reason == "expired" || reason == "revoked" || reason == "workspace_archived" {
		action = serviceTokenRejectedAction
	}
	return appendEvent(ctx, s.pool, t.WorkspaceID, t.UserID, serviceTokenEntity, t.TokenID, action, map[string]any{
		"reason": reason, "method": method, "route": route,
	})
}

// WorkspaceForServiceToken returns the token's own workspace by slug with
// the token's role. Another slug is ErrNotFound: a token never reaches a
// second workspace, whatever memberships exist.
func (s *Store) WorkspaceForServiceToken(ctx context.Context, slug string, t TokenPrincipal) (Workspace, error) {
	rows, err := s.pool.Query(ctx, `SELECT w.id, w.slug, w.name, $3::text, w.created_at, w.timezone, w.overdue_digest_hour, w.accent, w.home_projects,
			w.archived_at, w.deletion_requested_at IS NOT NULL
		FROM workspaces w WHERE w.slug = $1 AND w.id = $2`, strings.ToLower(slug), t.WorkspaceID, t.Role)
	if err != nil {
		return Workspace{}, err
	}
	ws, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Workspace])
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	return ws, err
}
