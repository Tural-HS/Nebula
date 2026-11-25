-- Workspace service tokens for integrations (ADR-0043). Each token is a
-- non-human principal: it has its own users row (oidc_issuer
-- 'nebula:service-token', subject = token id, display name "🔑 <name>"), so
-- events, comments and item history name it as the actor. It is never a
-- workspace member: its role, scopes and project allow-list live here and
-- apply only to the token's own workspace. Only SHA-256 of the secret is
-- stored (secret_hash); the secret itself is shown once at creation.
--
-- service_token_projects is the optional allow-list; no rows = every
-- project the role can see. Deleting a project removes it from the list.
--
-- Locking: two new, empty tables. Their foreign keys take SHARE ROW
-- EXCLUSIVE on workspaces, users and projects for milliseconds; no existing
-- row is scanned or rewritten.

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE service_tokens (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    -- The token's own principal; kept after revocation (inactive) because
    -- events and comments reference it.
    user_id      uuid NOT NULL UNIQUE REFERENCES users (id),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    role         text NOT NULL CHECK (role IN ('admin', 'member', 'guest')),
    scopes       text[] NOT NULL CHECK (cardinality(scopes) BETWEEN 1 AND 10),
    secret_hash  bytea NOT NULL CHECK (length(secret_hash) = 32),
    created_by   uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz NULL,
    revoked_by   uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    -- Written at most once a minute per token (ADR-0043).
    last_used_at timestamptz NULL,
    last_used_ip inet NULL,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '366 days')
);
CREATE INDEX service_tokens_workspace_idx ON service_tokens (workspace_id, created_at DESC);

CREATE TABLE service_token_projects (
    token_id     uuid NOT NULL REFERENCES service_tokens (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL,
    project_id   uuid NOT NULL,
    PRIMARY KEY (token_id, project_id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX service_token_projects_project_idx ON service_token_projects (project_id);

-- +goose Down
-- Every service token stops working at once (integrations get 401 from the
-- previous image, which does not know the nbl_ format either). The token
-- principals stay in users, deactivated, so history keeps naming them.
SET LOCAL lock_timeout = '5s';
UPDATE users SET is_active = false WHERE oidc_issuer = 'nebula:service-token';
DROP TABLE IF EXISTS service_token_projects;
DROP TABLE IF EXISTS service_tokens;
