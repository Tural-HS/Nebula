-- Removes E2E workspaces whose slug matches :'pattern' (a POSIX regex).
-- Test data only: run against the local docker compose database, never prod.
-- Order matters: items -> item_types -> statuses/workflows use non-cascading
-- composite FKs on purpose (a type or status in use cannot be deleted), so
-- recurrences, items, forms and item types go first; everything else cascades
-- from workspaces.
\set ON_ERROR_STOP on
BEGIN;
CREATE TEMP TABLE doomed ON COMMIT DROP AS
    SELECT id FROM workspaces WHERE slug ~ :'pattern';
DELETE FROM recurrences WHERE workspace_id IN (SELECT id FROM doomed); -- references item_types
DELETE FROM items      WHERE workspace_id IN (SELECT id FROM doomed);
DELETE FROM forms      WHERE workspace_id IN (SELECT id FROM doomed); -- references item_types (ADR-0032)
DELETE FROM item_types WHERE workspace_id IN (SELECT id FROM doomed);
DELETE FROM workspaces WHERE id IN (SELECT id FROM doomed);
COMMIT;
