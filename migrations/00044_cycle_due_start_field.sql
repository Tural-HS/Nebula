-- First due date after a start date (ADR-0060): a cycle can name a date
-- field (e.g. the loan date) after whose month the first due date falls, and
-- whose day is used when the due-day field is empty. A loan given on 5
-- January is first due on 5 February.
--
-- Locking: ADD COLUMN without a default is metadata-only: ACCESS EXCLUSIVE on
-- period_cycles for milliseconds, no rewrite; the CHECK scans a few rows.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE period_cycles ADD COLUMN due_start_field text
    CONSTRAINT period_cycles_due_start_field_check CHECK (due_start_field ~ '^[a-z][a-z0-9_]{0,62}$');

-- +goose Down
-- Only the setting is lost; due dates written already stay. The previous
-- image does not read the column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE period_cycles DROP COLUMN IF EXISTS due_start_field;
