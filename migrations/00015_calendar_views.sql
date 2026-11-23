-- Saved views of kind "calendar" (ADR-0023). Only the CHECK changes, no data.
-- goose runs this in one transaction, so the ACCESS EXCLUSIVE lock from ADD
-- CONSTRAINT is held until commit; NOT VALID + VALIDATE does not shorten it
-- here. saved_views is small (a few views per project), so the lock lasts
-- milliseconds.

-- +goose Up
ALTER TABLE saved_views DROP CONSTRAINT IF EXISTS saved_views_kind_check;
ALTER TABLE saved_views ADD CONSTRAINT saved_views_kind_check CHECK (kind IN ('table', 'board', 'calendar')) NOT VALID;
ALTER TABLE saved_views VALIDATE CONSTRAINT saved_views_kind_check;

-- +goose Down
-- Refuses instead of silently turning calendar views into something else:
-- delete or change them first (ADR-0023).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM saved_views WHERE kind = 'calendar') THEN
        RAISE EXCEPTION 'saved views of kind calendar exist; change them to table or board (or delete them) before migrating down';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE saved_views DROP CONSTRAINT IF EXISTS saved_views_kind_check;
ALTER TABLE saved_views ADD CONSTRAINT saved_views_kind_check CHECK (kind IN ('table', 'board'));
