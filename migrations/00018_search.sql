-- Workspace-wide search (ADR-0024). Items (key/title/description) and
-- comment bodies are indexed in one side table, search_docs, kept current by
-- triggers. A side table instead of a column on items/comments because:
--   * adding a STORED generated column rewrites the whole table under an
--     ACCESS EXCLUSIVE lock (minutes on a big table, every read and write
--     blocked), and
--   * a trigger-maintained column would need a backfill UPDATE of every row,
--     which bumps items.updated_at (items_updated_at trigger) and bloats the
--     hottest table.
-- Here nothing rewrites items or comments: the triggers take a brief SHARE ROW
-- EXCLUSIVE lock when created (bounded by lock_timeout) and the backfill
-- commits in small batches, so the application keeps reading and writing
-- throughout. Locks per step are listed in deploy/production/README.md §9.
--
-- Azerbaijani has no Postgres dictionary: text goes through search_fold()
-- (ə→e, ı/İ→i, ö→o, ü→u, ğ→g, ş→s, ç→c, common Latin accents, stray combining
-- marks dropped) and the 'simple' configuration (lowercase, no stemming), so
-- "ödəniş" and "odenis" are the same lexeme. pg_trgm on folded titles serves
-- typo-tolerant matching when full-text finds nothing.
--
-- items.search and items_search_idx (00001) stay for the previous release
-- (expand/contract); a later migration drops them.

-- +goose NO TRANSACTION

-- +goose Up
-- Fail fast instead of queueing behind a long transaction while holding up
-- every writer behind us (lock queue). Reset at the end: goose keeps this
-- session for later migrations.
SET lock_timeout = '5s';

