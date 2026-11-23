-- Webhooks can be limited to projects (ADR-0013). An empty list means every
-- workspace-visible project; private projects are delivered only to webhooks
-- that list them explicitly, so a channel never receives a private project's
-- events by default.

-- +goose Up
ALTER TABLE webhooks ADD COLUMN project_ids uuid[] NOT NULL DEFAULT '{}'
    CHECK (cardinality(project_ids) <= 50);

-- +goose Down
ALTER TABLE webhooks DROP COLUMN IF EXISTS project_ids;
