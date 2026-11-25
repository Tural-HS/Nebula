package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Custom field types. Each has exactly one canonical JSON representation in
// items.fields, so equality filters (fields @> ...) are exact and index-backed.
const (
	FieldText        = "text"         // string, <= 1000 runes
	FieldLongText    = "long_text"    // string, <= 10000 runes
	FieldNumber      = "number"       // JSON number, |v| <= 1e15
	FieldDate        = "date"         // "YYYY-MM-DD"
	FieldDateTime    = "datetime"     // "YYYY-MM-DDTHH:MM:SSZ" (UTC, second precision)
	FieldBoolean     = "boolean"      // true / false
	FieldSelect      = "select"       // choice id
	FieldMultiSelect = "multi_select" // sorted, unique choice ids
	FieldUser        = "user"         // workspace member id
	FieldURL         = "url"          // http(s) URL
	FieldEmail       = "email"        // bare address
	// FieldFormula (formulas.go) is computed from other fields: read-only.
)

var fieldTypes = []string{FieldText, FieldLongText, FieldNumber, FieldDate, FieldDateTime, FieldBoolean,
	FieldSelect, FieldMultiSelect, FieldUser, FieldURL, FieldEmail, FieldFormula, FieldMoney}

const (
	maxFieldsPerWorkspace = 200 // active definitions
	maxChoices            = 500
	maxChoiceLabel        = 100
	maxMultiSelectValues  = 100
	maxTextValue          = 1000
	maxLongTextValue      = 10_000
	maxURLLen             = 2048
	maxEmailLen           = 254
	maxNumberAbs          = 1e15 // keeps values exactly representable as float64
	maxFieldsPerWrite     = 200
	// MaxFieldFilters bounds the number of field filters in one list request.
	MaxFieldFilters = 10
	datetimeLayout  = "2006-01-02T15:04:05Z"
)

var (
	fieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	colorRe    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// Choice is one option of a select / multi_select field. Items store the ID,
// so labels can be renamed without rewriting items.
type Choice struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Color    *string `json:"color,omitempty"`
	Archived bool    `json:"archived,omitempty"`
}

// FieldOptions holds type-specific settings.
type FieldOptions struct {
	Choices []Choice        `json:"choices,omitempty"`
	Formula *FormulaOptions `json:"formula,omitempty"` // formula fields only (ADR-0030)
	Money   *MoneyOptions   `json:"money,omitempty"`   // money fields only (ADR-0044)
	Unit    string          `json:"unit,omitempty"`    // number fields only: a display suffix such as "₼" or "%"
}

const maxFieldUnit = 8

// normalizeUnit validates a number field's display unit: up to 8 printable
// characters, no digits (they would read as part of the value). Blank clears it.
func normalizeUnit(v string) (string, error) {
	u, err := text("options.unit", v, 0, maxFieldUnit)
	if err != nil {
		return "", err
	}
	for _, r := range u {
		if unicode.IsControl(r) || unicode.IsDigit(r) {
			return "", invalid("options.unit", "must not contain digits or control characters")
		}
	}
	return u, nil
}

