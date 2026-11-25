-- Approval delegation for absences (ADR-0042). A member records an absence
-- (workspace-local dates, inclusive) with an optional delegate; while it is
-- active the delegate may decide on approval requests on the absent
-- member's behalf. Overlap ("one absence at a time per member and
-- workspace") is enforced by the application under a lock on the member's
-- workspace_members row: an exclusion constraint would need btree_gist,
-- which managed Postgres images do not always allow.
--
-- approval_decisions.user_id stays the human who acted; on_behalf_of is the
-- absent member whose approval slot the decision fills. The new unique
-- index (request, COALESCE(on_behalf_of, user_id)) makes each slot count at
-- most once; together with the primary key (request, user_id) one person
-- can never provide two approvals of the same request (own + delegated).
--
-- Locking: a new table; its foreign keys take SHARE ROW EXCLUSIVE on
-- workspaces and users for an empty table (milliseconds). ADD COLUMN without
-- a default is metadata-only (ACCESS EXCLUSIVE on approval_decisions for
-- milliseconds); its FK locks users the same way. The CHECK scans
-- approval_decisions once under that lock and cannot fail (the column is
-- NULL everywhere). CREATE UNIQUE INDEX (not CONCURRENTLY: goose runs the
-- file in one transaction) blocks writes to approval_decisions while it is
-- built: a small table (one row per vote), well under a second; existing
-- rows are unique already (on_behalf_of NULL, user_id is in the PK).

-- +goose Up
SET LOCAL lock_timeout = '5s';

CREATE TABLE workspace_absences (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users (id),
    delegate_id  uuid NULL REFERENCES users (id),
    starts_on    date NOT NULL,
    ends_on      date NOT NULL,
    -- Free text the absent member, the delegate and admins see; never put in
    -- events (it may say "sick leave").
    note         text NULL CHECK (length(note) <= 500),
    created_by   uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    -- Ended early (or canceled before it began); NULL while it runs its course.
    ended_at     timestamptz NULL,
    ended_by     uuid NULL REFERENCES users (id) ON DELETE SET NULL,
    CHECK (starts_on <= ends_on),
    CHECK (ends_on - starts_on <= 366),
    CHECK (delegate_id IS NULL OR delegate_id <> user_id)
);
-- Lookups: "is this member absent today" and "whom does this delegate
-- stand in for today"; ended rows are history and never searched this way.
CREATE INDEX workspace_absences_user_idx ON workspace_absences (workspace_id, user_id, ends_on)
    WHERE ended_at IS NULL;
CREATE INDEX workspace_absences_delegate_idx ON workspace_absences (workspace_id, delegate_id, ends_on)
    WHERE ended_at IS NULL AND delegate_id IS NOT NULL;

ALTER TABLE approval_decisions
    ADD COLUMN on_behalf_of uuid NULL REFERENCES users (id),
    ADD CONSTRAINT approval_decisions_on_behalf_check CHECK (on_behalf_of IS NULL OR on_behalf_of <> user_id);
CREATE UNIQUE INDEX approval_decisions_principal_idx ON approval_decisions (request_id, (COALESCE(on_behalf_of, user_id)));

-- +goose Down
-- Absences are lost (the previous image has no delegation). Decisions made
-- on someone's behalf keep their row with the real actor in user_id; who
-- they stood in for stays in the approval.approved / approval.rejected
-- event payloads (on_behalf_of) and so in the audit log and evidence
-- exports. The previous image ignores both.
-- Refuses while a pending request has a delegated decision: the previous
-- image would count it as the delegate's own vote (the absent approver's
-- slot would open again and the delegate's slot would be taken), changing
-- who still has to approve. Let those requests close (or cancel them) first.
SET LOCAL lock_timeout = '5s';
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM approval_decisions d JOIN approval_requests r ON r.id = d.request_id
               WHERE d.on_behalf_of IS NOT NULL AND r.state = 'pending') THEN
        RAISE EXCEPTION 'pending approval requests have delegated decisions; let them close (or cancel them) before rolling back 00032';
    END IF;
END
$$;
-- +goose StatementEnd
DROP INDEX IF EXISTS approval_decisions_principal_idx;
ALTER TABLE approval_decisions DROP CONSTRAINT IF EXISTS approval_decisions_on_behalf_check;
ALTER TABLE approval_decisions DROP COLUMN IF EXISTS on_behalf_of;
DROP TABLE IF EXISTS workspace_absences;
