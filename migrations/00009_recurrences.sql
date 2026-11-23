-- Recurring items (ADR-0012): a rule per project that creates the next item
-- lead_days before its due date. Tenant-safe like every other table.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE recurrences (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   uuid NOT NULL,
    title        text NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    description  text CHECK (length(description) <= 100000),
    item_type_id uuid NOT NULL,
    assignee_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    fields       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(fields) = 'object'),
    freq         text NOT NULL CHECK (freq IN ('daily', 'weekly', 'monthly', 'yearly')),
    every        smallint NOT NULL CHECK (every BETWEEN 1 AND 366),
    weekdays     smallint[] NOT NULL DEFAULT '{}' CHECK (weekdays <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::smallint[]),
    month_day    smallint CHECK (month_day = -1 OR month_day BETWEEN 1 AND 31),
    month        smallint CHECK (month BETWEEN 1 AND 12),
    start_date   date NOT NULL,
    end_date     date CHECK (end_date IS NULL OR end_date >= start_date),
    lead_days    smallint NOT NULL DEFAULT 3 CHECK (lead_days BETWEEN 0 AND 365),
    next_due     date, -- next occurrence without an item; NULL when the schedule has ended
    is_enabled   boolean NOT NULL DEFAULT true,
    last_error   text,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_type_id) REFERENCES item_types (workspace_id, id)
);
CREATE INDEX recurrences_due_idx ON recurrences (next_due) WHERE is_enabled AND next_due IS NOT NULL;
CREATE INDEX recurrences_project_idx ON recurrences (workspace_id, project_id);
CREATE TRIGGER recurrences_updated_at BEFORE UPDATE ON recurrences FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One item per rule and date, however often the scheduler runs.
CREATE TABLE recurrence_runs (
    workspace_id  uuid NOT NULL,
    recurrence_id uuid NOT NULL,
    due_date      date NOT NULL,
    item_id       uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (recurrence_id, due_date),
    FOREIGN KEY (workspace_id, recurrence_id) REFERENCES recurrences (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX recurrence_runs_item_idx ON recurrence_runs (item_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS recurrence_runs;
DROP TABLE IF EXISTS recurrences;
-- +goose StatementEnd
