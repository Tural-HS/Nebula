-- Per-workspace SMTP settings (ADR-0038). A workspace admin can send the
-- workspace's email (automation send_email, invitations, mention emails)
-- through the team's own mail server. No row, or a disabled row, means the
-- instance configuration (NEBULA_SMTP_*) is used, as before.
--
-- password_encrypted is AES-256-GCM (internal/secretbox, ADR-0007) bound to
-- the workspace ID; it is never returned by the API. NULL: no password (an
-- unauthenticated relay, or none set yet). The application validates the
-- host, port, TLS mode and addresses (and checks the destination IP at send
-- time, SSRF); the CHECKs are a coarse backstop.
--
-- Locking: a new table; the foreign keys take SHARE ROW EXCLUSIVE on
-- workspaces and users for an empty table, i.e. milliseconds.

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE workspace_email_settings (
    workspace_id       uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    host               text NOT NULL CHECK (host ~ '^[A-Za-z0-9.:-]{1,253}$'),
    port               integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
    tls_mode           text NOT NULL CHECK (tls_mode IN ('starttls', 'tls', 'none')),
    username           text NOT NULL DEFAULT '' CHECK (length(username) <= 320),
    password_encrypted bytea NULL CHECK (length(password_encrypted) <= 2048),
    from_address       text NOT NULL CHECK (length(from_address) BETWEEN 3 AND 400),
    reply_to           text NULL CHECK (length(reply_to) BETWEEN 3 AND 400),
    is_enabled         boolean NOT NULL DEFAULT true,
    updated_by         uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Workspace SMTP settings (and their encrypted passwords) are lost; every
-- workspace sends through the instance configuration again. The previous
-- image does not read the table, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
DROP TABLE IF EXISTS workspace_email_settings;
