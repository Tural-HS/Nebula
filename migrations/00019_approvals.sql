-- Four-eyes approvals on status changes (ADR-0026).
--
-- An approval policy guards *entering* one status of a workflow: moving an
-- item there creates an approval request instead of changing the status.
-- The request snapshots the policy (approvers, min_approvals), so editing or
-- deleting the policy never changes the rules for requests already open.
-- At most one request per item is pending at a time (partial unique index).
-- Decisions are one row per approver; the request row is the lock that
-- serializes concurrent approvals, so the transition is applied exactly once.

-- +goose Up
CREATE TABLE approval_policies (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    workflow_id       uuid NOT NULL REFERENCES workflows (id) ON DELETE CASCADE,
    status_id         uuid NOT NULL REFERENCES statuses (id) ON DELETE CASCADE,
    min_approvals     int  NOT NULL CHECK (min_approvals BETWEEN 1 AND 5),
    -- Workspace roles whose members may approve (guests never approve).
    approver_roles    text[] NOT NULL DEFAULT '{}'
        CHECK (approver_roles <@ ARRAY['member', 'admin', 'owner']::text[]),
    -- Specific users who may approve; membership and project access are
    -- checked at decision time, so a user who left is simply not eligible.
    approver_user_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(approver_user_ids) <= 50),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(approver_roles) > 0 OR cardinality(approver_user_ids) > 0),
    UNIQUE (status_id)
);
CREATE INDEX approval_policies_workflow_idx ON approval_policies (workflow_id);
CREATE INDEX approval_policies_workspace_idx ON approval_policies (workspace_id);

CREATE TABLE approval_requests (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    item_id           uuid NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    policy_id         uuid REFERENCES approval_policies (id) ON DELETE SET NULL,
    -- SET NULL keeps the history of closed requests when a status is deleted;
    -- the store refuses to delete a status a pending request targets.
    from_status_id    uuid REFERENCES statuses (id) ON DELETE SET NULL,
    to_status_id      uuid REFERENCES statuses (id) ON DELETE SET NULL,
    requested_by      uuid NOT NULL REFERENCES users (id),
    comment           text CHECK (length(comment) <= 2000),
    -- Policy snapshot at request time.
    min_approvals     int    NOT NULL CHECK (min_approvals BETWEEN 1 AND 5),
    approver_roles    text[] NOT NULL,
    approver_user_ids uuid[] NOT NULL,
    approvals         int    NOT NULL DEFAULT 0 CHECK (approvals >= 0),
    state             text   NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'approved', 'rejected', 'canceled')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    closed_at         timestamptz,
    closed_by         uuid REFERENCES users (id),
    CHECK ((state = 'pending') = (closed_at IS NULL))
);
CREATE UNIQUE INDEX approval_requests_one_pending_idx ON approval_requests (item_id) WHERE state = 'pending';
CREATE INDEX approval_requests_item_idx ON approval_requests (item_id, created_at DESC);
CREATE INDEX approval_requests_to_status_idx ON approval_requests (to_status_id) WHERE state = 'pending';
CREATE INDEX approval_requests_workspace_idx ON approval_requests (workspace_id);
CREATE INDEX approval_requests_policy_idx ON approval_requests (policy_id) WHERE policy_id IS NOT NULL;
CREATE INDEX approval_requests_from_status_idx ON approval_requests (from_status_id) WHERE from_status_id IS NOT NULL;
CREATE INDEX approval_requests_requested_by_idx ON approval_requests (requested_by);
CREATE INDEX approval_requests_closed_by_idx ON approval_requests (closed_by) WHERE closed_by IS NOT NULL;

CREATE TABLE approval_decisions (
    request_id uuid NOT NULL REFERENCES approval_requests (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id),
    decision   text NOT NULL CHECK (decision IN ('approve', 'reject')),
    comment    text CHECK (length(comment) <= 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (request_id, user_id),
    -- A rejection always carries its reason.
    CHECK (decision = 'approve' OR length(btrim(comment)) > 0)
);
CREATE INDEX approval_decisions_user_idx ON approval_decisions (user_id);

-- New in-app notification kinds (ADR-0017). goose runs this file in one
-- transaction, so NOT VALID + VALIDATE does not shorten the ACCESS EXCLUSIVE
-- lock on notifications (held until commit); the check is a cheap scan of a
-- small table, and every existing row satisfies the wider list.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled')) NOT VALID;
ALTER TABLE notifications VALIDATE CONSTRAINT notifications_kind_check;

-- +goose Down
-- Approval notifications cannot satisfy the old kind list: drop them first.
DELETE FROM notifications WHERE kind LIKE 'approval\_%';
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('mention', 'assigned', 'overdue', 'comment'));
DROP TABLE IF EXISTS approval_decisions;
DROP TABLE IF EXISTS approval_requests;
DROP TABLE IF EXISTS approval_policies;
