-- Formula fields (ADR-0030): a custom field of type 'formula' computes its
-- value from other fields of the same item. Values are computed on write and
-- stored in items.fields like any other value, so totals, sorting and
-- filtering work in SQL unchanged.
--
-- formula_recompute: one row per workspace whose stored formula values must
-- be recomputed (a formula was created or changed, the calendar or the time
-- zone changed, or a new day began for formulas that use today()). The
-- worker walks the workspace's items in id order, a batch per transaction;
-- after_id is the position of the running pass.
--
-- items_updated_at: a recompute only changes computed values. The worker
-- sets nebula.computed_only for its own transaction so items.updated_at keeps
-- meaning "a person or a rule last changed this item".
--
-- No table is rewritten: field_definitions is small (<= 200 active rows per
-- workspace) and the trigger swap takes a millisecond lock on items.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE field_definitions DROP CONSTRAINT IF EXISTS field_definitions_field_type_check;
ALTER TABLE field_definitions ADD CONSTRAINT field_definitions_field_type_check CHECK (field_type IN (
    'text', 'long_text', 'number', 'date', 'datetime', 'boolean',
    'select', 'multi_select', 'user', 'url', 'email', 'formula'));

CREATE TABLE formula_recompute (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    pending      boolean NOT NULL DEFAULT true,
    after_id     uuid,        -- last item of the running pass; NULL = from the start
    volatile_day date,        -- workspace-local date of the last daily refresh
    requested_at timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz
);
CREATE INDEX formula_recompute_pending_idx ON formula_recompute (requested_at) WHERE pending;

-- +goose StatementBegin
CREATE FUNCTION items_set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('nebula.computed_only', true) IS DISTINCT FROM 'on' THEN
        NEW.updated_at := now();
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS items_updated_at ON items;
CREATE TRIGGER items_updated_at BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION items_set_updated_at();

-- +goose Down
SET LOCAL lock_timeout = '5s';
-- The previous release does not know the type and would reject its rows.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM field_definitions WHERE field_type = 'formula') THEN
        RAISE EXCEPTION 'formula fields exist (ADR-0030); remove them before migrating down (see deploy/production/README.md, rollback table, 00023)';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS items_updated_at ON items;
CREATE TRIGGER items_updated_at BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION set_updated_at();
DROP FUNCTION IF EXISTS items_set_updated_at();
DROP TABLE IF EXISTS formula_recompute;
ALTER TABLE field_definitions DROP CONSTRAINT IF EXISTS field_definitions_field_type_check;
ALTER TABLE field_definitions ADD CONSTRAINT field_definitions_field_type_check CHECK (field_type IN (
    'text', 'long_text', 'number', 'date', 'datetime', 'boolean',
    'select', 'multi_select', 'user', 'url', 'email'));
