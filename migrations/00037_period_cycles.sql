-- Monthly period close (ADR-0051): on a chosen day of the month a project
-- saves the values of its items for the month that ended, then clears the
-- monthly marks (e.g. "Paid") and refills fields from others (e.g. balance
-- := monthly payment) on its open items. One close per project and month,
-- however often the scheduler runs. Tenant-safe like every other table.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE period_cycles (
    project_id   uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    is_enabled   boolean NOT NULL DEFAULT false,
    day_of_month smallint NOT NULL DEFAULT 1 CHECK (day_of_month BETWEEN 1 AND 28),
    clear_fields text[] NOT NULL DEFAULT '{}' CHECK (cardinality(clear_fields) <= 20),
    refill       jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(refill) = 'array' AND jsonb_array_length(refill) <= 20),
    -- Reset closed items too (when the status tracks the month, e.g. "Done" = paid).
    include_closed  boolean NOT NULL DEFAULT false,
    -- Status the reset items get; NULL keeps theirs. Checked when saved and
    -- per item when applied (it must be in the item's workflow).
    reset_status_id uuid,
    next_run     date, -- next scheduled close; NULL while disabled
    last_error   text,
    updated_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, project_id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX period_cycles_due_idx ON period_cycles (next_run) WHERE is_enabled AND next_run IS NOT NULL;
CREATE TRIGGER period_cycles_updated_at BEFORE UPDATE ON period_cycles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE period_closes (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    project_id   uuid NOT NULL,
    period       date NOT NULL CHECK (extract(day FROM period) = 1), -- first day of the closed month
    closed_by    uuid REFERENCES users (id) ON DELETE SET NULL, -- NULL: the scheduler
    item_count   integer NOT NULL CHECK (item_count >= 0),
    changed      integer NOT NULL CHECK (changed >= 0),
    failed       jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(failed) = 'array'),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, period),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);

-- The month's values of every item, as they were just before the close.
CREATE TABLE period_snapshots (
    workspace_id uuid NOT NULL,
    close_id     uuid NOT NULL,
    item_id      uuid NOT NULL,
    number       bigint NOT NULL,
    title        text NOT NULL,
    status_id    uuid NOT NULL,
    assignee_id  uuid,
    fields       jsonb NOT NULL CHECK (jsonb_typeof(fields) = 'object'),
    PRIMARY KEY (close_id, item_id),
    FOREIGN KEY (workspace_id, close_id) REFERENCES period_closes (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX period_snapshots_item_idx ON period_snapshots (item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS period_snapshots;
DROP TABLE IF EXISTS period_closes;
DROP TABLE IF EXISTS period_cycles;
-- +goose StatementEnd
