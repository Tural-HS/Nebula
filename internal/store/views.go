package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Saved views (ADR-0014): a named set of list filters, table columns, sort
// and board grouping for one project. Personal views are visible to their
// owner only; shared views to everyone who can read the project.

// View kinds: how the project page shows the view's items.
const (
	ViewTable    = "table"
	ViewBoard    = "board"
	ViewCalendar = "calendar" // month grid by due date (ADR-0023)
)

const (
	maxViewName            = 100
	maxViewsPerUserProject = 100
	maxViewColumns         = 30
	maxViewQuery           = 500
	maxViewStatuses        = 50
)

// Built-in table columns, in their default order. Custom fields are
// "field.<key>".
var builtinColumns = []string{"key", "title", "type", "status", "assignee", "due_date", "created_at", "updated_at"}

// ViewFilters are the list filters of GET …/items, in the same terms.
type ViewFilters struct {
	Q        string            `json:"q,omitempty"`
	Assignee string            `json:"assignee,omitempty"` // "", "me" (the viewer), "none" or a user id
	Due      string            `json:"due,omitempty"`
	Status   []uuid.UUID       `json:"status,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"` // field key -> exact value, like ?field.<key>=
}

// ViewConfig is what a view applies.
type ViewConfig struct {
	Filters ViewFilters `json:"filters"`
	Columns []string    `json:"columns"`  // table columns in order; empty = the default set
	Sort    string      `json:"sort"`     // SortNumber or SortDue
	GroupBy string      `json:"group_by"` // board grouping: "status" or "category"
}

// View is a saved view as seen by one caller.
type View struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID uuid.UUID  `json:"project_id"`
	OwnerID   uuid.UUID  `json:"owner_id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"`
	Shared    bool       `json:"shared"`
	Config    ViewConfig `json:"config"`
	CanEdit   bool       `json:"can_edit"` // may the caller rename, change or delete it
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ViewCaller identifies the caller of a view operation.
type ViewCaller struct {
	WorkspaceID uuid.UUID
	ProjectID   uuid.UUID
	UserID      uuid.UUID
	Role        string // workspace role
}

// canEdit: personal views belong to their owner alone; shared views may be
// changed by their owner and by admins.
func (v ViewCaller) canEdit(view View) bool {
	if view.OwnerID == v.UserID {
		return true
	}
	return view.Shared && RoleAtLeast(v.Role, RoleAdmin)
}

const viewSelect = `SELECT id, project_id, owner_id, name, kind, shared, config, created_at, updated_at FROM saved_views`

func scanView(row pgx.Row, v ViewCaller) (View, error) {
	var out View
	err := row.Scan(&out.ID, &out.ProjectID, &out.OwnerID, &out.Name, &out.Kind, &out.Shared, &out.Config, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if out.Config.Columns == nil {
		out.Config.Columns = []string{}
	}
	out.CanEdit = v.canEdit(out)
	return out, err
}

// ListViews returns the caller's personal views and the project's shared
// views, by name.
func (s *Store) ListViews(ctx context.Context, v ViewCaller) ([]View, error) {
	rows, err := s.pool.Query(ctx, viewSelect+`
		WHERE workspace_id = $1 AND project_id = $2 AND (shared OR owner_id = $3)
		ORDER BY lower(name), id`, v.WorkspaceID, v.ProjectID, v.UserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (View, error) { return scanView(r, v) })
}

// GetView returns a view the caller may see; others are ErrNotFound, so the
// existence of someone else's personal view is not revealed.
func (s *Store) GetView(ctx context.Context, v ViewCaller, id uuid.UUID) (View, error) {
	return getView(ctx, s.pool, v, id, false)
}

func getView(ctx context.Context, q querier, v ViewCaller, id uuid.UUID, lock bool) (View, error) {
	sql := viewSelect + ` WHERE workspace_id = $1 AND project_id = $2 AND id = $3 AND (shared OR owner_id = $4)`
	if lock {
		sql += " FOR UPDATE"
	}
	return scanView(q.QueryRow(ctx, sql, v.WorkspaceID, v.ProjectID, id, v.UserID), v)
}

// ViewInput creates a view (all fields) or patches one (set fields only).
type ViewInput struct {
	Name   opt.Field[string]     `json:"name"`
	Kind   opt.Field[string]     `json:"kind"`
	Shared opt.Field[bool]       `json:"shared"`
	Config opt.Field[ViewConfig] `json:"config"`
}

// CreateView saves a new view owned by the caller. Any reader of the project
// may save personal views; sharing one requires admin.
func (s *Store) CreateView(ctx context.Context, v ViewCaller, in ViewInput) (View, error) {
	var out View
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createView(ctx, tx, v, in)
		return err
	})
	return out, err
}

