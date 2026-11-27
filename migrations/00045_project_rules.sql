-- Project item rules (ADR-0061). complete_fields: boolean fields that must
-- all be ticked before people move an item to a done status, and that keep
-- it there while all are ticked (FinTez: paid, portal, power of attorney).
-- late_fields: fields whose table cells take the overdue band colour, beside
-- the title and the due date. System changes (monthly close, automations)
-- are not bound by complete_fields. Tenant-safe like every other table.
--
-- Locking: a new table only.

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE project_rules (
    project_id      uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    complete_fields text[] NOT NULL DEFAULT '{}'
                        CONSTRAINT project_rules_complete_fields_check CHECK (cardinality(complete_fields) <= 5),
    late_fields     text[] NOT NULL DEFAULT '{}'
                        CONSTRAINT project_rules_late_fields_check CHECK (cardinality(late_fields) <= 10),
    updated_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);
CREATE TRIGGER project_rules_updated_at BEFORE UPDATE ON project_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
-- Only the settings are lost. The previous image does not read the table.
SET LOCAL lock_timeout = '5s';
DROP TABLE IF EXISTS project_rules;
