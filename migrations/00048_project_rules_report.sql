-- Report panel (ADR-0066): which fields are a project's monthly plan, its
-- remaining amount and its breakdown (e.g. payment method). NULL = no panel.
--
-- Locking: ADD COLUMN without a default is metadata-only: ACCESS EXCLUSIVE
-- on project_rules for milliseconds, no rewrite.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE project_rules ADD COLUMN report jsonb
    CONSTRAINT project_rules_report_check CHECK (report IS NULL OR jsonb_typeof(report) = 'object');

-- +goose Down
-- Only the panel setting is lost. The previous image does not read the column.
SET LOCAL lock_timeout = '5s';
ALTER TABLE project_rules DROP COLUMN IF EXISTS report;
