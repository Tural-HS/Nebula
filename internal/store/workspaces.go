package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
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
	// Timezone (IANA) defines "today" for due dates and digests (ADR-0009).
	Timezone string `json:"timezone"`
	// OverdueDigestHour is the local hour of the daily overdue digest; nil = off.
	OverdueDigestHour *int `json:"overdue_digest_hour"`
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

// Workspace templates: the language of the default statuses, item types and
// notification rules a new workspace starts with (ADR-0020).
const (
	TemplateEnglish     = "en"
	TemplateAzerbaijani = "az"
	TemplateTurkish     = "tr"
	TemplateRussian     = "ru"
)

// CreateWorkspace creates a workspace owned by owner and installs the default
// template in the given language ("" means English, the historical default).
func (s *Store) CreateWorkspace(ctx context.Context, owner uuid.UUID, slug, name, template string) (Workspace, error) {
	return s.CreateWorkspaceOfKind(ctx, owner, slug, name, template, KindGeneral)
}

// CreateWorkspaceOfKind is CreateWorkspace with a team kind (ADR-0033): the
// language template decides the names, the kind what is pre-configured. ""
// means KindGeneral. Everything is created in one transaction.
func (s *Store) CreateWorkspaceOfKind(ctx context.Context, owner uuid.UUID, slug, name, template, kind string) (Workspace, error) {
	tpl, ok := workspaceTemplates[template]
	if !ok {
		return Workspace{}, invalid("template", "must be one of az, en, tr, ru")
	}
	if kind == "" {
		kind = KindGeneral
	}
	spec, ok := workspaceKinds[kind]
	if !ok {
		return Workspace{}, invalid("kind", "must be one of %s", strings.Join(WorkspaceKinds(), ", "))
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRe.MatchString(slug) {
		return Workspace{}, invalid("slug", "must be 2-63 characters: lowercase letters, digits and hyphens, not starting with a hyphen")
	}
	name, err := text("name", name, 1, 200)
	if err != nil {
		return Workspace{}, err
	}

	ws := Workspace{ID: newID(), Slug: slug, Name: name, Role: RoleOwner, Timezone: tpl.timezone}
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO workspaces (id, slug, name, timezone) VALUES ($1, $2, $3, $4) RETURNING created_at, overdue_digest_hour`,
			ws.ID, slug, name, tpl.timezone).Scan(&ws.CreatedAt, &ws.OverdueDigestHour)
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
		lang := tpl.key
		if spec.statuses != nil {
			tpl = spec.localize(tpl, lang)
		}
		wf, err := installDefaultTemplate(ctx, tx, ws.ID, tpl)
		if err != nil {
			return err
		}
		payload := map[string]any{"slug": slug, "name": name, "template": lang}
		if kind != KindGeneral {
			payload["kind"] = kind
		}
		// workspace.created comes first: the kind's own events follow it.
		if err := appendEvent(ctx, tx, ws.ID, owner, "workspace", ws.ID, "workspace.created", payload); err != nil {
			return err
		}
		if kind == KindGeneral {
			return nil
		}
		return installKind(ctx, tx, kindTarget{wsID: ws.ID, wsSlug: slug, owner: owner, lang: lang, workflow: wf}, spec)
	})
	return ws, err
}

type templateStatus struct {
	name, category string
	initial        bool
}

// workspaceTemplate is the starting configuration of a new workspace. Item
// type keys stay English identifiers (they appear in URLs, the API and
// automation rules); only display names are translated.
type workspaceTemplate struct {
	key       string
	timezone  string
	workflow  string
	statuses  []templateStatus
	itemTypes [][2]string // key, name
	rules     func() []RuleInput
}

var workspaceTemplates = map[string]workspaceTemplate{
	"": templateEN, TemplateEnglish: templateEN, TemplateAzerbaijani: templateAZ,
	TemplateTurkish: templateTR, TemplateRussian: templateRU,
}

var templateEN = workspaceTemplate{
	key: TemplateEnglish, timezone: "UTC", workflow: "Default",
	statuses: []templateStatus{
		{"To Do", "todo", true},
		{"In Progress", "in_progress", false},
		{"Done", "done", false},
		{"Canceled", "canceled", false},
	},
	itemTypes: [][2]string{{"task", "Task"}, {"bug", "Bug"}},
	rules:     defaultRules,
}

// templateAZ matches how FinTez teams name their columns on Mattermost Boards
// ("Görüləcək → İcrada → Yoxlamada → Tamamlandı").
var templateAZ = workspaceTemplate{
	key: TemplateAzerbaijani, timezone: "Asia/Baku", workflow: "Əsas",
	statuses: []templateStatus{
		{"Görüləcək", "todo", true},
		{"İcrada", "in_progress", false},
		{"Yoxlamada", "in_progress", false},
		{"Tamamlandı", "done", false},
		{"Ləğv edildi", "canceled", false},
	},
	itemTypes: [][2]string{{"task", "Tapşırıq"}, {"bug", "Xəta"}},
	rules:     defaultRulesAZ,
}

// templateTR and templateRU follow the az layout (with a review column).
// Timezones are a starting point; admins change them in Settings.
var templateTR = workspaceTemplate{
	key: TemplateTurkish, timezone: "Europe/Istanbul", workflow: "Varsayılan",
	statuses: []templateStatus{
		{"Yapılacak", "todo", true},
		{"Devam ediyor", "in_progress", false},
		{"İncelemede", "in_progress", false},
		{"Tamamlandı", "done", false},
		{"İptal edildi", "canceled", false},
	},
	itemTypes: [][2]string{{"task", "Görev"}, {"bug", "Hata"}},
	rules:     defaultRulesTR,
}

var templateRU = workspaceTemplate{
	key: TemplateRussian, timezone: "UTC", workflow: "Основной",
	statuses: []templateStatus{
		{"К выполнению", "todo", true},
		{"В работе", "in_progress", false},
		{"На проверке", "in_progress", false},
		{"Готово", "done", false},
		{"Отменено", "canceled", false},
	},
	itemTypes: [][2]string{{"task", "Задача"}, {"bug", "Ошибка"}},
	rules:     defaultRulesRU,
}

// installedWorkflow is the workflow a template created, with its status IDs
// in template order.
type installedWorkflow struct {
	id       uuid.UUID
	statuses []uuid.UUID
}

// installDefaultTemplate gives every new workspace a usable workflow and item types.
func installDefaultTemplate(ctx context.Context, tx pgx.Tx, wsID uuid.UUID, tpl workspaceTemplate) (installedWorkflow, error) {
	wf := installedWorkflow{id: newID()}
	if _, err := tx.Exec(ctx, `INSERT INTO workflows (id, workspace_id, name) VALUES ($1, $2, $3)`, wf.id, wsID, tpl.workflow); err != nil {
		return wf, err
	}
	for i, st := range tpl.statuses {
		id := newID()
		if _, err := tx.Exec(ctx, `INSERT INTO statuses (id, workspace_id, workflow_id, name, category, position, is_initial)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, wsID, wf.id, st.name, st.category, i, st.initial); err != nil {
			return wf, err
		}
		wf.statuses = append(wf.statuses, id)
	}
	for _, it := range tpl.itemTypes {
		if _, err := tx.Exec(ctx, `INSERT INTO item_types (id, workspace_id, workflow_id, key, name) VALUES ($1, $2, $3, $4, $5)`,
			newID(), wsID, wf.id, it[0], it[1]); err != nil {
			return wf, err
		}
	}
	for _, r := range tpl.rules() {
		if _, err := insertRule(ctx, tx, wsID, uuid.Nil, r); err != nil {
			return wf, err
		}
	}
	return wf, nil
}

