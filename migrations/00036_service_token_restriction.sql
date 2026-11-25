-- Explicit project restriction for service tokens (ADR-0043). Until now "no
-- rows in service_token_projects" meant "every project the role can see",
-- so deleting the last allow-listed project (the FK cascades) silently
-- widened a restricted token to the whole workspace. project_restricted
-- records the restriction itself: a restricted token whose list is empty
-- reaches no project and no workspace-wide data. The purge also revokes
-- such tokens (ADR-0043), so this column is the second line of defense.
--
-- Backfill: a token is restricted when it has allow-list rows today, or
-- when its service_token.created event lists projects (that catches tokens
-- whose last project was already deleted and that were widened by the bug:
-- they now reach nothing, as their creator intended).
--
-- Locking: ADD COLUMN with a constant default is metadata-only (ACCESS
-- EXCLUSIVE on service_tokens for milliseconds). The UPDATE touches only
-- restricted tokens (at most 50 active per workspace) under row locks; the
-- events lookup uses events_entity_idx.

-- +goose Up
SET LOCAL lock_timeout = '5s';
ALTER TABLE service_tokens ADD COLUMN project_restricted boolean NOT NULL DEFAULT false;
UPDATE service_tokens t SET project_restricted = true
    WHERE EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id)
       OR EXISTS (SELECT 1 FROM events e
                  WHERE e.entity_type = 'service_token' AND e.entity_id = t.id AND e.action = 'service_token.created'
                    AND jsonb_path_exists(e.payload, 'strict $.project_ids[0]', '{}', true));

-- +goose Down
-- Restricted tokens whose allow-list is empty would become workspace-wide
-- under the previous image: revoke them (and deactivate their principals)
-- before dropping the column. Tokens with a non-empty list keep working.
SET LOCAL lock_timeout = '5s';
UPDATE users SET is_active = false WHERE id IN (
    SELECT t.user_id FROM service_tokens t
    WHERE t.project_restricted AND NOT EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id));
UPDATE service_tokens t SET revoked_at = now()
    WHERE t.project_restricted AND t.revoked_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id);
ALTER TABLE service_tokens DROP COLUMN IF EXISTS project_restricted;
