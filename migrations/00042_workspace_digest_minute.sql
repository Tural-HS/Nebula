-- Morning digest at a minute, not only an hour (ADR-0059): FinTez's old
-- channel bot posted at 09:30. The digest, and the yearly date reminders
-- that share its time, run once the local time reaches hour:minute.
--
-- Locking: ADD COLUMN with a constant default is metadata-only in
-- PostgreSQL 11+: ACCESS EXCLUSIVE on workspaces for milliseconds, no
-- rewrite; the CHECK scans the small table once.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE workspaces ADD COLUMN overdue_digest_minute smallint NOT NULL DEFAULT 0
    CONSTRAINT workspaces_overdue_digest_minute_check CHECK (overdue_digest_minute BETWEEN 0 AND 59);

-- +goose Down
-- Only the minute is lost; digests go back to the full hour. The previous
-- image does not read the column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE workspaces DROP COLUMN IF EXISTS overdue_digest_minute;