// FieldDefinition is a workspace-level custom field.
type FieldDefinition struct {
	ID        uuid.UUID    `json:"id"`
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	Options   FieldOptions `json:"options"`
	Archived  bool         `json:"archived"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// ChoiceInput describes a choice in a create/update request. An empty ID adds
// a new choice; an existing ID updates that choice in place.
type ChoiceInput struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Color    *string `json:"color"`
	Archived bool    `json:"archived"`
}

// ItemTypeField is a field attached to an item type.
type ItemTypeField struct {
	Key      string `json:"key"`
	Required bool   `json:"required"`
	Position int    `json:"position"`
}

const fieldSelect = `SELECT id, key, name, field_type, options, archived_at IS NOT NULL, created_at, updated_at
	FROM field_definitions`

func scanField(row pgx.Row) (FieldDefinition, error) {
	var f FieldDefinition
	err := row.Scan(&f.ID, &f.Key, &f.Name, &f.Type, &f.Options, &f.Archived, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FieldDefinition{}, ErrNotFound
	}
	return f, err
}

func isChoiceType(t string) bool { return t == FieldSelect || t == FieldMultiSelect }

// CreateFieldParams are the inputs for CreateField.
type CreateFieldParams struct {
	WorkspaceID uuid.UUID
	ActorID     uuid.UUID
	Key         string
	Name        string
	Type        string
	Choices     []ChoiceInput
	Formula     *FormulaInput // required for formula fields, refused otherwise
	Money       *MoneyInput   // money fields only; nil means the defaults
	Unit        *string       // number fields only; nil or blank: no unit
}

// CreateField adds a custom field definition to a workspace.
func (s *Store) CreateField(ctx context.Context, p CreateFieldParams) (FieldDefinition, error) {
	var out FieldDefinition
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createField(ctx, tx, p)
		return err
	})
	return out, err
}

// createField is CreateField inside the caller's transaction (workspace kinds, ADR-0033).
func createField(ctx context.Context, tx pgx.Tx, p CreateFieldParams) (FieldDefinition, error) {
	key := strings.TrimSpace(p.Key)
	if !fieldKeyRe.MatchString(key) {
		return FieldDefinition{}, invalid("key", "must be 1-63 characters: lowercase letters, digits and underscores, starting with a letter")
	}
	name, err := text("name", p.Name, 1, 100)
	if err != nil {
		return FieldDefinition{}, err
	}
	if !slices.Contains(fieldTypes, p.Type) {
		return FieldDefinition{}, invalid("type", "must be one of %s", strings.Join(fieldTypes, ", "))
	}
	var opts FieldOptions
	switch {
	case isChoiceType(p.Type):
		if opts.Choices, err = normalizeChoices(nil, p.Choices); err != nil {
			return FieldDefinition{}, err
		}
	case p.Choices != nil:
		return FieldDefinition{}, invalid("options.choices", "only select and multi_select fields have choices")
	}
	switch {
	case p.Type == FieldFormula && p.Formula == nil:
		return FieldDefinition{}, invalid("options.formula", "is required for formula fields")
	case p.Type != FieldFormula && p.Formula != nil:
		return FieldDefinition{}, invalid("options.formula", "only formula fields have a formula")
	}
	switch {
	case p.Type == FieldMoney:
		in := MoneyInput{}
		if p.Money != nil {
			in = *p.Money
		}
		if opts.Money, err = normalizeMoneyOptions(nil, in); err != nil {
			return FieldDefinition{}, err
		}
	case p.Money != nil:
		return FieldDefinition{}, invalid("options.money", "only money fields have money options")
	}
	if p.Unit != nil {
		if p.Type != FieldNumber {
			return FieldDefinition{}, invalid("options.unit", "only number fields have a unit")
		}
		if opts.Unit, err = normalizeUnit(*p.Unit); err != nil {
			return FieldDefinition{}, err
		}
	}

	var f FieldDefinition
	err = func() error {
		if err := checkFieldLimit(ctx, tx, p.WorkspaceID); err != nil {
			return err
		}
		payload := map[string]any{"key": key, "type": p.Type}
		if opts.Money != nil {
			payload["money"] = opts.Money
		}
		if p.Type == FieldFormula {
			defs, err := loadFormulaDefs(ctx, tx, p.WorkspaceID)
			if err != nil {
				return err
			}
			if _, taken := defs[key]; taken {
				return &ConflictError{Code: "field_key_taken", Message: "a field with this key already exists (keys of archived fields stay reserved)"}
			}
			if opts.Formula, _, err = compileFormulaDef(defs, key, *p.Formula); err != nil {
				return err
			}
			payload["formula"] = formulaAudit(opts.Formula)
		}
		f, err = scanField(tx.QueryRow(ctx, `INSERT INTO field_definitions (id, workspace_id, key, name, field_type, options)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, key, name, field_type, options, archived_at IS NOT NULL, created_at, updated_at`,
			newID(), p.WorkspaceID, key, name, p.Type, opts))
		if isUniqueViolation(err, "field_definitions_workspace_id_key_key") {
			return &ConflictError{Code: "field_key_taken", Message: "a field with this key already exists (keys of archived fields stay reserved)"}
		}
		if err != nil {
			return err
		}
		if p.Type == FieldFormula {
			// Existing items get their values from the background pass.
			if err := requestFormulaRecompute(ctx, tx, p.WorkspaceID, ""); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "field", f.ID, "field.created", payload)
	}()
	return f, err
}

// checkFieldLimit serializes definition changes per workspace (NO KEY UPDATE
// does not block item writes) so concurrent creates cannot exceed the limit.
func checkFieldLimit(ctx context.Context, tx pgx.Tx, wsID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, wsID); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM field_definitions WHERE workspace_id = $1 AND archived_at IS NULL`,
		wsID).Scan(&n); err != nil {
		return err
	}
	if n >= maxFieldsPerWorkspace {
		return &ConflictError{Code: "field_limit_reached",
			Message: fmt.Sprintf("a workspace can have at most %d active fields; archive unused ones", maxFieldsPerWorkspace)}
	}
	return nil
}

