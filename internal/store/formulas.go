package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/formula"
	"github.com/Tural-HS/Nebula/internal/recur"
)

// Formula fields (ADR-0030). A formula is compiled when its definition is
// saved and again whenever values are computed; the stored options carry
// the derived facts (result type, references, volatile, calendar) so SQL can
// find dependents without parsing.
//
// Values are computed on write: every item create, update, import, bulk
// edit and recurring item runs the workspace's active formulas over the
// item's new fields and stores the results in items.fields. A background
// pass (RecomputeFormulas) rewrites existing items when a formula changes,
// when the working-day calendar or time zone changes, and once per local
// day for formulas that use today().

// FieldFormula is the formula field type. Its stored value is the result in
// the form of the matching plain type (JSON number, "YYYY-MM-DD", boolean),
// or {"error": code} when it cannot be computed (e.g. division by zero).
const FieldFormula = "formula"

const (
	defaultFormulaScale = 2
	// maxFormulaChain bounds formulas that use formulas: a -> b -> c ...
	maxFormulaChain = 10
	// formulaBatch is how many items one recompute transaction rewrites.
	formulaBatch = 500
)

// FormulaOptions is options.formula of a formula field. Expression and
// Scale are set by admins; the rest is derived on save.
type FormulaOptions struct {
	Expression string   `json:"expression"`
	Scale      *int     `json:"scale,omitempty"` // decimals of a number result (0..10)
	ResultType string   `json:"result_type"`     // number, date or boolean
	Refs       []string `json:"refs"`            // field keys and built-ins used
	Volatile   bool     `json:"volatile,omitempty"`
	Calendar   bool     `json:"calendar,omitempty"`
}

// FormulaInput is the admin-editable part of a formula.
type FormulaInput struct {
	Expression string `json:"expression"`
	Scale      *int   `json:"scale"`
}

// FormulaError is a compile error with its position, so the UI can point at
// the problem. It is also a ValidationError for the API's 422 shape.
type FormulaError struct {
	Position int    `json:"position"`
	Message  string `json:"message"`
}

// formulaDef is the subset of a definition that compiling needs.
type formulaDef struct {
	key      string
	typ      string
	archived bool
	formula  *FormulaOptions
}

// loadFormulaDefs returns every definition of the workspace by key.
func loadFormulaDefs(ctx context.Context, q querier, wsID uuid.UUID) (map[string]formulaDef, error) {
	rows, err := q.Query(ctx, `SELECT key, field_type, archived_at IS NOT NULL, options FROM field_definitions WHERE workspace_id = $1`, wsID)
	if err != nil {
		return nil, err
	}
	defs := map[string]formulaDef{}
	var d formulaDef
	var opts FieldOptions
	_, err = pgx.ForEachRow(rows, []any{&d.key, &d.typ, &d.archived, &opts}, func() error {
		d.formula = opts.Formula
		defs[d.key] = d
		opts = FieldOptions{}
		return nil
	})
	return defs, err
}

// plainFormulaType maps a field type to the type a formula sees.
func plainFormulaType(t string) (formula.Type, bool) {
	switch t {
	case FieldNumber:
		return formula.TypeNumber, true
	case FieldDate:
		return formula.TypeDate, true
	case FieldBoolean:
		return formula.TypeBool, true
	}
	return 0, false
}

// formulaResolver resolves references against defs for the formula self.
func formulaResolver(defs map[string]formulaDef, self string) formula.Resolver {
	return func(ref string) (formula.Type, error) {
		if ref == formula.DueDateRef {
			return formula.TypeDate, nil
		}
		if strings.HasPrefix(ref, "@") {
			return 0, fmt.Errorf("unknown field %q (built-in fields: %s)", ref, formula.DueDateRef)
		}
		if ref == self {
			return 0, errors.New("a formula cannot use its own value")
		}
		d, ok := defs[ref]
		switch {
		case !ok:
			return 0, fmt.Errorf("unknown field %q", ref)
		case d.archived:
			return 0, fmt.Errorf("field %q is archived", ref)
		case d.typ == FieldFormula:
			if d.formula != nil {
				if t, ok := formula.ParseType(d.formula.ResultType); ok {
					return t, nil
				}
			}
			return 0, fmt.Errorf("formula field %q is invalid", ref)
		}
		if t, ok := plainFormulaType(d.typ); ok {
			return t, nil
		}
		return 0, fmt.Errorf("field %q is a %s field; formulas can use number, date, boolean and formula fields", ref, d.typ)
	}
}