const workspaceSelect = `SELECT w.id, w.slug, w.name, m.role, w.created_at, w.timezone, w.overdue_digest_hour
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

// ListMembers returns active members of a workspace ordered by name.
func (s *Store) ListMembers(ctx context.Context, wsID uuid.UUID) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.email, u.display_name, m.role
		FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 AND u.is_active ORDER BY u.display_name, u.id`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
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

// lockMembers locks all memberships of a workspace so owner counts can't race.
func lockMembers(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := tx.Query(ctx, `SELECT user_id, role FROM workspace_members WHERE workspace_id = $1 FOR UPDATE`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			return nil, err
		}
		roles[id] = role
	}
	return roles, rows.Err()
}

func ownerCount(roles map[uuid.UUID]string) int {
	n := 0
	for _, r := range roles {
		if r == RoleOwner {
			n++
		}
	}
	return n
}

var errLastOwner = &ConflictError{Code: "last_owner", Message: "a workspace must keep at least one owner"}

// UpdateMemberRole changes a member's role. Only owners may grant the owner
// role or change another owner; the last owner cannot be demoted.
func (s *Store) UpdateMemberRole(ctx context.Context, wsID, actorID uuid.UUID, actorRole string, userID uuid.UUID, role string) (Member, error) {
	if _, ok := roleRank[role]; !ok {
		return Member{}, invalid("role", "must be one of guest, member, admin, owner")
	}
	var m Member
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		roles, err := lockMembers(ctx, tx, wsID)
		if err != nil {
			return err
		}
		cur, ok := roles[userID]
		if !ok {
			return ErrNotFound
		}
		if (role == RoleOwner || cur == RoleOwner) && actorRole != RoleOwner {
			return ErrForbidden
		}
		if cur == RoleOwner && role != RoleOwner && ownerCount(roles) <= 1 {
			return errLastOwner
		}
		if cur != role {
			if _, err := tx.Exec(ctx, `UPDATE workspace_members SET role = $1 WHERE workspace_id = $2 AND user_id = $3`, role, wsID, userID); err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, wsID, actorID, "member", userID, "member.role_changed",
				map[string]any{"changes": map[string]any{"role": diff(cur, role)}}); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `SELECT u.id, u.email, u.display_name, m.role FROM workspace_members m
			JOIN users u ON u.id = m.user_id WHERE m.workspace_id = $1 AND m.user_id = $2`, wsID, userID).
			Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role)
	})
	return m, err
}