// normalizeChoices validates a full choice list. Existing choices may be
// relabeled or archived but never removed, so stored item values stay valid.
func normalizeChoices(existing []Choice, in []ChoiceInput) ([]Choice, error) {
	if len(in) > maxChoices {
		return nil, invalid("options.choices", "at most %d choices", maxChoices)
	}
	known := make(map[string]bool, len(existing))
	for _, c := range existing {
		known[c.ID] = true
	}
	seenID := map[string]bool{}
	seenLabel := map[string]bool{}
	out := make([]Choice, 0, len(in))
	for i, c := range in {
		path := fmt.Sprintf("options.choices[%d]", i)
		label, err := text(path+".label", c.Label, 1, maxChoiceLabel)
		if err != nil {
			return nil, err
		}
		l := strings.ToLower(label)
		if seenLabel[l] {
			return nil, invalid(path+".label", "duplicate label %q", label)
		}
		seenLabel[l] = true
		if c.Color != nil && !colorRe.MatchString(*c.Color) {
			return nil, invalid(path+".color", "must be a hex color like #1a2b3c")
		}
		id := c.ID
		switch {
		case id == "":
			id = newChoiceID()
		case !known[id]:
			return nil, invalid(path+".id", "unknown choice id")
		case seenID[id]:
			return nil, invalid(path+".id", "duplicate choice id")
		}
		seenID[id] = true
		out = append(out, Choice{ID: id, Label: label, Color: c.Color, Archived: c.Archived})
	}
	for _, c := range existing {
		if !seenID[c.ID] {
			return nil, invalid("options.choices", "choice %q cannot be removed; archive it instead", c.Label)
		}
	}
	return out, nil
}

func newChoiceID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // never fails (crypto/rand, Go 1.24+)
	return hex.EncodeToString(b)
}

// ListFields returns all field definitions, archived ones included.
func (s *Store) ListFields(ctx context.Context, wsID uuid.UUID) ([]FieldDefinition, error) {
	rows, err := s.pool.Query(ctx, fieldSelect+` WHERE workspace_id = $1 ORDER BY key`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (FieldDefinition, error) { return scanField(r) })
}

// GetField returns a field definition by key.
func (s *Store) GetField(ctx context.Context, wsID uuid.UUID, key string) (FieldDefinition, error) {
	return scanField(s.pool.QueryRow(ctx, fieldSelect+` WHERE workspace_id = $1 AND key = $2`, wsID, key))
}

// UpdateFieldParams is a partial update. Key and type are immutable.
type UpdateFieldParams struct {
	WorkspaceID uuid.UUID
	ActorID     uuid.UUID
	Key         string
	Name        opt.Field[string]
	Choices     *[]ChoiceInput // nil: unchanged; else the complete new list
	Formula     *FormulaInput  // nil: unchanged; formula fields only
	Money       *MoneyInput    // nil: unchanged; money fields only
	Unit        *string        // nil: unchanged; "" clears; number fields only
	Archived    opt.Field[bool]
}

