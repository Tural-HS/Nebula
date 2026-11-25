package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Intake forms (ADR-0032): an admin publishes a form for a project; signed-in
// requesters submit it and every submission becomes an item. Requesters never
// read the item itself, only their own submission (MyRequest).

// Form audiences.
const (
	FormAudienceMembers  = "members"  // members of the form's workspace, any role (guests too)
	FormAudienceAccounts = "accounts" // anyone who can sign in to this Nebula
)

// Built-in form field keys besides custom fields.
const (
	FormFieldDescription = "description"
	FormFieldDueDate     = "due_date"
)

const (
	maxFormsPerWorkspace = 100
	maxFormFields        = 50
	maxFormLabel         = 200
	maxFormHelp          = 1000
	maxFormIntro         = 5000
	maxRequestMessage    = 5000
	// MaxFormFiles bounds the files of one submission (forms.max_files CHECK).
	MaxFormFiles = 10
)

var (
	formSlugRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)
	placeholderRe  = regexp.MustCompile(`\{\{\s*([a-z0-9_.]*)\s*\}\}`)
	formFieldTypes = map[string]bool{ // what a requester can type in; no user pickers, nothing computed
		FieldText: true, FieldLongText: true, FieldNumber: true, FieldDate: true, FieldDateTime: true,
		FieldBoolean: true, FieldSelect: true, FieldMultiSelect: true, FieldURL: true, FieldEmail: true,
	}
)

// FormField is one entry of a form: a custom field key, "description" or "due_date".
type FormField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Help     string `json:"help,omitempty"`
	Required bool   `json:"required"`
}

