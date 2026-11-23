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

// Roles, lowest to highest privilege.
const (
	RoleGuest  = "guest"
	RoleMember = "member"
	RoleAdmin  = "admin"
	RoleOwner  = "owner"
)

var roleRank = map[string]int{RoleGuest: 0, RoleMember: 1, RoleAdmin: 2, RoleOwner: 3}

// RoleAtLeast reports whether role grants at least min's privileges.
func RoleAtLeast(role, minRole string) bool {
	r, ok := roleRank[role]
	return ok && r >= roleRank[minRole]
}

// Workspace is a tenant, as seen by one member.
type Workspace struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Member is a user's membership in a workspace.
type Member struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
}

// Status is a workflow state.
type Status struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Category string    `json:"category"`
}

// ItemType is a kind of work item (task, bug, lead, candidate, ...).
type ItemType struct {
	ID         uuid.UUID       `json:"id"`
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Icon       *string         `json:"icon"`
	WorkflowID uuid.UUID       `json:"workflow_id"`
	Statuses   []Status        `json:"statuses"`
	Fields     []ItemTypeField `json:"fields"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

// CreateWorkspace creates a workspace owned by owner and installs the default template.
func (s *Store) CreateWorkspace(ctx context.Context, owner uuid.UUID, slug, name string) (Workspace, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRe.MatchString(slug) {
		return Workspace{}, invalid("slug", "must be 2-63 characters: lowercase letters, digits and hyphens, not starting with a hyphen")
	}
	name, err := text("name", name, 1, 200)
	if err != nil {
		return Workspace{}, err
	}

	ws := Workspace{ID: newID(), Slug: slug, Name: name, Role: RoleOwner}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO workspaces (id, slug, name) VALUES ($1, $2, $3) RETURNING created_at`,
			ws.ID, slug, name).Scan(&ws.CreatedAt)
		if isUniqueViolation(err, "workspaces_slug_key") {
			return &ConflictError{Code: "slug_taken", Message: "workspace slug is already taken"}
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
			ws.ID, owner); err != nil {
			return err
		}
		if err := installDefaultTemplate(ctx, tx, ws.ID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, ws.ID, owner, "workspace", ws.ID, "workspace.created",
			map[string]any{"slug": slug, "name": name})
	})
	return ws, err
}

