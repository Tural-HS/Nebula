-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS citext;

-- Keeps updated_at correct without relying on application code.
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
-- Tenancy and identity
-- ---------------------------------------------------------------------------

CREATE TABLE workspaces (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        citext NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    name        text   NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Passwords are never stored: identity comes from the OIDC provider.
CREATE TABLE users (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_issuer      text   NOT NULL,
    oidc_subject     text   NOT NULL,
    email            citext NOT NULL UNIQUE,
    display_name     text   NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    is_active        boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (oidc_issuer, oidc_subject)
);

CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
    role         text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'guest')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user_idx ON workspace_members (user_id);

-- ---------------------------------------------------------------------------
-- Configuration: workflows, item types, custom fields
-- ---------------------------------------------------------------------------

CREATE TABLE workflows (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name),
    UNIQUE (workspace_id, id)
);

CREATE TABLE statuses (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    workflow_id  uuid NOT NULL,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    category     text NOT NULL CHECK (category IN ('todo', 'in_progress', 'done', 'canceled')),
    position     integer NOT NULL DEFAULT 0,
    is_initial   boolean NOT NULL DEFAULT false,
    UNIQUE (workflow_id, name),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, workflow_id) REFERENCES workflows (workspace_id, id) ON DELETE CASCADE
);
-- Exactly zero or one initial status per workflow.
CREATE UNIQUE INDEX statuses_one_initial_idx ON statuses (workflow_id) WHERE is_initial;

-- from_status_id NULL means "from any status".
CREATE TABLE workflow_transitions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id    uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    from_status_id uuid REFERENCES statuses(id) ON DELETE CASCADE,
    to_status_id   uuid NOT NULL REFERENCES statuses(id) ON DELETE CASCADE,
    name           text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    CHECK (from_status_id IS DISTINCT FROM to_status_id)
);
CREATE UNIQUE INDEX workflow_transitions_unique_idx
    ON workflow_transitions (workflow_id, COALESCE(from_status_id, '00000000-0000-0000-0000-000000000000'::uuid), to_status_id);

CREATE TABLE item_types (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    workflow_id  uuid NOT NULL,
    key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    icon         text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    archived_at  timestamptz,
    UNIQUE (workspace_id, key),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, workflow_id) REFERENCES workflows (workspace_id, id)
);

CREATE TABLE field_definitions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    field_type   text NOT NULL CHECK (field_type IN (
                     'text', 'long_text', 'number', 'date', 'datetime', 'boolean',
                     'select', 'multi_select', 'user', 'url', 'email')),
    -- Type-specific settings, e.g. {"choices": [...]} for select.
    options      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(options) = 'object'),
    is_required  boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    archived_at  timestamptz,
    UNIQUE (workspace_id, key)
);

-- ---------------------------------------------------------------------------
-- Work
-- ---------------------------------------------------------------------------

CREATE TABLE projects (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    key              text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}$'),
    name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description      text,
    -- Allocated with UPDATE ... RETURNING inside the item-creating transaction.
    next_item_number bigint NOT NULL DEFAULT 1 CHECK (next_item_number > 0),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    archived_at      timestamptz,
    UNIQUE (workspace_id, key),
    UNIQUE (workspace_id, id)
);

CREATE TABLE items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid   NOT NULL,
    project_id   uuid   NOT NULL,
    item_type_id uuid   NOT NULL,
    status_id    uuid   NOT NULL,
    number       bigint NOT NULL CHECK (number > 0),
    title        text   NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    description  text,
    assignee_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    reporter_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    fields       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(fields) = 'object'),
    -- Optimistic locking: UPDATE ... WHERE id = $1 AND version = $2.
    version      integer NOT NULL DEFAULT 1,
    search       tsvector GENERATED ALWAYS AS (
                     setweight(to_tsvector('simple', title), 'A') ||
                     setweight(to_tsvector('simple', coalesce(description, '')), 'B')
                 ) STORED,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    UNIQUE (project_id, number),
    -- Composite FKs: an item can never point to another workspace's data.
    FOREIGN KEY (workspace_id, project_id)   REFERENCES projects   (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, item_type_id) REFERENCES item_types (workspace_id, id),
    FOREIGN KEY (workspace_id, status_id)    REFERENCES statuses   (workspace_id, id)
);
CREATE INDEX items_project_status_idx  ON items (project_id, status_id)              WHERE deleted_at IS NULL;
CREATE INDEX items_assignee_idx        ON items (assignee_id)                        WHERE deleted_at IS NULL;
CREATE INDEX items_workspace_recent_idx ON items (workspace_id, updated_at DESC, id) WHERE deleted_at IS NULL;
CREATE INDEX items_item_type_idx       ON items (item_type_id);
CREATE INDEX items_status_idx          ON items (status_id);
CREATE INDEX items_reporter_idx        ON items (reporter_id);
CREATE INDEX items_fields_idx          ON items USING gin (fields jsonb_path_ops);
CREATE INDEX items_search_idx          ON items USING gin (search);

CREATE TABLE comments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id    uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    author_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 100000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX comments_item_idx   ON comments (item_id, created_at);
CREATE INDEX comments_author_idx ON comments (author_id);

-- ---------------------------------------------------------------------------
-- Append-only event log: audit trail, activity feed, webhooks, reports.
-- ---------------------------------------------------------------------------

CREATE TABLE events (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    entity_type  text NOT NULL CHECK (entity_type ~ '^[a-z_]{1,50}$'),
    entity_id    uuid NOT NULL,
    action       text NOT NULL CHECK (action ~ '^[a-z_.]{1,100}$'),
    payload      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object'),
    occurred_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX events_workspace_time_idx ON events (workspace_id, occurred_at DESC);
CREATE INDEX events_entity_idx         ON events (entity_type, entity_id, id);
CREATE INDEX events_actor_idx          ON events (actor_id);

CREATE FUNCTION events_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'events is append-only';
END;
$$;
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
    FOR EACH ROW EXECUTE FUNCTION events_append_only();

-- updated_at triggers
CREATE TRIGGER workspaces_updated_at        BEFORE UPDATE ON workspaces        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER users_updated_at             BEFORE UPDATE ON users             FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER workflows_updated_at         BEFORE UPDATE ON workflows         FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER item_types_updated_at        BEFORE UPDATE ON item_types        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER field_definitions_updated_at BEFORE UPDATE ON field_definitions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER projects_updated_at          BEFORE UPDATE ON projects          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER items_updated_at             BEFORE UPDATE ON items             FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER comments_updated_at          BEFORE UPDATE ON comments          FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
DROP FUNCTION IF EXISTS events_append_only();
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS field_definitions;
DROP TABLE IF EXISTS item_types;
DROP TABLE IF EXISTS workflow_transitions;
DROP TABLE IF EXISTS statuses;
DROP TABLE IF EXISTS workflows;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS workspaces;
DROP FUNCTION IF EXISTS set_updated_at();
-- +goose StatementEnd
