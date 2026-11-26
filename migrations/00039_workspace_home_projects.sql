-- Home page shortcuts (ADR-0053): an admin picks up to eight projects that
-- every member sees as links on the home page, in this order. The UI shows
-- only the ones the viewer can open (private projects stay hidden from
-- non-members). Ids, not keys, so renaming a project keeps its link; a
-- deleted project simply drops out of the list when it is read.
--
-- Locking: ADD COLUMN with a constant default is metadata-only in
-- PostgreSQL 11+: ACCESS EXCLUSIVE on workspaces for milliseconds, no rewrite.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE workspaces ADD COLUMN home_projects uuid[] NOT NULL DEFAULT '{}'
    CONSTRAINT workspaces_home_projects_check CHECK (cardinality(home_projects) <= 8);

-- +goose Down
-- Only the shortcut choice is lost. The previous image does not read the
-- column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE workspaces DROP COLUMN IF EXISTS home_projects;
