-- File attachments on items and comments (ADR-0022).
--
-- The bytes live in a blob store (local volume or S3-compatible), never in
-- Postgres; a row only points at a random storage key. Cascade rules:
--   * workspace/item hard delete  -> attachment rows cascade (composite FKs keep
--     every row inside its own workspace and on its own item).
--   * comment hard delete         -> the comment's attachments cascade too.
--     (The API soft-deletes comments; it removes their attachments itself.)
--   * uploader account deleted    -> uploader_id becomes NULL, the file stays.
-- Every deleted attachment row, however it was deleted, queues its blob for
-- removal in attachment_blob_purges (trigger below), so no cascade can leave
-- an orphaned file behind. An upload also registers its key there *before*
-- writing the blob, with a grace period, and clears it when the row commits:
-- a crash between "blob written" and "row inserted" is cleaned up as well.

-- +goose Up
-- +goose StatementBegin

-- The (workspace, item, comment) key lets attachments reference a comment of
-- the same item in the same workspace.
ALTER TABLE comments ADD CONSTRAINT comments_workspace_item_id_key UNIQUE (workspace_id, item_id, id);

CREATE TABLE attachments (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    item_id      uuid NOT NULL,
    comment_id   uuid,
    uploader_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    filename     text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
    -- Sniffed from the bytes on upload; the client's Content-Type is ignored.
    content_type text NOT NULL CHECK (length(content_type) BETWEEN 1 AND 127),
    size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
    sha256       bytea NOT NULL CHECK (length(sha256) = 32),
    storage_key  text NOT NULL UNIQUE CHECK (storage_key ~ '^[0-9a-f]{64}$'),
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, item_id) REFERENCES items (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_id, comment_id) REFERENCES comments (workspace_id, item_id, id) ON DELETE CASCADE
);
-- Listing an item's files (oldest first) and the per-workspace quota sum.
CREATE INDEX attachments_item_idx      ON attachments (workspace_id, item_id, created_at, id);
CREATE INDEX attachments_comment_idx   ON attachments (comment_id) WHERE comment_id IS NOT NULL;
CREATE INDEX attachments_uploader_idx  ON attachments (uploader_id) WHERE uploader_id IS NOT NULL;

-- Blobs waiting to be removed from the blob store by the worker.
--   kind 'upload':  a key reserved by an upload in progress; purged once
--                   purge_after passes (the upload never committed a row).
--   kind 'deleted': the attachment row is gone; the worker keeps the blob for
--                   NEBULA_ATTACHMENTS_PURGE_DELAY more, so a database restored
--                   to an earlier point in time still finds its files in the
--                   next file backup (runbook, section "Fayllar").
CREATE TABLE attachment_blob_purges (
    storage_key text PRIMARY KEY CHECK (storage_key ~ '^[0-9a-f]{64}$'),
    kind        text NOT NULL DEFAULT 'deleted' CHECK (kind IN ('upload', 'deleted')),
    purge_after timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attachment_blob_purges_due_idx ON attachment_blob_purges (purge_after);

CREATE FUNCTION attachments_queue_purge() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO attachment_blob_purges (storage_key, kind) VALUES (OLD.storage_key, 'deleted')
    ON CONFLICT (storage_key) DO UPDATE SET kind = 'deleted', purge_after = now();
    RETURN NULL;
END;
$$;
CREATE TRIGGER attachments_queue_purge AFTER DELETE ON attachments
    FOR EACH ROW EXECUTE FUNCTION attachments_queue_purge();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Rolling back forgets which blobs exist: empty the blob store separately.
DROP TABLE IF EXISTS attachments;
DROP FUNCTION IF EXISTS attachments_queue_purge();
DROP TABLE IF EXISTS attachment_blob_purges;
ALTER TABLE comments DROP CONSTRAINT IF EXISTS comments_workspace_item_id_key;
-- +goose StatementEnd
