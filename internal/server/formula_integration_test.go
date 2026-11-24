package server

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tural-HS/Nebula/internal/store"
)

// Formula fields (ADR-0030), end to end through the API. Synthetic data
// shaped like a payment tracker: amount, paid, due.

func formulaBody(key, expr string, scale any) map[string]any {
	f := map[string]any{"expression": expr}
	if scale != nil {
		f["scale"] = scale
	}
	return map[string]any{"key": key, "name": strings.ToUpper(key[:1]) + key[1:], "type": "formula", "options": map[string]any{"formula": f}}
}

func itemFields(t *testing.T, r resp) map[string]any {
	t.Helper()
	f, ok := r.Body["fields"].(map[string]any)
	if !ok {
		t.Fatalf("no fields in %s", r.Raw)
	}
	return f
}

func errMessage(r resp) string {
	if e, ok := r.Body["error"].(map[string]any); ok {
		s, _ := e["message"].(string)
		return s
	}
	return ""
}

// recompute runs background batches until the pending pass is done.
func (e *env) recompute() {
	e.t.Helper()
	s := store.New(e.pool)
	for range 100 {
		n, err := s.RecomputeFormulas(context.Background())
		if err != nil {
			e.t.Fatal(err)
		}
		if n == 0 {
			return
		}
	}
	e.t.Fatal("recompute did not finish")
}

