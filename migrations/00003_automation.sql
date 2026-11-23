-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- Workflow transitions (ADR-0005)
-- ---------------------------------------------------------------------------

-- Off: any status of the workflow is reachable. On: only defined transitions.
ALTER TABLE workflows ADD COLUMN enforce_transitions boolean NOT NULL DEFAULT false;

ALTER TABLE workflow_transitions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();

-- ---------------------------------------------------------------------------
-- Event provenance: lets automation ignore its own changes (no loops).
-- ---------------------------------------------------------------------------

ALTER TABLE events
    ADD COLUMN source text NOT NULL DEFAULT 'user' CHECK (source IN ('user', 'automation', 'system')),
    ADD COLUMN automation_rule_id uuid;

-- ---------------------------------------------------------------------------
-- Job queue (transactional outbox). Jobs are inserted in the same transaction
-- as the change that causes them, and claimed with FOR UPDATE SKIP LOCKED.
-- ---------------------------------------------------------------------------

CREATE TABLE jobs (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind         text   NOT NULL CHECK (kind ~ '^[a-z_.]{1,64}$'),
    payload      jsonb  NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object'),
    run_at       timestamptz NOT NULL DEFAULT now(),
    attempts     integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 10 CHECK (max_attempts BETWEEN 1 AND 100),
    locked_until timestamptz,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'done', 'failed')),
    -- Fan-out jobs (one per webhook/rule and event) are enqueued idempotently,
    -- so a retried dispatcher never produces duplicate deliveries.
    dedupe_key   text UNIQUE CHECK (length(dedupe_key) <= 200)
);
-- Workers only ever scan runnable jobs.
CREATE INDEX jobs_runnable_idx ON jobs (run_at, id) WHERE state = 'pending';
CREATE INDEX jobs_finished_idx ON jobs (finished_at) WHERE state <> 'pending';

-- ---------------------------------------------------------------------------
-- Outgoing webhooks
-- ---------------------------------------------------------------------------

CREATE TABLE webhooks (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    url              text NOT NULL CHECK (length(url) BETWEEN 8 AND 2048),
    -- AES-256-GCM ciphertext of the HMAC signing secret; never stored in plain text.
    secret_encrypted bytea NOT NULL,
    events           text[] NOT NULL CHECK (cardinality(events) BETWEEN 1 AND 50),
    is_active        boolean NOT NULL DEFAULT true,
    failure_count    integer NOT NULL DEFAULT 0,
    disabled_reason  text,
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);
CREATE INDEX webhooks_workspace_active_idx ON webhooks (workspace_id) WHERE is_active;
CREATE TRIGGER webhooks_updated_at BEFORE UPDATE ON webhooks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE webhook_deliveries (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    webhook_id   uuid   NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id     bigint NOT NULL,
    job_id       bigint NOT NULL,
    attempt      integer NOT NULL,
    status_code  integer,
    error        text,
    duration_ms  integer NOT NULL,
    delivered_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_webhook_idx ON webhook_deliveries (webhook_id, id DESC);

-- ---------------------------------------------------------------------------
-- Automation rules: when <trigger> and all <conditions>, run <actions>.
-- ---------------------------------------------------------------------------

CREATE TABLE automation_rules (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    trigger      text NOT NULL CHECK (trigger IN ('item.created', 'item.updated')),
    conditions   jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(conditions) = 'array'),
    actions      jsonb NOT NULL CHECK (jsonb_typeof(actions) = 'array' AND jsonb_array_length(actions) BETWEEN 1 AND 10),
    is_enabled   boolean NOT NULL DEFAULT true,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, name)
);
CREATE INDEX automation_rules_trigger_idx ON automation_rules (workspace_id, trigger) WHERE is_enabled;
CREATE TRIGGER automation_rules_updated_at BEFORE UPDATE ON automation_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS automation_rules;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
DROP TABLE IF EXISTS jobs;
ALTER TABLE events DROP COLUMN IF EXISTS automation_rule_id, DROP COLUMN IF EXISTS source;
ALTER TABLE workflow_transitions DROP COLUMN IF EXISTS created_at;
ALTER TABLE workflows DROP COLUMN IF EXISTS enforce_transitions;
-- +goose StatementEnd
