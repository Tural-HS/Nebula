-- Personal sticky notes on the home page (ADR-0052): short notes only their
-- author can read, on every device. Not tied to a workspace, so leaving or
-- deleting a workspace never touches them; they go with the user.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE user_notes (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    color      text NOT NULL DEFAULT 'yellow' CHECK (color IN ('yellow', 'green', 'blue', 'pink', 'purple')),
    pinned     boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_notes_user_idx ON user_notes (user_id, pinned DESC, updated_at DESC);
CREATE TRIGGER user_notes_updated_at BEFORE UPDATE ON user_notes FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS user_notes;