func formulaValidation(err error) error {
	var fe *formula.Error
	if errors.As(err, &fe) {
		return &ValidationError{Field: "options.formula.expression", Message: fe.Error()}
	}
	return err
}

// compileFormulaDef validates in for the field key against the workspace's
// definitions and returns the options to store. The cycle check treats in as
// key's new expression.
func compileFormulaDef(defs map[string]formulaDef, key string, in FormulaInput) (*FormulaOptions, *formula.Program, error) {
	expr := strings.TrimSpace(in.Expression)
	prog, err := formula.Compile(expr, formulaResolver(defs, key))
	if err != nil {
		return nil, nil, formulaValidation(err)
	}
	out := &FormulaOptions{Expression: expr, ResultType: prog.Result.String(), Refs: prog.Refs,
		Volatile: prog.Volatile, Calendar: prog.Calendar}
	if out.Refs == nil {
		out.Refs = []string{}
	}
	switch {
	case prog.Result == formula.TypeNumber:
		scale := defaultFormulaScale
		if in.Scale != nil {
			scale = *in.Scale
		}
		if scale < 0 || scale > formula.MaxScale {
			return nil, nil, invalid("options.formula.scale", "must be 0 to %d decimals", formula.MaxScale)
		}
		out.Scale = &scale
	case in.Scale != nil:
		return nil, nil, invalid("options.formula.scale", "only formulas with a number result have decimals")
	}
	next := maps.Clone(defs)
	next[key] = formulaDef{key: key, typ: FieldFormula, formula: out}
	if err := checkFormulaGraph(next, key); err != nil {
		return nil, nil, err
	}
	return out, prog, nil
}

