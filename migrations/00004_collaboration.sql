-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Comments: tenant-safe like every other table (ADR-0007)
-- ---------------------------------------------------------------------------

ALTER TABLE items ADD CONSTRAINT items_workspace_id_id_key UNIQUE (workspace_id, id);

ALTER TABLE comments ADD COLUMN workspace_id uuid;
UPDATE comments c SET workspace_id = i.workspace_id FROM items i WHERE i.id = c.item_id;
ALTER TABLE comments ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE comments
    ADD CONSTRAINT comments_workspace_item_fkey
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE;
-- Comments are rendered as plain text; 20k characters is plenty for a thread.
ALTER TABLE comments ADD CONSTRAINT comments_body_len CHECK (length(body) <= 20000);

DROP INDEX comments_item_idx;
CREATE INDEX comments_item_idx ON comments (item_id, created_at, id);

CREATE TABLE comment_mentions (
    comment_id uuid NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    PRIMARY KEY (comment_id, user_id)
);
CREATE INDEX comment_mentions_user_idx ON comment_mentions (user_id);

-- Keyset pagination for the workspace audit log and item activity feeds.
CREATE INDEX events_workspace_id_idx ON events (workspace_id, id DESC);

-- ---------------------------------------------------------------------------
-- Wake workers as soon as jobs are committed (LISTEN/NOTIFY), instead of
-- waiting for the next poll. Notifications are delivered on COMMIT and
-- de-duplicated per transaction, so this costs one message per transaction.
-- ---------------------------------------------------------------------------

CREATE FUNCTION jobs_notify() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('nebula_jobs', '');
    RETURN NULL;
END;
$$;
CREATE TRIGGER jobs_notify AFTER INSERT ON jobs FOR EACH STATEMENT EXECUTE FUNCTION jobs_notify();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS jobs_notify ON jobs;
DROP FUNCTION IF EXISTS jobs_notify();
DROP INDEX IF EXISTS events_workspace_id_idx;
DROP TABLE IF EXISTS comment_mentions;
DROP INDEX IF EXISTS comments_item_idx;
CREATE INDEX comments_item_idx ON comments (item_id, created_at);
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_body_len;
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_workspace_item_fkey;
ALTER TABLE comments DROP COLUMN IF EXISTS workspace_id;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_workspace_id_id_key;
-- +goose StatementEnd