// UpdateField renames a field, edits its choices or (un)archives it.
func (s *Store) UpdateField(ctx context.Context, p UpdateFieldParams) (FieldDefinition, error) {
	var f FieldDefinition
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		// Serializes definition changes with each other and with item writes
		// (loadFormulaSet share-locks the same row), see ADR-0030.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, p.WorkspaceID); err != nil {
			return err
		}
		cur, err := scanField(tx.QueryRow(ctx, fieldSelect+` WHERE workspace_id = $1 AND key = $2 FOR UPDATE`,
			p.WorkspaceID, p.Key))
		if err != nil {
			return err
		}
		next := cur
		changes := map[string]any{}
		recompute := false
		if p.Name.Set {
			if p.Name.Null {
				return invalid("name", "cannot be null")
			}
			if next.Name, err = text("name", p.Name.Value, 1, 100); err != nil {
				return err
			}
			if next.Name != cur.Name {
				changes["name"] = diff(cur.Name, next.Name)
			}
		}
		if p.Choices != nil {
			if !isChoiceType(cur.Type) {
				return invalid("options.choices", "only select and multi_select fields have choices")
			}
			if next.Options.Choices, err = normalizeChoices(cur.Options.Choices, *p.Choices); err != nil {
				return err
			}
			if !reflect.DeepEqual(cur.Options.Choices, next.Options.Choices) {
				changes["options.choices"] = diff(cur.Options.Choices, next.Options.Choices)
			}
		}
		if p.Money != nil {
			if cur.Type != FieldMoney {
				return invalid("options.money", "only money fields have money options")
			}
			if next.Options.Money, err = normalizeMoneyOptions(cur.Options.Money, *p.Money); err != nil {
				return err
			}
			if !reflect.DeepEqual(cur.Options.Money, next.Options.Money) {
				changes["options.money"] = diff(cur.Options.Money, next.Options.Money)
			}
		}
		if p.Unit != nil {
			if cur.Type != FieldNumber {
				return invalid("options.unit", "only number fields have a unit")
			}
			if next.Options.Unit, err = normalizeUnit(*p.Unit); err != nil {
				return err
			}
			if next.Options.Unit != cur.Options.Unit {
				changes["options.unit"] = diff(cur.Options.Unit, next.Options.Unit)
			}
		}
		if p.Archived.Set {
			if p.Archived.Null {
				return invalid("archived", "cannot be null")
			}
			next.Archived = p.Archived.Value
			if next.Archived != cur.Archived {
				if !next.Archived {
					if err := checkFieldLimit(ctx, tx, p.WorkspaceID); err != nil {
						return err
					}
				} else {
					users, err := formulaUsers(ctx, tx, p.WorkspaceID, cur.Key)
					if err != nil {
						return err
					}
					if len(users) > 0 {
						return &ConflictError{Code: "field_in_use", Message: fmt.Sprintf(
							"field is used by formula field(s) %s; change or archive them first", strings.Join(users, ", "))}
					}
				}
				changes["archived"] = diff(cur.Archived, next.Archived)
			}
		}
		if p.Formula != nil && cur.Type != FieldFormula {
			return invalid("options.formula", "only formula fields have a formula")
		}
		// A changed or restored formula is compiled against the current
		// fields; its result type cannot change (like a field's type).
		if cur.Type == FieldFormula && !next.Archived && (p.Formula != nil || cur.Archived) {
			in := FormulaInput{}
			if cur.Options.Formula != nil {
				in = FormulaInput{Expression: cur.Options.Formula.Expression, Scale: cur.Options.Formula.Scale}
			}
			if p.Formula != nil {
				in = *p.Formula
			}
			defs, err := loadFormulaDefs(ctx, tx, p.WorkspaceID)
			if err != nil {
				return err
			}
			opts, _, err := compileFormulaDef(defs, cur.Key, in)
			if err != nil {
				return err
			}
			if cur.Options.Formula != nil && opts.ResultType != cur.Options.Formula.ResultType {
				return invalid("options.formula.expression", "the result must stay a %s (it is now a %s); create a new field instead",
					cur.Options.Formula.ResultType, opts.ResultType)
			}
			if !reflect.DeepEqual(cur.Options.Formula, opts) {
				changes["options.formula"] = diff(formulaAudit(cur.Options.Formula), formulaAudit(opts))
			}
			next.Options.Formula = opts
			recompute = true
		}
		if len(changes) == 0 {
			f = cur
			return nil
		}
		f, err = scanField(tx.QueryRow(ctx, `UPDATE field_definitions SET name = $1, options = $2,
			archived_at = CASE WHEN $3 THEN coalesce(archived_at, now()) END
			WHERE id = $4
			RETURNING id, key, name, field_type, options, archived_at IS NOT NULL, created_at, updated_at`,
			next.Name, next.Options, next.Archived, cur.ID))
		if err != nil {
			return err
		}
		if recompute {
			if err := requestFormulaRecompute(ctx, tx, p.WorkspaceID, ""); err != nil {
				return err
			}
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "field", cur.ID, "field.updated",
			map[string]any{"key": cur.Key, "changes": changes})
	})
	return f, err
}

// BindField attaches a field to an item type, or updates an existing binding.
func (s *Store) BindField(ctx context.Context, wsID, actorID uuid.UUID, typeKey, fieldKey string,
	required bool, position int) (ItemTypeField, error) {
	var out ItemTypeField
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = bindField(ctx, tx, wsID, actorID, typeKey, fieldKey, required, position)
		return err
	})
	return out, err
}