func (e *env) formulaSetup() {
	e.t.Helper()
	e.setup()
	for _, f := range []map[string]any{
		{"key": "amount", "name": "Amount", "type": "number"},
		{"key": "paid", "name": "Paid", "type": "number"},
		{"key": "due", "name": "Due", "type": "date"},
		{"key": "note", "name": "Note", "type": "text"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
		e.must(200, "alice", "PUT", bindPath("task", f["key"].(string)), map[string]any{})
	}
}

func TestFormulaDefinitions(t *testing.T) {
	e := newEnv(t)
	e.formulaSetup()

	// --- Validation: clear errors with the position ----------------------------
	for name, tc := range map[string]struct {
		body  map[string]any
		field string
		msg   string
	}{
		"unknown field":   {formulaBody("x", "amout - paid", nil), "options.formula.expression", `at 1: unknown field "amout"`},
		"text field":      {formulaBody("x", "amount + note", nil), "options.formula.expression", "at 10: field \"note\" is a text field"},
		"syntax":          {formulaBody("x", "amount +", nil), "options.formula.expression", "at 9: expected a number"},
		"type mismatch":   {formulaBody("x", "amount + due + due", nil), "options.formula.expression", "cannot use +"},
		"self reference":  {formulaBody("x", "x + 1", nil), "options.formula.expression", "at 1: a formula cannot use its own value"},
		"bad scale":       {formulaBody("x", "amount", 11), "options.formula.scale", "0 to 10"},
		"scale on date":   {formulaBody("x", "due + 1", 2), "options.formula.scale", "number result"},
		"no formula":      {map[string]any{"key": "x", "name": "X", "type": "formula"}, "options.formula", "required"},
		"formula on text": {map[string]any{"key": "x", "name": "X", "type": "text", "options": map[string]any{"formula": map[string]any{"expression": "1"}}}, "options.formula", "only formula"},
		"too long":        {formulaBody("x", strings.Repeat("1+", 600)+"1", nil), "options.formula.expression", "too long"},
	} {
		r := e.call("alice", "POST", fieldsPath, tc.body)
		if r.Status != 422 || errField(r) != tc.field || !strings.Contains(errMessage(r), tc.msg) {
			t.Errorf("%s: %d %q %q, want 422 %q containing %q", name, r.Status, errField(r), errMessage(r), tc.field, tc.msg)
		}
	}
	if r := e.call("carol", "POST", fieldsPath, formulaBody("remaining", "amount - paid", nil)); r.Status != 403 {
		t.Fatalf("member created a formula: %d", r.Status)
	}
	// Unknown option keys are rejected by the strict decoder.
	if r := e.call("alice", "POST", fieldsPath, map[string]any{"key": "x", "name": "X", "type": "formula",
		"options": map[string]any{"formula": map[string]any{"expression": "1", "result_type": "date"}}}); r.Status != 400 {
		t.Fatalf("derived option accepted: %d %s", r.Status, r.Raw)
	}

	rem := e.must(201, "alice", "POST", fieldsPath, formulaBody("remaining", "amount - paid", nil)).Body
	opts := rem["options"].(map[string]any)["formula"].(map[string]any)
	if opts["result_type"] != "number" || opts["scale"] != 2.0 || fmt.Sprint(opts["refs"]) != "[amount paid]" || opts["volatile"] != nil {
		t.Fatalf("derived options: %v", opts)
	}
	e.must(201, "alice", "POST", fieldsPath, formulaBody("half", "remaining / 2", 0))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("days_left", "due - today()", nil))
	dl := e.must(200, "bob", "GET", fieldsPath+"/days_left", nil).Body["options"].(map[string]any)["formula"].(map[string]any)
	if dl["volatile"] != true {
		t.Fatalf("today() not volatile: %v", dl)
	}

	// --- Cycles: a formula may use formulas, never itself through others ---
	r := e.call("alice", "PATCH", fieldsPath+"/remaining", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "half + 1"}}})
	if r.Status != 422 || !strings.Contains(errMessage(r), "circular reference: remaining → half → remaining") {
		t.Fatalf("cycle: %d %s", r.Status, r.Raw)
	}
	r = e.call("alice", "PATCH", fieldsPath+"/remaining", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "remaining + 1"}}})
	if r.Status != 422 || !strings.Contains(errMessage(r), "cannot use its own value") {
		t.Fatalf("self: %d %s", r.Status, r.Raw)
	}
	// The result type is immutable, like a field's type.
	r = e.call("alice", "PATCH", fieldsPath+"/remaining", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "due"}}})
	if r.Status != 422 || !strings.Contains(errMessage(r), "must stay a number") {
		t.Fatalf("result type change: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "PATCH", fieldsPath+"/amount", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "1"}}}); r.Status != 422 {
		t.Fatalf("formula on a number field: %d", r.Status)
	}

	// --- Referenced fields cannot be archived; formulas cannot be required ---
	r = e.call("alice", "PATCH", fieldsPath+"/paid", map[string]any{"archived": true})
	if r.Status != 409 || errCode(r) != "field_in_use" || !strings.Contains(errMessage(r), "remaining") {
		t.Fatalf("archive referenced field: %d %s", r.Status, r.Raw)
	}
	r = e.call("alice", "PATCH", fieldsPath+"/remaining", map[string]any{"archived": true})
	if r.Status != 409 || !strings.Contains(errMessage(r), "half") {
		t.Fatalf("archive referenced formula: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "PUT", bindPath("task", "remaining"), map[string]any{"required": true}); r.Status != 422 {
		t.Fatalf("required formula: %d %s", r.Status, r.Raw)
	}
	// Archive the user first, then the input; restoring a formula whose input
	// is archived fails with the reason.
	e.must(200, "alice", "PATCH", fieldsPath+"/half", map[string]any{"archived": true})
	e.must(200, "alice", "PATCH", fieldsPath+"/remaining", map[string]any{"archived": true})
	e.must(200, "alice", "PATCH", fieldsPath+"/paid", map[string]any{"archived": true})
	r = e.call("alice", "PATCH", fieldsPath+"/remaining", map[string]any{"archived": false})
	if r.Status != 422 || !strings.Contains(errMessage(r), `field "paid" is archived`) {
		t.Fatalf("restore with archived input: %d %s", r.Status, r.Raw)
	}
	e.must(200, "alice", "PATCH", fieldsPath+"/paid", map[string]any{"archived": false})
	e.must(200, "alice", "PATCH", fieldsPath+"/remaining", map[string]any{"archived": false})

	// --- Definition changes are audited, with the expression -----------------
	e.must(200, "alice", "PATCH", fieldsPath+"/remaining", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "coalesce(amount, 0) - coalesce(paid, 0)", "scale": 3}}})
	var payload string
	if err := e.pool.QueryRow(context.Background(), `SELECT payload::text FROM events WHERE action = 'field.updated'
		AND payload ->> 'key' = 'remaining' AND payload -> 'changes' ? 'options.formula' ORDER BY id DESC LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"expression": "amount - paid"`) || !strings.Contains(payload, `coalesce(amount, 0)`) {
		t.Fatalf("audit payload: %s", payload)
	}
	if n := e.count(`SELECT count(*) FROM events WHERE action = 'field.created' AND payload -> 'formula' ->> 'expression' = 'due - today()'`); n != 1 {
		t.Fatalf("field.created without the expression: %d", n)
	}
}