// Form is a form definition as admins see it.
type Form struct {
	ID                uuid.UUID   `json:"id"`
	Slug              string      `json:"slug"`
	ProjectKey        string      `json:"project_key"`
	ItemType          ItemTypeRef `json:"item_type"`
	Title             string      `json:"title"`
	Intro             *string     `json:"intro"`
	TitleTemplate     string      `json:"title_template"`
	Fields            []FormField `json:"fields"`
	DefaultAssigneeID *uuid.UUID  `json:"default_assignee_id"`
	InitialStatusID   *uuid.UUID  `json:"initial_status_id"`
	Audience          string      `json:"audience"`
	Enabled           bool        `json:"enabled"`
	MaxFiles          int         `json:"max_files"`
	Submissions       int64       `json:"submissions"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// FormInput is a complete form definition to create or replace.
type FormInput struct {
	Slug              string
	Title             string
	Intro             *string
	ItemType          string // item type key
	TitleTemplate     string
	Fields            []FormField
	DefaultAssigneeID *uuid.UUID
	InitialStatusID   *uuid.UUID
	Audience          string
	Enabled           bool
	MaxFiles          int
}

const formSelect = `SELECT f.id, f.slug, p.key, t.id, t.key, t.name, f.title, f.intro, f.title_template, f.fields,
	f.default_assignee_id, f.initial_status_id, f.audience, f.is_enabled, f.max_files,
	(SELECT count(*) FROM form_submissions s WHERE s.form_id = f.id), f.created_at, f.updated_at
	FROM forms f JOIN projects p ON p.id = f.project_id JOIN item_types t ON t.id = f.item_type_id`

func scanForm(row pgx.Row) (Form, error) {
	var f Form
	err := row.Scan(&f.ID, &f.Slug, &f.ProjectKey, &f.ItemType.ID, &f.ItemType.Key, &f.ItemType.Name, &f.Title, &f.Intro,
		&f.TitleTemplate, &f.Fields, &f.DefaultAssigneeID, &f.InitialStatusID, &f.Audience, &f.Enabled, &f.MaxFiles,
		&f.Submissions, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, ErrNotFound
	}
	if f.Fields == nil {
		f.Fields = []FormField{}
	}
	return f, err
}

// ListForms returns a project's forms, oldest first.
func (s *Store) ListForms(ctx context.Context, wsID, projectID uuid.UUID) ([]Form, error) {
	rows, err := s.pool.Query(ctx, formSelect+` WHERE f.workspace_id = $1 AND f.project_id = $2 ORDER BY f.created_at, f.id`,
		wsID, projectID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Form, error) { return scanForm(r) })
}

// GetForm returns one form of a project.
func (s *Store) GetForm(ctx context.Context, wsID, projectID, id uuid.UUID) (Form, error) {
	return scanForm(s.pool.QueryRow(ctx, formSelect+` WHERE f.workspace_id = $1 AND f.project_id = $2 AND f.id = $3`,
		wsID, projectID, id))
}

// checkedForm is a validated FormInput.
type checkedForm struct {
	in     FormInput
	typeID uuid.UUID
}

// checkForm validates a definition against the project and item type as they
// are now. Submissions check again: the schema may change after a save.
func checkForm(ctx context.Context, q querier, wsID, projectID uuid.UUID, in FormInput) (checkedForm, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !formSlugRe.MatchString(in.Slug) {
		return checkedForm{}, invalid("slug", "must be 3-63 characters: lowercase letters, digits and hyphens, starting with a letter or digit")
	}
	var err error
	if in.Title, err = text("title", in.Title, 1, maxFormLabel); err != nil {
		return checkedForm{}, err
	}
	if in.Intro, err = optionalText("intro", in.Intro, maxFormIntro); err != nil {
		return checkedForm{}, err
	}
	switch in.Audience {
	case "":
		in.Audience = FormAudienceMembers
	case FormAudienceMembers, FormAudienceAccounts:
	default:
		return checkedForm{}, invalid("audience", "must be members or accounts")
	}
	if in.MaxFiles < 0 || in.MaxFiles > MaxFormFiles {
		return checkedForm{}, invalid("max_files", "must be between 0 and %d", MaxFormFiles)
	}
	if strings.TrimSpace(in.ItemType) == "" {
		return checkedForm{}, invalid("item_type", "is required")
	}
	c := newItemCache(wsID, nil)
	typ, err := c.itemType(ctx, q, in.ItemType)
	if err != nil {
		return checkedForm{}, relabel(err, "type", "item_type")
	}
	if len(in.Fields) > maxFormFields {
		return checkedForm{}, invalid("fields", "at most %d fields", maxFormFields)
	}
	keys := make([]string, 0, len(in.Fields))
	seen := map[string]bool{}
	for i := range in.Fields {
		f := &in.Fields[i]
		path := fmt.Sprintf("fields[%d]", i)
		f.Key = strings.TrimSpace(f.Key)
		if seen[f.Key] {
			return checkedForm{}, invalid(path+".key", "%q is on the form twice", f.Key)
		}
		seen[f.Key] = true
		if f.Label, err = text(path+".label", f.Label, 1, maxFormLabel); err != nil {
			return checkedForm{}, err
		}
		if f.Help, err = text(path+".help", f.Help, 0, maxFormHelp); err != nil {
			return checkedForm{}, err
		}
		if f.Key != FormFieldDescription && f.Key != FormFieldDueDate {
			keys = append(keys, f.Key)
		}
	}
	schema, err := loadFieldSchema(ctx, q, wsID, typ.ID, keys)
	if err != nil {
		return checkedForm{}, err
	}
	for i, f := range in.Fields {
		if f.Key == FormFieldDescription || f.Key == FormFieldDueDate {
			continue
		}
		path := fmt.Sprintf("fields[%d].key", i)
		sf, ok := schema[f.Key]
		switch {
		case !ok:
			return checkedForm{}, invalid(path, "unknown field %q", f.Key)
		case sf.def.Archived:
			return checkedForm{}, invalid(path, "field %q is archived", f.Key)
		case !sf.bound:
			return checkedForm{}, invalid(path, "field %q is not used by item type %q", f.Key, typ.Key)
		case !formFieldTypes[sf.def.Type]:
			return checkedForm{}, invalid(path, "%s fields cannot be on a form", sf.def.Type)
		}
	}
	// Every field the item type requires must be asked, and asked as required:
	// otherwise no submission could ever succeed.
	for k, sf := range schema {
		if !sf.bound || !sf.required || sf.def.Archived {
			continue
		}
		i := slices.IndexFunc(in.Fields, func(f FormField) bool { return f.Key == k })
		if i < 0 {
			return checkedForm{}, invalid("fields", "the item type requires %q: add it to the form", k)
		}
		if !in.Fields[i].Required {
			return checkedForm{}, invalid(fmt.Sprintf("fields[%d].required", i), "%q is required by the item type", k)
		}
	}
	if in.TitleTemplate, err = text("title_template", in.TitleTemplate, 1, maxTitleLen); err != nil {
		return checkedForm{}, err
	}
	if err := checkTitleTemplate(in.TitleTemplate, seen); err != nil {
		return checkedForm{}, err
	}
	if in.InitialStatusID != nil {
		// Same rule as creating an item: not straight into a status that needs approval.
		if _, err := c.status(ctx, q, typ.workflowID, in.InitialStatusID); err != nil {
			return checkedForm{}, relabel(err, "status_id", "initial_status_id")
		}
	}
	if in.DefaultAssigneeID != nil {
		ok, err := canSeeProject(ctx, q, wsID, projectID, *in.DefaultAssigneeID)
		if err != nil {
			return checkedForm{}, err
		}
		if !ok {
			return checkedForm{}, invalid("default_assignee_id", "user is not a member of this workspace or cannot see this project")
		}
	}
	return checkedForm{in: in, typeID: typ.ID}, nil
}

// checkTitleTemplate allows {{field.<key>}} for keys on the form (custom
// fields only), {{requester}} and {{form}}; any other {{…}} is a mistake.
func checkTitleTemplate(tpl string, onForm map[string]bool) error {
	rest := placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		if name == "requester" || name == "form" {
			return ""
		}
		if k, ok := strings.CutPrefix(name, "field."); ok && onForm[k] && k != FormFieldDescription && k != FormFieldDueDate {
			return ""
		}
		return "\x00" + name + "\x00"
	})
	if i := strings.IndexByte(rest, 0); i >= 0 {
		name := rest[i+1:]
		name = name[:strings.IndexByte(name, 0)]
		return invalid("title_template", "unknown placeholder {{%s}}: use {{field.<key>}} of a field on the form, {{requester}} or {{form}}", name)
	}
	if strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
		return invalid("title_template", "unbalanced {{ }}")
	}
	return nil
}

// relabel renames the field of a validation error (shared validators speak
// of item fields; forms name them differently).
func relabel(err error, from, to string) error {
	var ve *ValidationError
	if errors.As(err, &ve) && ve.Field == from {
		return &ValidationError{Field: to, Message: ve.Message}
	}
	return err
}

func formEventPayload(projectID uuid.UUID, f Form) map[string]any {
	return map[string]any{"project_id": projectID, "slug": f.Slug, "title": f.Title, "audience": f.Audience,
		"enabled": f.Enabled, "item_type": f.ItemType.Key, "fields": f.Fields, "max_files": f.MaxFiles,
		"initial_status_id": f.InitialStatusID, "default_assignee_id": f.DefaultAssigneeID}
}

// CreateForm adds a form to a project.
func (s *Store) CreateForm(ctx context.Context, wsID, projectID, actorID uuid.UUID, in FormInput) (Form, error) {
	var out Form
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createForm(ctx, tx, wsID, projectID, actorID, in)
		return err
	})
	return out, err
}

// createForm is CreateForm inside the caller's transaction (workspace kinds, ADR-0033).
func createForm(ctx context.Context, tx pgx.Tx, wsID, projectID, actorID uuid.UUID, in FormInput) (Form, error) {
	var f Form
	err := func() error {
		cf, err := checkForm(ctx, tx, wsID, projectID, in)
		if err != nil {
			return err
		}
		// Serializes the per-workspace limit.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR UPDATE`, wsID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM forms WHERE workspace_id = $1`, wsID).Scan(&n); err != nil {
			return err
		}
		if n >= maxFormsPerWorkspace {
			return &ConflictError{Code: "limit_reached", Message: fmt.Sprintf("a workspace can have at most %d forms", maxFormsPerWorkspace)}
		}
		id := newID()
		c := cf.in
		_, err = tx.Exec(ctx, `INSERT INTO forms (id, workspace_id, project_id, item_type_id, slug, title, intro,
				title_template, fields, default_assignee_id, initial_status_id, audience, is_enabled, max_files, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			id, wsID, projectID, cf.typeID, c.Slug, c.Title, c.Intro, c.TitleTemplate, c.Fields, c.DefaultAssigneeID,
			c.InitialStatusID, c.Audience, c.Enabled, c.MaxFiles, actorID)
		if isUniqueViolation(err, "forms_slug_key") {
			return &ConflictError{Code: "slug_taken", Message: "another form already uses this address"}
		}
		if err != nil {
			return err
		}
		if f, err = scanForm(tx.QueryRow(ctx, formSelect+` WHERE f.id = $1`, id)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "form", id, "form.created", formEventPayload(projectID, f))
	}()
	return f, err
}

