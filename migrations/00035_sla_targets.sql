-- SLA targets (ADR-0045): a per-project policy with an optional first
-- response and resolution target in working days of the workspace
-- calendar (ADR-0027), and one row per item with the computed due
-- timestamps, so filters and the notification scans are index lookups.
--
-- item_sla is filled by the worker (and synchronously on item creation and
-- policy changes); the triggers below stamp responded_at / resolved_at on
-- every status change and comment, whatever code path made it (UI, bulk,
-- automation, approvals, API tokens).
--
-- Locking: two new tables (no lock on existing data). CREATE TRIGGER takes
-- SHARE ROW EXCLUSIVE on items and comments for milliseconds (no rewrite,
-- no scan): writes wait at most lock_timeout, reads are not blocked.
-- The notification kind CHECK gains sla_at_risk and sla_breached: DROP/ADD
-- takes ACCESS EXCLUSIVE on notifications and re-checks its rows (bounded
-- by the 90/180-day retention of ADR-0017): seconds at most on a large
-- install; run it outside peak hours.

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS project_sla_policies (
    project_id      uuid PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    response_days   smallint CHECK (response_days BETWEEN 1 AND 60),
    resolution_days smallint CHECK (resolution_days BETWEEN 1 AND 120),
    -- The clock of an item starts at max(created_at, since): enabling a
    -- policy never makes the backlog breached at once.
    since           timestamptz NOT NULL DEFAULT now(),
    -- Bumped by policy and calendar changes; item_sla rows computed before
    -- it are recomputed by the worker.
    changed_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (response_days IS NOT NULL OR resolution_days IS NOT NULL),
    CHECK (response_days IS NULL OR resolution_days IS NULL OR response_days <= resolution_days)
);
CREATE INDEX IF NOT EXISTS project_sla_policies_ws_idx ON project_sla_policies (workspace_id);

CREATE TABLE IF NOT EXISTS item_sla (
    item_id         uuid PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    workspace_id    uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    started_at      timestamptz,
    response_due    timestamptz,
    resolution_due  timestamptz,
    responded_at    timestamptz,
    resolved_at     timestamptz,
    computed_at     timestamptz,
    -- Notification markers (deduplication): the due timestamp a reminder
    -- was sent for. A recomputed due gets its own reminder.
    response_risk_for    timestamptz,
    response_breach_for  timestamptz,
    resolution_risk_for  timestamptz,
    resolution_breach_for timestamptz
);
CREATE INDEX IF NOT EXISTS item_sla_open_response_idx ON item_sla (response_due)
    WHERE responded_at IS NULL AND resolved_at IS NULL AND response_due IS NOT NULL;
CREATE INDEX IF NOT EXISTS item_sla_open_resolution_idx ON item_sla (resolution_due)
    WHERE resolved_at IS NULL AND resolution_due IS NOT NULL;
CREATE INDEX IF NOT EXISTS item_sla_ws_idx ON item_sla (workspace_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION item_sla_on_status() RETURNS trigger AS $$
DECLARE
    cat text;
BEGIN
    IF NEW.status_id IS NOT DISTINCT FROM OLD.status_id THEN
        RETURN NULL;
    END IF;
    SELECT category INTO cat FROM statuses WHERE id = NEW.status_id;
    -- First response: the first move out of the initial (todo) category.
    -- Resolution: a done or canceled category; reopening clears it.
    UPDATE item_sla SET
        responded_at = CASE WHEN cat <> 'todo' THEN COALESCE(responded_at, now()) ELSE responded_at END,
        resolved_at  = CASE WHEN cat IN ('done', 'canceled') THEN COALESCE(resolved_at, now()) ELSE NULL END
    WHERE item_id = NEW.id;
    RETURN NULL;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION item_sla_on_comment() RETURNS trigger AS $$
BEGIN
    -- First response: the first comment by someone other than the requester.
    UPDATE item_sla q SET responded_at = now()
    FROM items i
    WHERE q.item_id = NEW.item_id AND i.id = NEW.item_id AND q.responded_at IS NULL
      AND NEW.author_id IS NOT NULL AND NEW.author_id IS DISTINCT FROM i.reporter_id;
    RETURN NULL;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS items_sla_status ON items;
CREATE TRIGGER items_sla_status AFTER UPDATE OF status_id ON items
    FOR EACH ROW EXECUTE FUNCTION item_sla_on_status();
DROP TRIGGER IF EXISTS comments_sla_response ON comments;
CREATE TRIGGER comments_sla_response AFTER INSERT ON comments
    FOR EACH ROW EXECUTE FUNCTION item_sla_on_comment();

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added',
    'approval_overdue', 'approval_invalidated', 'sla_at_risk', 'sla_breached'));

-- +goose Down
SET LOCAL lock_timeout = '5s';
-- SLA policies and computed dues are dropped (they are recomputable
-- configuration; past notifications and events stay). Write down the
-- policies first if they should be re-entered after a roll-forward.
DELETE FROM notifications WHERE kind IN ('sla_at_risk', 'sla_breached');
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
-- Back to the list as of 00027.
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added',
    'approval_overdue', 'approval_invalidated'));
DROP TRIGGER IF EXISTS comments_sla_response ON comments;
DROP TRIGGER IF EXISTS items_sla_status ON items;
DROP FUNCTION IF EXISTS item_sla_on_comment();
DROP FUNCTION IF EXISTS item_sla_on_status();
DROP TABLE IF EXISTS item_sla;
DROP TABLE IF EXISTS project_sla_policies;
