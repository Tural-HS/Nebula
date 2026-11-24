-- Archive, restore and permanently delete projects and workspaces (ADR-0039).
--
-- projects.archived_at already exists (00001) and every list already filters
-- on it; this adds the same state to workspaces, plus a "deletion requested"
-- marker on both. A deletion is asynchronous: the request sets
-- deletion_requested_at (only on an archived row, CHECK below), the worker's
-- purge removes the rows in bounded batches and finally the project or
-- workspace row itself. Re-running the purge is safe at every step.
--
-- workspace_deletions is the audit record of a deleted workspace: the
-- workspace's own events are deleted with it, so the record lives outside.
-- It has no foreign key to workspaces on purpose (it outlives the row).
--
-- Locking: ADD COLUMN without a default is metadata-only (ACCESS EXCLUSIVE on
-- workspaces and projects for milliseconds, no rewrite). The CHECKs scan the
-- two small tables once while the lock is held; both columns they test are
-- NULL on every row (nothing set projects.archived_at before ADR-0039), so
-- they cannot fail. The new table's FK to users takes SHARE ROW EXCLUSIVE on
-- users for an empty table.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE workspaces
    ADD COLUMN archived_at timestamptz NULL,
    ADD COLUMN deletion_requested_at timestamptz NULL,
    ADD CONSTRAINT workspaces_deletion_archived_check CHECK (deletion_requested_at IS NULL OR archived_at IS NOT NULL);

ALTER TABLE projects
    ADD COLUMN deletion_requested_at timestamptz NULL,
    ADD CONSTRAINT projects_deletion_archived_check CHECK (deletion_requested_at IS NULL OR archived_at IS NOT NULL);

CREATE TABLE IF NOT EXISTS workspace_deletions (
    workspace_id  uuid PRIMARY KEY, -- no FK: the record outlives the workspace
    slug          text NOT NULL CHECK (length(slug) <= 100),
    name          text NOT NULL CHECK (length(name) <= 200),
    requested_by  uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    requested_at  timestamptz NOT NULL DEFAULT now(),
    completed_at  timestamptz NULL,
    -- Counts at request time and whether an evidence package (ADR-0029) was
    -- exported after archiving; never item contents.
    summary       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(summary) = 'object')
);

-- +goose Down
-- Refuses while a deletion is in progress: the previous image does not know
-- the marker and would show a half-purged workspace or project as active.
-- Wait for the purge to finish (or run the new image's worker) first.
-- Archived workspaces become active again under the previous image (it does
-- not read the column); archived projects stay hidden (it filters on
-- projects.archived_at). workspace_deletions is kept on purpose: it is the
-- only record of deleted workspaces, and the previous image ignores it.
SET LOCAL lock_timeout = '5s';
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM workspaces WHERE deletion_requested_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM projects WHERE deletion_requested_at IS NOT NULL) THEN
        RAISE EXCEPTION 'a workspace or project deletion is in progress; let the purge finish before rolling back 00031';
    END IF;
END
$$;
-- +goose StatementEnd
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_deletion_archived_check;
ALTER TABLE projects DROP COLUMN IF EXISTS deletion_requested_at;
ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_deletion_archived_check;
ALTER TABLE workspaces DROP COLUMN IF EXISTS deletion_requested_at;
ALTER TABLE workspaces DROP COLUMN IF EXISTS archived_at;
