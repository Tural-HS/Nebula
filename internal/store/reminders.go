package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EventItemAnniversary is emitted when an item's yearly date (e.g. a birthday)
// is today (ADR-0054). Emitted by the scheduler; becomes in-app
// notifications for everyone (members and up) who can see the project.
const EventItemAnniversary = "item.anniversary"

const (
	// reminderHour is when the reminders go out if the workspace has no
	// digest hour: the start of the working day.
	reminderHour = 9
	// maxRemindersPerDay bounds one project's run (one notification per
	// viewer each): a misconfigured field must not flood everyone.
	maxRemindersPerDay = 200
	reminderBatch      = 100 // projects per tick
)

// DateReminder is a project's yearly date reminder setting.
type DateReminder struct {
	// Field is the date field whose month and day trigger the reminder; nil = off.
	Field *string `json:"field"`
	// LastSent is the workspace-local date of the last run.
	LastSent *string `json:"last_sent"`
	// Hour is the local hour the reminders go out (the digest hour, else 9).
	Hour int `json:"hour"`
}

// GetDateReminder returns the project's setting (Field nil when off).
func (s *Store) GetDateReminder(ctx context.Context, wsID, projectID uuid.UUID) (DateReminder, error) {
	var r DateReminder
	var last *time.Time
	err := s.pool.QueryRow(ctx, `SELECT d.field_key, d.last_sent, COALESCE(w.overdue_digest_hour, $3)
		FROM workspaces w LEFT JOIN date_reminders d ON d.workspace_id = w.id AND d.project_id = $2
		WHERE w.id = $1`, wsID, projectID, reminderHour).Scan(&r.Field, &last, &r.Hour)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if last != nil {
		r.LastSent = strPtr(last.Format(time.DateOnly))
	}
	return r, err
}

// SaveDateReminder sets (a date field of the workspace) or clears (nil) the
// project's reminder field (admin+, checked by the caller) and records a
// date_reminder.updated event. Choosing a field again keeps last_sent, so
// re-saving the same day never repeats today's reminders.
func (s *Store) SaveDateReminder(ctx context.Context, wsID, projectID, actorID uuid.UUID, field *string) (DateReminder, error) {
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if field == nil {
			tag, err := tx.Exec(ctx, `DELETE FROM date_reminders WHERE workspace_id = $1 AND project_id = $2`, wsID, projectID)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "date_reminder.updated", map[string]any{"field": nil})
		}
		var typ string
		var archived bool
		err := tx.QueryRow(ctx, `SELECT field_type, archived_at IS NOT NULL FROM field_definitions WHERE workspace_id = $1 AND key = $2`,
			wsID, *field).Scan(&typ, &archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (typ != FieldDate || archived)) {
			return invalid("field", "must be an active date field of this workspace")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO date_reminders (project_id, workspace_id, field_key, updated_by)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id) DO UPDATE SET field_key = EXCLUDED.field_key, updated_by = EXCLUDED.updated_by`,
			projectID, wsID, *field, nullUUID(actorID)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "date_reminder.updated", map[string]any{"field": *field})
	})
	if err != nil {
		return DateReminder{}, err
	}
	return s.GetDateReminder(ctx, wsID, projectID)
}

// EmitDateReminders runs the projects whose reminders are due: not yet run
// today (workspace-local) and past the reminder hour. Returns the number of
// item.anniversary events. A failing project does not stop the others.
func (s *Store) EmitDateReminders(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.project_id FROM date_reminders d
		JOIN workspaces w ON w.id = d.workspace_id AND w.archived_at IS NULL AND w.deletion_requested_at IS NULL
		JOIN projects p ON p.id = d.project_id AND p.archived_at IS NULL AND p.deletion_requested_at IS NULL
		WHERE (d.last_sent IS NULL OR d.last_sent < (now() AT TIME ZONE w.timezone)::date)
		  AND (now() AT TIME ZONE w.timezone)::time >= make_time(COALESCE(w.overdue_digest_hour, $2), w.overdue_digest_minute, 0)
		ORDER BY d.project_id LIMIT $1`, reminderBatch, reminderHour)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	total := 0
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		n, err := s.runDateReminder(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", id, err))
			continue
		}
		total += n
	}
	return total, errors.Join(errs...)
}

// anniversaryDays returns the "MM-DD" values that fall on today: today's,
// plus 02-29 on 28 February of a common year (no birthday is skipped).
func anniversaryDays(today time.Time) []string {
	days := []string{today.Format("01-02")}
	if today.Month() == time.February && today.Day() == 28 && time.Date(today.Year(), time.March, 0, 0, 0, 0, 0, time.UTC).Day() == 28 {
		days = append(days, "02-29")
	}
	return days
}

func (s *Store) runDateReminder(ctx context.Context, projectID uuid.UUID) (int, error) {
	ctx = withSystem(ctx)
	n := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var wsID uuid.UUID
		var key, fieldName string
		var today time.Time
		var due bool
		// SKIP LOCKED: another replica is running it now.
		err := tx.QueryRow(ctx, `SELECT d.workspace_id, d.field_key, COALESCE(f.name, d.field_key),
				(now() AT TIME ZONE w.timezone)::date,
				(d.last_sent IS NULL OR d.last_sent < (now() AT TIME ZONE w.timezone)::date)
					AND (now() AT TIME ZONE w.timezone)::time >= make_time(COALESCE(w.overdue_digest_hour, $2), w.overdue_digest_minute, 0)
			FROM date_reminders d
			JOIN workspaces w ON w.id = d.workspace_id
			LEFT JOIN field_definitions f ON f.workspace_id = d.workspace_id AND f.key = d.field_key
				AND f.field_type = 'date' AND f.archived_at IS NULL
			WHERE d.project_id = $1 FOR UPDATE OF d SKIP LOCKED`, projectID, reminderHour).
			Scan(&wsID, &key, &fieldName, &today, &due)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil || !due {
			return err
		}
		// Every item of the project, open or closed: a customer whose
		// monthly payment is "done" is still a customer. Deleted items no.
		// The text comparison never casts, so an odd stored value is skipped.
		rows, err := tx.Query(ctx, `SELECT i.id, i.fields->>$3 FROM items i
			WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.deleted_at IS NULL
			  AND jsonb_typeof(i.fields->$3) = 'string' AND length(i.fields->>$3) = 10
			  AND right(i.fields->>$3, 5) = ANY($4)
			ORDER BY i.number LIMIT $5`, wsID, projectID, key, anniversaryDays(today), maxRemindersPerDay)
		if err != nil {
			return err
		}
		type hit struct {
			id   uuid.UUID
			date string
		}
		hits, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (hit, error) {
			var h hit
			return h, r.Scan(&h.id, &h.date)
		})
		if err != nil {
			return err
		}
		for _, h := range hits {
			if err := appendEvent(ctx, tx, wsID, uuid.Nil, "item", h.id, EventItemAnniversary, map[string]any{
				"field": key, "field_name": fieldName, "date": h.date, "on": today.Format(time.DateOnly),
			}); err != nil {
				return err
			}
		}
		n = len(hits)
		_, err = tx.Exec(ctx, `UPDATE date_reminders SET last_sent = $2 WHERE project_id = $1`, projectID, today)
		return err
	})
	return n, err
}