// RemoveMember removes someone from a workspace (or lets a member leave).
// Admins may remove non-owners; only owners may remove owners; the last
// owner cannot leave. Items stay assigned for history; reassign as needed.
func (s *Store) RemoveMember(ctx context.Context, wsID, actorID uuid.UUID, actorRole string, userID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		roles, err := lockMembers(ctx, tx, wsID)
		if err != nil {
			return err
		}
		cur, ok := roles[userID]
		if !ok {
			return ErrNotFound
		}
		self := actorID == userID
		if !self && !RoleAtLeast(actorRole, RoleAdmin) {
			return ErrForbidden
		}
		if cur == RoleOwner && !self && actorRole != RoleOwner {
			return ErrForbidden
		}
		if cur == RoleOwner && ownerCount(roles) <= 1 {
			return errLastOwner
		}
		if _, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, wsID, userID); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "member", userID, "member.removed", map[string]any{"role": cur})
	})
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

// UpdateWorkspaceParams is a partial update of workspace settings.
type UpdateWorkspaceParams struct {
	Name              opt.Field[string]
	Timezone          opt.Field[string]
	OverdueDigestHour opt.Field[int] // null turns the digest off
}

// UpdateWorkspace changes name, time zone or digest hour (admin+, checked by
// the caller) and records a workspace.updated event.
func (s *Store) UpdateWorkspace(ctx context.Context, wsID, actorID uuid.UUID, p UpdateWorkspaceParams) error {
	if (p.Name.Set && p.Name.Null) || (p.Timezone.Set && p.Timezone.Null) {
		return invalid("body", "name and timezone cannot be null")
	}
	var name string
	if p.Name.Set {
		var err error
		if name, err = text("name", p.Name.Value, 1, 200); err != nil {
			return err
		}
	}
	tz := strings.TrimSpace(p.Timezone.Value)
	if p.Timezone.Set {
		// Both Go and PostgreSQL must know the zone: Go for the scheduler,
		// PostgreSQL for "today" in queries.
		if _, err := time.LoadLocation(tz); err != nil || tz == "" || tz == "Local" || len(tz) > 64 {
			return invalid("timezone", "must be an IANA time zone such as Asia/Baku")
		}
	}
	if p.OverdueDigestHour.Set && !p.OverdueDigestHour.Null && (p.OverdueDigestHour.Value < 0 || p.OverdueDigestHour.Value > 23) {
		return invalid("overdue_digest_hour", "must be 0 to 23 or null")
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		if p.Timezone.Set {
			var known bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = $1)`, tz).Scan(&known); err != nil {
				return err
			}
			if !known {
				return invalid("timezone", "must be an IANA time zone such as Asia/Baku")
			}
		}
		var curName, curTZ string
		var curHour *int
		if err := tx.QueryRow(ctx, `SELECT name, timezone, overdue_digest_hour FROM workspaces WHERE id = $1 FOR UPDATE`, wsID).
			Scan(&curName, &curTZ, &curHour); err != nil {
			return err
		}
		changes := map[string]any{}
		newName, newTZ, newHour := curName, curTZ, curHour
		if p.Name.Set && name != curName {
			changes["name"] = diff(curName, name)
			newName = name
		}
		if p.Timezone.Set && tz != curTZ {
			changes["timezone"] = diff(curTZ, tz)
			newTZ = tz
		}
		if p.OverdueDigestHour.Set {
			var h *int
			if !p.OverdueDigestHour.Null {
				v := p.OverdueDigestHour.Value // range checked above
				h = &v
			}
			if !equalPtr(curHour, h) {
				changes["overdue_digest_hour"] = diff(curHour, h)
				newHour = h
			}
		}
		if len(changes) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE workspaces SET name = $1, timezone = $2, overdue_digest_hour = $3 WHERE id = $4`,
			newName, newTZ, newHour, wsID); err != nil {
			return err
		}
		if _, ok := changes["timezone"]; ok {
			// today() is the local date (ADR-0030).
			if err := requestFormulaRecompute(ctx, tx, wsID, "volatile"); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, wsID, actorID, "workspace", wsID, "workspace.updated", map[string]any{"changes": changes})
	})
}
