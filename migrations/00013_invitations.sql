-- Workspace invitations (ADR-0016). Only a SHA-256 hash of the token is
-- stored; the token itself exists only in the email link.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE workspace_invitations (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email        citext NOT NULL CHECK (length(email) BETWEEN 3 AND 320),
    role         text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'guest')),
    token_hash   bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    invited_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    expires_at   timestamptz NOT NULL,
    accepted_at  timestamptz,
    accepted_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    revoked_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK (accepted_at IS NULL OR revoked_at IS NULL)
);
-- One pending invitation per workspace and address.
CREATE UNIQUE INDEX workspace_invitations_pending_idx ON workspace_invitations (workspace_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX workspace_invitations_created_idx ON workspace_invitations (workspace_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS workspace_invitations;
-- +goose StatementEnd