func TestFormulaValues(t *testing.T) {
	e := newEnv(t)
	e.formulaSetup()
	e.must(201, "alice", "POST", fieldsPath, formulaBody("remaining", "amount - paid", nil))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("vat", "round(amount * 0.18, 2)", nil))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("ratio", "amount / paid", 4))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("late", "@due_date < due", nil))
	e.recompute() // nothing to do yet: no items

	// --- Computed on create, exact decimals ---------------------------------
	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Invoice 1", "due_date": "2026-12-01",
		"fields": map[string]any{"amount": 1250.5, "paid": 200.25, "due": "2026-12-15"}})
	f := itemFields(t, it)
	if f["remaining"] != 1050.25 || f["vat"] != 225.09 || f["ratio"] != 6.2447 || f["late"] != true {
		t.Fatalf("computed values: %v", f)
	}
	path := items + "/1"

	// --- Read-only: API writes are refused --------------------------------------
	for name, body := range map[string]map[string]any{
		"create": {"title": "x", "fields": map[string]any{"remaining": 1}},
		"null":   {"title": "x", "fields": map[string]any{"vat": nil}},
	} {
		if r := e.call("carol", "POST", items, body); r.Status != 422 || !strings.HasPrefix(errField(r), "fields.") {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	r := e.call("carol", "PATCH", path, map[string]any{"version": 1, "fields": map[string]any{"remaining": 5}})
	if r.Status != 422 || errField(r) != "fields.remaining" || !strings.Contains(errMessage(r), "computed") {
		t.Fatalf("patch formula: %d %s", r.Status, r.Raw)
	}

	// --- Recomputed when an input changes; not in the change log -----------------
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": 1, "fields": map[string]any{"paid": 0}})
	f = itemFields(t, it)
	if f["remaining"] != 1250.5 {
		t.Fatalf("remaining after paid=0: %v", f)
	}
	if m, ok := f["ratio"].(map[string]any); !ok || m["error"] != "division_by_zero" {
		t.Fatalf("division by zero: %v", f["ratio"])
	}
	var changes string
	if err := e.pool.QueryRow(context.Background(), `SELECT payload -> 'changes' FROM events WHERE action = 'item.updated' ORDER BY id DESC LIMIT 1`).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changes, "fields.paid") || strings.Contains(changes, "remaining") || strings.Contains(changes, "ratio") {
		t.Fatalf("event changes: %s", changes)
	}
	// The built-in due date is an input too.
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": 2, "due_date": "2026-12-20"})
	if itemFields(t, it)["late"] != false {
		t.Fatalf("late after due date change: %v", itemFields(t, it))
	}
	// Clearing an input empties the result.
	it = e.must(200, "carol", "PATCH", path, map[string]any{"version": 3, "fields": map[string]any{"amount": nil}})
	if _, ok := itemFields(t, it)["remaining"]; ok {
		t.Fatalf("remaining without amount: %v", itemFields(t, it))
	}
	e.must(200, "carol", "PATCH", path, map[string]any{"version": 4, "fields": map[string]any{"amount": 1000, "paid": 150}})

	// --- Formula change: background pass, no version, updated_at or events --
	var before time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT updated_at FROM items WHERE number = 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	events := e.count(`SELECT count(*) FROM events WHERE entity_type = 'item'`)
	e.must(200, "alice", "PATCH", fieldsPath+"/vat", map[string]any{"options": map[string]any{"formula": map[string]any{"expression": "round(amount * 0.2, 2)"}}})
	if n := e.count(`SELECT count(*) FROM formula_recompute WHERE pending`); n != 1 {
		t.Fatalf("pending passes: %d", n)
	}
	e.recompute()
	it = e.must(200, "bob", "GET", path, nil)
	if itemFields(t, it)["vat"] != 200.0 || it.Body["version"] != 5.0 {
		t.Fatalf("after formula change: v%v %v", it.Body["version"], itemFields(t, it))
	}
	var after time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT updated_at FROM items WHERE number = 1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) || e.count(`SELECT count(*) FROM events WHERE entity_type = 'item'`) != events {
		t.Fatalf("recompute touched updated_at (%v -> %v) or wrote events", before, after)
	}
	if n := e.count(`SELECT count(*) FROM formula_recompute WHERE pending`); n != 0 {
		t.Fatalf("pass not finished: %d", n)
	}

	// --- A new formula fills existing items -----------------------------------
	e.must(201, "alice", "POST", fieldsPath, formulaBody("double_rem", "remaining * 2", nil))
	if _, ok := itemFields(t, e.must(200, "bob", "GET", path, nil))["double_rem"]; ok {
		t.Fatal("computed before the pass")
	}
	e.recompute()
	if v := itemFields(t, e.must(200, "bob", "GET", path, nil))["double_rem"]; v != 1700.0 {
		t.Fatalf("double_rem = %v", v)
	}

	// --- Bulk edit and import refuse formula values ----------------------------
	r = e.call("alice", "POST", items+"/bulk", map[string]any{"items": []map[string]any{{"number": 1, "version": 5}},
		"fields": map[string]any{"remaining": 1}})
	if r.Status != 422 || errField(r) != "fields.remaining" {
		t.Fatalf("bulk: %d %s", r.Status, r.Raw)
	}
	// Bulk edits of inputs recompute like single edits.
	e.must(200, "alice", "POST", items+"/bulk", map[string]any{"items": []map[string]any{{"number": 1, "version": 5}},
		"fields": map[string]any{"paid": 400}})
	if v := itemFields(t, e.must(200, "bob", "GET", path, nil))["remaining"]; v != 600.0 {
		t.Fatalf("remaining after bulk = %v", v)
	}
	r = e.upload("alice", importsPath+"/preview", "x.csv", []byte("Title;Amount;Remaining\nA;10;99\n"), "", nil)
	for _, tg := range r.Body["targets"].([]any) {
		if tg.(map[string]any)["id"] == "field.remaining" {
			t.Fatalf("formula offered as an import target: %v", tg)
		}
	}
	r = e.upload("alice", importsPath, "x.csv", []byte("Title;Amount;Remaining\nA;10;99\n"), "", map[string]any{"columns": []map[string]any{
		{"column": 0, "target": "title"}, {"column": 1, "target": "field.amount"}, {"column": 2, "target": "field.remaining"}}})
	if r.Status < 400 {
		t.Fatalf("import into a formula: %d %s", r.Status, r.Raw)
	}
	r = e.upload("alice", importsPath, "x.csv", []byte("Title;Amount;Paid\nA;10;4\n"), "", map[string]any{"columns": []map[string]any{
		{"column": 0, "target": "title"}, {"column": 1, "target": "field.amount"}, {"column": 2, "target": "field.paid"}}})
	if r.Status != 201 && r.Status != 200 {
		t.Fatalf("import: %d %s", r.Status, r.Raw)
	}
	if v := itemFields(t, e.must(200, "bob", "GET", items+"/2", nil))["remaining"]; v != 6.0 {
		t.Fatalf("imported item remaining = %v", v)
	}
}

