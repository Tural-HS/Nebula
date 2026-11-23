-- In-app notifications (ADR-0017). One row per (event, recipient), written by
-- the notifications.create job from the event log; the unique key makes a
-- retried job a no-op. Rows belong to a workspace membership: leaving the
-- workspace deletes them. Access to private projects is checked again on read.

-- +goose Up
CREATE TABLE notifications (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    recipient_id uuid NOT NULL,
    event_id     bigint NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('mention', 'assigned', 'overdue', 'comment')),
    project_id   uuid NOT NULL,
    item_id      uuid NOT NULL,
    actor_id     uuid REFERENCES users (id) ON DELETE SET NULL,
    read_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, recipient_id),
    FOREIGN KEY (workspace_id, recipient_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, project_id)   REFERENCES projects (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_id)      REFERENCES items (workspace_id, id) ON DELETE CASCADE
);
-- "My notifications, newest first" (keyset on id) and the unread badge.
CREATE INDEX notifications_recipient_idx ON notifications (recipient_id, id DESC);
CREATE INDEX notifications_unread_idx    ON notifications (recipient_id, id DESC) WHERE read_at IS NULL;
-- Retention: read rows after 90 days, every row after 180 days.
CREATE INDEX notifications_created_idx   ON notifications (created_at);
CREATE INDEX notifications_read_created_idx ON notifications (created_at) WHERE read_at IS NOT NULL;
-- FK lookups on cascading deletes of memberships, items and projects.
CREATE INDEX notifications_member_idx    ON notifications (workspace_id, recipient_id);
CREATE INDEX notifications_item_idx      ON notifications (workspace_id, item_id);
CREATE INDEX notifications_project_idx   ON notifications (workspace_id, project_id);
CREATE INDEX notifications_actor_idx     ON notifications (actor_id) WHERE actor_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS notifications;
