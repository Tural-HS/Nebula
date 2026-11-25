-- Money fields (ADR-0044): a custom field of type 'money' stores
-- {"amount": "<exact decimal text>", "currency": "<ISO 4217>"} in
-- items.fields, like every other value. Its options (allowed currencies,
-- default currency, scale) live in field_definitions.options. The only
-- schema change is the field type CHECK constraint.
--
-- Locking: DROP/ADD CONSTRAINT takes ACCESS EXCLUSIVE on field_definitions
-- and re-checks its rows (<= 200 active definitions per workspace, a small
-- table): milliseconds. items is not touched.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE field_definitions DROP CONSTRAINT IF EXISTS field_definitions_field_type_check;
ALTER TABLE field_definitions ADD CONSTRAINT field_definitions_field_type_check CHECK (field_type IN (
    'text', 'long_text', 'number', 'date', 'datetime', 'boolean',
    'select', 'multi_select', 'user', 'url', 'email', 'formula', 'money'));

-- +goose Down
SET LOCAL lock_timeout = '5s';
-- The previous release does not know the type and would fail on its rows.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM field_definitions WHERE field_type = 'money') THEN
        RAISE EXCEPTION 'money fields exist (ADR-0044); remove them before migrating down (see deploy/production/README.md, rollback table, 00034)';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE field_definitions DROP CONSTRAINT IF EXISTS field_definitions_field_type_check;
ALTER TABLE field_definitions ADD CONSTRAINT field_definitions_field_type_check CHECK (field_type IN (
    'text', 'long_text', 'number', 'date', 'datetime', 'boolean',
    'select', 'multi_select', 'user', 'url', 'email', 'formula'));
