-- Board columns page through one status (or a few) newest-first:
--   WHERE project_id = $1 AND status_id = ANY($2) ORDER BY number DESC LIMIT n
-- This index serves that directly and also covers the old (project_id,
-- status_id) one, which is dropped. Built CONCURRENTLY so a large items table
-- is not locked against writes while it is created (ADR-0008).

-- +goose NO TRANSACTION

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS items_project_status_number_idx
    ON items (project_id, status_id, number DESC) WHERE deleted_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS items_project_status_idx;

-- +goose Down
CREATE INDEX CONCURRENTLY IF NOT EXISTS items_project_status_idx
    ON items (project_id, status_id) WHERE deleted_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS items_project_status_number_idx;
