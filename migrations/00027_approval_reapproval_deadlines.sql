-- Approval improvements (ADR-0034, on top of ADR-0026).
--
-- Re-approval: a policy may say that changing approved content needs a new
-- approval (reapprove_on_change). reapprove_fields narrows what is watched to
-- "title", "description" and "fields.<key>" tokens; empty means title,
-- description and every custom field. Like the rest of the policy, both are
-- snapshotted on the request: the rule an approval was given under decides
-- whether a later change invalidates it. invalidated_at marks an approved
-- request whose content was changed afterwards; reopened_from links the new
-- request opened for the changed content to the one it replaces.
--
-- Deadlines: due_in_working_days (1..30) on the policy; the request stores the
-- resulting local date (due_on, computed with the workspace working-day
-- calendar of ADR-0027 when the request opens). overdue_notified_at makes the
-- scheduler's one reminder per request idempotent across worker replicas.
--
-- New notification kinds approval_overdue and approval_invalidated. The CHECK
-- is the full union of every kind so far (00019 approvals, 00021 watchers).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE approval_policies
    ADD COLUMN reapprove_on_change boolean NOT NULL DEFAULT false,
    ADD COLUMN reapprove_fields    text[]  NOT NULL DEFAULT '{}'
        CHECK (cardinality(reapprove_fields) <= 50),
    ADD COLUMN due_in_working_days int
        CHECK (due_in_working_days BETWEEN 1 AND 30);

ALTER TABLE approval_requests
    ADD COLUMN reapprove_on_change boolean NOT NULL DEFAULT false,
    ADD COLUMN reapprove_fields    text[]  NOT NULL DEFAULT '{}',
    ADD COLUMN due_on              date,
    ADD COLUMN overdue_notified_at timestamptz,
    ADD COLUMN invalidated_at      timestamptz,
    ADD COLUMN reopened_from       uuid REFERENCES approval_requests (id) ON DELETE SET NULL,
    -- Only an approved request can be invalidated.
    ADD CONSTRAINT approval_requests_invalidated_check CHECK (invalidated_at IS NULL OR state = 'approved');

-- The workspace inbox walks pending requests oldest first (keyset on
-- created_at, id); the scheduler looks for deadlines not yet reminded of.
CREATE INDEX approval_requests_inbox_idx ON approval_requests (workspace_id, created_at, id) WHERE state = 'pending';
CREATE INDEX approval_requests_due_idx ON approval_requests (due_on)
    WHERE state = 'pending' AND due_on IS NOT NULL AND overdue_notified_at IS NULL;
CREATE INDEX approval_requests_reopened_from_idx ON approval_requests (reopened_from) WHERE reopened_from IS NOT NULL;
-- The covering (latest approved) request of an item is looked up on edits.
CREATE INDEX approval_requests_item_approved_idx ON approval_requests (item_id, closed_at DESC)
    WHERE state = 'approved' AND invalidated_at IS NULL;

-- goose runs this file in one transaction: NOT VALID + VALIDATE does not
-- shorten the lock, but every existing row satisfies the wider list.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added',
    'approval_overdue', 'approval_invalidated')) NOT VALID;
ALTER TABLE notifications VALIDATE CONSTRAINT notifications_kind_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM notifications WHERE kind IN ('approval_overdue', 'approval_invalidated');
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
-- Back to the list as of 00021.
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
    'mention', 'assigned', 'overdue', 'comment',
    'approval_requested', 'approval_applied', 'approval_rejected', 'approval_canceled',
    'status_changed', 'assignee_changed', 'due_changed', 'link_added'));
DROP INDEX IF EXISTS approval_requests_item_approved_idx;
DROP INDEX IF EXISTS approval_requests_reopened_from_idx;
DROP INDEX IF EXISTS approval_requests_due_idx;
DROP INDEX IF EXISTS approval_requests_inbox_idx;
ALTER TABLE approval_requests
    DROP CONSTRAINT IF EXISTS approval_requests_invalidated_check,
    DROP COLUMN IF EXISTS reopened_from,
    DROP COLUMN IF EXISTS invalidated_at,
    DROP COLUMN IF EXISTS overdue_notified_at,
    DROP COLUMN IF EXISTS due_on,
    DROP COLUMN IF EXISTS reapprove_fields,
    DROP COLUMN IF EXISTS reapprove_on_change;
ALTER TABLE approval_policies
    DROP COLUMN IF EXISTS due_in_working_days,
    DROP COLUMN IF EXISTS reapprove_fields,
    DROP COLUMN IF EXISTS reapprove_on_change;
-- +goose StatementEnd
