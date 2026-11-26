-- Payment due dates driven by the monthly cycle (ADR-0055): a cycle may name
-- a date field (e.g. the loan date) whose day of the month becomes every
-- item's due date each month, and a boolean "paid" field. On close, items
-- that are late and unpaid are carried (not reset); the scheduler rolls them
-- once they are paid. period_closes.carried counts the carried items.
--
-- Locking: ADD COLUMN without a default (NULL) or with a constant default is
-- metadata-only in PostgreSQL 11+: ACCESS EXCLUSIVE on period_cycles and
-- period_closes for milliseconds, no rewrite. The new CHECKs scan each table
-- once under that lock; both are small (one row per project / per project
-- and month).

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE period_cycles
    ADD COLUMN due_day_field text NULL
        CONSTRAINT period_cycles_due_day_field_check CHECK (due_day_field ~ '^[a-z][a-z0-9_]{0,62}$'),
    ADD COLUMN paid_field text NULL
        CONSTRAINT period_cycles_paid_field_check CHECK (paid_field ~ '^[a-z][a-z0-9_]{0,62}$'),
    -- A paid field only makes sense with due dates.
    ADD CONSTRAINT period_cycles_paid_needs_due_check CHECK (paid_field IS NULL OR due_day_field IS NOT NULL);
ALTER TABLE period_closes ADD COLUMN carried int NOT NULL DEFAULT 0
    CONSTRAINT period_closes_carried_check CHECK (carried >= 0);

-- +goose Down
-- Only the due-date settings and the carried counts are lost; due dates
-- already set on items stay. The previous image reads neither column, so it
-- runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE period_closes DROP COLUMN IF EXISTS carried;
ALTER TABLE period_cycles DROP COLUMN IF EXISTS paid_field;
ALTER TABLE period_cycles DROP COLUMN IF EXISTS due_day_field;
