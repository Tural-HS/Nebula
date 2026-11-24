package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/Tural-HS/Nebula/internal/observability"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/testdb"
)

// Item table export (ADR-0040): privacy, filters, typed cells, injection.

func (e *env) export(token, path string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequestWithContext(e.t.Context(), "GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func readCSV(t *testing.T, rec *httptest.ResponseRecorder, comma rune) [][]string {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content type %q", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "\xef\xbb\xbf") {
		t.Fatal("CSV without a byte order mark")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\xef\xbb\xbf")))
	r.Comma = comma
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func exportSetup(t *testing.T, e *env) (carol string) {
	t.Helper()
	e.formulaSetup() // fields amount, paid (number), due (date), note (text) on "task"
	e.must(201, "alice", "POST", fieldsPath, formulaBody("remaining", "amount - paid", 2))
	e.must(200, "alice", "PUT", bindPath("task", "remaining"), map[string]any{})
	carol = e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	e.must(201, "alice", "POST", items, map[string]any{"title": `=HYPERLINK("http://x.test","y")`, "assignee_id": carol,
		"fields": map[string]any{"amount": -1500.5, "paid": 0, "due": "2026-12-01", "note": "-ödənilməyib"}})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Ödəniş №2", "fields": map[string]any{"amount": 200, "paid": 50}})
	return carol
}

const exportCols = "cols=key,title,field.amount,field.remaining,field.due,field.note,assignee"

func TestItemExportCSV(t *testing.T) {
	e := newEnv(t)
	carol := exportSetup(t, e)

	// A guest exports what the list shows, in the table's columns and words.
	recs := readCSV(t, e.export("bob", items+"/export?format=csv&"+exportCols), ',')
	want := [][]string{
		{"#", "Başlıq", "Amount", "Remaining", "Due", "Note", "İcraçı"},
		{"ACME-2", "Ödəniş №2", "200", "150.00", "", "", ""}, // newest first, as the list
		{"ACME-1", `'=HYPERLINK("http://x.test","y")`, "-1500.5", "-1500.50", "2026-12-01", "'-ödənilməyib", "Carol"},
	}
	if len(recs) != len(want) {
		t.Fatalf("rows: %q", recs)
	}
	for i := range want {
		if strings.Join(recs[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d: %q, want %q", i, recs[i], want[i])
		}
	}

	// Semicolons with decimal commas, English headers; key and title are
	// always there even when not asked for.
	recs = readCSV(t, e.export("bob", items+"/export?format=csv&delimiter=semicolon&lang=en&cols=field.amount"), ';')
	if strings.Join(recs[0], "|") != "#|Title|Amount" || recs[2][2] != "-1500,5" {
		t.Fatalf("semicolon export: %q", recs)
	}

	// Filters, search and sort are the list's.
	recs = readCSV(t, e.export("carol", items+"/export?format=csv&assignee=me"), ',')
	if len(recs) != 2 || recs[1][0] != "ACME-1" {
		t.Fatalf("assignee filter: %q", recs)
	}
	recs = readCSV(t, e.export("carol", items+"/export?format=csv&assignee="+carol+"&q="+url.QueryEscape("nothing")), ',')
	if len(recs) != 1 {
		t.Fatalf("search filter: %q", recs)
	}
	recs = readCSV(t, e.export("carol", items+"/export?format=csv&q="+url.QueryEscape("Ödəniş")), ',')
	if len(recs) != 2 || recs[1][0] != "ACME-2" {
		t.Fatalf("search: %q", recs)
	}
	recs = readCSV(t, e.export("carol", items+"/export?format=csv&sort=field.amount"), ',')
	if recs[1][0] != "ACME-1" || recs[2][0] != "ACME-2" {
		t.Fatalf("sort: %q", recs)
	}

	// Bad parameters are refused before any work.
	for _, q := range []string{"", "format=pdf", "format=csv&cols=title,secret", "format=csv&lang=de",
		"format=csv&delimiter=tab", "format=csv&status=x", "format=csv&format=xlsx"} {
		if r := e.call("bob", "GET", items+"/export?"+q, nil); r.Status != 400 {
			t.Errorf("%q: %d", q, r.Status)
		}
	}
	if r := e.call("dave", "GET", items+"/export?format=csv", nil); r.Status != 404 {
		t.Errorf("outsider: %d", r.Status)
	}

	// Audited without cell data or the search text.
	var n int
	var payload string
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*), max(payload::text) FROM events WHERE action = 'items.exported'
		AND entity_type = 'project'`).Scan(&n, &payload); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Errorf("export events: %d", n)
	}
	for _, leak := range []string{"HYPERLINK", "Ödəniş", "nothing", "ödənilməyib"} {
		if strings.Contains(payload, leak) {
			t.Errorf("audit payload contains %q: %s", leak, payload)
		}
	}
}

func TestItemExportXLSX(t *testing.T) {
	e := newEnv(t)
	exportSetup(t, e)

	rec := e.export("carol", items+"/export?format=xlsx&"+exportCols+",updated_at")
	if rec.Code != 200 {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content type %q", ct)
	}
	today := time.Now().UTC().Format(time.DateOnly) // the test workspace's zone is UTC
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="ACME-`+today+`.xlsx"; filename*=UTF-8''Acme-`+today+`.xlsx` {
		t.Fatalf("disposition %q", cd)
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("ACME", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][2] != "Amount" || rows[0][7] != "Yenilənib" {
		t.Fatalf("rows: %q", rows)
	}
	// Numbers and dates are typed cells; text that looks like a formula is not one.
	if rows[2][1] != `'=HYPERLINK("http://x.test","y")` || rows[2][2] != "-1500.5" || rows[2][3] != "-1500.5" {
		t.Errorf("row 2: %q", rows[2])
	}
	if rows[2][4] != "46357" { // 2026-12-01
		t.Errorf("due serial %q", rows[2][4])
	}
	if shown, _ := f.GetCellValue("ACME", "D3"); shown != "-1,500.50" {
		t.Errorf("formula shown as %q", shown)
	}
	if formula, _ := f.GetCellFormula("ACME", "B3"); formula != "" {
		t.Errorf("a title became a formula: %q", formula)
	}
}

func TestItemExportPrivacy(t *testing.T) {
	e := newEnv(t)
	e.setup()
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq şöbəsi", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Məhkəmə işi"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq iş"})

	for _, user := range []string{"bob", "carol", "dave"} {
		for _, f := range []string{"csv", "xlsx"} {
			if rec := e.export(user, secItems+"/export?format="+f); rec.Code != 404 || strings.Contains(rec.Body.String(), "Məhkəmə") {
				t.Errorf("%s exports the private project as %s: %d", user, f, rec.Code)
			}
		}
	}
	carol := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	recs := readCSV(t, e.export("carol", secItems+"/export?format=csv"), ',')
	if len(recs) != 2 || recs[1][1] != "Məhkəmə işi" {
		t.Fatalf("project member export: %q", recs)
	}
	cd := e.export("carol", secItems+"/export?format=csv").Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="SEC-`) || !strings.Contains(cd, "filename*=UTF-8''H%C3%BCquq%20%C5%9F%C3%B6b%C9%99si-") {
		t.Fatalf("disposition %q", cd)
	}
	// Each project exports its own items only.
	recs = readCSV(t, e.export("bob", items+"/export?format=csv"), ',')
	if len(recs) != 2 || recs[1][1] != "Açıq iş" {
		t.Fatalf("workspace project export: %q", recs)
	}
}

func TestItemExportLimits(t *testing.T) {
	pool := testdb.New(t)
	h := NewAPIHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: observability.NewMetrics(),
		DB: pool, Store: store.New(pool), Verifier: testUsers, Version: "test", RequestTimeout: 10 * time.Second, ExportMaxRows: 2})
	e := &env{t: t, h: h, pool: pool}
	e.setup()
	for _, title := range []string{"a", "b"} {
		e.must(201, "alice", "POST", items, map[string]any{"title": title})
	}
	readCSV(t, e.export("alice", items+"/export?format=csv"), ',')
	e.must(201, "alice", "POST", items, map[string]any{"title": "c"})
	r := e.call("alice", "GET", items+"/export?format=xlsx", nil)
	if r.Status != 422 || errCode(r) != "export_too_large" {
		t.Fatalf("over the cap: %d %s", r.Status, r.Raw)
	}
	// Filters that narrow it under the cap work again.
	readCSV(t, e.export("alice", items+"/export?format=csv&q=c"), ',')

	// Per-user rate limit: a burst of 10, then 429 with Retry-After
	// (alice used hers above).
	for range 7 {
		readCSV(t, e.export("bob", items+"/export?format=csv&q=c"), ',')
	}
	readCSV(t, e.export("bob", items+"/export?format=csv&q=c"), ',') // bob has his own budget
	var last *httptest.ResponseRecorder
	for range 3 {
		last = e.export("bob", items+"/export?format=csv&q=c")
	}
	if last.Code != 429 || last.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d", last.Code)
	}
}