func TestFormulaTotalsSortFilter(t *testing.T) {
	e := newEnv(t)
	e.formulaSetup()
	e.must(201, "alice", "POST", fieldsPath, formulaBody("remaining", "amount - paid", nil))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("next_call", "due + 7", nil))
	e.must(422, "alice", "POST", fieldsPath, formulaBody("label", "note", nil)) // text cannot be used
	rows := []struct {
		amount, paid float64
		due          string
	}{
		{100.10, 0.05, "2026-12-03"}, // 100.05
		{0.3, 0.1, "2026-12-01"},     // 0.2
		{50, 50, ""},                 // 0
		{999999999999999, 0.01, ""},  // 999999999999998.99
	}
	for i, r := range rows {
		fields := map[string]any{"amount": r.amount, "paid": r.paid}
		if r.due != "" {
			fields["due"] = r.due
		}
		e.must(201, "carol", "POST", items, map[string]any{"title": fmt.Sprintf("Row %d", i+1), "fields": fields})
	}
	e.must(201, "carol", "POST", items, map[string]any{"title": "No amount"}) // no value

	// Totals are exact sums of the stored values.
	sums := e.must(200, "bob", "GET", items+"/sums?fields=remaining", nil).Body["sums"].(map[string]any)["remaining"].(map[string]any)
	if sums["sum"] != "1000000000000099.24" || sums["count"] != 4.0 {
		t.Fatalf("sums: %v", sums)
	}
	if r := e.call("bob", "GET", items+"/sums?fields=next_call", nil); r.Status != 422 {
		t.Fatalf("summed a date formula: %d", r.Status)
	}

	// Sorting, both directions, empty last, paged one by one through cursors.
	order := func(sort string) []string {
		var out []string
		cursor := ""
		for range 10 {
			q := items + "?limit=2&sort=" + sort
			if cursor != "" {
				q += "&cursor=" + cursor
			}
			r := e.must(200, "bob", "GET", q, nil)
			for _, it := range r.Body["items"].([]any) {
				out = append(out, it.(map[string]any)["title"].(string))
			}
			next, _ := r.Body["next_cursor"].(string)
			if next == "" {
				return out
			}
			cursor = next
		}
		t.Fatal("pagination did not end")
		return nil
	}
	if got := strings.Join(order("field.remaining"), ","); got != "Row 3,Row 2,Row 1,Row 4,No amount" {
		t.Fatalf("asc: %s", got)
	}
	if got := strings.Join(order("-field.remaining"), ","); got != "Row 4,Row 1,Row 2,Row 3,No amount" {
		t.Fatalf("desc: %s", got)
	}
	if got := strings.Join(order("field.next_call"), ","); got != "Row 2,Row 1,Row 3,Row 4,No amount" {
		t.Fatalf("date asc: %s", got) // undated rows by number
	}
	if got := strings.Join(order("-field.amount"), ","); got != "Row 4,Row 1,Row 3,Row 2,No amount" {
		t.Fatalf("number field desc: %s", got)
	}
	for _, bad := range []string{"field.note", "field.nope", "field.Bad", "-due"} {
		if r := e.call("bob", "GET", items+"?sort="+bad, nil); r.Status != 400 && r.Status != 422 {
			t.Errorf("sort=%s: %d", bad, r.Status)
		}
	}
	// A cursor from one sort cannot be replayed on another.
	first := e.must(200, "bob", "GET", items+"?limit=1&sort=field.remaining", nil).Body["next_cursor"].(string)
	if r := e.call("bob", "GET", items+"?sort=-field.remaining&cursor="+first, nil); r.Status != 400 {
		t.Fatalf("foreign cursor: %d", r.Status)
	}
	// Saved views may sort by a formula.
	e.must(201, "alice", "POST", items[:len(items)-len("/items")]+"/views", map[string]any{"name": "By remaining", "kind": "table", "shared": true,
		"config": map[string]any{"sort": "-field.remaining", "columns": []string{"title", "field.remaining"}}})
	if r := e.call("alice", "POST", items[:len(items)-len("/items")]+"/views", map[string]any{"name": "Bad", "kind": "table",
		"config": map[string]any{"sort": "field.note"}}); r.Status != 422 {
		t.Fatalf("view sorted by text: %d %s", r.Status, r.Raw)
	}

	// Equality filters use the result type.
	list := e.must(200, "bob", "GET", items+"?field.remaining=100.05", nil).Body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "Row 1" {
		t.Fatalf("filter: %v", list)
	}
	list = e.must(200, "bob", "GET", items+"?field.next_call=2026-12-08", nil).Body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["title"] != "Row 2" {
		t.Fatalf("date filter: %v", list)
	}
	if r := e.call("bob", "GET", items+"?field.remaining=abc", nil); r.Status != 422 {
		t.Fatalf("bad filter: %d", r.Status)
	}
}