// bindField is BindField inside the caller's transaction (workspace kinds, ADR-0033).
func bindField(ctx context.Context, tx pgx.Tx, wsID, actorID uuid.UUID, typeKey, fieldKey string,
	required bool, position int) (ItemTypeField, error) {
	if position < 0 || position > 10000 {
		return ItemTypeField{}, invalid("position", "must be between 0 and 10000")
	}
	b := ItemTypeField{Key: fieldKey, Required: required, Position: position}
	err := func() error {
		typeID, fieldID, archived, err := resolveBinding(ctx, tx, wsID, typeKey, fieldKey)
		if err != nil {
			return err
		}
		if archived {
			return invalid("field", "archived fields cannot be attached")
		}
		if required {
			var typ string
			if err := tx.QueryRow(ctx, `SELECT field_type FROM field_definitions WHERE id = $1`, fieldID).Scan(&typ); err != nil {
				return err
			}
			if typ == FieldFormula {
				return invalid("required", "formula fields are computed and cannot be required")
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO item_type_fields (workspace_id, item_type_id, field_id, is_required, position)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (item_type_id, field_id) DO UPDATE SET is_required = EXCLUDED.is_required, position = EXCLUDED.position`,
			wsID, typeID, fieldID, required, position); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "item_type", typeID, "item_type.field_bound",
			map[string]any{"field": fieldKey, "required": required, "position": position})
	}()
	return b, err
}

// UnbindField detaches a field from an item type. Existing item values are kept
// (and can still be cleared) but new values are rejected.
func (s *Store) UnbindField(ctx context.Context, wsID, actorID uuid.UUID, typeKey, fieldKey string) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		typeID, fieldID, _, err := resolveBinding(ctx, tx, wsID, typeKey, fieldKey)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM item_type_fields WHERE item_type_id = $1 AND field_id = $2`, typeID, fieldID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return appendEvent(ctx, tx, wsID, actorID, "item_type", typeID, "item_type.field_unbound",
			map[string]any{"field": fieldKey})
	})
}

// itemTypeFieldsByType returns the active fields bound to each item type, in display order.
func itemTypeFieldsByType(ctx context.Context, q querier, wsID uuid.UUID) (map[uuid.UUID][]ItemTypeField, error) {
	rows, err := q.Query(ctx, `SELECT b.item_type_id, f.key, b.is_required, b.position
		FROM item_type_fields b JOIN field_definitions f ON f.id = b.field_id
		WHERE b.workspace_id = $1 AND f.archived_at IS NULL
		ORDER BY b.position, f.key`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]ItemTypeField{}
	for rows.Next() {
		var typeID uuid.UUID
		var f ItemTypeField
		if err := rows.Scan(&typeID, &f.Key, &f.Required, &f.Position); err != nil {
			return nil, err
		}
		out[typeID] = append(out[typeID], f)
	}
	return out, rows.Err()
}

func resolveBinding(ctx context.Context, q querier, wsID uuid.UUID, typeKey, fieldKey string) (typeID, fieldID uuid.UUID, archived bool, err error) {
	err = q.QueryRow(ctx, `SELECT id FROM item_types WHERE workspace_id = $1 AND key = $2 AND archived_at IS NULL`,
		wsID, typeKey).Scan(&typeID)
	if err == nil {
		err = q.QueryRow(ctx, `SELECT id, archived_at IS NOT NULL FROM field_definitions WHERE workspace_id = $1 AND key = $2`,
			wsID, fieldKey).Scan(&fieldID, &archived)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return typeID, fieldID, archived, err
}

// ---------------------------------------------------------------------------
// Item values
// ---------------------------------------------------------------------------

// schemaField is a definition as seen by one item type.
type schemaField struct {
	def      FieldDefinition
	bound    bool
	required bool
}

// loadFieldSchema returns the definitions named in keys plus every field the
// item type requires.
func loadFieldSchema(ctx context.Context, q querier, wsID, itemTypeID uuid.UUID, keys []string) (map[string]schemaField, error) {
	rows, err := q.Query(ctx, `SELECT f.id, f.key, f.name, f.field_type, f.options, f.archived_at IS NOT NULL,
			f.created_at, f.updated_at, b.field_id IS NOT NULL, coalesce(b.is_required, false)
		FROM field_definitions f
		LEFT JOIN item_type_fields b ON b.field_id = f.id AND b.item_type_id = $2
		WHERE f.workspace_id = $1 AND (f.key = ANY($3) OR b.is_required)`, wsID, itemTypeID, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	schema := map[string]schemaField{}
	for rows.Next() {
		var sf schemaField
		d := &sf.def
		if err := rows.Scan(&d.ID, &d.Key, &d.Name, &d.Type, &d.Options, &d.Archived, &d.CreatedAt, &d.UpdatedAt,
			&sf.bound, &sf.required); err != nil {
			return nil, err
		}
		schema[d.Key] = sf
	}
	return schema, rows.Err()
}

// applyFields validates the requested changes (nil value = remove) against the
// schema and returns the new field map plus a per-field diff for the event log.
// On create, every required field must end up set. On update, required fields
// cannot be removed, but items created before a field became required are not
// blocked (ADR-0003: required is enforced on write).
func applyFields(ctx context.Context, q querier, wsID uuid.UUID, schema map[string]schemaField, typeKey string,
	cur map[string]any, raw map[string]json.RawMessage, creating bool) (map[string]any, map[string]any, error) {
	in, err := decodeFieldInput(raw)
	if err != nil {
		return nil, nil, err
	}
	next := make(map[string]any, len(cur)+len(in))
	for k, v := range cur {
		next[k] = v
	}
	changes := map[string]any{}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		path := "fields." + k
		f, known := schema[k]
		if known && f.def.Type == FieldFormula {
			return nil, nil, invalid(path, "is a formula field: its value is computed and cannot be set")
		}
		var val any
		empty := in[k] == nil
		if !empty {
			switch {
			case !known:
				return nil, nil, invalid(path, "unknown field")
			case f.def.Archived:
				return nil, nil, invalid(path, "field is archived")
			case !f.bound:
				return nil, nil, invalid(path, "field is not used by item type %q", typeKey)
			}
			var err error
			if val, empty, err = canonicalValue(f.def, in[k], path); err != nil {
				return nil, nil, err
			}
		}
		if empty {
			old, present := cur[k]
			if known && f.bound && f.required && !f.def.Archived {
				return nil, nil, invalid(path, "is required")
			}
			if present {
				delete(next, k)
				changes[path] = diff(old, nil)
			}
			continue
		}
		if reflect.DeepEqual(cur[k], val) {
			continue
		}
		if f.def.Type == FieldUser {
			id, _ := uuid.Parse(val.(string)) // canonicalValue guarantees a valid uuid
			ok, err := isMember(ctx, q, wsID, id)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				return nil, nil, invalid(path, "user is not a member of this workspace")
			}
		}
		next[k] = val
		changes[path] = diff(cur[k], val)
	}

	if creating {
		required := []string{}
		for k, f := range schema {
			if f.bound && f.required && !f.def.Archived {
				if _, ok := next[k]; !ok {
					required = append(required, k)
				}
			}
		}
		if len(required) > 0 {
			slices.Sort(required)
			return nil, nil, invalid("fields."+required[0], "is required")
		}
	}
	return next, changes, nil
}

// decodeFieldInput decodes each raw value; JSON null becomes a nil value (= remove).
func decodeFieldInput(raw map[string]json.RawMessage) (map[string]any, error) {
	if len(raw) > maxFieldsPerWrite {
		return nil, invalid("fields", "at most %d fields per request", maxFieldsPerWrite)
	}
	in := make(map[string]any, len(raw))
	for k, r := range raw {
		var v any
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, invalid("fields."+k, "invalid JSON value")
		}
		in[k] = v
	}
	return in, nil
}