// checkFormulaGraph rejects a cycle through start and chains longer than
// maxFormulaChain among the active formulas of defs.
func checkFormulaGraph(defs map[string]formulaDef, start string) error {
	refsOf := func(k string) []string {
		d, ok := defs[k]
		if !ok || d.archived || d.typ != FieldFormula || d.formula == nil {
			return nil
		}
		var out []string
		for _, r := range d.formula.Refs {
			if rd, ok := defs[r]; ok && !rd.archived && rd.typ == FieldFormula {
				out = append(out, r)
			}
		}
		return out
	}
	var path []string
	var walk func(k string) error
	walk = func(k string) error {
		if i := slices.Index(path, k); i >= 0 {
			return invalid("options.formula.expression", "circular reference: %s", strings.Join(append(slices.Clone(path[i:]), k), " → "))
		}
		path = append(path, k)
		defer func() { path = path[:len(path)-1] }()
		if len(path) > maxFormulaChain {
			return invalid("options.formula.expression", "formulas may use other formulas at most %d levels deep", maxFormulaChain)
		}
		for _, r := range refsOf(k) {
			if err := walk(r); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(start); err != nil {
		return err
	}
	// Formulas that use start get longer chains too.
	for k := range defs {
		if slices.Contains(refsOf(k), start) {
			path = nil
			if err := walk(k); err != nil {
				return err
			}
		}
	}
	return nil
}

// formulaUsers lists active formulas (other than key itself) that use key.
func formulaUsers(ctx context.Context, q querier, wsID uuid.UUID, key string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT key FROM field_definitions
		WHERE workspace_id = $1 AND field_type = 'formula' AND archived_at IS NULL AND key <> $2
		  AND options -> 'formula' -> 'refs' ? $2 ORDER BY key`, wsID, key)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// requestFormulaRecompute (re)starts the background pass of a workspace.
// when narrows it: "" always, "volatile" or "calendar" only if an active
// formula uses today() or workdays(). Call it in the transaction that made
// the change, after locking the workspace row (see loadFormulaSet).
func requestFormulaRecompute(ctx context.Context, q querier, wsID uuid.UUID, when string) error {
	cond := "true"
	switch when {
	case "volatile":
		cond = "(f.options -> 'formula' ->> 'volatile')::boolean"
	case "calendar":
		cond = "(f.options -> 'formula' ->> 'calendar')::boolean"
	}
	_, err := q.Exec(ctx, `INSERT INTO formula_recompute (workspace_id)
		SELECT $1 WHERE EXISTS (SELECT 1 FROM field_definitions f
			WHERE f.workspace_id = $1 AND f.field_type = 'formula' AND f.archived_at IS NULL AND `+cond+`)
		ON CONFLICT (workspace_id) DO UPDATE SET pending = true, after_id = NULL, requested_at = now(), finished_at = NULL`, wsID)
	return err
}

// ---------------------------------------------------------------------------
// Computing values
// ---------------------------------------------------------------------------

type compiledFormula struct {
	key   string
	prog  *formula.Program
	scale int
}

// formulaSet is the workspace's active formulas in dependency order, ready
// to compute values for any number of items.
type formulaSet struct {
	wsID               uuid.UUID
	list               []compiledFormula
	types              map[string]formula.Type // every reference's type
	today              time.Time
	cal                *recur.Calendar
	needCal, calLoaded bool
}

// loadFormulaSet compiles the active formulas. With lock, it share-locks the
// workspace row: definition, calendar and time zone changes lock it
// exclusively (FOR NO KEY UPDATE), so they wait for in-flight item writes
// and every item write after them sees the new definitions. That is what
// makes a single recompute pass enough after a change.
func loadFormulaSet(ctx context.Context, q querier, wsID uuid.UUID, lock bool) (*formulaSet, error) {
	sql := `SELECT (now() AT TIME ZONE w.timezone)::date, f.key, f.field_type, f.options
		FROM workspaces w
		LEFT JOIN field_definitions f ON f.workspace_id = w.id AND f.archived_at IS NULL
			AND f.field_type IN ('number', 'date', 'boolean', 'formula')
		WHERE w.id = $1`
	if lock {
		sql += ` FOR SHARE OF w`
	}
	rows, err := q.Query(ctx, sql, wsID)
	if err != nil {
		return nil, err
	}
	fs := &formulaSet{wsID: wsID, types: map[string]formula.Type{formula.DueDateRef: formula.TypeDate}}
	defs := map[string]formulaDef{}
	var (
		today    time.Time
		key, typ *string
		opts     *FieldOptions
	)
	if _, err := pgx.ForEachRow(rows, []any{&today, &key, &typ, &opts}, func() error {
		fs.today = today
		if key != nil && typ != nil {
			d := formulaDef{key: *key, typ: *typ}
			if opts != nil {
				d.formula = opts.Formula
			}
			defs[*key] = d
		}
		opts = nil
		return nil
	}); err != nil {
		return nil, err
	}
	if fs.today.IsZero() {
		return nil, ErrNotFound
	}
	compiled := map[string]compiledFormula{}
	for k, d := range defs {
		if d.typ != FieldFormula {
			fs.types[k], _ = plainFormulaType(d.typ)
			continue
		}
		if d.formula == nil {
			continue
		}
		if t, ok := formula.ParseType(d.formula.ResultType); ok {
			fs.types[k] = t
		}
	}
	for k, d := range defs {
		if d.typ != FieldFormula || d.formula == nil {
			continue
		}
		// Validated on save; a stored formula that no longer compiles (a
		// manual SQL edit) is skipped and its values are left untouched.
		prog, err := formula.Compile(d.formula.Expression, formulaResolver(defs, k))
		if err != nil || prog.Result.String() != d.formula.ResultType {
			continue
		}
		scale := defaultFormulaScale
		if d.formula.Scale != nil {
			scale = *d.formula.Scale
		}
		compiled[k] = compiledFormula{key: k, prog: prog, scale: scale}
	}
	// Dependency order: a formula is computed after the formulas it uses.
	// Formulas in a cycle (impossible through the API) are left out.
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(k string) bool
	visit = func(k string) bool {
		switch state[k] {
		case 1:
			return false
		case 2:
			return true
		}
		state[k] = 1
		c := compiled[k]
		for _, r := range c.prog.Refs {
			if _, ok := compiled[r]; ok && !visit(r) {
				return false
			}
		}
		state[k] = 2
		fs.list = append(fs.list, c)
		return true
	}
	keys := make([]string, 0, len(compiled))
	for k := range compiled {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		visit(k)
		fs.needCal = fs.needCal || compiled[k].prog.Calendar
	}
	return fs, nil
}

// apply returns fields with every formula value recomputed (fields itself
// is not modified). due is the item's built-in due date.
func (fs *formulaSet) apply(ctx context.Context, q querier, fields map[string]any, due *string) (map[string]any, error) {
	if fs == nil || len(fs.list) == 0 {
		return fields, nil
	}
	if fs.needCal && !fs.calLoaded {
		cal, err := loadCalendar(ctx, q, fs.wsID, false)
		if err != nil {
			return nil, err
		}
		fs.cal, fs.calLoaded = cal, true
	}
	out := maps.Clone(fields)
	if out == nil {
		out = map[string]any{}
	}
	computed := make(map[string]formula.Value, len(fs.list))
	env := formula.Env{
		Today:    fs.today,
		Workdays: fs.cal.WorkdaysBetween,
		Ref: func(ref string) formula.Value {
			t := fs.types[ref]
			if ref == formula.DueDateRef {
				if due == nil {
					return formula.Null(t)
				}
				return formula.FromJSON(t, *due)
			}
			if v, ok := computed[ref]; ok {
				return v
			}
			return formula.FromJSON(t, fields[ref])
		},
	}
	for _, c := range fs.list {
		stored := c.prog.Eval(env).JSON(c.scale)
		// Later formulas read the stored (rounded) value: what users see is
		// what is used.
		computed[c.key] = formula.FromJSON(c.prog.Result, stored)
		if stored == nil {
			delete(out, c.key)
		} else {
			out[c.key] = stored
		}
	}
	return out, nil
}

// formulaValuesDiffer reports whether a and b store different formula values.
func (fs *formulaSet) formulaValuesDiffer(a, b map[string]any) bool {
	if fs == nil {
		return false
	}
	for _, c := range fs.list {
		if !sameStoredValue(c.prog.Result, a[c.key], b[c.key]) {
			return true
		}
	}
	return false
}

// sameStoredValue compares stored values; numbers by exact value, since a
// value read back from the database is a float64 and a fresh one a
// json.Number.
func sameStoredValue(t formula.Type, a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA == nil && errB == nil && string(ja) == string(jb) {
		return true
	}
	if t == formula.TypeNumber {
		ra, rb := formula.FromJSON(t, a).Rat(), formula.FromJSON(t, b).Rat()
		return ra != nil && rb != nil && ra.Cmp(rb) == 0
	}
	return false
}

// writeComputedFields stores recomputed values without touching the
// version, updated_at or the event log (ADR-0030: computed values are not
// audited; their inputs and the formula are).
func writeComputedFields(ctx context.Context, q querier, id uuid.UUID, fields map[string]any) error {
	if _, err := q.Exec(ctx, `SELECT set_config('nebula.computed_only', 'on', true)`); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `UPDATE items SET fields = $1 WHERE id = $2`, fields, id); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `SELECT set_config('nebula.computed_only', 'off', true)`)
	return err
}

// ---------------------------------------------------------------------------
// Background recompute
// ---------------------------------------------------------------------------

// ScheduleVolatileFormulas starts the daily pass of every workspace with a
// formula that uses today() whose local date has moved on. Idempotent:
// call it every tick on every replica.
func (s *Store) ScheduleVolatileFormulas(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO formula_recompute (workspace_id, volatile_day)
		SELECT w.id, (now() AT TIME ZONE w.timezone)::date FROM workspaces w
		WHERE EXISTS (SELECT 1 FROM field_definitions f WHERE f.workspace_id = w.id AND f.field_type = 'formula'
			AND f.archived_at IS NULL AND (f.options -> 'formula' ->> 'volatile')::boolean)
		ON CONFLICT (workspace_id) DO UPDATE SET pending = true, after_id = NULL, requested_at = now(),
			finished_at = NULL, volatile_day = EXCLUDED.volatile_day
		WHERE formula_recompute.volatile_day IS DISTINCT FROM EXCLUDED.volatile_day`)
	return tag.RowsAffected(), err
}

// RecomputeFormulas runs one batch of the oldest pending pass and returns
// how many items it examined (0: nothing to do). Replicas skip each
// other's workspaces, and rows locked by a concurrent write are retried by
// the next batch instead of waited for, so the pass never deadlocks with
// bulk edits.
func (s *Store) RecomputeFormulas(ctx context.Context) (int, error) {
	examined := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var wsID uuid.UUID
		var after *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT workspace_id, after_id FROM formula_recompute WHERE pending
			ORDER BY requested_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&wsID, &after)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		finish := func() error {
			_, err := tx.Exec(ctx, `UPDATE formula_recompute SET pending = false, after_id = NULL, finished_at = now()
				WHERE workspace_id = $1`, wsID)
			return err
		}
		fs, err := loadFormulaSet(ctx, tx, wsID, false)
		if err != nil {
			return err
		}
		if len(fs.list) == 0 {
			return finish()
		}
		rows, err := tx.Query(ctx, `SELECT id FROM items WHERE workspace_id = $1 AND ($2::uuid IS NULL OR id > $2)
			ORDER BY id LIMIT $3`, wsID, after, formulaBatch)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return finish()
		}
		rows, err = tx.Query(ctx, `SELECT id, fields, due_date::text FROM items WHERE id = ANY($1) FOR UPDATE SKIP LOCKED`, ids)
		if err != nil {
			return err
		}
		type itemRow struct {
			id     uuid.UUID
			fields map[string]any
			due    *string
		}
		locked, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (itemRow, error) {
			var it itemRow
			return it, r.Scan(&it.id, &it.fields, &it.due)
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('nebula.computed_only', 'on', true)`); err != nil {
			return err
		}
		got := make(map[uuid.UUID]bool, len(locked))
		batch := &pgx.Batch{}
		for _, it := range locked {
			got[it.id] = true
			if it.fields == nil {
				it.fields = map[string]any{}
			}
			next, err := fs.apply(ctx, tx, it.fields, it.due)
			if err != nil {
				return err
			}
			if fs.formulaValuesDiffer(it.fields, next) {
				batch.Queue(`UPDATE items SET fields = $1 WHERE id = $2`, next, it.id)
			}
		}
		if batch.Len() > 0 {
			if err := tx.SendBatch(ctx, batch).Close(); err != nil {
				return err
			}
		}
		examined = len(locked)
		// Resume before the first row that was locked by someone else.
		pos := after
		skipped := false
		for i := range ids {
			if !got[ids[i]] {
				skipped = true
				break
			}
			pos = &ids[i]
		}
		if !skipped && len(ids) < formulaBatch {
			return finish()
		}
		examined = max(examined, 1) // locked rows still mean "more to do"
		_, err = tx.Exec(ctx, `UPDATE formula_recompute SET after_id = $2 WHERE workspace_id = $1`, wsID, pos)
		return err
	})
	return examined, err
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

// FormulaPreviewParams describe a formula to try before saving it.
type FormulaPreviewParams struct {
	WorkspaceID uuid.UUID
	Viewer      Viewer
	Key         string // the field being edited, if it exists (for self and cycle checks)
	Formula     FormulaInput
	Item        string                     // optional sample item key, e.g. "FIN-12"
	Values      map[string]json.RawMessage // optional sample values; override the item's
}

// FormulaPreview is the result of a preview. On a compile error only Error
// is set.
type FormulaPreview struct {
	Valid      bool          `json:"valid"`
	Error      *FormulaError `json:"error,omitempty"`
	ResultType string        `json:"result_type,omitempty"`
	Refs       []string      `json:"refs,omitempty"`
	Volatile   bool          `json:"volatile"`
	Calendar   bool          `json:"calendar"`
	Value      any           `json:"value"` // stored form of the result
	Item       string        `json:"item,omitempty"`
}

// PreviewFormula compiles a formula and computes it for a sample item and/or
// sample values. It writes nothing.
func (s *Store) PreviewFormula(ctx context.Context, p FormulaPreviewParams) (FormulaPreview, error) {
	defs, err := loadFormulaDefs(ctx, s.pool, p.WorkspaceID)
	if err != nil {
		return FormulaPreview{}, err
	}
	key := p.Key
	if key == "" {
		key = "\x00new" // matches no reference
	} else if d, ok := defs[key]; !ok || d.typ != FieldFormula {
		return FormulaPreview{}, invalid("key", "unknown formula field")
	}
	if _, err := formula.Compile(strings.TrimSpace(p.Formula.Expression), formulaResolver(defs, key)); err != nil {
		var fe *formula.Error
		if errors.As(err, &fe) {
			return FormulaPreview{Error: &FormulaError{Position: fe.Pos, Message: fe.Msg}}, nil
		}
		return FormulaPreview{}, err
	}
	opts, prog, err := compileFormulaDef(defs, key, p.Formula)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) { // scale, cycles, chain depth
			return FormulaPreview{Error: &FormulaError{Message: ve.Message}}, nil
		}
		return FormulaPreview{}, err
	}
	out := FormulaPreview{Valid: true, ResultType: opts.ResultType, Refs: opts.Refs, Volatile: opts.Volatile, Calendar: opts.Calendar}

	fields := map[string]any{}
	var due *string
	if p.Item != "" {
		pk, number, ok := parseItemKey(p.Item)
		if !ok {
			return FormulaPreview{}, invalid("item", "must be an item key like FIN-12")
		}
		var projectID uuid.UUID
		err := s.pool.QueryRow(ctx, `SELECT i.project_id, i.fields, i.due_date::text FROM items i JOIN projects pr ON pr.id = i.project_id
			WHERE i.workspace_id = $1 AND pr.key = $2 AND i.number = $3 AND i.deleted_at IS NULL`, p.WorkspaceID, pk, number).
			Scan(&projectID, &fields, &due)
		if err == nil {
			var visible bool
			if visible, err = canSeeProject(ctx, s.pool, p.WorkspaceID, projectID, p.Viewer.UserID); err == nil && !visible {
				err = pgx.ErrNoRows // a private project's items do not exist for this viewer (ADR-0013)
			}
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return FormulaPreview{}, invalid("item", "item not found")
		}
		if err != nil {
			return FormulaPreview{}, err
		}
		if fields == nil {
			fields = map[string]any{}
		}
		out.Item = itemKey(pk, number)
	}
	if len(p.Values) > maxFieldsPerWrite {
		return FormulaPreview{}, invalid("values", "at most %d values", maxFieldsPerWrite)
	}
	for k, raw := range p.Values {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return FormulaPreview{}, invalid("values."+k, "invalid JSON value")
		}
		if k == formula.DueDateRef {
			if v == nil {
				due = nil
				continue
			}
			sv, ok := v.(string)
			if !ok {
				return FormulaPreview{}, invalid("values."+k, "must be a date (YYYY-MM-DD)")
			}
			due = &sv
			continue
		}
		if v == nil {
			delete(fields, k)
			continue
		}
		fields[k] = v
	}

	fs, err := loadFormulaSet(ctx, s.pool, p.WorkspaceID, false)
	if err != nil {
		return FormulaPreview{}, err
	}
	// Existing formulas the preview uses are computed first, from the same
	// sample, so a draft that uses "net" sees net's value for this sample.
	computed, err := fs.apply(ctx, s.pool, fields, due)
	if err != nil {
		return FormulaPreview{}, err
	}
	if prog.Calendar && !fs.calLoaded {
		if fs.cal, err = loadCalendar(ctx, s.pool, p.WorkspaceID, false); err != nil {
			return FormulaPreview{}, err
		}
	}
	scale := defaultFormulaScale
	if opts.Scale != nil {
		scale = *opts.Scale
	}
	types := fs.types
	v := prog.Eval(formula.Env{
		Today:    fs.today,
		Workdays: fs.cal.WorkdaysBetween,
		Ref: func(ref string) formula.Value {
			t, ok := types[ref]
			if !ok {
				t, _ = formulaResolver(defs, key)(ref)
			}
			if ref == formula.DueDateRef {
				if due == nil {
					return formula.Null(t)
				}
				return formula.FromJSON(t, *due)
			}
			return formula.FromJSON(t, computed[ref])
		},
	})
	out.Value = v.JSON(scale)
	return out, nil
}

// formulaAudit is what field events record about a formula.
func formulaAudit(o *FormulaOptions) any {
	if o == nil {
		return nil
	}
	m := map[string]any{"expression": o.Expression, "result_type": o.ResultType}
	if o.Scale != nil {
		m["scale"] = *o.Scale
	}
	return m
}