// UpdateForm replaces a form's definition. Existing submissions keep their
// snapshot; the next submission uses the new definition.
func (s *Store) UpdateForm(ctx context.Context, wsID, projectID, id, actorID uuid.UUID, in FormInput) (Form, error) {
	var f Form
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM forms WHERE workspace_id = $1 AND project_id = $2 AND id = $3 FOR UPDATE`,
			wsID, projectID, id).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		cf, err := checkForm(ctx, tx, wsID, projectID, in)
		if err != nil {
			return err
		}
		c := cf.in
		_, err = tx.Exec(ctx, `UPDATE forms SET item_type_id = $2, slug = $3, title = $4, intro = $5, title_template = $6,
				fields = $7, default_assignee_id = $8, initial_status_id = $9, audience = $10, is_enabled = $11, max_files = $12
			WHERE id = $1`,
			id, cf.typeID, c.Slug, c.Title, c.Intro, c.TitleTemplate, c.Fields, c.DefaultAssigneeID, c.InitialStatusID,
			c.Audience, c.Enabled, c.MaxFiles)
		if isUniqueViolation(err, "forms_slug_key") {
			return &ConflictError{Code: "slug_taken", Message: "another form already uses this address"}
		}
		if err != nil {
			return err
		}
		if f, err = scanForm(tx.QueryRow(ctx, formSelect+` WHERE f.id = $1`, id)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "form", id, "form.updated", formEventPayload(projectID, f))
	})
	return f, err
}

// DeleteForm removes a form; its submissions (and items) stay.
func (s *Store) DeleteForm(ctx context.Context, wsID, projectID, id, actorID uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var slug, title string
		err := tx.QueryRow(ctx, `DELETE FROM forms WHERE workspace_id = $1 AND project_id = $2 AND id = $3 RETURNING slug, title`,
			wsID, projectID, id).Scan(&slug, &title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "form", id, "form.deleted",
			map[string]any{"project_id": projectID, "slug": slug, "title": title})
	})
}

// ---------------------------------------------------------------------------
// Filling in a form
// ---------------------------------------------------------------------------

// PublicForm is what a requester sees: labels, help and input types, never
// the project, other items or the member directory.
type PublicForm struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Intro     *string   `json:"intro"`
	Workspace string    `json:"workspace"` // display name
	Open      bool      `json:"open"`      // accepting submissions
	// ClosedReason says why a closed form is closed: "archived" (its
	// project or workspace, ADR-0039) or "disabled"; empty when open.
	ClosedReason string            `json:"closed_reason,omitempty"`
	MaxFiles     int               `json:"max_files"`
	Fields       []PublicFormField `json:"fields"`
	// CanManage: the viewer is an admin of the form's workspace (preview,
	// link to the builder).
	CanManage bool `json:"can_manage"`
}

// PublicFormField is one input of a PublicForm.
type PublicFormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Help     string   `json:"help,omitempty"`
	Required bool     `json:"required"`
	Type     string   `json:"type"`
	Choices  []Choice `json:"choices,omitempty"` // active choices of select fields
}

// formRow is a form with what a submission needs.
type formRow struct {
	Form
	workspaceID, projectID uuid.UUID
	workspaceName          string
	open                   bool
	archived               bool   // the form's project or workspace is archived (ADR-0039)
	role                   string // the viewer's workspace role; "" = not a member
}

// loadFormFor resolves a form by id or slug for userID, applying the
// audience: a form the user may not fill in does not exist for them.
func loadFormFor(ctx context.Context, q querier, ref string, userID uuid.UUID, lock bool) (formRow, error) {
	ref = strings.TrimSpace(ref)
	var byID *uuid.UUID
	if id, err := uuid.Parse(ref); err == nil {
		byID = &id
	} else if !formSlugRe.MatchString(strings.ToLower(ref)) {
		return formRow{}, ErrNotFound
	}
	sql := `SELECT f.id, f.slug, p.key, t.id, t.key, t.name, f.title, f.intro, f.title_template, f.fields,
		f.default_assignee_id, f.initial_status_id, f.audience, f.is_enabled, f.max_files, f.created_at, f.updated_at,
		f.workspace_id, f.project_id, w.name,
		f.is_enabled AND p.archived_at IS NULL AND w.archived_at IS NULL AND t.archived_at IS NULL,
		p.archived_at IS NOT NULL OR w.archived_at IS NOT NULL,
		COALESCE((SELECT m.role FROM workspace_members m WHERE m.workspace_id = f.workspace_id AND m.user_id = $3), '')
		FROM forms f JOIN workspaces w ON w.id = f.workspace_id
		JOIN projects p ON p.id = f.project_id JOIN item_types t ON t.id = f.item_type_id
		WHERE ` + "(f.id = $1 OR ($1 IS NULL AND f.slug = $2))"
	if lock {
		sql += ` FOR SHARE OF f`
	}
	var r formRow
	f := &r.Form
	err := q.QueryRow(ctx, sql, byID, strings.ToLower(ref), userID).Scan(&f.ID, &f.Slug, &f.ProjectKey, &f.ItemType.ID,
		&f.ItemType.Key, &f.ItemType.Name, &f.Title, &f.Intro, &f.TitleTemplate, &f.Fields, &f.DefaultAssigneeID,
		&f.InitialStatusID, &f.Audience, &f.Enabled, &f.MaxFiles, &f.CreatedAt, &f.UpdatedAt,
		&r.workspaceID, &r.projectID, &r.workspaceName, &r.open, &r.archived, &r.role)
	if errors.Is(err, pgx.ErrNoRows) {
		return formRow{}, ErrNotFound
	}
	if err != nil {
		return formRow{}, err
	}
	if r.Audience != FormAudienceAccounts && r.role == "" {
		return formRow{}, ErrNotFound // same answer as a form that does not exist
	}
	return r, nil
}

// formInputs pairs the form's entries with the definitions they ask for.
// Entries whose field was archived, unbound or retyped since the form was
// saved are dropped: nobody could fill them in.
func formInputs(ctx context.Context, q querier, r formRow) ([]PublicFormField, map[string]FieldDefinition, error) {
	keys := []string{}
	for _, f := range r.Fields {
		if f.Key != FormFieldDescription && f.Key != FormFieldDueDate {
			keys = append(keys, f.Key)
		}
	}
	schema, err := loadFieldSchema(ctx, q, r.workspaceID, r.ItemType.ID, keys)
	if err != nil {
		return nil, nil, err
	}
	defs := map[string]FieldDefinition{}
	out := make([]PublicFormField, 0, len(r.Fields))
	for _, f := range r.Fields {
		pf := PublicFormField{Key: f.Key, Label: f.Label, Help: f.Help, Required: f.Required}
		switch f.Key {
		case FormFieldDescription:
			pf.Type = FieldLongText
		case FormFieldDueDate:
			pf.Type = FieldDate
		default:
			sf, ok := schema[f.Key]
			if !ok || !sf.bound || sf.def.Archived || !formFieldTypes[sf.def.Type] {
				continue
			}
			pf.Type = sf.def.Type
			defs[f.Key] = sf.def
			for _, c := range sf.def.Options.Choices {
				if !c.Archived {
					pf.Choices = append(pf.Choices, Choice{ID: c.ID, Label: c.Label, Color: c.Color})
				}
			}
		}
		out = append(out, pf)
	}
	return out, defs, nil
}

// FormForSubmitter returns the form as userID may see it, or ErrNotFound.
// A closed form shows its title only, except to the workspace's admins.
func (s *Store) FormForSubmitter(ctx context.Context, ref string, userID uuid.UUID) (PublicForm, error) {
	r, err := loadFormFor(ctx, s.pool, ref, userID, false)
	if err != nil {
		return PublicForm{}, err
	}
	pf := PublicForm{ID: r.ID, Slug: r.Slug, Title: r.Title, Intro: r.Intro, Workspace: r.workspaceName, Open: r.open,
		MaxFiles: r.MaxFiles, Fields: []PublicFormField{}, CanManage: r.role != "" && RoleAtLeast(r.role, RoleAdmin)}
	if !r.open {
		pf.ClosedReason = "disabled"
		if r.archived {
			pf.ClosedReason = "archived"
		}
	}
	if !r.open && !pf.CanManage {
		pf.Intro, pf.MaxFiles = nil, 0
		return pf, nil
	}
	if pf.Fields, _, err = formInputs(ctx, s.pool, r); err != nil {
		return PublicForm{}, err
	}
	return pf, nil
}

// FormAcceptsFiles is the fail-fast check before upload bytes are read: the
// form exists for userID, is open, and takes this many files.
func (s *Store) FormAcceptsFiles(ctx context.Context, ref string, userID uuid.UUID) (wsID uuid.UUID, maxFiles int, err error) {
	r, err := loadFormFor(ctx, s.pool, ref, userID, false)
	if err != nil {
		return uuid.Nil, 0, err
	}
	if !r.open {
		return uuid.Nil, 0, errFormClosed(r.archived)
	}
	return r.workspaceID, r.MaxFiles, nil
}

func errFormClosed(archived bool) error {
	if archived {
		return &ConflictError{Code: "form_closed", Message: "this form belongs to an archived project and is not accepting submissions"}
	}
	return &ConflictError{Code: "form_closed", Message: "this form is not accepting submissions"}
}

func errFormMisconfigured(err error) error {
	return &ConflictError{Code: "form_misconfigured",
		Message: "this form cannot create items right now; ask its owners to check it (" + err.Error() + ")"}
}

// SubmittedFile is an uploaded blob, already stored, to attach to the new item.
type SubmittedFile struct {
	Filename, ContentType string
	Size                  int64
	SHA256                []byte
	StorageKey            string
}

// SubmitFormParams are the inputs of SubmitForm.
type SubmitFormParams struct {
	Ref           string
	SubmitterID   uuid.UUID
	SubmitterName string
	Values        map[string]json.RawMessage
	Files         []SubmittedFile
	Limits        AttachmentLimits
}

// Submission is the result of a successful submission.
type Submission struct {
	ID        uuid.UUID `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}

