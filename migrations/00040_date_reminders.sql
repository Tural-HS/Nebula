-- Yearly date reminders (ADR-0054): a project names one date field (e.g. a
-- customer's birthday); every day, at the workspace's digest hour (09:00 when
-- the digest is off), each item whose date falls on today's month and day
-- becomes an in-app notification ("anniversary") for every member who can see
-- the project. last_sent (the workspace-local date of the last run) makes it
-- once a day however many replicas tick. Tenant-safe like every other table.
--
-- Locking: a new table, and the notification kind CHECK is swapped like in
-- 00035: ACCESS EXCLUSIVE on notifications for the validation scan, inside
-- the migration's transaction (lock_timeout 5s). The table holds at most 180
-- days of notifications (housekeeping), so the scan is short.

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE date_reminders (
    project_id   uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    field_key    text NOT NULL CHECK (field_key ~ '^[a-z][a-z0-9_]{0,62}$'),
    last_sent    date,
    updated_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE
);
CREATE TRIGGER date_reminders_updated_at BEFORE UPDATE ON date_reminders FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added',
    'approval_overdue', 'approval_invalidated', 'sla_at_risk', 'sla_breached',
    'anniversary'));

-- +goose Down
-- The choice of field and the reminder notifications are lost; everything
-- else stays. The previous image neither reads the table nor sends the kind.
SET LOCAL lock_timeout = '5s';
DELETE FROM notifications WHERE kind = 'anniversary';
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added',
    'approval_overdue', 'approval_invalidated', 'sla_at_risk', 'sla_breached'));
DROP TABLE IF EXISTS date_reminders;