// createView is CreateView inside the caller's transaction (workspace kinds, ADR-0033).
func createView(ctx context.Context, tx pgx.Tx, v ViewCaller, in ViewInput) (View, error) {
	view := View{ID: newID(), ProjectID: v.ProjectID, OwnerID: v.UserID, Kind: ViewTable}
	if in.Kind.Set && !in.Kind.Null {
		view.Kind = in.Kind.Value
	}
	if in.Shared.Set && !in.Shared.Null {
		view.Shared = in.Shared.Value
	}
	if in.Config.Set && !in.Config.Null {
		view.Config = in.Config.Value
	}
	if !in.Name.Set || in.Name.Null {
		return View{}, invalid("name", "is required")
	}
	view.Name = in.Name.Value
	if view.Shared && !RoleAtLeast(v.Role, RoleAdmin) {
		return View{}, ErrForbidden
	}

	err := func() error {
		if err := validateView(ctx, tx, v, &view); err != nil {
			return err
		}
		// Serialize creates per owner and project so the limit holds under
		// concurrent requests.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('saved_views:' || $1::text || ':' || $2::text, 0))`,
			v.ProjectID, v.UserID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM saved_views WHERE workspace_id = $1 AND project_id = $2 AND owner_id = $3`,
			v.WorkspaceID, v.ProjectID, v.UserID).Scan(&n); err != nil {
			return err
		}
		if n >= maxViewsPerUserProject {
			return &ConflictError{Code: "view_limit", Message: fmt.Sprintf("at most %d views per user and project", maxViewsPerUserProject)}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO saved_views (id, workspace_id, project_id, owner_id, name, kind, shared, config)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING created_at, updated_at`,
			view.ID, v.WorkspaceID, v.ProjectID, v.UserID, view.Name, view.Kind, view.Shared, view.Config).
			Scan(&view.CreatedAt, &view.UpdatedAt); err != nil {
			return err
		}
		if view.Shared {
			return appendEvent(ctx, tx, v.WorkspaceID, v.UserID, "view", view.ID, "view.created", viewPayload(view))
		}
		return nil
	}()
	if err != nil {
		return View{}, err
	}
	view.CanEdit = true
	return view, nil
}