// Answer is one submitted value as the requester entered it, as text.
type Answer struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	Display string `json:"display"`
}

// withFormSource marks the events of a submission as source 'form': they are
// dispatched like a user's own changes (webhooks, automations, notifications).
func withFormSource(ctx context.Context) context.Context {
	return context.WithValue(ctx, sourceKey{}, eventSource{source: "form"})
}

// SubmitForm validates a submission with the item validators and creates the
// item (reporter = submitter), its files and the submission atomically.
func (s *Store) SubmitForm(ctx context.Context, p SubmitFormParams) (Submission, error) {
	var sub Submission
	ctx = withFormSource(ctx)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		r, err := loadFormFor(ctx, tx, p.Ref, p.SubmitterID, true)
		if err != nil {
			return err
		}
		if !r.open {
			return errFormClosed(r.archived)
		}
		if len(p.Files) > r.MaxFiles {
			if r.MaxFiles == 0 {
				return invalid("files", "this form does not take files")
			}
			return invalid("files", "at most %d files", r.MaxFiles)
		}
		inputs, defs, err := formInputs(ctx, tx, r)
		if err != nil {
			return err
		}
		cp, answers, display, err := formItemParams(r, inputs, defs, p.Values)
		if err != nil {
			return err
		}
		cp.ActorID = p.SubmitterID
		cp.Title = renderTitle(r.TitleTemplate, display, p.SubmitterName, r.Title)
		if r.DefaultAssigneeID != nil {
			// An assignee who lost access to the project is skipped, not an error
			// the requester cannot fix.
			ok, err := canSeeProject(ctx, tx, r.workspaceID, r.projectID, *r.DefaultAssigneeID)
			if err != nil {
				return err
			}
			if ok {
				cp.AssigneeID = r.DefaultAssigneeID
			}
		}
		subID := newID()
		item, err := createItemTx(ctx, tx, cp, map[string]any{"form_id": r.ID, "submission_id": subID})
		if err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				// Values were checked above: what is left is the form's setup
				// (a newly required field, a status that now needs approval, …).
				return errFormMisconfigured(err)
			}
			return err
		}
		for i, f := range p.Files {
			name, err := text(fmt.Sprintf("files[%d]", i), f.Filename, 1, 255)
			if err != nil {
				return err
			}
			if _, err := insertAttachmentTx(ctx, tx, CreateAttachmentParams{
				WorkspaceID: r.workspaceID, ProjectID: r.projectID, ActorID: p.SubmitterID,
				Filename: name, ContentType: f.ContentType, Size: f.Size, SHA256: f.SHA256, StorageKey: f.StorageKey,
				Limits: p.Limits,
			}, item.ID, name); err != nil {
				return err
			}
		}
		err = tx.QueryRow(ctx, `INSERT INTO form_submissions (id, workspace_id, form_id, item_id, submitter_id, form_title, title, answers)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING created_at`,
			subID, r.workspaceID, r.ID, item.ID, p.SubmitterID, r.Title, item.Title, answers).Scan(&sub.CreatedAt)
		if err != nil {
			return err
		}
		sub.ID, sub.Key, sub.Title = subID, item.Key, item.Title
		return nil
	})
	return sub, err
}

