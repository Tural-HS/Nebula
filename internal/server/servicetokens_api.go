package server

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/apitoken"
	"github.com/Tural-HS/Nebula/internal/auth"
	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Workspace service tokens (ADR-0043): non-human principals for
// integrations (1C, bank statement scripts, BI tools). A token is accepted
// as "Authorization: Bearer nbl_…" next to OIDC access tokens, reaches only
// the routes in tokenRoutes (default deny) of its own workspace, and only
// with the matching scope.

type tokenCtxKey struct{}

// serviceToken returns the request's service token, if it was made with one.
func serviceToken(r *http.Request) (store.TokenPrincipal, bool) {
	t, ok := r.Context().Value(tokenCtxKey{}).(store.TokenPrincipal)
	return t, ok
}

// tokenRoute is a route a service token may call. Paths are relative to
// /api/v1 and match the raw request path the router matches too.
type tokenRoute struct {
	method string
	path   *regexp.Regexp
	scope  string // "" = any valid token
	// workspaceWide routes aggregate over projects; a token with a project
	// allow-list may not call them.
	workspaceWide bool
}

const (
	wsPath   = `^/workspaces/[^/]+`
	itemPath = wsPath + `/projects/[^/]+/items`
)

var tokenRoutes = []tokenRoute{
	{http.MethodGet, regexp.MustCompile(`^/me$`), "", false},
	{http.MethodGet, regexp.MustCompile(wsPath + `/?$`), "", false},
	{http.MethodGet, regexp.MustCompile(wsPath + `/(item-types|workflows|fields|fields/[^/]+)$`), store.ScopeItemsRead, false},
	{http.MethodGet, regexp.MustCompile(wsPath + `/projects$`), store.ScopeItemsRead, false},
	{http.MethodGet, regexp.MustCompile(wsPath + `/projects/[^/]+/?$`), store.ScopeItemsRead, false},
	{http.MethodGet, regexp.MustCompile(itemPath + `(/counts|/[0-9]+|/[0-9]+/transitions|/[0-9]+/comments|/[0-9]+/activity)?$`), store.ScopeItemsRead, false},
	{http.MethodPost, regexp.MustCompile(itemPath + `$`), store.ScopeItemsWrite, false},
	{http.MethodPatch, regexp.MustCompile(itemPath + `/[0-9]+$`), store.ScopeItemsWrite, false},
	{http.MethodPost, regexp.MustCompile(itemPath + `/[0-9]+/comments$`), store.ScopeCommentsWrite, false},
	{http.MethodGet, regexp.MustCompile(itemPath + `/export$`), store.ScopeExportRead, false},
	{http.MethodGet, regexp.MustCompile(wsPath + `/reports(/workload\.csv)?$`), store.ScopeReportsRead, true},
}

// apiPath is the path below /api/v1 as the router sees it.
func apiPath(r *http.Request) string {
	p := r.URL.RawPath
	if p == "" {
		p = r.URL.Path
	}
	return strings.TrimPrefix(p, "/api/v1")
}

func matchTokenRoute(method, path string) (tokenRoute, bool) {
	if method == http.MethodHead {
		method = http.MethodGet
	}
	for _, rt := range tokenRoutes {
		if rt.method == method && rt.path.MatchString(path) {
			return rt, true
		}
	}
	return tokenRoute{}, false
}

// tokenDenials audits refused requests of valid tokens, at most once a
// minute per token and reason, so a misbehaving script cannot flood the
// audit log (and the webhooks that follow it).
type tokenDenials struct {
	store  *store.Store
	logger *slog.Logger
	limit  *ratelimit.Limiter
}

func newTokenDenials(st *store.Store, logger *slog.Logger) *tokenDenials {
	return &tokenDenials{store: st, logger: logger, limit: ratelimit.New(1.0/60, 1)}
}

func (d *tokenDenials) record(ctx context.Context, t store.TokenPrincipal, reason string, r *http.Request) {
	if ok, _ := d.limit.Allow(t.TokenID.String() + ":" + reason); !ok {
		return
	}
	route := apiPath(r)
	if len(route) > 200 {
		route = route[:200]
	}
	// Detached from the request: a client hanging up must not drop the record.
	if err := d.store.RecordServiceTokenDenied(context.WithoutCancel(ctx), t, reason, r.Method, route); err != nil {
		d.logger.ErrorContext(ctx, "record service token denial", "error", err, "token_id", t.TokenID)
	}
}