// fieldKeys returns the sorted keys of a raw field map (for loadFieldSchema).
func fieldKeys(raw map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// canonicalValue converts a decoded JSON value into the field type's stored
// form. empty is true for "empty" input (blank text, empty multi-select),
// which removes the value just like null.
func canonicalValue(def FieldDefinition, v any, path string) (val any, empty bool, err error) {
	str := func() (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", invalid(path, "must be a string")
		}
		if !utf8.ValidString(s) {
			return "", invalid(path, "must be valid UTF-8")
		}
		return strings.TrimSpace(s), nil
	}

	switch def.Type {
	case FieldText, FieldLongText:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		limit := maxTextValue
		if def.Type == FieldLongText {
			limit = maxLongTextValue
		}
		if utf8.RuneCountInString(s) > limit {
			return nil, false, invalid(path, "must be at most %d characters", limit)
		}
		return s, false, nil

	case FieldNumber:
		n, ok := v.(float64)
		if !ok {
			return nil, false, invalid(path, "must be a number")
		}
		if math.IsNaN(n) || math.Abs(n) > maxNumberAbs {
			return nil, false, invalid(path, "must be between -1e15 and 1e15")
		}
		return n, false, nil

	case FieldBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, false, invalid(path, "must be true or false")
		}
		return b, false, nil

	case FieldMoney:
		return canonicalMoney(def, v, path)

	case FieldDate:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return nil, false, invalid(path, "must be a date (YYYY-MM-DD)")
		}
		return t.Format(time.DateOnly), false, nil

	case FieldDateTime:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, false, invalid(path, "must be an RFC 3339 timestamp with time zone")
		}
		return t.UTC().Truncate(time.Second).Format(datetimeLayout), false, nil

	case FieldSelect:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		if err := activeChoice(def, s, path); err != nil {
			return nil, false, err
		}
		return s, false, nil

	case FieldMultiSelect:
		list, ok := v.([]any)
		if !ok {
			return nil, false, invalid(path, "must be a list of choice ids")
		}
		if len(list) == 0 {
			return nil, true, nil
		}
		if len(list) > maxMultiSelectValues {
			return nil, false, invalid(path, "at most %d values", maxMultiSelectValues)
		}
		ids := make([]string, 0, len(list))
		for _, e := range list {
			s, ok := e.(string)
			if !ok {
				return nil, false, invalid(path, "must be a list of choice ids")
			}
			if err := activeChoice(def, s, path); err != nil {
				return nil, false, err
			}
			ids = append(ids, s)
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		out := make([]any, len(ids))
		for i, id := range ids {
			out[i] = id
		}
		return out, false, nil

	case FieldUser:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, false, invalid(path, "must be a user id")
		}
		return id.String(), false, nil

	case FieldURL:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		if len(s) > maxURLLen {
			return nil, false, invalid(path, "must be at most %d bytes", maxURLLen)
		}
		u, err := url.Parse(s)
		// Only http(s): javascript:, data: etc. would be XSS vectors in any UI.
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, false, invalid(path, "must be an http or https URL")
		}
		return s, false, nil

	case FieldEmail:
		s, err := str()
		if err != nil || s == "" {
			return nil, s == "", err
		}
		a, err := mail.ParseAddress(s)
		if len(s) > maxEmailLen || err != nil || a.Address != s {
			return nil, false, invalid(path, "must be an email address")
		}
		return s, false, nil
	}
	return nil, false, fmt.Errorf("field %s: unsupported type %q", def.Key, def.Type)
}

