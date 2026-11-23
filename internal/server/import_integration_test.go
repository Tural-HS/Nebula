package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

const importsPath = "/api/v1/workspaces/acme/projects/ACME/imports"

// upload posts a multipart import request. mapping may be nil (preview).
func (e *env) upload(token, path, fileName string, data []byte, sheet string, mapping any) resp {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", fileName)
	_, _ = fw.Write(data)
	if sheet != "" {
		_ = mw.WriteField("sheet", sheet)
	}
	if mapping != nil {
		b, _ := json.Marshal(mapping)
		_ = mw.WriteField("mapping", string(b))
	}
	_ = mw.Close()
	req := httptest.NewRequestWithContext(e.t.Context(), "POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	r := resp{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.Body)
	return r
}

// importSetup adds custom fields modeled on a customer payment tracker.
func (e *env) importSetup() {
	e.t.Helper()
	e.setup()
	for _, f := range []map[string]any{
		{"key": "branch", "name": "Filial", "type": "select", "options": map[string]any{"choices": []map[string]any{{"label": "Bakı"}, {"label": "Gəncə"}, {"label": "İsmayıllı"}}}},
		{"key": "contract_no", "name": "Müqavilə №", "type": "text"},
		{"key": "start_date", "name": "Başlama tarixi", "type": "date"},
		{"key": "monthly", "name": "Aylıq məbləğ", "type": "number"},
		{"key": "vip", "name": "VIP", "type": "boolean"},
		{"key": "owner", "name": "Kurator", "type": "user"},
	} {
		e.must(201, "alice", "POST", fieldsPath, f)
		e.must(200, "alice", "PUT", bindPath("task", f["key"].(string)), map[string]any{"required": f["key"] == "contract_no"})
	}
}

// Synthetic data only: invented names, no real customers.
const customersCSV = "\xEF\xBB\xBFMüştəri;Filial;Müqavilə №;Başlama tarixi;Aylıq məbləğ;VIP;Məsul;Status;Qeyd\r\n" +
	"Şəhla Məmmədova;Bakı;AZ-001;15.03.2026;1 250,50 ₼;bəli;carol@example.com;In Progress;Ödəniş vaxtında\r\n" +
	"Ömər Əliyev;gəncə;AZ-002;2026-04-01;300;xeyr;;;\r\n" +
	";;;;;;;;\r\n" +
	"İlkin Qasımov;İSMAYILLI;AZ-003;01.05.2026;1.200,00;;;Done;\r\n"

func suggestedTargets(r resp) map[int]string {
	out := map[int]string{}
	for _, s := range r.Body["suggested"].([]any) {
		m := s.(map[string]any)
		out[int(m["column"].(float64))] = m["target"].(string)
	}
	return out
}

func fullMapping(r resp) map[string]any {
	cols := []map[string]any{}
	for col, target := range suggestedTargets(r) {
		cols = append(cols, map[string]any{"column": col, "target": target})
	}
	return map[string]any{"columns": cols, "default_type": "task"}
}

func TestImportPreview(t *testing.T) {
	e := newEnv(t)
	e.importSetup()

	r := e.upload("alice", importsPath+"/preview", "müştərilər.csv", []byte(customersCSV), "", nil)
	if r.Status != 200 {
		t.Fatalf("preview: %d %s", r.Status, r.Raw)
	}
	if r.Body["format"] != "csv" || r.Body["delimiter"] != ";" || r.Body["row_count"].(float64) != 3 ||
		r.Body["file_name"] != "müştərilər.csv" {
		t.Fatalf("preview summary: %s", r.Raw)
	}
	headers := r.Body["headers"].([]any)
	rows := r.Body["rows"].([]any)
	if len(headers) != 9 || headers[0] != "Müştəri" || len(rows) != 3 {
		t.Fatalf("headers/rows: %s", r.Raw)
	}
	if last := rows[2].(map[string]any); last["row"].(float64) != 5 || last["cells"].([]any)[1] != "İSMAYILLI" {
		t.Fatalf("row numbers must skip blank rows but keep file lines: %v", last)
	}
	want := map[int]string{0: "title", 1: "field.branch", 2: "field.contract_no", 3: "field.start_date",
		4: "field.monthly", 5: "field.vip", 6: "assignee", 7: "status", 8: "description"}
	got := suggestedTargets(r)
	for col, target := range want {
		if got[col] != target {
			t.Errorf("suggested column %d: %q, want %q (all %v)", col, got[col], target, got)
		}
	}
	var targets []string
	for _, tg := range r.Body["targets"].([]any) {
		targets = append(targets, tg.(map[string]any)["id"].(string))
	}
	if !strings.Contains(strings.Join(targets, ","), "field.owner") || targets[0] != "title" {
		t.Fatalf("targets: %v", targets)
	}

	// xlsx: sheet list, first visible sheet by default, another sheet on request.
	book := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetSheetRow("Sheet1", "A1", &[]any{"Ad"})
		_ = f.SetSheetRow("Sheet1", "A2", &[]any{"Birinci"})
		_, _ = f.NewSheet("İkinci")
		_ = f.SetSheetRow("İkinci", "A1", &[]any{"Başlıq", "Filial"})
	})
	r = e.upload("alice", importsPath+"/preview", "kitab.xlsx", book, "", nil)
	if r.Status != 200 || r.Body["sheet"] != "Sheet1" || len(r.Body["sheets"].([]any)) != 2 || r.Body["row_count"].(float64) != 1 {
		t.Fatalf("xlsx preview: %d %s", r.Status, r.Raw)
	}
	r = e.upload("alice", importsPath+"/preview", "kitab.xlsx", book, "İkinci", nil)
	if r.Status != 200 || r.Body["sheet"] != "İkinci" || r.Body["row_count"].(float64) != 0 {
		t.Fatalf("chosen sheet: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath+"/preview", "kitab.xlsx", book, "Yoxdur", nil); r.Status != 400 || errCode(r) != "sheet_not_found" {
		t.Fatalf("missing sheet: %d %s", r.Status, r.Raw)
	}
}