// authenticate accepts service tokens and hands every other bearer value to
// the OIDC middleware. A value with the nbl_ prefix never reaches the OIDC
// verifier.
func authenticate(st *store.Store, denials *tokenDenials, clientIP func(*http.Request) string, logger *slog.Logger,
	oidc func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		viaOIDC := oidc(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, raw, _ := strings.Cut(r.Header.Get("Authorization"), " ")
			raw = strings.TrimSpace(raw)
			if !strings.EqualFold(scheme, "Bearer") || !apitoken.Looks(raw) {
				viaOIDC.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			t, err := st.ResolveServiceToken(ctx, raw, clientIP(r))
			reject := func(code, msg, reason string) {
				if reason != "" {
					denials.record(ctx, t, reason, r)
				}
				w.Header().Set("WWW-Authenticate", `Bearer realm="nebula", error="invalid_token"`)
				writeError(w, http.StatusUnauthorized, code, msg)
			}
			switch {
			case err == nil:
			case errors.Is(err, store.ErrTokenInvalid):
				reject("invalid_token", "invalid or unknown service token", "")
				return
			case errors.Is(err, store.ErrTokenExpired):
				reject("token_expired", "this service token has expired; an admin can create a new one", "expired")
				return
			case errors.Is(err, store.ErrTokenRevoked):
				reject("token_revoked", "this service token was revoked", "revoked")
				return
			case errors.Is(err, store.ErrTokenWorkspaceArchived):
				reject("token_workspace_archived", "the token's workspace is archived", "workspace_archived")
				return
			default:
				logger.ErrorContext(ctx, "resolve service token", "error", err)
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
				return
			}
			ctx = auth.WithPrincipal(ctx, auth.Principal{UserID: t.UserID, DisplayName: t.DisplayName})
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, tokenCtxKey{}, t)))
		})
	}
}

// tokenPolicy enforces the route table, scopes, the token's workspace and
// the per-token rate limit. Requests made with OIDC tokens pass untouched.
func tokenPolicy(denials *tokenDenials, limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t, ok := serviceToken(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			path := apiPath(r)
			rt, ok := matchTokenRoute(r.Method, path)
			if !ok {
				denials.record(r.Context(), t, "route_not_allowed", r)
				writeError(w, http.StatusForbidden, "token_route_forbidden", "service tokens cannot use this route")
				return
			}
			if seg := strings.Split(path, "/"); len(seg) > 2 && seg[1] == "workspaces" && !strings.EqualFold(seg[2], t.WorkspaceSlug) {
				denials.record(r.Context(), t, "other_workspace", r)
				writeError(w, http.StatusNotFound, "not_found", "resource not found")
				return
			}
			if rt.scope != "" && !t.HasScope(rt.scope) {
				denials.record(r.Context(), t, "scope_missing", r)
				writeError(w, http.StatusForbidden, "token_scope_missing", "this service token lacks the "+rt.scope+" scope")
				return
			}
			if rt.workspaceWide && t.ProjectIDs != nil {
				denials.record(r.Context(), t, "project_restricted", r)
				writeError(w, http.StatusForbidden, "token_project_restricted", "a token limited to projects cannot read workspace-wide data")
				return
			}
			if ok, wait := limiter.Allow(t.TokenID.String()); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests for this service token; retry later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Management (admins, never tokens: tokenRoutes does not list these)
// ---------------------------------------------------------------------------

func (a *api) serviceTokenRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	admin.Get("/service-tokens", a.listServiceTokens)
	admin.Post("/service-tokens", a.createServiceToken)
	admin.Delete("/service-tokens/{tokenID}", a.revokeServiceToken)
}

func (a *api) listServiceTokens(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListServiceTokens(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": nonNil(list), "scopes": store.ServiceTokenScopes, "max_days": store.ServiceTokenMaxDays})
}

func (a *api) createServiceToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string      `json:"name"`
		Role          string      `json:"role"`
		Scopes        []string    `json:"scopes"`
		ProjectIDs    []uuid.UUID `json:"project_ids"`
		ExpiresInDays int         `json:"expires_in_days"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	t, err := a.store.CreateServiceToken(r.Context(), store.CreateServiceTokenParams{
		WorkspaceID: ws.ID, ActorID: principal(r).UserID, ActorRole: ws.Role,
		Name: body.Name, Role: body.Role, Scopes: body.Scopes, ProjectIDs: body.ProjectIDs, ExpiresDays: body.ExpiresInDays,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.logger.InfoContext(r.Context(), "service token created", "workspace_id", ws.ID, "token_id", t.ID, "actor_id", principal(r).UserID)
	writeJSON(w, http.StatusCreated, t)
}

func (a *api) revokeServiceToken(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "tokenID")
	if !ok {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.RevokeServiceToken(r.Context(), ws.ID, id, principal(r).UserID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.logger.InfoContext(r.Context(), "service token revoked", "workspace_id", ws.ID, "token_id", id, "actor_id", principal(r).UserID)
	w.WriteHeader(http.StatusNoContent)
}
