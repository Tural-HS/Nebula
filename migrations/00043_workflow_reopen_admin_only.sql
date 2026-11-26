-- Reopening closed items (ADR-0060): a workflow can reserve moves from a
-- done/canceled status back to an open one for admins. System changes
-- (monthly close, automations) are not affected.
--
-- Locking: ADD COLUMN with a constant default is metadata-only in
-- PostgreSQL 11+: ACCESS EXCLUSIVE on workflows for milliseconds, no rewrite.

-- +goose Up
SET LOCAL lock_timeout = '5s';

ALTER TABLE workflows ADD COLUMN reopen_admin_only boolean NOT NULL DEFAULT false;

-- +goose Down
-- Only the setting is lost; everyone may reopen again. The previous image
-- does not read the column, so it runs on either schema.
SET LOCAL lock_timeout = '5s';
ALTER TABLE workflows DROP COLUMN IF EXISTS reopen_admin_only;