func activeChoice(def FieldDefinition, id, path string) error {
	for _, c := range def.Options.Choices {
		if c.ID == id {
			if c.Archived {
				return invalid(path, "choice %q is archived", c.Label)
			}
			return nil
		}
	}
	return invalid(path, "unknown choice %q", id)
}

// fieldCond is one SQL condition of a field filter, with "?" placeholders
// for its arguments (the caller numbers them).
type fieldCond struct {
	SQL  string
	Args []any
}

// Field filter operators beyond the exact match (ADR-0048), in the value of
// ?field.<key>=:
//   - "~text" on text-like fields: contains, ignoring case and Azerbaijani
//     letter variants (search_fold), up to 100 characters;
//   - "min..max" on number and date fields (either side may be empty):
//     an inclusive range;
//   - "is:empty" / "is:set" on any field;
//   - "in:id1,id2" on select and multi_select fields: any of the choices.
const maxContainsFilter = 100

// Filter values every field type accepts, and the choice-list limit.
const (
	FilterEmpty     = "is:empty"
	FilterSet       = "is:set"
	maxChoiceFilter = 20
)

var textLike = map[string]bool{FieldText: true, FieldLongText: true, FieldURL: true, FieldEmail: true}

// likeEscape escapes LIKE wildcards with a backslash (the default escape).
func likeEscape(v string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
}

// rangeConds builds the conditions of a "min..max" filter on a number or
// date field (resultType "number" or "date"). Values of another JSON type
// never match, so a bad stored value cannot break the cast.
func rangeConds(key, value, resultType, path string) ([]fieldCond, error) {
	lo, hi, _ := strings.Cut(value, "..")
	lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
	if lo == "" && hi == "" {
		return nil, invalid(path, "range needs a lower or an upper bound")
	}
	var out []fieldCond
	for _, b := range []struct {
		v  string
		op string
	}{{lo, ">="}, {hi, "<="}} {
		if b.v == "" {
			continue
		}
		switch resultType {
		case FieldNumber:
			n, err := strconv.ParseFloat(b.v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, invalid(path, "range bounds must be numbers")
			}
			out = append(out, fieldCond{
				SQL:  "(jsonb_typeof(i.fields -> ?::text) = 'number' AND (i.fields ->> ?::text)::numeric " + b.op + " ?::numeric)",
				Args: []any{key, key, strconv.FormatFloat(n, 'f', -1, 64)},
			})
		case FieldDate:
			if _, err := time.Parse(time.DateOnly, b.v); err != nil {
				return nil, invalid(path, "range bounds must be dates (YYYY-MM-DD)")
			}
			// ISO dates compare as text.
			out = append(out, fieldCond{
				SQL:  "(jsonb_typeof(i.fields -> ?::text) = 'string' AND (i.fields ->> ?::text) " + b.op + " ?::text)",
				Args: []any{key, key, b.v},
			})
		default:
			return nil, invalid(path, "ranges apply to number and date fields")
		}
	}
	return out, nil
}

