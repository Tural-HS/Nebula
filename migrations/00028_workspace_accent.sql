-- Workspace accent colour (ADR-0035): one of the curated, contrast-checked
-- scales of web/src/lib/accent.ts. NULL is the default Nebula teal, so the
-- default is never stored as a key. The store validates the key first; the
-- named CHECK is the backstop.
--
-- Locking: ADD COLUMN with no default (NULL) is a metadata-only change in
-- PostgreSQL: ACCESS EXCLUSIVE on workspaces for milliseconds, no rewrite and
-- no scan (the CHECK has nothing to validate on an all-NULL column).

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE workspaces ADD COLUMN accent text NULL
    CONSTRAINT workspaces_accent_check CHECK (accent IN ('ocean', 'indigo', 'violet', 'plum', 'forest', 'graphite'));

-- +goose Down
-- Only the accent choice is lost; workspaces return to the default colour.
-- The previous image does not read the column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE workspaces DROP COLUMN IF EXISTS accent;
