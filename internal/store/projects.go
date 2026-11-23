package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Project groups items and owns their human-readable numbering (KEY-123).
type Project struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Visibility is "workspace" (every member) or "private" (project members
	// and workspace admins only), ADR-0013.
	Visibility string `json:"visibility"`
}

// Viewer is who is asking: project reads always go through it, so a caller
// cannot load a project without an access check.
type Viewer struct {
	UserID uuid.UUID
	Role   string // workspace role
}

var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

const projectSelect = `SELECT id, key, name, description, created_at, updated_at, visibility FROM projects`

// visibleToViewer is the access predicate for project rows (table projects):
// $2 = viewer user id, $3 = viewer is admin+. Keep in sync with canSeeProject.
const visibleToViewer = `(visibility = 'workspace' OR $3::boolean
	OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = projects.id AND pm.user_id = $2))`

// canSeeProject reports whether a (third) user can see a project: used for
// assignees and mentions. Non-members of the workspace never can.
func canSeeProject(ctx context.Context, q querier, wsID, projectID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM projects p
		JOIN workspace_members m ON m.workspace_id = p.workspace_id AND m.user_id = $3
		WHERE p.workspace_id = $1 AND p.id = $2
		  AND (p.visibility = 'workspace' OR m.role IN ('admin', 'owner')
		       OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = $3)))`,
		wsID, projectID, userID).Scan(&ok)
	return ok, err
}

// ProjectTemplate copies the setup of an existing project into a new one
// (ADR-0020). Items are never copied; workflows, item types and fields are
// workspace-wide and need no copying.
type ProjectTemplate struct {
	From        string // source project key; "" = no template
	Members     bool   // project members (matters for private projects)
	Recurrences bool   // recurring items, scheduled from the source's next date
	Views       bool   // shared saved views (personal views stay with their owners)
}

// CreateProject creates a project in a workspace, optionally copying the
// setup of another project (tpl).
func (s *Store) CreateProject(ctx context.Context, wsID, actorID uuid.UUID, key, name string, description *string, visibility string, tpl ProjectTemplate) (Project, error) {
	if visibility == "" {
		visibility = "workspace"
	}
	if visibility != "workspace" && visibility != "private" {
		return Project{}, invalid("visibility", "must be workspace or private")
	}
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

	p := Project{ID: newID(), Key: key, Name: name, Description: description, Visibility: visibility}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO projects (id, workspace_id, key, name, description, visibility)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at, updated_at`,
			p.ID, wsID, key, name, description, visibility).Scan(&p.CreatedAt, &p.UpdatedAt)
		if isUniqueViolation(err, "projects_workspace_id_key_key") {
			return &ConflictError{Code: "project_key_taken", Message: "a project with this key already exists"}
		}
		if err != nil {
			return err
		}
		payload := map[string]any{"key": key, "name": name, "visibility": visibility}
		if tpl.From != "" {
			copied, err := copyProjectSetup(ctx, tx, wsID, actorID, p.ID, tpl)
			if err != nil {
				return err
			}
			payload["template"] = copied
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", p.ID, "project.created", payload)
	})
	return p, err
}

// copyProjectSetup copies what tpl asks for from the source project into
// dst, inside the caller's transaction, and returns what was copied.
func copyProjectSetup(ctx context.Context, tx pgx.Tx, wsID, actorID, dst uuid.UUID, tpl ProjectTemplate) (map[string]any, error) {
	var src uuid.UUID
	// Only admins create projects and admins see every project, so no
	// visibility check is needed beyond the workspace scope.
	err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE workspace_id = $1 AND key = $2 AND archived_at IS NULL`,
		wsID, strings.ToUpper(strings.TrimSpace(tpl.From))).Scan(&src)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid("template", "source project not found")
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{"from": strings.ToUpper(strings.TrimSpace(tpl.From))}
	if tpl.Members {
		tag, err := tx.Exec(ctx, `INSERT INTO project_members (workspace_id, project_id, user_id, added_by)
			SELECT workspace_id, $3, user_id, $4 FROM project_members WHERE workspace_id = $1 AND project_id = $2
			ON CONFLICT DO NOTHING`, wsID, src, dst, actorID)
		if err != nil {
			return nil, err
		}
		out["members"] = tag.RowsAffected()
	}
	if tpl.Recurrences {
		var have, add int
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE true), count(*) FILTER (WHERE project_id = $2)
			FROM recurrences WHERE workspace_id = $1`, wsID, src).Scan(&have, &add); err != nil {
			return nil, err
		}
		if have+add > maxRecurrencesPerWorkspace {
			return nil, invalid("template", "a workspace can have at most %d recurring items", maxRecurrencesPerWorkspace)
		}
		// next_due is kept: the copy starts where the source would continue,
		// never in the past (the scheduler catches up at most 24 per tick).
		rows, err := tx.Query(ctx, `INSERT INTO recurrences (id, workspace_id, project_id, title, description, item_type_id,
				assignee_id, fields, freq, every, weekdays, month_day, month, start_date, end_date, lead_days, next_due,
				is_enabled, created_by)
			SELECT gen_random_uuid(), workspace_id, $3, title, description, item_type_id, assignee_id, fields, freq, every,
				weekdays, month_day, month, start_date, end_date, lead_days, next_due, is_enabled, $4
			FROM recurrences WHERE workspace_id = $1 AND project_id = $2 ORDER BY created_at, id
			RETURNING id, title, freq, next_due::text, is_enabled`, wsID, src, dst, actorID)
		if err != nil {
			return nil, err
		}
		type rec struct {
			id      uuid.UUID
			title   string
			freq    string
			next    *string
			enabled bool
		}
		var recs []rec
		for rows.Next() {
			var r rec
			if err := rows.Scan(&r.id, &r.title, &r.freq, &r.next, &r.enabled); err != nil {
				rows.Close()
				return nil, err
			}
			recs = append(recs, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for _, r := range recs {
			if err := appendEvent(ctx, tx, wsID, actorID, "recurrence", r.id, "recurrence.created",
				map[string]any{"title": r.title, "freq": r.freq, "next_due": r.next, "enabled": r.enabled}); err != nil {
				return nil, err
			}
		}
		out["recurrences"] = len(recs)
	}
	if tpl.Views {
		tag, err := tx.Exec(ctx, `INSERT INTO saved_views (id, workspace_id, project_id, owner_id, name, kind, shared, config)
			SELECT gen_random_uuid(), workspace_id, $3, $4, name, kind, true, config
			FROM saved_views WHERE workspace_id = $1 AND project_id = $2 AND shared`, wsID, src, dst, actorID)
		if err != nil {
			return nil, err
		}
		out["views"] = tag.RowsAffected()
	}
	return out, nil
}