-- pg_trgm is a trusted extension: the schema owner may create it.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- IMMUTABLE and SQL-standard bodies (bound at creation, immune to search_path;
-- inlined by the planner). Changing a definition later requires rebuilding
-- search_docs (UPDATE in batches) because stored lexemes depend on it.
-- The Go side (internal/store/searchfold.go) mirrors this mapping for
-- highlighting; a test checks both agree.
CREATE OR REPLACE FUNCTION search_fold(t text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN pg_catalog.translate(t,
        'ƏəİIıÖöÜüĞğŞşÇçÁáÀàÂâÄäÉéÈèÊêËëÍíÌìÎîÏïÓóÒòÔôÕõÚúÙùÛûÑņ̃̀́̂̃̆̇̈',
        'eeiiioouuggssccaaaaaaaaeeeeeeeeiiiiiiiioooooooouuuuuunn');

CREATE OR REPLACE FUNCTION search_item_doc(title text, description text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN pg_catalog.setweight(pg_catalog.to_tsvector('pg_catalog.simple'::regconfig, search_fold(title)), 'A')
        || pg_catalog.setweight(pg_catalog.to_tsvector('pg_catalog.simple'::regconfig, search_fold(coalesce(description, ''))), 'B');

-- Mention tokens (<@uuid>) are ids, not words: left out of the index.
CREATE OR REPLACE FUNCTION search_comment_doc(body text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    RETURN pg_catalog.setweight(pg_catalog.to_tsvector('pg_catalog.simple'::regconfig,
        search_fold(pg_catalog.regexp_replace(body, '<@[0-9a-fA-F-]{36}>', ' ', 'g'))), 'C');

-- One row per live item (id = item id) and per live comment (id = comment
-- id). Rows exist only while the item/comment is not deleted, so a query
-- filters by project visibility alone; the API re-checks the item on read.
CREATE TABLE IF NOT EXISTS search_docs (
    id           uuid PRIMARY KEY,
    kind         text NOT NULL CHECK (kind IN ('item', 'comment')),
    workspace_id uuid NOT NULL,
    project_id   uuid NOT NULL,
    item_id      uuid NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    comment_id   uuid REFERENCES comments (id) ON DELETE CASCADE,
    doc          tsvector NOT NULL,
    title_fold   text, -- items only: trigram (typo) matching
    CHECK ((kind = 'item' AND id = item_id AND comment_id IS NULL AND title_fold IS NOT NULL)
        OR (kind = 'comment' AND id = comment_id AND title_fold IS NULL))
);
-- Indexes on the still-empty table: instant, no CONCURRENTLY needed.
CREATE INDEX IF NOT EXISTS search_docs_doc_idx ON search_docs USING gin (doc);
CREATE INDEX IF NOT EXISTS search_docs_title_trgm_idx ON search_docs USING gin (title_fold gin_trgm_ops) WHERE kind = 'item';
-- FK cascades and trigger maintenance by item / comment.
CREATE INDEX IF NOT EXISTS search_docs_item_idx ON search_docs (item_id);
CREATE INDEX IF NOT EXISTS search_docs_comment_idx ON search_docs (comment_id) WHERE comment_id IS NOT NULL;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION search_docs_item_sync() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF NEW.deleted_at IS NOT NULL THEN
        DELETE FROM search_docs WHERE item_id = NEW.id;
        RETURN NULL;
    END IF;
    INSERT INTO search_docs (id, kind, workspace_id, project_id, item_id, doc, title_fold)
    VALUES (NEW.id, 'item', NEW.workspace_id, NEW.project_id, NEW.id,
            search_item_doc(NEW.title, NEW.description), search_fold(NEW.title))
    ON CONFLICT (id) DO UPDATE
        SET project_id = EXCLUDED.project_id, doc = EXCLUDED.doc, title_fold = EXCLUDED.title_fold;
    IF TG_OP = 'UPDATE' THEN
        IF OLD.deleted_at IS NOT NULL THEN -- restored: its comments come back too
            INSERT INTO search_docs (id, kind, workspace_id, project_id, item_id, comment_id, doc)
            SELECT c.id, 'comment', NEW.workspace_id, NEW.project_id, NEW.id, c.id, search_comment_doc(c.body)
            FROM comments c WHERE c.item_id = NEW.id AND c.deleted_at IS NULL
            ON CONFLICT (id) DO NOTHING;
        ELSIF OLD.project_id IS DISTINCT FROM NEW.project_id THEN
            UPDATE search_docs SET project_id = NEW.project_id WHERE item_id = NEW.id AND kind = 'comment';
        END IF;
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION search_docs_comment_sync() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF NEW.deleted_at IS NOT NULL THEN
        DELETE FROM search_docs WHERE id = NEW.id;
        RETURN NULL;
    END IF;
    -- FOR SHARE waits for a concurrent soft delete of the item and then sees
    -- it, so a comment can never re-add a document for a deleted item.
    INSERT INTO search_docs (id, kind, workspace_id, project_id, item_id, comment_id, doc)
    SELECT NEW.id, 'comment', i.workspace_id, i.project_id, i.id, NEW.id, search_comment_doc(NEW.body)
    FROM (SELECT workspace_id, project_id, id FROM items
          WHERE id = NEW.item_id AND deleted_at IS NULL FOR SHARE) i
    ON CONFLICT (id) DO UPDATE SET doc = EXCLUDED.doc;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

-- Created before the backfill, so no write between the two is missed.
DROP TRIGGER IF EXISTS items_search_docs_insert ON items;
CREATE TRIGGER items_search_docs_insert AFTER INSERT ON items
    FOR EACH ROW EXECUTE FUNCTION search_docs_item_sync();
DROP TRIGGER IF EXISTS items_search_docs_update ON items;
CREATE TRIGGER items_search_docs_update AFTER UPDATE OF title, description, deleted_at, project_id ON items
    FOR EACH ROW WHEN (OLD.title IS DISTINCT FROM NEW.title OR OLD.description IS DISTINCT FROM NEW.description
                       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at OR OLD.project_id IS DISTINCT FROM NEW.project_id)
    EXECUTE FUNCTION search_docs_item_sync();
DROP TRIGGER IF EXISTS comments_search_docs_insert ON comments;
CREATE TRIGGER comments_search_docs_insert AFTER INSERT ON comments
    FOR EACH ROW EXECUTE FUNCTION search_docs_comment_sync();
DROP TRIGGER IF EXISTS comments_search_docs_update ON comments;
CREATE TRIGGER comments_search_docs_update AFTER UPDATE OF body, deleted_at ON comments
    FOR EACH ROW WHEN (OLD.body IS DISTINCT FROM NEW.body OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION search_docs_comment_sync();

RESET lock_timeout;

-- Backfill in keyset batches of 2000, one transaction each (COMMIT inside DO
-- works because this migration runs outside a transaction). FOR SHARE makes
-- a batch wait for, then re-read, rows being changed concurrently, and
-- ON CONFLICT DO NOTHING keeps what the triggers already wrote (newer).
-- Re-running after an interruption continues where it stopped.
-- +goose StatementBegin
DO $$
DECLARE
    last_id uuid := '00000000-0000-0000-0000-000000000000';
    ids     uuid[];
BEGIN
    LOOP
        SELECT array_agg(id ORDER BY id) INTO ids
        FROM (SELECT id FROM items WHERE id > last_id ORDER BY id LIMIT 2000) s;
        EXIT WHEN ids IS NULL;
        WITH src AS (
            SELECT id, workspace_id, project_id, title, description FROM items
            WHERE id = ANY (ids) AND deleted_at IS NULL FOR SHARE
        )
        INSERT INTO search_docs (id, kind, workspace_id, project_id, item_id, doc, title_fold)
        SELECT id, 'item', workspace_id, project_id, id, search_item_doc(title, description), search_fold(title) FROM src
        ON CONFLICT (id) DO NOTHING;
        last_id := ids[array_length(ids, 1)];
        COMMIT;
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
    last_id uuid := '00000000-0000-0000-0000-000000000000';
    ids     uuid[];
BEGIN
    LOOP
        SELECT array_agg(id ORDER BY id) INTO ids
        FROM (SELECT id FROM comments WHERE id > last_id ORDER BY id LIMIT 2000) s;
        EXIT WHEN ids IS NULL;
        WITH src AS (
            SELECT c.id, c.item_id, c.body, i.workspace_id, i.project_id
            FROM comments c JOIN items i ON i.id = c.item_id
            WHERE c.id = ANY (ids) AND c.deleted_at IS NULL AND i.deleted_at IS NULL
            FOR SHARE OF c, i
        )
        INSERT INTO search_docs (id, kind, workspace_id, project_id, item_id, comment_id, doc)
        SELECT id, 'comment', workspace_id, project_id, item_id, id, search_comment_doc(body) FROM src
        ON CONFLICT (id) DO NOTHING;
        last_id := ids[array_length(ids, 1)];
        COMMIT;
    END LOOP;
END
$$;
-- +goose StatementEnd

ANALYZE search_docs;

-- +goose Down
SET lock_timeout = '5s';
DROP TRIGGER IF EXISTS comments_search_docs_update ON comments;
DROP TRIGGER IF EXISTS comments_search_docs_insert ON comments;
DROP TRIGGER IF EXISTS items_search_docs_update ON items;
DROP TRIGGER IF EXISTS items_search_docs_insert ON items;
RESET lock_timeout;
DROP TABLE IF EXISTS search_docs;
DROP FUNCTION IF EXISTS search_docs_comment_sync();
DROP FUNCTION IF EXISTS search_docs_item_sync();
DROP FUNCTION IF EXISTS search_comment_doc(text);
DROP FUNCTION IF EXISTS search_item_doc(text, text);
DROP FUNCTION IF EXISTS search_fold(text);
-- pg_trgm stays: other objects may use it and it is harmless.
