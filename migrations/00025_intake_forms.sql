-- Intake forms (ADR-0032): an admin publishes a form for a project; signed-in
-- requesters fill it in and every submission becomes an item.
--
-- forms: the definition. `fields` is an ordered JSON array of
--   {"key": "<custom field key>|description|due_date", "label": ..., "help": ..., "required": bool}
-- validated by the application against the item type's schema on every save
-- and again on every submission. The slug is unique per installation: the
-- shareable URL is /f/<slug>. A deleted status falls back to the workflow's
-- initial status (SET NULL); a removed default assignee leaves items unassigned.
--
-- form_submissions: one row per submitted item, owned by the requester. The
-- requester never reads the item itself: "My requests" shows this row's
-- snapshot (the title and answers as submitted, with the labels of the time),
-- the item's current status and the team's explicit message. A deleted form
-- keeps its submissions (form_id NULL, form_title stays). Removing the item for
-- good (project or workspace deletion) removes the row.
--
-- status_changed_at is maintained by a trigger on items, so every path that
-- moves an item (PATCH, bulk edit, approvals, automations) marks the request
-- as updated for its requester without touching that code.
--
-- events.source gains 'form': form-created items are dispatched like user
-- changes (webhooks, automations, notifications), unlike 'import'.

-- +goose Up
SET LOCAL lock_timeout = '5s';

-- +goose StatementBegin
CREATE TABLE forms (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id          uuid NOT NULL,
    item_type_id        uuid NOT NULL,
    slug                citext NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{2,62}$'),
    title               text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    intro               text CHECK (length(intro) BETWEEN 1 AND 5000),
    title_template      text NOT NULL CHECK (length(title_template) BETWEEN 1 AND 500),
    fields              jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(fields) = 'array' AND jsonb_array_length(fields) <= 50),
    default_assignee_id uuid REFERENCES users (id) ON DELETE SET NULL,
    initial_status_id   uuid,
    -- members: any workspace member (guests included); accounts: anyone who can sign in to this Nebula.
    audience            text NOT NULL DEFAULT 'members' CHECK (audience IN ('members', 'accounts')),
    is_enabled          boolean NOT NULL DEFAULT true,
    -- Files a requester may attach (ADR-0022 limits apply per file and per workspace); 0 = none.
    max_files           smallint NOT NULL DEFAULT 0 CHECK (max_files BETWEEN 0 AND 10),
    created_by          uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, project_id)        REFERENCES projects (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_type_id)      REFERENCES item_types (workspace_id, id),
    FOREIGN KEY (workspace_id, initial_status_id) REFERENCES statuses (workspace_id, id) ON DELETE SET NULL (initial_status_id)
);
-- +goose StatementEnd
CREATE INDEX forms_project_idx     ON forms (workspace_id, project_id, created_at);
CREATE INDEX forms_item_type_idx   ON forms (workspace_id, item_type_id);
CREATE INDEX forms_status_idx      ON forms (workspace_id, initial_status_id) WHERE initial_status_id IS NOT NULL;
CREATE INDEX forms_assignee_idx    ON forms (default_assignee_id) WHERE default_assignee_id IS NOT NULL;
CREATE INDEX forms_created_by_idx  ON forms (created_by) WHERE created_by IS NOT NULL;
CREATE TRIGGER forms_updated_at BEFORE UPDATE ON forms FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementBegin
CREATE TABLE form_submissions (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    form_id           uuid,
    item_id           uuid NOT NULL UNIQUE,
    submitter_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    form_title        text NOT NULL CHECK (length(form_title) BETWEEN 1 AND 200),
    title             text NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    -- [{"key", "label", "type", "display"}]: what the requester sent, as text.
    answers           jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(answers) = 'array'),
    -- The team's message to the requester (the only text they see besides the status).
    message           text CHECK (length(message) BETWEEN 1 AND 5000),
    message_at        timestamptz,
    message_by        uuid REFERENCES users (id) ON DELETE SET NULL,
    status_changed_at timestamptz,
    seen_at           timestamptz NOT NULL DEFAULT now(),
    created_at        timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, form_id) REFERENCES forms (workspace_id, id) ON DELETE SET NULL (form_id),
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE
);
-- +goose StatementEnd
-- "My requests", newest first (keyset on created_at, id).
CREATE INDEX form_submissions_submitter_idx ON form_submissions (submitter_id, created_at DESC, id DESC);
CREATE INDEX form_submissions_form_idx      ON form_submissions (workspace_id, form_id) WHERE form_id IS NOT NULL;
CREATE INDEX form_submissions_ws_item_idx   ON form_submissions (workspace_id, item_id);
CREATE INDEX form_submissions_message_by_idx ON form_submissions (message_by) WHERE message_by IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION form_submissions_touch() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE form_submissions SET status_changed_at = now() WHERE item_id = NEW.id;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
-- A status change or a deletion is news for the requester. The WHEN clause
-- keeps every other item update free of the extra lookup.
CREATE TRIGGER items_form_submission_touch AFTER UPDATE OF status_id, deleted_at ON items
    FOR EACH ROW WHEN (OLD.status_id IS DISTINCT FROM NEW.status_id OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION form_submissions_touch();

-- 'form' joins the event sources of 00003 and 00008. The table is not
-- rewritten: NOT VALID + VALIDATE only holds the exclusive lock for the swap.
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_source_check;
ALTER TABLE events ADD CONSTRAINT events_source_check
    CHECK (source IN ('user', 'automation', 'system', 'import', 'form')) NOT VALID;
ALTER TABLE events VALIDATE CONSTRAINT events_source_check;

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP TRIGGER IF EXISTS items_form_submission_touch ON items;
DROP FUNCTION IF EXISTS form_submissions_touch();
DROP TABLE IF EXISTS form_submissions;
DROP TABLE IF EXISTS forms;
-- events.source keeps 'form' in its CHECK: the audit log is append-only and
-- is not rewritten; older code never writes the value, so the wider CHECK is
-- harmless.