// formItemParams turns submitted values into item inputs, checking each with
// the item validators (errors name the form value: "values.<key>") and the
// form's own required flags. It returns the answer snapshot and, per custom
// field, the display text for the title template.
func formItemParams(r formRow, inputs []PublicFormField, defs map[string]FieldDefinition,
	values map[string]json.RawMessage) (CreateItemParams, []Answer, map[string]string, error) {
	cp := CreateItemParams{WorkspaceID: r.workspaceID, ProjectID: r.projectID, TypeKey: r.ItemType.Key,
		StatusID: r.InitialStatusID, Fields: map[string]json.RawMessage{}}
	byKey := map[string]PublicFormField{}
	for _, in := range inputs {
		byKey[in.Key] = in
	}
	for k := range values {
		if _, ok := byKey[k]; !ok {
			return cp, nil, nil, invalid("values."+k, "is not on this form")
		}
	}
	answers := []Answer{}
	display := map[string]string{}
	for _, in := range inputs {
		path := "values." + in.Key
		raw, given := values[in.Key]
		var v any
		if given {
			if err := json.Unmarshal(raw, &v); err != nil {
				return cp, nil, nil, invalid(path, "invalid JSON value")
			}
		}
		var shown string
		switch in.Key {
		case FormFieldDescription, FormFieldDueDate:
			var str *string
			if v != nil {
				s, ok := v.(string)
				if !ok {
					return cp, nil, nil, invalid(path, "must be a string")
				}
				str = &s
			}
			var err error
			if in.Key == FormFieldDescription {
				if str, err = optionalText(path, str, maxDescriptionLen); err != nil {
					return cp, nil, nil, err
				}
				cp.Description = str
			} else {
				if str != nil && strings.TrimSpace(*str) == "" {
					str = nil
				}
				if str, err = dueDateField(path, str); err != nil {
					return cp, nil, nil, err
				}
				cp.DueDate = str
			}
			if str != nil {
				shown = *str
			}
		default:
			def := defs[in.Key]
			if v != nil {
				val, empty, err := canonicalValue(def, v, path)
				if err != nil {
					return cp, nil, nil, err
				}
				if !empty {
					if in.Required && def.Type == FieldBoolean && val == false {
						return cp, nil, nil, invalid(path, "must be checked")
					}
					b, err := json.Marshal(val)
					if err != nil {
						return cp, nil, nil, err
					}
					cp.Fields[in.Key] = b
					shown = displayValue(def, val)
				}
			}
		}
		if shown == "" {
			if in.Required {
				return cp, nil, nil, invalid(path, "is required")
			}
			continue
		}
		display[in.Key] = shown
		answers = append(answers, Answer{Key: in.Key, Label: in.Label, Type: in.Type, Display: shown})
	}
	return cp, answers, display, nil
}