// ListProjects returns the workspace's active projects the viewer can see.
func (s *Store) ListProjects(ctx context.Context, wsID uuid.UUID, v Viewer) ([]Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND archived_at IS NULL AND `+visibleToViewer+` ORDER BY key`,
		wsID, v.UserID, RoleAtLeast(v.Role, RoleAdmin))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Project])
}

// GetProject returns an active project by key if the viewer can see it;
// otherwise ErrNotFound, so a private project's existence does not leak.
func (s *Store) GetProject(ctx context.Context, wsID uuid.UUID, key string, v Viewer) (Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE workspace_id = $1 AND key = $4 AND archived_at IS NULL AND `+visibleToViewer,
		wsID, v.UserID, RoleAtLeast(v.Role, RoleAdmin), strings.ToUpper(key))
	if err != nil {
		return Project{}, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Project])
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// UpdateProjectParams is a partial project update.
type UpdateProjectParams struct {
	Name        opt.Field[string]
	Description opt.Field[string]
	Visibility  opt.Field[string]
}

// UpdateProject changes name, description or visibility (admin+, checked by
// the caller) and records project.updated.
func (s *Store) UpdateProject(ctx context.Context, wsID, projectID, actorID uuid.UUID, p UpdateProjectParams) error {
	if (p.Name.Set && p.Name.Null) || (p.Visibility.Set && p.Visibility.Null) {
		return invalid("body", "name and visibility cannot be null")
	}
	if p.Visibility.Set && p.Visibility.Value != "workspace" && p.Visibility.Value != "private" {
		return invalid("visibility", "must be workspace or private")
	}
	var name string
	if p.Name.Set {
		var err error
		if name, err = text("name", p.Name.Value, 1, 200); err != nil {
			return err
		}
	}
	var desc *string
	if p.Description.Set {
		var err error
		if desc, err = optionalText("description", p.Description.Ptr(), 10_000); err != nil {
			return err
		}
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var curName, curVis string
		var curDesc *string
		if err := tx.QueryRow(ctx, `SELECT name, description, visibility FROM projects WHERE workspace_id = $1 AND id = $2 FOR UPDATE`,
			wsID, projectID).Scan(&curName, &curDesc, &curVis); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		changes := map[string]any{}
		newName, newDesc, newVis := curName, curDesc, curVis
		if p.Name.Set && name != curName {
			changes["name"], newName = diff(curName, name), name
		}
		if p.Description.Set && !equalPtr(curDesc, desc) {
			changes["description"], newDesc = diff(curDesc, desc), desc
		}
		if p.Visibility.Set && p.Visibility.Value != curVis {
			changes["visibility"], newVis = diff(curVis, p.Visibility.Value), p.Visibility.Value
		}
		if len(changes) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE projects SET name = $1, description = $2, visibility = $3 WHERE id = $4`,
			newName, newDesc, newVis, projectID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "project.updated", map[string]any{"changes": changes})
	})
}

// ProjectMember is a user explicitly given access to a (private) project.
type ProjectMember struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"` // workspace role
	AddedAt     time.Time `json:"added_at"`
}

// ListProjectMembers returns a project's explicit members by name.
func (s *Store) ListProjectMembers(ctx context.Context, wsID, projectID uuid.UUID) ([]ProjectMember, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.display_name, u.email, m.role, pm.created_at
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		JOIN workspace_members m ON m.workspace_id = pm.workspace_id AND m.user_id = pm.user_id
		WHERE pm.workspace_id = $1 AND pm.project_id = $2
		ORDER BY u.display_name, u.id`, wsID, projectID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ProjectMember])
}

// AddProjectMember gives a workspace member access to a project (idempotent).
func (s *Store) AddProjectMember(ctx context.Context, wsID, projectID, actorID, userID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		ok, err := isMember(ctx, tx, wsID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return invalid("user_id", "user is not a member of this workspace")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO project_members (workspace_id, project_id, user_id, added_by)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, wsID, projectID, userID, actorID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "project.member_added", map[string]any{"user_id": userID})
	})
}

// RemoveProjectMember revokes explicit access. Workspace admins keep seeing
// private projects regardless (ADR-0013).
func (s *Store) RemoveProjectMember(ctx context.Context, wsID, projectID, actorID, userID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM project_members WHERE workspace_id = $1 AND project_id = $2 AND user_id = $3`,
			wsID, projectID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "project.member_removed", map[string]any{"user_id": userID})
	})
}
