-- Subtasks (ADR-0021): an item may have a parent item in the same project.
-- One level only: a subtask cannot have subtasks, and an item that has
-- subtasks cannot become one. With one level a cycle is impossible apart from
-- an item being its own parent, which a CHECK forbids.
--
-- Runs without a transaction so the new unique index is built CONCURRENTLY
-- (no long write lock on a large items table); every statement is
-- idempotent, so a migration that stopped half-way can simply be run again.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE items ADD COLUMN IF NOT EXISTS parent_id uuid;

-- Target of the composite FK below: the parent must be in the same workspace
-- AND the same project. id is already unique, so this can never fail.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS items_workspace_project_id_key
    ON items (workspace_id, project_id, id);

-- Children of a parent (subtask lists, progress counts, FK actions).
CREATE INDEX CONCURRENTLY IF NOT EXISTS items_parent_idx
    ON items (parent_id) WHERE parent_id IS NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'items_workspace_project_id_key') THEN
        ALTER TABLE items ADD CONSTRAINT items_workspace_project_id_key
            UNIQUE USING INDEX items_workspace_project_id_key;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'items_parent_fkey') THEN
        -- Hard deletes only happen when a whole project goes; the API
        -- soft-deletes and detaches subtasks itself. SET NULL (parent_id) keeps
        -- workspace_id/project_id intact (a plain SET NULL would null them).
        ALTER TABLE items ADD CONSTRAINT items_parent_fkey
            FOREIGN KEY (workspace_id, project_id, parent_id)
            REFERENCES items (workspace_id, project_id, id) ON DELETE SET NULL (parent_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'items_parent_not_self') THEN
        ALTER TABLE items ADD CONSTRAINT items_parent_not_self CHECK (parent_id <> id);
    END IF;
END
$$;
-- +goose StatementEnd

-- Depth guard. Locks the parent row (FOR SHARE) so two concurrent changes
-- cannot together build a second level: "X under Y" and "Z under X" would
-- each pass alone. Whichever runs second waits for the other's row lock and
-- then sees its committed result (plpgsql takes a new snapshot per query
-- under READ COMMITTED; REPEATABLE READ gets a serialization failure).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION items_check_parent() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    grandparent uuid;
    parent_deleted timestamptz;
BEGIN
    SELECT parent_id, deleted_at INTO grandparent, parent_deleted
        FROM items WHERE id = NEW.parent_id FOR SHARE;
    IF NOT FOUND THEN
        RETURN NEW; -- the FK reports it
    END IF;
    IF parent_deleted IS NOT NULL THEN
        RAISE EXCEPTION 'parent item is deleted'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'items_parent_live';
    END IF;
    IF grandparent IS NOT NULL THEN
        RAISE EXCEPTION 'a subtask cannot have subtasks'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'items_parent_depth';
    END IF;
    IF TG_OP = 'UPDATE' AND EXISTS (SELECT 1 FROM items WHERE parent_id = NEW.id) THEN
        RAISE EXCEPTION 'an item with subtasks cannot become a subtask'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'items_parent_depth';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS items_check_parent ON items;
CREATE TRIGGER items_check_parent
    BEFORE INSERT OR UPDATE OF parent_id ON items
    FOR EACH ROW WHEN (NEW.parent_id IS NOT NULL)
    EXECUTE FUNCTION items_check_parent();

-- +goose Down
DROP TRIGGER IF EXISTS items_check_parent ON items;
DROP FUNCTION IF EXISTS items_check_parent();
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_parent_not_self;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_parent_fkey;
DROP INDEX IF EXISTS items_parent_idx;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_workspace_project_id_key;
ALTER TABLE items DROP COLUMN IF EXISTS parent_id;