// displayValue is a canonical field value as plain text.
func displayValue(def FieldDefinition, v any) string {
	label := func(id string) string {
		for _, c := range def.Options.Choices {
			if c.ID == id {
				return c.Label
			}
		}
		return id
	}
	switch x := v.(type) {
	case string:
		if def.Type == FieldSelect {
			return label(x)
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "✓"
		}
		return "✗"
	case []string:
		parts := make([]string, len(x))
		for i, id := range x {
			parts[i] = label(id)
		}
		return strings.Join(parts, ", ")
	case []any:
		parts := make([]string, 0, len(x))
		for _, id := range x {
			if s, ok := id.(string); ok {
				parts = append(parts, label(s))
			}
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

// renderTitle fills the title template. Values are single-lined; an empty
// result falls back to the form title; the result fits an item title.
func renderTitle(tpl string, display map[string]string, requester, formTitle string) string {
	oneLine := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	out := placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		switch name {
		case "requester":
			return oneLine(requester)
		case "form":
			return oneLine(formTitle)
		}
		k, _ := strings.CutPrefix(name, "field.")
		return oneLine(display[k])
	})
	out = strings.TrimSpace(out)
	if out == "" {
		out = formTitle
	}
	if utf8.RuneCountInString(out) > maxTitleLen {
		out = strings.TrimSpace(string([]rune(out)[:maxTitleLen-1])) + "…"
	}
	return out
}

// ---------------------------------------------------------------------------
// My requests
// ---------------------------------------------------------------------------

// MyRequest is one of the requester's own submissions. Status is the item's
// current status (nil once the item was deleted: Removed).
type MyRequest struct {
	ID        uuid.UUID `json:"id"`
	FormTitle string    `json:"form_title"`
	Workspace string    `json:"workspace"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Status    *Status   `json:"status"`
	Removed   bool      `json:"removed"`
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt: the last change the requester can see (status, removal,
	// message); internal edits of the item do not count.
	UpdatedAt time.Time `json:"updated_at"`
	Unseen    bool      `json:"unseen"`
}

// MyRequestDetail adds what was submitted and the team's message.
type MyRequestDetail struct {
	MyRequest
	Answers   []Answer      `json:"answers"`
	Files     []RequestFile `json:"files"`
	Message   *string       `json:"message"`
	MessageAt *time.Time    `json:"message_at"`
}

// RequestFile is a file the requester attached when submitting.
type RequestFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// MyRequestCursor is the keyset position of the "My requests" list.
type MyRequestCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

const myRequestSelect = `SELECT s.id, s.form_title, w.name, p.key, i.number, s.title, st.id, st.name, st.category,
	i.deleted_at IS NOT NULL, s.created_at, GREATEST(s.created_at, s.status_changed_at, s.message_at),
	COALESCE(GREATEST(s.status_changed_at, s.message_at) > s.seen_at, false)
	FROM form_submissions s
	JOIN items i ON i.id = s.item_id
	JOIN projects p ON p.id = i.project_id
	JOIN statuses st ON st.id = i.status_id
	JOIN workspaces w ON w.id = s.workspace_id`

func scanMyRequest(row pgx.Row) (MyRequest, error) {
	var m MyRequest
	var number int64
	var projectKey string
	var st Status
	err := row.Scan(&m.ID, &m.FormTitle, &m.Workspace, &projectKey, &number, &m.Title, &st.ID, &st.Name, &st.Category,
		&m.Removed, &m.CreatedAt, &m.UpdatedAt, &m.Unseen)
	if errors.Is(err, pgx.ErrNoRows) {
		return MyRequest{}, ErrNotFound
	}
	m.Key = itemKey(projectKey, number)
	if !m.Removed {
		m.Status = &st
	}
	return m, err
}

// ListMyRequests returns userID's submissions, newest first, and how many of
// all of them have news the requester has not opened.
func (s *Store) ListMyRequests(ctx context.Context, userID uuid.UUID, after *MyRequestCursor, limit int) (
	list []MyRequest, next *MyRequestCursor, unseen int, err error) {
	args := []any{userID, limit + 1}
	where := `s.submitter_id = $1`
	if after != nil {
		where += ` AND (s.created_at, s.id) < ($3, $4)`
		args = append(args, after.CreatedAt, after.ID)
	}
	rows, err := s.pool.Query(ctx, myRequestSelect+` WHERE `+where+` ORDER BY s.created_at DESC, s.id DESC LIMIT $2`, args...)
	if err != nil {
		return nil, nil, 0, err
	}
	list, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (MyRequest, error) { return scanMyRequest(r) })
	if err != nil {
		return nil, nil, 0, err
	}
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		next = &MyRequestCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM form_submissions s
		WHERE s.submitter_id = $1 AND GREATEST(s.status_changed_at, s.message_at) > s.seen_at`, userID).Scan(&unseen)
	return list, next, unseen, err
}

