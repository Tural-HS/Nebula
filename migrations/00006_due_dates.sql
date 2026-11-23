-- Due dates, workspace time zones and time-based automation (ADR-0009).
-- Runs without a transaction so the index can be built CONCURRENTLY on a
-- large items table; every statement is idempotent, so a migration that
-- stopped half-way can simply be run again.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE workspaces
    ADD COLUMN IF NOT EXISTS timezone text NOT NULL DEFAULT 'UTC'
        CONSTRAINT workspaces_timezone_check CHECK (length(timezone) BETWEEN 1 AND 64),
    ADD COLUMN IF NOT EXISTS overdue_digest_hour smallint
        CONSTRAINT workspaces_overdue_digest_hour_check CHECK (overdue_digest_hour BETWEEN 0 AND 23);

ALTER TABLE items ADD COLUMN IF NOT EXISTS due_date date;

CREATE INDEX CONCURRENTLY IF NOT EXISTS items_due_date_idx
    ON items (due_date) WHERE deleted_at IS NULL AND due_date IS NOT NULL;

-- One item.overdue event per item and due date, however often the scan runs.
CREATE TABLE IF NOT EXISTS item_overdue_marks (
    workspace_id uuid NOT NULL,
    item_id      uuid NOT NULL,
    due_date     date NOT NULL,
    marked_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, due_date),
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE
);

-- One overdue digest per workspace and local day.
CREATE TABLE IF NOT EXISTS workspace_digest_runs (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    local_date   date NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, local_date)
);

ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_trigger_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_trigger_check
    CHECK (trigger IN ('item.created', 'item.updated', 'item.overdue'));

-- +goose Down
DELETE FROM automation_rules WHERE trigger = 'item.overdue';
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_trigger_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_trigger_check
    CHECK (trigger IN ('item.created', 'item.updated'));
DROP TABLE IF EXISTS workspace_digest_runs;
DROP TABLE IF EXISTS item_overdue_marks;
DROP INDEX CONCURRENTLY IF EXISTS items_due_date_idx;
ALTER TABLE items DROP COLUMN IF EXISTS due_date;
ALTER TABLE workspaces DROP COLUMN IF EXISTS overdue_digest_hour, DROP COLUMN IF EXISTS timezone;
