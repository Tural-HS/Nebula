// Package store is the only package that talks SQL. Every mutation runs in a
// transaction together with its audit event, so the event log can never miss
// or invent a change.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a Store backed by pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, fn)
}

// querier is implemented by both the pool and a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ErrNotFound means the resource does not exist or the caller may not see it.
var ErrNotFound = errors.New("not found")

// ErrForbidden means the caller is authenticated but lacks permission.
var ErrForbidden = errors.New("forbidden")

// ValidationError is a client input problem tied to one field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// ConflictError is a state conflict, e.g. a taken slug or a stale version.
type ConflictError struct {
	Code    string
	Message string
}

func (e *ConflictError) Error() string { return e.Code + ": " + e.Message }

func isUniqueViolation(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}

// newID returns a time-ordered UUIDv7 (better B-tree locality than v4).
func newID() uuid.UUID {
	if id, err := uuid.NewV7(); err == nil {
		return id
	}
	return uuid.New()
}

// text normalizes a required string and enforces a rune length range.
func text(field, v string, minLen, maxLen int) (string, error) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if !utf8.ValidString(v) {
		return "", invalid(field, "must be valid UTF-8")
	}
	if n < minLen || n > maxLen {
		if minLen > 0 && n == 0 {
			return "", invalid(field, "is required")
		}
		return "", invalid(field, "must be between %d and %d characters", minLen, maxLen)
	}
	return v, nil
}

// optionalText normalizes an optional string: blank becomes nil.
func optionalText(field string, v *string, maxLen int) (*string, error) {
	if v == nil {
		return nil, nil
	}
	s, err := text(field, *v, 0, maxLen)
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	return &s, nil
}

func appendEvent(ctx context.Context, q querier, workspaceID, actorID uuid.UUID,
	entityType string, entityID uuid.UUID, action string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := q.Exec(ctx, `INSERT INTO events (workspace_id, actor_id, entity_type, entity_id, action, payload)
		VALUES ($1, $2, $3, $4, $5, $6)`, workspaceID, actorID, entityType, entityID, action, payload)
	if err != nil {
		return fmt.Errorf("append event %s: %w", action, err)
	}
	return nil
}