// GetMyRequest returns one of userID's own submissions; anyone else's is ErrNotFound.
func (s *Store) GetMyRequest(ctx context.Context, userID, id uuid.UUID) (MyRequestDetail, error) {
	var d MyRequestDetail
	var itemID uuid.UUID
	var err error
	d.MyRequest, err = scanMyRequest(s.pool.QueryRow(ctx, myRequestSelect+` WHERE s.id = $1 AND s.submitter_id = $2`, id, userID))
	if err != nil {
		return d, err
	}
	err = s.pool.QueryRow(ctx, `SELECT item_id, answers, message, message_at FROM form_submissions WHERE id = $1`, id).
		Scan(&itemID, &d.Answers, &d.Message, &d.MessageAt)
	if err != nil {
		return d, err
	}
	if d.Answers == nil {
		d.Answers = []Answer{}
	}
	// Only the files the requester attached to the item, never the team's.
	rows, err := s.pool.Query(ctx, `SELECT filename, size_bytes FROM attachments
		WHERE item_id = $1 AND uploader_id = $2 AND comment_id IS NULL ORDER BY created_at, id`, itemID, userID)
	if err != nil {
		return d, err
	}
	d.Files, err = pgx.CollectRows(rows, pgx.RowToStructByPos[RequestFile])
	if d.Files == nil {
		d.Files = []RequestFile{}
	}
	return d, err
}

