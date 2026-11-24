-- Project default view: the shared saved view a project opens with when the
-- link carries no list state (ADR-0014 addendum). Workspace templates
-- (ADR-0033) set it, so a finance project opens with its amount columns.
-- Only a shared view of the same project qualifies (checked in the store);
-- deleting the view clears the default.
--
-- Locking: ADD COLUMN NULL is metadata-only. The composite foreign key takes
-- SHARE ROW EXCLUSIVE on projects and saved_views while it checks existing
-- rows, which are all NULL here (no lookups), for milliseconds.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE projects ADD COLUMN default_view_id uuid NULL;
ALTER TABLE projects ADD CONSTRAINT projects_default_view_fkey
    FOREIGN KEY (workspace_id, default_view_id) REFERENCES saved_views (workspace_id, id)
    ON DELETE SET NULL (default_view_id);

-- +goose Down
-- Only the default choice is lost; projects open with "All items" again.
-- The previous image does not read the column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE projects DROP COLUMN IF EXISTS default_view_id;