// installDefaultTemplate gives every new workspace a usable workflow and item types.
func installDefaultTemplate(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) error {
	wfID := newID()
	if _, err := tx.Exec(ctx, `INSERT INTO workflows (id, workspace_id, name) VALUES ($1, $2, 'Default')`, wfID, wsID); err != nil {
		return err
	}
	statuses := []struct {
		name, category string
		initial        bool
	}{
		{"To Do", "todo", true},
		{"In Progress", "in_progress", false},
		{"Done", "done", false},
		{"Canceled", "canceled", false},
	}
	for i, st := range statuses {
		if _, err := tx.Exec(ctx, `INSERT INTO statuses (id, workspace_id, workflow_id, name, category, position, is_initial)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, newID(), wsID, wfID, st.name, st.category, i, st.initial); err != nil {
			return err
		}
	}
	for _, it := range [][2]string{{"task", "Task"}, {"bug", "Bug"}} {
		if _, err := tx.Exec(ctx, `INSERT INTO item_types (id, workspace_id, workflow_id, key, name) VALUES ($1, $2, $3, $4, $5)`,
			newID(), wsID, wfID, it[0], it[1]); err != nil {
			return err
		}
	}
	for _, r := range defaultRules() {
		if _, err := insertRule(ctx, tx, wsID, uuid.Nil, r); err != nil {
			return err
		}
	}
	return nil
}

const workspaceSelect = `SELECT w.id, w.slug, w.name, m.role, w.created_at
	FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id`

// ListWorkspaces returns the workspaces userID belongs to.
func (s *Store) ListWorkspaces(ctx context.Context, userID uuid.UUID) ([]Workspace, error) {
	rows, err := s.pool.Query(ctx, workspaceSelect+` WHERE m.user_id = $1 ORDER BY w.name, w.id`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Workspace])
}

// WorkspaceForMember returns the workspace only if userID is a member.
// Non-members get ErrNotFound, so workspace existence is never leaked.
func (s *Store) WorkspaceForMember(ctx context.Context, slug string, userID uuid.UUID) (Workspace, error) {
	rows, err := s.pool.Query(ctx, workspaceSelect+` WHERE w.slug = $1 AND m.user_id = $2`, strings.ToLower(slug), userID)
	if err != nil {
		return Workspace{}, err
	}
	ws, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Workspace])
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	return ws, err
}

// AddMember adds an existing user (one who has signed in at least once) by email.
func (s *Store) AddMember(ctx context.Context, wsID, actorID uuid.UUID, actorRole, email, role string) (Member, error) {
	if _, ok := roleRank[role]; !ok {
		return Member{}, invalid("role", "must be one of guest, member, admin, owner")
	}
	if role == RoleOwner && actorRole != RoleOwner {
		return Member{}, ErrForbidden
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return Member{}, invalid("email", "is required")
	}
	m := Member{Role: role}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, email, display_name FROM users WHERE email = $1 AND is_active`, email).
			Scan(&m.UserID, &m.Email, &m.DisplayName)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("email", "no active user with this email has signed in yet")
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)`, wsID, m.UserID, role)
		if isUniqueViolation(err, "workspace_members_pkey") {
			return &ConflictError{Code: "already_member", Message: "user is already a member of this workspace"}
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "member", m.UserID, "member.added", map[string]any{"role": role})
	})
	return m, err
}

// ListItemTypes returns active item types with their workflow statuses in order.
func (s *Store) ListItemTypes(ctx context.Context, wsID uuid.UUID) ([]ItemType, error) {
	return listItemTypes(ctx, s.pool, wsID, "")
}

// listItemTypes returns active item types, or only the one with key when set.
func listItemTypes(ctx context.Context, q querier, wsID uuid.UUID, key string) ([]ItemType, error) {
	rows, err := q.Query(ctx, `SELECT id, key, name, icon, workflow_id FROM item_types
		WHERE workspace_id = $1 AND archived_at IS NULL AND ($2 = '' OR key = $2) ORDER BY key`, wsID, key)
	if err != nil {
		return nil, err
	}
	types, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ItemType, error) {
		var t ItemType
		err := r.Scan(&t.ID, &t.Key, &t.Name, &t.Icon, &t.WorkflowID)
		t.Statuses = []Status{}
		return t, err
	})
	if err != nil {
		return nil, err
	}

	rows, err = q.Query(ctx, `SELECT workflow_id, id, name, category FROM statuses
		WHERE workspace_id = $1 ORDER BY position, name`, wsID)
	if err != nil {
		return nil, err
	}
	byWorkflow := map[uuid.UUID][]Status{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var wf uuid.UUID
		var st Status
		err := r.Scan(&wf, &st.ID, &st.Name, &st.Category)
		byWorkflow[wf] = append(byWorkflow[wf], st)
		return struct{}{}, err
	})
	if err != nil {
		return nil, err
	}
	for i := range types {
		if sts := byWorkflow[types[i].WorkflowID]; sts != nil {
			types[i].Statuses = sts
		}
	}
	fieldsByType, err := itemTypeFieldsByType(ctx, q, wsID)
	if err != nil {
		return nil, err
	}
	for i := range types {
		types[i].Fields = fieldsByType[types[i].ID]
		if types[i].Fields == nil {
			types[i].Fields = []ItemTypeField{}
		}
	}
	return types, nil
}

func isMember(ctx context.Context, q querier, wsID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2)`,
		wsID, userID).Scan(&ok)
	return ok, err
}