// MarkRequestSeen records that userID opened their submission (clears Unseen).
func (s *Store) MarkRequestSeen(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE form_submissions SET seen_at = now() WHERE id = $1 AND submitter_id = $2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---------------------------------------------------------------------------
// The team's side of a request
// ---------------------------------------------------------------------------

// ItemRequest is the submission behind an item, as the project's members see it.
type ItemRequest struct {
	ID          uuid.UUID      `json:"id"`
	FormID      *uuid.UUID     `json:"form_id"`
	FormTitle   string         `json:"form_title"`
	Requester   *RequesterInfo `json:"requester"`
	SubmittedAt time.Time      `json:"submitted_at"`
	Message     *string        `json:"message"`
	MessageAt   *time.Time     `json:"message_at"`
	MessageBy   *CommentAuthor `json:"message_by"`
}

// RequesterInfo identifies the requester to the team, who may not otherwise
// know them (requesters need not be workspace members).
type RequesterInfo struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Member      bool      `json:"member"`
}

func itemRequest(ctx context.Context, q querier, wsID, itemID uuid.UUID) (ItemRequest, error) {
	var r ItemRequest
	var reqID, byID *uuid.UUID
	var reqName, reqEmail, byName *string
	var member bool
	err := q.QueryRow(ctx, `SELECT s.id, s.form_id, s.form_title, s.created_at, s.message, s.message_at,
			u.id, u.display_name, u.email, EXISTS (SELECT 1 FROM workspace_members m WHERE m.workspace_id = s.workspace_id AND m.user_id = u.id),
			b.id, b.display_name
		FROM form_submissions s
		LEFT JOIN users u ON u.id = s.submitter_id
		LEFT JOIN users b ON b.id = s.message_by
		WHERE s.workspace_id = $1 AND s.item_id = $2`, wsID, itemID).
		Scan(&r.ID, &r.FormID, &r.FormTitle, &r.SubmittedAt, &r.Message, &r.MessageAt, &reqID, &reqName, &reqEmail, &member, &byID, &byName)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if reqID != nil && reqName != nil && reqEmail != nil {
		r.Requester = &RequesterInfo{ID: *reqID, DisplayName: *reqName, Email: *reqEmail, Member: member}
	}
	if byID != nil && byName != nil {
		r.MessageBy = &CommentAuthor{ID: *byID, DisplayName: *byName}
	}
	return r, err
}

// GetItemRequest returns the submission behind a live item, or ErrNotFound
// when the item was not created by a form.
func (s *Store) GetItemRequest(ctx context.Context, wsID, projectID uuid.UUID, number int64) (ItemRequest, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return ItemRequest{}, err
	}
	return itemRequest(ctx, s.pool, wsID, item.id)
}

// SetRequestMessage sets (or, with nil, clears) the message the requester sees.
func (s *Store) SetRequestMessage(ctx context.Context, wsID, projectID uuid.UUID, number int64, actorID uuid.UUID, message *string) (ItemRequest, error) {
	msg, err := optionalText("message", message, maxRequestMessage)
	if err != nil {
		return ItemRequest{}, err
	}
	var out ItemRequest
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		item, err := liveItem(ctx, tx, wsID, projectID, number)
		if err != nil {
			return err
		}
		var subID uuid.UUID
		var old *string
		err = tx.QueryRow(ctx, `SELECT id, message FROM form_submissions WHERE workspace_id = $1 AND item_id = $2 FOR UPDATE`,
			wsID, item.id).Scan(&subID, &old)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if equalPtr(old, msg) {
			out, err = itemRequest(ctx, tx, wsID, item.id)
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE form_submissions SET message = $2,
				message_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END,
				message_by = CASE WHEN $2::text IS NULL THEN NULL ELSE $3::uuid END
			WHERE id = $1`, subID, msg, actorID); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "item", item.id, "request.message_set",
			map[string]any{"submission_id": subID, "message": diff(old, msg)}); err != nil {
			return err
		}
		out, err = itemRequest(ctx, tx, wsID, item.id)
		return err
	})
	return out, err
}
