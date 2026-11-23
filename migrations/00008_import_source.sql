-- Events written by a spreadsheet import are marked source = 'import'
-- (ADR-0011): they stay in the activity and audit history, but they are not
-- dispatched, so a large import does not fire one webhook message or one
-- automation per row. The single project.imported event is dispatched instead.

-- +goose Up
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_source_check;
ALTER TABLE events ADD CONSTRAINT events_source_check
    CHECK (source IN ('user', 'automation', 'system', 'import'));

-- +goose Down
UPDATE events SET source = 'user' WHERE source = 'import';
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_source_check;
ALTER TABLE events ADD CONSTRAINT events_source_check
    CHECK (source IN ('user', 'automation', 'system'));
