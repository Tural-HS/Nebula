-- Working-day calendar per workspace (ADR-0027): weekend days, non-working
-- dates (holidays) and working-day transfers (a weekend date declared a
-- working day). Recurring items (ADR-0012) gain "N-th / last working day of
-- the month" and a shift policy for dates that land on a non-working day.
--
-- Every ADD COLUMN has a constant default or none, so no table is rewritten;
-- the new CHECKs only scan recurrences (at most 1000 rows per workspace).

-- +goose Up
SET LOCAL lock_timeout = '5s';

-- Weekday numbers as in recurrences.weekdays: 0 = Sunday .. 6 = Saturday.
ALTER TABLE workspaces ADD COLUMN weekend_days smallint[] NOT NULL DEFAULT '{0,6}'
    CHECK (weekend_days <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::smallint[] AND cardinality(weekend_days) <= 3);

-- +goose StatementBegin
CREATE TABLE workspace_calendar_days (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    day          date NOT NULL CHECK (day BETWEEN '2000-01-01' AND '2100-12-31'),
    -- holiday: a non-working date; workday: a weekend date declared working.
    kind         text NOT NULL CHECK (kind IN ('holiday', 'workday')),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    -- One entry per date: a date cannot be both a holiday and a working day.
    PRIMARY KEY (workspace_id, day)
);
-- +goose StatementEnd
CREATE TRIGGER workspace_calendar_days_updated_at BEFORE UPDATE ON workspace_calendar_days
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- work_day: N-th working day of the month (1..23) or -1 = last working day;
-- replaces month_day for that rule. shift: what a plain date does when it
-- lands on a non-working day.
ALTER TABLE recurrences
    ADD COLUMN work_day smallint CHECK (work_day = -1 OR work_day BETWEEN 1 AND 23),
    ADD COLUMN shift text NOT NULL DEFAULT 'none' CHECK (shift IN ('none', 'previous', 'next')),
    ADD CONSTRAINT recurrences_work_day_rule_check CHECK (
        work_day IS NULL OR (freq IN ('monthly', 'yearly') AND month_day IS NULL AND shift = 'none'));

-- The date the rule named for this item (before any shift). NULL for items
-- created before 00020: their nominal date is their due date.
ALTER TABLE recurrence_runs ADD COLUMN nominal_date date;

-- +goose Down
-- Refuse to drop rules the previous release cannot run: it would read a
-- working-day rule as a monthly rule without a day.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM recurrences WHERE work_day IS NOT NULL OR shift <> 'none') THEN
        RAISE EXCEPTION 'recurrences use working-day options (ADR-0027); change or delete them before migrating down';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE recurrence_runs DROP COLUMN IF EXISTS nominal_date;
ALTER TABLE recurrences DROP CONSTRAINT IF EXISTS recurrences_work_day_rule_check,
    DROP COLUMN IF EXISTS shift, DROP COLUMN IF EXISTS work_day;
DROP TABLE IF EXISTS workspace_calendar_days;
ALTER TABLE workspaces DROP COLUMN IF EXISTS weekend_days;
