package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Project groups items and owns their human-readable numbering (KEY-123).
type Project struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

const projectSelect = `SELECT id, key, name, description, created_at, updated_at FROM projects`

// CreateProject creates a project in a workspace.
func (s *Store) CreateProject(ctx context.Context, wsID, actorID uuid.UUID, key, name string, description *string) (Project, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !projectKeyRe.MatchString(key) {
		return Project{}, invalid("key", "must be 2-10 characters: uppercase letters and digits, starting with a letter")
	}
	name, err := text("name", name, 1, 200)
	if err != nil {
		return Project{}, err
	}
	description, err = optionalText("description", description, 10_000)
	if err != nil {
		return Project{}, err
	}

	p := Project{ID: newID(), Key: key, Name: name, Description: description}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO projects (id, workspace_id, key, name, description)
			VALUES ($1, $2, $3, $4, $5) RETURNING created_at, updated_at`,
			p.ID, wsID, key, name, description).Scan(&p.CreatedAt, &p.UpdatedAt)
		if isUniqueViolation(err, "projects_workspace_id_key_key") {
			return &ConflictError{Code: "project_key_taken", Message: "a project with this key already exists"}
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", p.ID, "project.created", map[string]any{"key": key, "name": name})
	})
	return p, err
}

// ListProjects returns the workspace's active projects.
func (s *Store) ListProjects(ctx context.Context, wsID uuid.UUID) ([]Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND archived_at IS NULL ORDER BY key`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Project])
}

// GetProject returns an active project by key within a workspace.
func (s *Store) GetProject(ctx context.Context, wsID uuid.UUID, key string) (Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND key = $2 AND archived_at IS NULL`,
		wsID, strings.ToUpper(key))
	if err != nil {
		return Project{}, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Project])
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}