// UpdateView patches a view the caller may edit. Sharing a personal view is
// admin+, like creating a shared one.
func (s *Store) UpdateView(ctx context.Context, v ViewCaller, id uuid.UUID, in ViewInput) (View, error) {
	if (in.Name.Set && in.Name.Null) || (in.Kind.Set && in.Kind.Null) || (in.Shared.Set && in.Shared.Null) || (in.Config.Set && in.Config.Null) {
		return View{}, invalid("view", "fields cannot be null")
	}
	var view View
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := getView(ctx, tx, v, id, true)
		if err != nil {
			return err
		}
		if !cur.CanEdit {
			return ErrForbidden
		}
		next := cur
		if in.Name.Set {
			next.Name = in.Name.Value
		}
		if in.Kind.Set {
			next.Kind = in.Kind.Value
		}
		if in.Config.Set {
			next.Config = in.Config.Value
		}
		if in.Shared.Set {
			next.Shared = in.Shared.Value
		}
		if next.Shared && !cur.Shared && !RoleAtLeast(v.Role, RoleAdmin) {
			return ErrForbidden
		}
		if err := validateView(ctx, tx, v, &next); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE saved_views SET name = $1, kind = $2, shared = $3, config = $4
			WHERE id = $5 RETURNING updated_at`, next.Name, next.Kind, next.Shared, next.Config, id).Scan(&next.UpdatedAt); err != nil {
			return err
		}
		next.CanEdit = v.canEdit(next)
		view = next
		if cur.Shared || next.Shared {
			p := viewPayload(next)
			p["changes"] = viewChanges(cur, next)
			return appendEvent(ctx, tx, v.WorkspaceID, v.UserID, "view", id, "view.updated", p)
		}
		return nil
	})
	return view, err
}

// DeleteView deletes a view the caller may edit.
func (s *Store) DeleteView(ctx context.Context, v ViewCaller, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := getView(ctx, tx, v, id, true)
		if err != nil {
			return err
		}
		if !cur.CanEdit {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `DELETE FROM saved_views WHERE id = $1`, id); err != nil {
			return err
		}
		if cur.Shared {
			return appendEvent(ctx, tx, v.WorkspaceID, v.UserID, "view", id, "view.deleted", viewPayload(cur))
		}
		return nil
	})
}

func viewPayload(v View) map[string]any {
	return map[string]any{"name": v.Name, "kind": v.Kind, "shared": v.Shared, "project_id": v.ProjectID, "owner_id": v.OwnerID}
}

func viewChanges(a, b View) []string {
	var out []string
	if a.Name != b.Name {
		out = append(out, "name")
	}
	if a.Kind != b.Kind {
		out = append(out, "kind")
	}
	if a.Shared != b.Shared {
		out = append(out, "shared")
	}
	if !equalConfig(a.Config, b.Config) {
		out = append(out, "config")
	}
	return out
}

func equalConfig(a, b ViewConfig) bool {
	return a.Filters.Q == b.Filters.Q && a.Filters.Assignee == b.Filters.Assignee && a.Filters.Due == b.Filters.Due &&
		slices.Equal(a.Filters.Status, b.Filters.Status) && mapsEqual(a.Filters.Fields, b.Filters.Fields) &&
		slices.Equal(a.Columns, b.Columns) && a.Sort == b.Sort && a.GroupBy == b.GroupBy
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// validateView normalizes and checks a view against the workspace's
// statuses, fields and members. Everything a client sends is checked here:
// the web UI's own checks are a convenience only.
func validateView(ctx context.Context, q querier, v ViewCaller, view *View) error {
	name, err := text("name", view.Name, 1, maxViewName)
	if err != nil {
		return err
	}
	view.Name = name
	if view.Kind != ViewTable && view.Kind != ViewBoard && view.Kind != ViewCalendar {
		return invalid("kind", "must be table, board or calendar")
	}

	c := &view.Config
	c.Sort = normSort(c.Sort)
	if !ValidSort(c.Sort) {
		return invalid("config.sort", SortError)
	}
	if _, err := resolveFieldSort(ctx, q, v.WorkspaceID, c.Sort, "config.sort"); err != nil {
		return err
	}
	if c.GroupBy == "" {
		c.GroupBy = "status"
	}
	if c.GroupBy != "status" && c.GroupBy != "category" {
		return invalid("config.group_by", "must be status or category")
	}

	f := &c.Filters
	if f.Q, err = text("config.filters.q", f.Q, 0, maxViewQuery); err != nil {
		return err
	}
	switch f.Due {
	case "", "overdue", "today", "week", "none":
	default:
		return invalid("config.filters.due", "must be overdue, today, week or none")
	}
	switch f.Assignee {
	case "", "me", "none":
	default:
		id, err := uuid.Parse(f.Assignee)
		if err != nil {
			return invalid("config.filters.assignee", "must be a user id, me or none")
		}
		ok, err := isMember(ctx, q, v.WorkspaceID, id)
		if err != nil {
			return err
		}
		if !ok {
			return invalid("config.filters.assignee", "user is not a member of this workspace")
		}
		f.Assignee = id.String()
	}
	if len(f.Status) > maxViewStatuses {
		return invalid("config.filters.status", "at most %d statuses", maxViewStatuses)
	}
	slices.SortFunc(f.Status, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	f.Status = slices.Compact(f.Status)
	if len(f.Status) > 0 {
		var n int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM statuses WHERE workspace_id = $1 AND id = ANY($2)`,
			v.WorkspaceID, f.Status).Scan(&n); err != nil {
			return err
		}
		if n != len(f.Status) {
			return invalid("config.filters.status", "unknown status")
		}
	}
	if len(f.Fields) == 0 {
		f.Fields = nil
	} else if _, err := fieldFilterConditions(ctx, q, v.WorkspaceID, f.Fields); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return invalid("config.filters.fields."+strings.TrimPrefix(ve.Field, "field."), "%s", ve.Message)
		}
		return err
	}

	if len(c.Columns) > maxViewColumns {
		return invalid("config.columns", "at most %d columns", maxViewColumns)
	}
	seen := map[string]bool{}
	var fieldKeys []string
	for _, col := range c.Columns {
		if seen[col] {
			return invalid("config.columns", "duplicate column %q", col)
		}
		seen[col] = true
		if key, ok := strings.CutPrefix(col, "field."); ok {
			fieldKeys = append(fieldKeys, key)
			continue
		}
		if !slices.Contains(builtinColumns, col) {
			return invalid("config.columns", "unknown column %q", col)
		}
	}
	if len(fieldKeys) > 0 {
		var n int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM field_definitions
			WHERE workspace_id = $1 AND key = ANY($2) AND archived_at IS NULL`, v.WorkspaceID, fieldKeys).Scan(&n); err != nil {
			return err
		}
		if n != len(fieldKeys) {
			return invalid("config.columns", "unknown or archived field column")
		}
	}
	if c.Columns == nil {
		c.Columns = []string{}
	}
	return nil
}
