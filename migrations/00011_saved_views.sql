-- Saved views per project and the due-date sort (ADR-0014).
-- Runs without a transaction so the sort index can be built CONCURRENTLY on
-- a large items table; every statement is idempotent, so a migration that
-- stopped half-way can simply be run again.

-- +goose NO TRANSACTION

-- +goose Up
CREATE TABLE IF NOT EXISTS saved_views (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   uuid NOT NULL,
    owner_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    kind         text NOT NULL CHECK (kind IN ('table', 'board')),
    shared       boolean NOT NULL DEFAULT false,
    -- Filters, columns, sort and grouping; validated by the API (store.ViewConfig).
    config       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(config) = 'object' AND pg_column_size(config) <= 16384),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS saved_views_project_idx ON saved_views (workspace_id, project_id, owner_id);
DROP TRIGGER IF EXISTS saved_views_updated_at ON saved_views;
CREATE TRIGGER saved_views_updated_at BEFORE UPDATE ON saved_views FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ?sort=due: ORDER BY due_date ASC NULLS LAST, number ASC within a project.
CREATE INDEX CONCURRENTLY IF NOT EXISTS items_project_due_number_idx
    ON items (project_id, due_date, number) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS items_project_due_number_idx;
DROP TABLE IF EXISTS saved_views;