func TestFormulaToday(t *testing.T) {
	e := newEnv(t)
	e.formulaSetup()
	e.must(201, "alice", "POST", fieldsPath, formulaBody("days_left", "due - today()", nil))
	e.must(201, "alice", "POST", fieldsPath, formulaBody("wd_left", "workdays(today(), due)", nil))

	// Kiritimati is UTC+14 and Pago Pago UTC-11: their dates differ for 23
	// hours of every day, so today() must be the workspace's local date.
	daysLeft := func(tz string) float64 {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().In(loc)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return float64(time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC).Sub(today).Hours() / 24)
	}
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Pacific/Kiritimati"})
	it := e.must(201, "carol", "POST", items, map[string]any{"title": "Due later", "fields": map[string]any{"due": "2030-01-10"}})
	if got, want := itemFields(t, it)["days_left"], daysLeft("Pacific/Kiritimati"); got != want {
		t.Fatalf("days_left in Kiritimati = %v, want %v", got, want)
	}
	// A time zone change recomputes volatile formulas.
	e.must(200, "alice", "PATCH", wsBase, map[string]any{"timezone": "Pacific/Pago_Pago"})
	if n := e.count(`SELECT count(*) FROM formula_recompute WHERE pending`); n != 1 {
		t.Fatalf("time zone change did not request a pass: %d", n)
	}
	e.recompute()
	if got, want := itemFields(t, e.must(200, "bob", "GET", items+"/1", nil))["days_left"], daysLeft("Pacific/Pago_Pago"); got != want {
		t.Fatalf("days_left in Pago Pago = %v, want %v", got, want)
	}

	// The daily pass: scheduled once per local day.
	s := store.New(e.pool)
	if _, err := s.ScheduleVolatileFormulas(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.recompute()
	if n, err := s.ScheduleVolatileFormulas(context.Background()); err != nil || n != 0 {
		t.Fatalf("second schedule on the same day: %d %v", n, err)
	}
	// Simulate yesterday's values and pass: the next tick refreshes them.
	if _, err := e.pool.Exec(context.Background(), `UPDATE formula_recompute SET volatile_day = volatile_day - 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE items SET fields = fields || '{"days_left": 1}'`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ScheduleVolatileFormulas(context.Background()); err != nil || n != 1 {
		t.Fatalf("schedule on a new day: %d %v", n, err)
	}
	e.recompute()
	if got := itemFields(t, e.must(200, "bob", "GET", items+"/1", nil))["days_left"]; got == 1.0 {
		t.Fatal("daily pass did not refresh days_left")
	}

	// A calendar change recomputes formulas that use workdays().
	e.must(200, "alice", "PUT", wsBase+"/calendar/days/2029-12-31", map[string]any{"kind": "holiday", "name": "Sınaq"})
	if n := e.count(`SELECT count(*) FROM formula_recompute WHERE pending`); n != 1 {
		t.Fatalf("calendar change did not request a pass: %d", n)
	}
	e.recompute()
}

func TestFormulaPreview(t *testing.T) {
	e := newEnv(t)
	e.formulaSetup()
	e.must(201, "alice", "POST", fieldsPath, formulaBody("remaining", "amount - paid", nil))
	e.must(201, "carol", "POST", items, map[string]any{"title": "Sample", "fields": map[string]any{"amount": 80, "paid": 30}})
	const preview = fieldsPath + "/formula-preview"

	if r := e.call("carol", "POST", preview, map[string]any{"expression": "1"}); r.Status != 403 {
		t.Fatalf("member preview: %d", r.Status)
	}
	r := e.must(200, "alice", "POST", preview, map[string]any{"expression": "amount - pad"})
	errObj, _ := r.Body["error"].(map[string]any)
	if r.Body["valid"] != false || errObj["position"] != 10.0 || !strings.Contains(errObj["message"].(string), `unknown field "pad"`) {
		t.Fatalf("invalid preview: %s", r.Raw)
	}
	r = e.must(200, "alice", "POST", preview, map[string]any{"expression": "round(remaining * 1.18, 2)", "item": "acme-1"})
	if r.Body["valid"] != true || r.Body["value"] != 59.0 || r.Body["item"] != "ACME-1" || r.Body["result_type"] != "number" {
		t.Fatalf("item preview: %s", r.Raw)
	}
	// Sample values override the item's.
	r = e.must(200, "alice", "POST", preview, map[string]any{"expression": "amount / paid", "item": "ACME-1", "values": map[string]any{"paid": 0}})
	if v, _ := r.Body["value"].(map[string]any); v["error"] != "division_by_zero" {
		t.Fatalf("values preview: %s", r.Raw)
	}
	r = e.must(200, "alice", "POST", preview, map[string]any{"key": "remaining", "expression": "remaining + 1"})
	if r.Body["valid"] != false {
		t.Fatalf("self reference preview: %s", r.Raw)
	}
	if r := e.call("alice", "POST", preview, map[string]any{"expression": "1", "item": "ACME-99"}); r.Status != 422 {
		t.Fatalf("unknown item: %d", r.Status)
	}
}
