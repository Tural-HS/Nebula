-- Item links and watchers (ADR-0028).
--
-- item_links: a typed, directed link between two items of the same workspace
-- (any projects). Fixed types; each is stored once, from its "active" side:
--   blocks      source blocks target        (target: "is blocked by" source)
--   duplicates  source duplicates target    (target: "is duplicated by" source)
--   relates     symmetric, stored as given
-- One link per unordered pair and type: A blocks B and B blocks A cannot both
-- exist, nor two "relates" rows for the same pair. Items are soft-deleted; the
-- API removes the links of a deleted item itself, the FK cascade covers hard
-- deletes (project or workspace removal).
--
-- item_watchers: who follows an item. Rows belong to a workspace membership
-- (leaving the workspace removes them). Access to private projects is checked
-- whenever watchers are listed or notified, so losing access silences a
-- watcher without deleting the row.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE item_links (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    source_id    uuid NOT NULL,
    target_id    uuid NOT NULL,
    type         text NOT NULL CHECK (type IN ('blocks', 'relates', 'duplicates')),
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT item_links_not_self CHECK (source_id <> target_id),
    FOREIGN KEY (workspace_id, source_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, target_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE
);
-- No duplicates in either direction.
CREATE UNIQUE INDEX item_links_pair_key ON item_links
    (type, LEAST(source_id, target_id), GREATEST(source_id, target_id));
-- Links of an item, from either side (drawer list, per-item cap, delete cleanup,
-- "blocked" lookups by target).
CREATE INDEX item_links_source_idx ON item_links (source_id, type);
CREATE INDEX item_links_target_idx ON item_links (target_id, type);
-- FK lookups on workspace-scoped cascades.
CREATE INDEX item_links_ws_source_idx ON item_links (workspace_id, source_id);
CREATE INDEX item_links_ws_target_idx ON item_links (workspace_id, target_id);
CREATE INDEX item_links_created_by_idx ON item_links (created_by) WHERE created_by IS NOT NULL;

CREATE TABLE item_watchers (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    item_id      uuid NOT NULL,
    user_id      uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, user_id),
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, user_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE CASCADE
);
CREATE INDEX item_watchers_ws_item_idx ON item_watchers (workspace_id, item_id);
CREATE INDEX item_watchers_member_idx  ON item_watchers (workspace_id, user_id);

-- Watcher notification kinds (ADR-0028) on top of ADR-0017's and the
-- approval kinds of 00019 (ADR-0026). Same short lock as in 00019.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added')) NOT VALID;
ALTER TABLE notifications VALIDATE CONSTRAINT notifications_kind_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM notifications WHERE kind IN ('status_changed', 'assignee_changed', 'due_changed', 'link_added');
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
-- Back to the list as of 00020 (ADR-0017 kinds plus the approval kinds of 00019).
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled'));
DROP TABLE IF EXISTS item_watchers;
DROP TABLE IF EXISTS item_links;
-- +goose StatementEnd
