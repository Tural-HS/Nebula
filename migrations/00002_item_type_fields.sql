-- +goose Up
-- +goose StatementBegin

-- Fields are defined once per workspace and attached to item types, so "severity"
-- can be required on bugs while tasks never see it (ADR-0003).

-- Needed for the tenant-safe composite foreign key below.
ALTER TABLE field_definitions
    ADD CONSTRAINT field_definitions_workspace_id_id_key UNIQUE (workspace_id, id);

-- Required-ness belongs to the binding, not the definition. The column was never
-- written by the application, so no data is lost.
ALTER TABLE field_definitions DROP COLUMN is_required;

CREATE TABLE item_type_fields (
    workspace_id uuid    NOT NULL,
    item_type_id uuid    NOT NULL,
    field_id     uuid    NOT NULL,
    is_required  boolean NOT NULL DEFAULT false,
    position     integer NOT NULL DEFAULT 0 CHECK (position BETWEEN 0 AND 10000),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_type_id, field_id),
    FOREIGN KEY (workspace_id, item_type_id) REFERENCES item_types        (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, field_id)     REFERENCES field_definitions (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX item_type_fields_field_idx ON item_type_fields (field_id);

-- Item values are stored under the field key in the field type's canonical format,
-- so changing either would silently orphan or corrupt existing data.
CREATE FUNCTION field_definitions_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.key IS DISTINCT FROM OLD.key OR NEW.field_type IS DISTINCT FROM OLD.field_type THEN
        RAISE EXCEPTION 'field_definitions.key and field_type are immutable'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER field_definitions_immutable BEFORE UPDATE ON field_definitions
    FOR EACH ROW EXECUTE FUNCTION field_definitions_immutable();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS field_definitions_immutable ON field_definitions;
DROP FUNCTION IF EXISTS field_definitions_immutable();
DROP TABLE IF EXISTS item_type_fields;
ALTER TABLE field_definitions ADD COLUMN is_required boolean NOT NULL DEFAULT false;
ALTER TABLE field_definitions DROP CONSTRAINT IF EXISTS field_definitions_workspace_id_id_key;
-- +goose StatementEnd
