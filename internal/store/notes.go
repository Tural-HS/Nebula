package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Personal sticky notes (ADR-0052). Every query is scoped by the author's
// user id: there is no way to read or change another user's note, and a
// wrong id is "not found" rather than "forbidden".

const (
	maxNotesPerUser = 100
	maxNoteLen      = 2000
)

var noteColors = map[string]bool{"yellow": true, "green": true, "blue": true, "pink": true, "purple": true}

// Note is one sticky note.
type Note struct {
	ID        uuid.UUID `json:"id"`
	Body      string    `json:"body"`
	Color     string    `json:"color"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NoteInput creates or changes a note; unset fields are kept on update.
type NoteInput struct {
	Body   opt.Field[string] `json:"body"`
	Color  opt.Field[string] `json:"color"`
	Pinned opt.Field[bool]   `json:"pinned"`
}

func noteBody(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", invalid("body", "write something")
	}
	if utf8.RuneCountInString(v) > maxNoteLen || !utf8.ValidString(v) {
		return "", invalid("body", "at most %d characters", maxNoteLen)
	}
	return v, nil
}

func noteColor(v string) (string, error) {
	if !noteColors[v] {
		return "", invalid("color", "must be yellow, green, blue, pink or purple")
	}
	return v, nil
}

const noteColumns = `id, body, color, pinned, created_at, updated_at`

func scanNote(row pgx.Row) (Note, error) {
	var n Note
	err := row.Scan(&n.ID, &n.Body, &n.Color, &n.Pinned, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

// ListNotes returns the user's notes: pinned first, then the newest.
func (s *Store) ListNotes(ctx context.Context, userID uuid.UUID) ([]Note, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+noteColumns+` FROM user_notes WHERE user_id = $1
		ORDER BY pinned DESC, updated_at DESC, id LIMIT $2`, userID, maxNotesPerUser)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Note, error) { return scanNote(r) })
}

// CreateNote adds a note (yellow unless chosen).
func (s *Store) CreateNote(ctx context.Context, userID uuid.UUID, in NoteInput) (Note, error) {
	if !in.Body.Set || in.Body.Null {
		return Note{}, invalid("body", "write something")
	}
	body, err := noteBody(in.Body.Value)
	if err != nil {
		return Note{}, err
	}
	color := "yellow"
	if in.Color.Set && !in.Color.Null {
		if color, err = noteColor(in.Color.Value); err != nil {
			return Note{}, err
		}
	}
	pinned := in.Pinned.Set && !in.Pinned.Null && in.Pinned.Value
	var n Note
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		// Serialize one user's creates so the cap holds under concurrency.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_notes WHERE user_id = $1`, userID).Scan(&count); err != nil {
			return err
		}
		if count >= maxNotesPerUser {
			return invalid("body", "at most %d notes; delete old ones first", maxNotesPerUser)
		}
		n, err = scanNote(tx.QueryRow(ctx, `INSERT INTO user_notes (id, user_id, body, color, pinned)
			VALUES ($1, $2, $3, $4, $5) RETURNING `+noteColumns, newID(), userID, body, color, pinned))
		return err
	})
	return n, err
}

// UpdateNote changes the user's own note.
func (s *Store) UpdateNote(ctx context.Context, userID, id uuid.UUID, in NoteInput) (Note, error) {
	if in.Body.Null || in.Color.Null || in.Pinned.Null {
		return Note{}, invalid("body", "fields cannot be null")
	}
	var body, color *string
	var pinned *bool
	if in.Body.Set {
		b, err := noteBody(in.Body.Value)
		if err != nil {
			return Note{}, err
		}
		body = &b
	}
	if in.Color.Set {
		c, err := noteColor(in.Color.Value)
		if err != nil {
			return Note{}, err
		}
		color = &c
	}
	if in.Pinned.Set {
		pinned = &in.Pinned.Value
	}
	n, err := scanNote(s.pool.QueryRow(ctx, `UPDATE user_notes SET body = coalesce($3, body), color = coalesce($4, color),
			pinned = coalesce($5, pinned)
		WHERE id = $1 AND user_id = $2 RETURNING `+noteColumns, id, userID, body, color, pinned))
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return n, err
}

// DeleteNote removes the user's own note.
func (s *Store) DeleteNote(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM user_notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
