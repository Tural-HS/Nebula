package server

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Money fields (ADR-0044): options, values, per-currency totals, filters,
// sort, formulas, import and export.

func (e *env) moneySetup() {
	e.t.Helper()
	e.setup()
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "price", "name": "Qiymət", "type": "money",
		"options": map[string]any{"money": map[string]any{"currencies": []string{"AZN", "USD", "EUR"}}}})
	e.must(200, "alice", "PUT", bindPath("task", "price"), map[string]any{})
}

func money(amount any, currency string) map[string]any {
	m := map[string]any{"amount": amount}
	if currency != "" {
		m["currency"] = currency
	}
	return m
}

func TestMoneyFieldDefinition(t *testing.T) {
	e := newEnv(t)
	e.setup()
	body := map[string]any{"key": "fee", "name": "Haqq", "type": "money"}
	e.must(403, "carol", "POST", fieldsPath, body) // member
	e.must(403, "bob", "POST", fieldsPath, body)   // guest
	e.must(404, "dave", "POST", fieldsPath, body)  // outsider

	f := e.must(201, "alice", "POST", fieldsPath, body)
	opts := f.Body["options"].(map[string]any)["money"].(map[string]any)
	if strings.Join(toStrings(opts["currencies"]), ",") != "AZN,USD,EUR" || opts["default_currency"] != "AZN" || opts["scale"] != 2.0 {
		t.Fatalf("defaults: %v", opts)
	}
	for name, o := range map[string]map[string]any{
		"unknown code":       {"currencies": []string{"AZN", "XYZ"}},
		"empty list":         {"currencies": []string{}},
		"scale too big":      {"scale": 5},
		"default not listed": {"currencies": []string{"USD"}, "default_currency": "AZN"},
	} {
		r := e.call("alice", "POST", fieldsPath, map[string]any{"key": "bad", "name": "Bad", "type": "money", "options": map[string]any{"money": o}})
		if r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	if r := e.call("alice", "POST", fieldsPath, map[string]any{"key": "n", "name": "N", "type": "number",
		"options": map[string]any{"money": map[string]any{}}}); r.Status != 422 {
		t.Errorf("money options on a number field: %d", r.Status)
	}
	// Lower-case codes are normalized; the scale is fixed after creation.
	u := e.must(200, "alice", "PATCH", fieldsPath+"/fee", map[string]any{"options": map[string]any{"money": map[string]any{
		"currencies": []string{"usd", "gbp"}}}})
	opts = u.Body["options"].(map[string]any)["money"].(map[string]any)
	if strings.Join(toStrings(opts["currencies"]), ",") != "USD,GBP" || opts["default_currency"] != "USD" {
		t.Fatalf("update: %v", opts)
	}
	if r := e.call("alice", "PATCH", fieldsPath+"/fee", map[string]any{"options": map[string]any{"money": map[string]any{"scale": 3}}}); r.Status != 422 {
		t.Errorf("scale change: %d %s", r.Status, r.Raw)
	}
	e.must(403, "carol", "PATCH", fieldsPath+"/fee", map[string]any{"options": map[string]any{"money": map[string]any{"currencies": []string{"AZN"}}}})
	if n := e.count(`SELECT count(*) FROM events WHERE action = 'field.updated'`); n < 1 {
		t.Errorf("no audit event for the options change")
	}

	// Formulas cannot use money fields (currencies would mix).
	r := e.call("alice", "POST", fieldsPath, formulaBody("double", "fee * 2", 2))
	if r.Status != 422 || !strings.Contains(r.Raw, "money") {
		t.Errorf("formula over money: %d %s", r.Status, r.Raw)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestMoneyValues(t *testing.T) {
	e := newEnv(t)
	e.moneySetup()
	r := e.must(201, "carol", "POST", items, map[string]any{"title": "A", "fields": map[string]any{"price": money("1250.5", "")}})
	if got := itemFields(t, r)["price"]; got.(map[string]any)["amount"] != "1250.50" || got.(map[string]any)["currency"] != "AZN" {
		t.Fatalf("stored %v", got)
	}
	r = e.must(201, "carol", "POST", items, map[string]any{"title": "B", "fields": map[string]any{"price": money(300, "usd")}})
	if got := itemFields(t, r)["price"].(map[string]any); got["amount"] != "300.00" || got["currency"] != "USD" {
		t.Fatalf("stored %v", got)
	}
	for name, v := range map[string]any{
		"too many decimals": money("1.005", "AZN"),
		"not allowed":       money("1", "GBP"),
		"not a code":        money("1", "manat"),
		"grouping":          money("1 250,50", "AZN"),
		"plain number":      1250.5,
		"extra key":         map[string]any{"amount": "1", "rate": 1},
		"too large":         money("10000000000000", "AZN"),
	} {
		r := e.call("carol", "POST", items, map[string]any{"title": name, "fields": map[string]any{"price": v}})
		if r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	e.must(403, "bob", "POST", items, map[string]any{"title": "guest", "fields": map[string]any{"price": money("1", "AZN")}})
}

func TestMoneyTotalsFilterSort(t *testing.T) {
	e := newEnv(t)
	e.moneySetup()
	for _, c := range []struct {
		title string
		price any
	}{
		{"a", money("1000.10", "AZN")}, {"b", money("200", "AZN")}, {"c", money("300", "USD")},
		{"d", money("-0.10", "AZN")}, {"e", money("5", "EUR")}, {"f", nil},
	} {
		f := map[string]any{}
		if c.price != nil {
			f["price"] = c.price
		}
		e.must(201, "carol", "POST", items, map[string]any{"title": c.title, "fields": f})
	}
	// Bad data that bypassed validation is ignored, never summed or cast.
	if _, err := e.pool.Exec(context.Background(), `UPDATE items SET fields = fields || '{"price": {"amount": "x1", "currency": "AZN"}}' WHERE title = 'f'`); err != nil {
		t.Fatal(err)
	}
	s := e.must(200, "bob", "GET", sumsPath+"?fields=price", nil).Body["sums"].(map[string]any)["price"].(map[string]any)
	cur := s["currencies"].([]any)
	got := []string{}
	for _, x := range cur {
		m := x.(map[string]any)
		got = append(got, m["currency"].(string)+"="+m["sum"].(string))
	}
	if strings.Join(got, " ") != "AZN=1200.00 EUR=5.00 USD=300.00" || s["count"] != 5.0 {
		t.Fatalf("sums %v", s)
	}
	// Totals follow the list filters, including the currency filter.
	s = e.must(200, "bob", "GET", sumsPath+"?fields=price&field.price=usd", nil).Body["sums"].(map[string]any)["price"].(map[string]any)
	if c := s["currencies"].([]any); len(c) != 1 || c[0].(map[string]any)["sum"] != "300.00" {
		t.Fatalf("filtered sums %v", s)
	}
	list := e.must(200, "carol", "GET", items+"?field.price=AZN", nil).Body["items"].([]any)
	if len(list) != 4 { // a, b, d and the bad-data item f
		t.Fatalf("AZN filter: %d items", len(list))
	}
	if r := e.call("carol", "GET", items+"?field.price=XYZ", nil); r.Status != 422 {
		t.Errorf("bad currency filter: %d", r.Status)
	}
	// Sort by amount (currency filter first for a meaningful order), paged.
	var titles []string
	cursor := ""
	for range 10 {
		r := e.must(200, "carol", "GET", items+"?sort=-field.price&limit=2"+cursor, nil)
		for _, it := range r.Body["items"].([]any) {
			titles = append(titles, it.(map[string]any)["title"].(string))
		}
		next, _ := r.Body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = "&cursor=" + next
	}
	if strings.Join(titles, ",") != "a,c,b,e,d,f" {
		t.Fatalf("sorted %v", titles)
	}
	e.must(404, "dave", "GET", sumsPath+"?fields=price", nil)
}

func TestMoneyImport(t *testing.T) {
	e := newEnv(t)
	e.moneySetup()
	csv := "Başlıq;Qiymət\n" +
		"A;1250.50 AZN\n" +
		"B;1 250,50 ₼\n" +
		"C;$300\n" +
		"D;€5,5\n" +
		"E;1.234 AZN\n" + // ambiguous separator
		"F;10 GBP\n" + // not allowed
		"G;1,005 USD\n" + // ambiguous
		"H;1.005\n" + // too many decimals
		"I;12 AZN USD\n" // two currencies
	mapping := map[string]any{"columns": []map[string]any{{"column": 0, "target": "title"}, {"column": 1, "target": "field.price"}}, "default_type": "task"}
	r := e.upload("alice", importsPath+"?dry_run=true", "m.csv", []byte(csv), "", mapping)
	if r.Status != 200 {
		t.Fatalf("dry run: %d %s", r.Status, r.Raw)
	}
	if r.Body["valid"].(float64) != 4 || r.Body["error_count"].(float64) != 5 {
		t.Fatalf("report: %s", r.Raw)
	}
	good := "Başlıq;Qiymət\nA;1250.50 AZN\nB;1 250,50 ₼\nC;$300\nD;€5,5\n"
	r = e.upload("alice", importsPath, "m.csv", []byte(good), "", mapping)
	if r.Status != 201 || r.Body["created"].(float64) != 4 {
		t.Fatalf("import: %d %s", r.Status, r.Raw)
	}
	s := e.must(200, "carol", "GET", sumsPath+"?fields=price", nil).Body["sums"].(map[string]any)["price"].(map[string]any)
	got := []string{}
	for _, x := range s["currencies"].([]any) {
		m := x.(map[string]any)
		got = append(got, m["currency"].(string)+"="+m["sum"].(string))
	}
	if strings.Join(got, " ") != "AZN=2501.00 EUR=5.50 USD=300.00" {
		t.Fatalf("imported sums %v", got)
	}
}

func TestMoneyExport(t *testing.T) {
	e := newEnv(t)
	e.moneySetup()
	e.must(201, "carol", "POST", items, map[string]any{"title": "A", "fields": map[string]any{"price": money("1250.5", "AZN")}})
	e.must(201, "carol", "POST", items, map[string]any{"title": "B", "fields": map[string]any{"price": money("300", "USD")}})

	recs := readCSV(t, e.export("carol", items+"/export?format=csv&delimiter=semicolon&cols=key,title,field.price"), ';')
	if strings.Join(recs[0], "|") != "#|Başlıq|Qiymət|Qiymət (valyuta)" {
		t.Fatalf("header %q", recs[0])
	}
	// Newest first: B, then A.
	if recs[1][2] != "300,00" || recs[1][3] != "USD" || recs[2][2] != "1250,50" || recs[2][3] != "AZN" {
		t.Fatalf("csv rows %q", recs)
	}

	rec := e.export("carol", items+"/export?format=xlsx&cols=key,title,field.price")
	if rec.Code != 200 {
		t.Fatalf("xlsx: %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if typ, _ := f.GetCellType("ACME", "C3"); typ != excelize.CellTypeUnset && typ != excelize.CellTypeNumber {
		t.Errorf("amount cell type %v, want numeric", typ)
	}
	if v, _ := f.GetCellValue("ACME", "C3", excelize.Options{RawCellValue: true}); v != "1250.5" {
		t.Errorf("raw amount %q", v)
	}
	if shown, _ := f.GetCellValue("ACME", "C3"); shown != "1,250.50" {
		t.Errorf("amount shown as %q", shown)
	}
	if v, _ := f.GetCellValue("ACME", "D3"); v != "AZN" {
		t.Errorf("currency %q", v)
	}
}
