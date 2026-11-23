-- Project-level privacy (ADR-0013): private projects are visible only to
-- their members and to workspace admins/owners.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE projects ADD COLUMN visibility text NOT NULL DEFAULT 'workspace'
    CHECK (visibility IN ('workspace', 'private'));

CREATE TABLE project_members (
    workspace_id uuid NOT NULL,
    project_id   uuid NOT NULL,
    user_id      uuid NOT NULL,
    added_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES projects (workspace_id, id) ON DELETE CASCADE,
    -- Leaving the workspace removes every project membership in it.
    FOREIGN KEY (workspace_id, user_id) REFERENCES workspace_members (workspace_id, user_id) ON DELETE CASCADE
);
CREATE INDEX project_members_user_idx ON project_members (workspace_id, user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS project_members;
ALTER TABLE projects DROP COLUMN IF EXISTS visibility;
-- +goose StatementEnd