func workbookBytes(t *testing.T, build func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	build(f)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestImportDryRunAndImport(t *testing.T) {
	e := newEnv(t)
	e.importSetup()
	preview := e.upload("alice", importsPath+"/preview", "müştərilər.csv", []byte(customersCSV), "", nil)
	mapping := fullMapping(preview)

	// --- Dry run with row errors: everything reported, nothing written ------
	bad := "Müştəri;Filial;Müqavilə №;Başlama tarixi;Aylıq məbləğ;VIP;Məsul;Status;Qeyd\n" +
		"Səbinə;Sumqayıt;AZ-010;15.03.2026;12,500;bəli;;;\n" + // unknown choice + ambiguous number
		"Rəşad;Bakı;;31.02.2026;100;;;;\n" + // missing required contract + invalid date
		";Bakı;AZ-012;;;;nobody@example.com;Arxiv;\n" + // no title, unknown member, unknown status
		"Nərmin;Bakı;AZ-013;;;;;;\n" // valid
	r := e.upload("alice", importsPath+"?dry_run=true", "pis.csv", []byte(bad), "", mapping)
	if r.Status != 200 || r.Body["dry_run"] != true || r.Body["created"].(float64) != 0 {
		t.Fatalf("dry run: %d %s", r.Status, r.Raw)
	}
	if r.Body["valid"].(float64) != 1 || r.Body["error_count"].(float64) < 6 {
		t.Fatalf("dry run report: %s", r.Raw)
	}
	byRow := map[int][]string{}
	for _, x := range r.Body["errors"].([]any) {
		m := x.(map[string]any)
		byRow[int(m["row"].(float64))] = append(byRow[int(m["row"].(float64))], fmt.Sprint(m["target"]))
	}
	for row, targets := range map[int][]string{2: {"field.branch", "field.monthly"}, 3: {"field.start_date"}, 4: {"assignee", "status"}} {
		for _, tg := range targets {
			if !strings.Contains(strings.Join(byRow[row], ","), tg) {
				t.Errorf("row %d: missing error for %s (got %v)", row, tg, byRow[row])
			}
		}
	}
	if n := e.count(`SELECT count(*) FROM items`); n != 0 {
		t.Fatalf("dry run wrote %d items", n)
	}
	// Store-level validation is reported too (required field, title) when conversion succeeds.
	r = e.upload("alice", importsPath+"?dry_run=true", "pis.csv",
		[]byte("Müştəri;Müqavilə №\nA;\n;B\n"), "", map[string]any{"columns": []map[string]any{
			{"column": 0, "target": "title"}, {"column": 1, "target": "field.contract_no"}}})
	if r.Status != 200 || r.Body["error_count"].(float64) != 2 {
		t.Fatalf("store errors: %s", r.Raw)
	}
	errs := r.Body["errors"].([]any)
	if first := errs[0].(map[string]any); first["field"] != "fields.contract_no" || first["header"] != "Müqavilə №" || first["column"].(float64) != 1 {
		t.Fatalf("required field error: %v", first)
	}
	if second := errs[1].(map[string]any); second["field"] != "title" || second["row"].(float64) != 3 {
		t.Fatalf("title error: %v", second)
	}

	// --- A real import with errors is rejected as a whole --------------------
	r = e.upload("alice", importsPath, "pis.csv", []byte(bad), "", mapping)
	if r.Status != 422 || errCode(r) != "import_invalid" || r.Body["created"].(float64) != 0 {
		t.Fatalf("invalid import: %d %s", r.Status, r.Raw)
	}
	if n := e.count(`SELECT count(*) FROM items`); n != 0 {
		t.Fatalf("invalid import wrote %d items", n)
	}

	// --- Clean dry run, then the import --------------------------------------
	r = e.upload("alice", importsPath+"?dry_run=true", "müştərilər.csv", []byte(customersCSV), "", mapping)
	if r.Status != 200 || r.Body["valid"].(float64) != 3 || r.Body["error_count"].(float64) != 0 {
		t.Fatalf("clean dry run: %d %s", r.Status, r.Raw)
	}
	r = e.upload("alice", importsPath, "müştərilər.csv", []byte(customersCSV), "", mapping)
	if r.Status != 201 || r.Body["created"].(float64) != 3 || r.Body["first_key"] != "ACME-1" || r.Body["last_key"] != "ACME-3" {
		t.Fatalf("import: %d %s", r.Status, r.Raw)
	}

	branches := choiceIDs(t, e.must(200, "alice", "GET", fieldsPath+"/branch", nil))
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	first := e.must(200, "bob", "GET", items+"/1", nil).Body
	f := first["fields"].(map[string]any)
	if first["title"] != "Şəhla Məmmədova" || first["description"] != "Ödəniş vaxtında" || first["assignee_id"] != carolID ||
		first["status"].(map[string]any)["name"] != "In Progress" {
		t.Fatalf("item 1: %v", first)
	}
	if f["branch"] != branches["Bakı"] || f["contract_no"] != "AZ-001" || f["start_date"] != "2026-03-15" ||
		f["monthly"] != 1250.5 || f["vip"] != true {
		t.Fatalf("item 1 fields: %v", f)
	}
	third := e.must(200, "bob", "GET", items+"/3", nil).Body
	if tf := third["fields"].(map[string]any); tf["branch"] != branches["İsmayıllı"] || tf["monthly"] != 1200.0 ||
		third["status"].(map[string]any)["category"] != "done" {
		t.Fatalf("item 3: %v", third)
	}
	second := e.must(200, "bob", "GET", items+"/2", nil).Body
	if second["status"].(map[string]any)["category"] != "todo" || second["fields"].(map[string]any)["vip"] != false {
		t.Fatalf("item 2 defaults: %v", second)
	}

	// Events: one item.created (with dispatch job) per item, one audit entry for the import.
	if n := e.count(`SELECT count(*) FROM events WHERE action = 'item.created'`); n != 3 {
		t.Fatalf("item.created events: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM jobs j JOIN events ev ON ev.id = (j.payload->>'event_id')::bigint
		WHERE j.kind = 'event.dispatch' AND ev.action = 'item.created'`); n != 3 {
		t.Fatalf("dispatch jobs: %d", n)
	}
	activity := e.must(200, "bob", "GET", items+"/1/activity", nil)
	if !strings.Contains(activity.Raw, "item.created") {
		t.Fatalf("item activity: %s", activity.Raw)
	}
	audit := e.must(200, "alice", "GET", "/api/v1/workspaces/acme/audit-log?action=project.imported", nil)
	if !strings.Contains(audit.Raw, `"file_name":"müştərilər.csv"`) || !strings.Contains(audit.Raw, `"created":3`) ||
		strings.Count(audit.Raw, "project.imported") != 1 {
		t.Fatalf("audit entry: %s", audit.Raw)
	}

	// Numbering continues after the imported range.
	created := e.must(201, "carol", "POST", items, map[string]any{"title": "Sonrakı", "fields": map[string]any{"contract_no": "AZ-100"}})
	if created.Body["key"] != "ACME-4" {
		t.Fatalf("next item: %s", created.Raw)
	}
}

func TestImportXLSX(t *testing.T) {
	e := newEnv(t)
	e.importSetup()
	start := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	book := workbookBytes(t, func(f *excelize.File) {
		_ = f.SetSheetName("Sheet1", "Müştərilər")
		s := "Müştərilər"
		_ = f.SetSheetRow(s, "A1", &[]any{"Ad", "Müqavilə №", "Başlama tarixi", "Aylıq məbləğ", "VIP", "Kurator"})
		_ = f.SetSheetRow(s, "A2", &[]any{"Fərid Hüseynov", "AZ-1", start, 1234.567, true, "CAROL@example.com"})
		_ = f.SetSheetRow(s, "A3", &[]any{"Gülnar Abbasova", 42, "01.04.2026", "1 500 ₼", "yox"})
		style, _ := f.NewStyle(&excelize.Style{NumFmt: 14})
		_ = f.SetCellStyle(s, "C2", "C2", style)
	})
	preview := e.upload("alice", importsPath+"/preview", "kitab.xlsx", book, "", nil)
	if preview.Status != 200 {
		t.Fatalf("preview: %s", preview.Raw)
	}
	mapping := fullMapping(preview)
	if got := suggestedTargets(preview); got[0] != "title" || got[5] != "field.owner" {
		t.Fatalf("suggestions: %v", got)
	}
	r := e.upload("alice", importsPath, "kitab.xlsx", book, "", mapping)
	if r.Status != 201 || r.Body["created"].(float64) != 2 {
		t.Fatalf("xlsx import: %d %s", r.Status, r.Raw)
	}
	carolID := e.must(200, "carol", "GET", "/api/v1/me", nil).Body["id"].(string)
	f1 := e.must(200, "alice", "GET", items+"/1", nil).Body["fields"].(map[string]any)
	if f1["start_date"] != "2026-03-15" || f1["monthly"] != 1234.567 || f1["vip"] != true || f1["owner"] != carolID {
		t.Fatalf("xlsx row 1: %v", f1)
	}
	f2 := e.must(200, "alice", "GET", items+"/2", nil).Body["fields"].(map[string]any)
	if f2["contract_no"] != "42" || f2["start_date"] != "2026-04-01" || f2["monthly"] != 1500.0 || f2["vip"] != false {
		t.Fatalf("xlsx row 2: %v", f2)
	}
}

func TestImportAccessAndLimits(t *testing.T) {
	e := newEnv(t)
	e.importSetup()
	csv := []byte("Başlıq\nBir\n")
	mapping := map[string]any{"columns": []map[string]any{{"column": 0, "target": "title"}}}

	// Role and tenant checks: admin+ only; non-members and other workspaces see 404.
	for _, u := range []string{"bob", "carol"} {
		if r := e.upload(u, importsPath+"/preview", "a.csv", csv, "", nil); r.Status != 403 {
			t.Errorf("%s preview: %d", u, r.Status)
		}
		if r := e.upload(u, importsPath, "a.csv", csv, "", mapping); r.Status != 403 {
			t.Errorf("%s import: %d", u, r.Status)
		}
	}
	if r := e.upload("dave", importsPath, "a.csv", csv, "", mapping); r.Status != 404 {
		t.Errorf("non-member: %d", r.Status)
	}
	e.must(201, "dave", "POST", "/api/v1/workspaces", map[string]any{"slug": "other", "name": "Other"})
	e.must(201, "dave", "POST", "/api/v1/workspaces/other/projects", map[string]any{"key": "OTH", "name": "Other"})
	if r := e.upload("alice", "/api/v1/workspaces/other/projects/OTH/imports", "a.csv", csv, "", mapping); r.Status != 404 {
		t.Errorf("other workspace: %d", r.Status)
	}
	if r := e.upload("alice", "/api/v1/workspaces/acme/projects/NOPE/imports/preview", "a.csv", csv, "", nil); r.Status != 404 {
		t.Errorf("unknown project: %d", r.Status)
	}
	if r := e.upload("dave", "/api/v1/workspaces/other/projects/OTH/imports", "a.csv", csv, "", mapping); r.Status != 201 {
		t.Errorf("owner of other workspace: %d %s", r.Status, r.Raw)
	}

	// Size and shape limits.
	huge := bytes.Repeat([]byte("x"), maxImportFileBytes+1)
	if r := e.upload("alice", importsPath+"/preview", "big.csv", huge, "", nil); r.Status != 413 || errCode(r) != "file_too_large" {
		t.Errorf("oversized file: %d %s", r.Status, r.Raw)
	}
	rows := "Başlıq\n" + strings.Repeat("x\n", 20_001)
	if r := e.upload("alice", importsPath+"/preview", "many.csv", []byte(rows), "", nil); r.Status != 413 || errCode(r) != "too_many_rows" {
		t.Errorf("too many rows: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath+"/preview", "w1254.csv", []byte("Ba\xfel\xfdq\nx\n"), "", nil); r.Status != 400 || errCode(r) != "invalid_encoding" {
		t.Errorf("non-UTF-8: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath+"/preview", "a.pdf", []byte("%PDF-1.7"), "", nil); r.Status != 400 || errCode(r) != "unsupported_format" {
		t.Errorf("pdf: %d %s", r.Status, r.Raw)
	}
	if r := e.call("alice", "POST", importsPath+"/preview", map[string]any{"file": "x"}); r.Status != 415 {
		t.Errorf("JSON body: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath, "a.csv", csv, "", nil); r.Status != 400 {
		t.Errorf("missing mapping: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath, "a.csv", []byte("Başlıq\n"), "", mapping); r.Status != 400 || errCode(r) != "empty_file" {
		t.Errorf("no data rows: %d %s", r.Status, r.Raw)
	}
	for name, m := range map[string]any{
		"no title":         map[string]any{"columns": []map[string]any{}},
		"column range":     map[string]any{"columns": []map[string]any{{"column": 5, "target": "title"}}},
		"unknown target":   map[string]any{"columns": []map[string]any{{"column": 0, "target": "field.nope"}}},
		"duplicate target": map[string]any{"columns": []map[string]any{{"column": 0, "target": "title"}, {"column": 0, "target": "title"}}},
		"unknown type":     map[string]any{"columns": []map[string]any{{"column": 0, "target": "title"}}, "default_type": "nope"},
	} {
		if r := e.upload("alice", importsPath, "a.csv", csv, "", m); r.Status != 422 {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	if r := e.upload("alice", importsPath, "a.csv", csv, "", map[string]any{"columns": []any{}, "extra": 1}); r.Status != 400 {
		t.Errorf("unknown mapping key: %d %s", r.Status, r.Raw)
	}
	if r := e.upload("alice", importsPath+"?dry_run=maybe", "a.csv", csv, "", mapping); r.Status != 400 {
		t.Errorf("bad dry_run: %d", r.Status)
	}
	if n := e.count(`SELECT count(*) FROM items i JOIN projects p ON p.id = i.project_id WHERE p.key = 'ACME'`); n != 0 {
		t.Fatalf("rejected requests wrote %d items", n)
	}
}

func TestCleanFileName(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":        "passwd",
		`C:\Users\x\Müştəri.xlsx`: "Müştəri.xlsx",
		"a\x00b\nc.csv":           "abc.csv",
		"":                        "upload",
		strings.Repeat("ə", 300):  strings.Repeat("ə", 200),
	} {
		if got := cleanFileName(in); got != want {
			t.Errorf("cleanFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
