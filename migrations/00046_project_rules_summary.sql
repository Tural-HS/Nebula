-- Summary bar (ADR-0064): a project lists totals shown above its table, e.g.
-- FinTez "Aylıq ödəniş cəmi", "Yığılası qalıq" and "Ödənilib" (one sum minus
-- another). Validated by the application; at most six entries.
--
-- Locking: ADD COLUMN with a constant default is metadata-only: ACCESS
-- EXCLUSIVE on project_rules for milliseconds, no rewrite.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE project_rules ADD COLUMN summary jsonb NOT NULL DEFAULT '[]'
    CONSTRAINT project_rules_summary_check CHECK (jsonb_typeof(summary) = 'array' AND jsonb_array_length(summary) <= 6);

-- +goose Down
-- Only the summary bar is lost. The previous image does not read the column.
SET LOCAL lock_timeout = '5s';
ALTER TABLE project_rules DROP COLUMN IF EXISTS summary;
