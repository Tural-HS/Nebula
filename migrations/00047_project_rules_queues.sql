-- Quick-filter counters per project (ADR-0065): which counters a project's
-- list shows, in order; empty = the default set. FinTez's monthly report
-- shows All, To do, Overdue, Done and Due this month.
--
-- Locking: ADD COLUMN with a constant default is metadata-only: ACCESS
-- EXCLUSIVE on project_rules for milliseconds, no rewrite.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE project_rules ADD COLUMN queues text[] NOT NULL DEFAULT '{}'
    CONSTRAINT project_rules_queues_check CHECK (cardinality(queues) <= 12);

-- +goose Down
-- Only the counter choice is lost; the list shows the default set again.
SET LOCAL lock_timeout = '5s';
ALTER TABLE project_rules DROP COLUMN IF EXISTS queues;