// fieldFilterConditions turns filters into JSONB containment fragments, which
// the items_fields_idx GIN (jsonb_path_ops) index serves directly.
// filters come from ?field.<key>=<value>; for multi_select they match items
// containing the choice.
func fieldFilterConditions(ctx context.Context, q querier, wsID uuid.UUID, filters map[string]string) ([]fieldCond, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	if len(filters) > MaxFieldFilters {
		return nil, invalid("field", "at most %d field filters", MaxFieldFilters)
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	rows, err := q.Query(ctx, fieldSelect+` WHERE workspace_id = $1 AND key = ANY($2)`, wsID, keys)
	if err != nil {
		return nil, err
	}
	defs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (FieldDefinition, error) { return scanField(r) })
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]FieldDefinition, len(defs))
	for _, d := range defs {
		byKey[d.Key] = d
	}

	out := make([]fieldCond, 0, len(keys))
	for _, key := range keys {
		f := struct{ Key, Value string }{key, filters[key]}
		path := "field." + f.Key
		def, ok := byKey[f.Key]
		if !ok {
			return nil, invalid(path, "unknown field")
		}
		result := def.Type
		if def.Type == FieldFormula && def.Options.Formula != nil {
			result = map[string]string{"number": FieldNumber, "date": FieldDate, "boolean": FieldBoolean}[def.Options.Formula.ResultType]
		}
		// "is:empty" / "is:set" on any field: no value (missing, null, "", [])
		// or some value. false is a value (a yes/no field answered "no").
		if f.Value == FilterEmpty || f.Value == FilterSet {
			cond := "(NOT jsonb_exists(i.fields, ?::text) OR i.fields -> ?::text IN ('null'::jsonb, '\"\"'::jsonb, '[]'::jsonb))"
			if f.Value == FilterSet {
				cond = "NOT " + cond
			}
			out = append(out, fieldCond{SQL: cond, Args: []any{f.Key, f.Key}})
			continue
		}
		// "in:id1,id2" on select and multi_select: any of the choices.
		if rest, ok := strings.CutPrefix(f.Value, "in:"); ok && isChoiceType(def.Type) {
			ids := strings.Split(rest, ",")
			if len(ids) == 0 || len(ids) > maxChoiceFilter || rest == "" {
				return nil, invalid(path, "pick 1 to %d choices", maxChoiceFilter)
			}
			for i, id := range ids {
				id = strings.TrimSpace(id)
				ids[i] = id
				known := false
				for _, c := range def.Options.Choices {
					if c.ID == id {
						known = true
						break
					}
				}
				if !known {
					return nil, invalid(path, "unknown choice %q", id)
				}
			}
			cond := "(i.fields ->> ?::text) = ANY(?::text[])"
			if def.Type == FieldMultiSelect {
				cond = "jsonb_exists_any(i.fields -> ?::text, ?::text[])"
			}
			out = append(out, fieldCond{SQL: cond, Args: []any{f.Key, ids}})
			continue
		}
		if rest, ok := strings.CutPrefix(f.Value, "~"); ok && textLike[def.Type] {
			v, err := text(path, rest, 1, maxContainsFilter)
			if err != nil {
				return nil, err
			}
			out = append(out, fieldCond{
				SQL:  "lower(search_fold(i.fields ->> ?::text)) LIKE '%' || lower(search_fold(?::text)) || '%'",
				Args: []any{f.Key, likeEscape(v)},
			})
			continue
		}
		if strings.Contains(f.Value, "..") && (result == FieldNumber || result == FieldDate) {
			conds, err := rangeConds(f.Key, f.Value, result, path)
			if err != nil {
				return nil, err
			}
			out = append(out, conds...)
			continue
		}
		var raw any = f.Value
		switch def.Type {
		case FieldNumber:
			n, err := strconv.ParseFloat(f.Value, 64)
			if err != nil {
				return nil, invalid(path, "must be a number")
			}
			raw = n
		case FieldBoolean:
			b, err := strconv.ParseBool(f.Value)
			if err != nil {
				return nil, invalid(path, "must be true or false")
			}
			raw = b
		case FieldMultiSelect:
			raw = []any{f.Value}
		case FieldMoney:
			// Money filters by currency (ADR-0044): ?field.<key>=USD.
			c := strings.ToUpper(strings.TrimSpace(f.Value))
			if !IsCurrencyCode(c) {
				return nil, invalid(path, "must be a currency code such as AZN")
			}
			frag, err := json.Marshal(map[string]any{f.Key: map[string]any{"currency": c}})
			if err != nil {
				return nil, err
			}
			out = append(out, fieldCond{SQL: "i.fields @> ?::jsonb", Args: []any{string(frag)}})
			continue
		}
		if def.Type == FieldFormula {
			// A formula filters like its result type: number, date or boolean.
			if def.Options.Formula == nil {
				return nil, invalid(path, "formula field is invalid")
			}
			def.Type = map[string]string{"number": FieldNumber, "date": FieldDate, "boolean": FieldBoolean}[def.Options.Formula.ResultType]
			switch def.Type {
			case FieldNumber:
				n, err := strconv.ParseFloat(f.Value, 64)
				if err != nil {
					return nil, invalid(path, "must be a number")
				}
				raw = n
			case FieldBoolean:
				b, err := strconv.ParseBool(f.Value)
				if err != nil {
					return nil, invalid(path, "must be true or false")
				}
				raw = b
			case FieldDate:
			default:
				return nil, invalid(path, "formula field is invalid")
			}
		}
		// Archived choices are still valid filter targets: old items carry them.
		filterDef := def
		filterDef.Options.Choices = slices.Clone(def.Options.Choices)
		for i := range filterDef.Options.Choices {
			filterDef.Options.Choices[i].Archived = false
		}
		val, empty, err := canonicalValue(filterDef, raw, path)
		if err != nil {
			return nil, err
		}
		if empty {
			return nil, invalid(path, "filter value is required")
		}
		frag, err := json.Marshal(map[string]any{f.Key: val})
		if err != nil {
			return nil, err
		}
		out = append(out, fieldCond{SQL: "i.fields @> ?::jsonb", Args: []any{string(frag)}})
	}
	return out, nil
}
