package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func parse(t *testing.T, name string, data []byte, sheet string, lim Limits) *Table {
	t.Helper()
	tab, err := Parse(context.Background(), name, data, sheet, lim)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return tab
}

func wantCode(t *testing.T, err error, code string, large bool) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want importer error %s, got %v", code, err)
	}
	if e.Code != code || e.TooLarge != large {
		t.Fatalf("got %s (too large %v), want %s (too large %v): %s", e.Code, e.TooLarge, code, large, e.Message)
	}
}

func TestCSVDelimitersAndBOM(t *testing.T) {
	cases := map[string]struct {
		data  string
		delim string
	}{
		"comma":           {"Ad,Məbləğ\nƏli,\"1,5\"\n", ","},
		"semicolon + BOM": {"\xEF\xBB\xBFAd;Məbləğ\r\nƏli;1,5\r\n", ";"},
		"tab":             {"Ad\tMəbləğ\nƏli\t1,5\n", "\t"},
		"quoted commas":   {"\"Ad, soyad\";Məbləğ\nƏli;1,5\n", ";"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tab := parse(t, "müştərilər.csv", []byte(c.data), "", DefaultLimits)
			if tab.Delimiter != c.delim || tab.Format != FormatCSV {
				t.Fatalf("delimiter %q, want %q", tab.Delimiter, c.delim)
			}
			if tab.Headers[1] != "Məbləğ" || len(tab.Rows) != 1 || tab.Rows[0].Cells[0] != "Əli" || tab.Rows[0].Cells[1] != "1,5" {
				t.Fatalf("table: %#v", tab)
			}
			if tab.Rows[0].Line != 2 {
				t.Fatalf("line %d", tab.Rows[0].Line)
			}
		})
	}
	if tab := parse(t, "x.tsv", []byte("a,b\tc\n1\t2\n"), "", DefaultLimits); tab.Delimiter != "\t" || tab.Headers[0] != "a,b" {
		t.Fatalf("tsv: %#v", tab)
	}
}

func TestCSVBlankRowsRaggedRowsAndLines(t *testing.T) {
	data := "\n\nAd;Filial;;\n\nŞirin;Bakı\n;;;\nÜmid;Gəncə;əlavə\n"
	tab := parse(t, "a.csv", []byte(data), "", DefaultLimits)
	if tab.Width() != 3 || tab.Headers[2] != "" {
		t.Fatalf("headers: %q", tab.Headers)
	}
	if len(tab.Rows) != 2 || tab.Rows[0].Line != 5 || tab.Rows[1].Line != 7 {
		t.Fatalf("rows: %#v", tab.Rows)
	}
	if got := tab.Rows[0].Cells; len(got) != 3 || got[2] != "" {
		t.Fatalf("padding: %q", got)
	}
	if tab.Rows[1].Cells[2] != "əlavə" {
		t.Fatalf("extra column lost: %q", tab.Rows[1].Cells)
	}
}

func TestCSVEncodingAndSyntaxErrors(t *testing.T) {
	_, err := Parse(context.Background(), "a.csv", []byte("Ad\n\xC6li\n"), "", DefaultLimits) // Windows-1254 'Ə'-ish byte
	wantCode(t, err, "invalid_encoding", false)
	_, err = Parse(context.Background(), "a.csv", []byte{0xFF, 0xFE, 'A', 0}, "", DefaultLimits)
	wantCode(t, err, "invalid_encoding", false)
	_, err = Parse(context.Background(), "a.csv", []byte("Ad\n\"open\n"), "", DefaultLimits)
	wantCode(t, err, "malformed_file", false)
	_, err = Parse(context.Background(), "a.csv", []byte("Ad\x00\n"), "", DefaultLimits)
	wantCode(t, err, "malformed_file", false)
	_, err = Parse(context.Background(), "a.csv", []byte("\n \n"), "", DefaultLimits)
	wantCode(t, err, "empty_file", false)
	_, err = Parse(context.Background(), "a.csv", nil, "", DefaultLimits)
	wantCode(t, err, "empty_file", false)
	_, err = Parse(context.Background(), "a.ods", []byte("Ad\n"), "", DefaultLimits)
	wantCode(t, err, "unsupported_format", false)
	_, err = Parse(context.Background(), "a.xls", append(append([]byte{}, oleMagic...), 0, 0), "", DefaultLimits)
	wantCode(t, err, "unsupported_format", false)
	_, err = Parse(context.Background(), "a.xlsx", []byte("not a zip"), "", DefaultLimits)
	wantCode(t, err, "malformed_file", false)
	_, err = Parse(context.Background(), "a.csv", []byte("Ad\nx\n"), "Sheet1", DefaultLimits)
	wantCode(t, err, "sheet_not_found", false)
}

func TestLimits(t *testing.T) {
	lim := Limits{MaxRows: 3, MaxColumns: 4, MaxCellRunes: 10, MaxUnzippedBytes: 1 << 20, MaxZipEntries: 100}
	if tab := parse(t, "a.csv", []byte("h\n1\n2\n3\n\n\n"), "", lim); len(tab.Rows) != 3 {
		t.Fatalf("exactly the limit must pass: %d", len(tab.Rows))
	}
	_, err := Parse(context.Background(), "a.csv", []byte("h\n1\n2\n3\n4\n"), "", lim)
	wantCode(t, err, "too_many_rows", true)
	_, err = Parse(context.Background(), "a.csv", []byte("a,b,c,d,e\n"), "", lim)
	wantCode(t, err, "too_many_columns", true)
	_, err = Parse(context.Background(), "a.csv", []byte("h\n"+strings.Repeat("ə", 11)+"\n"), "", lim)
	wantCode(t, err, "cell_too_long", true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	big := "h\n" + strings.Repeat("x\n", 300)
	_, err = Parse(ctx, "a.csv", []byte(big), "", DefaultLimits)
	wantCode(t, err, "timeout", false)
}

func TestXLSXZipBombRejectedBeforeInflating(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xl/worksheets/sheet1.xml")
	zeros := make([]byte, 1<<20)
	for range 3 { // 3 MiB of zeros compress to a few KiB
		_, _ = w.Write(zeros)
	}
	_ = zw.Close()
	if buf.Len() > 64<<10 {
		t.Fatalf("fixture is not a bomb: %d bytes", buf.Len())
	}
	lim := DefaultLimits
	lim.MaxUnzippedBytes = 2 << 20
	_, err := Parse(context.Background(), "bomb.xlsx", buf.Bytes(), "", lim)
	wantCode(t, err, "file_too_large", true)

	buf.Reset()
	zw = zip.NewWriter(&buf)
	for i := range 20 {
		_, _ = zw.Create(strings.Repeat("p", i+1))
	}
	_ = zw.Close()
	lim.MaxZipEntries = 10
	_, err = Parse(context.Background(), "many.xlsx", buf.Bytes(), "", lim)
	wantCode(t, err, "file_too_large", true)

	_, err = Parse(context.Background(), "a.zip", buf.Bytes(), "", DefaultLimits)
	wantCode(t, err, "unsupported_format", false)
}

func workbook(t *testing.T, build func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	build(f)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestXLSXSheetsRawValuesAndFormulas(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		_ = f.SetSheetName("Sheet1", "Gizli")
		_ = f.SetCellValue("Gizli", "A1", "secret")
		_, _ = f.NewSheet("Müştərilər")
		s := "Müştərilər"
		_ = f.SetSheetRow(s, "A1", &[]any{"Ad", "Məbləğ", "Tarix", "Cəm"})
		_ = f.SetSheetRow(s, "A2", &[]any{"Şəhla İsmayılova", 1234.5, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)})
		_ = f.SetCellFormula(s, "D2", "B2*2")
		// Row 3 left blank on purpose; row 4 has data.
		_ = f.SetSheetRow(s, "A4", &[]any{"Ömər", "1 200 ₼"})
		f.SetActiveSheet(1)
		if err := f.SetSheetVisible("Gizli", false); err != nil {
			t.Fatal(err)
		}
	})

	tab := parse(t, "book.xlsx", data, "", DefaultLimits)
	if tab.Sheet != "Müştərilər" || len(tab.Sheets) != 2 {
		t.Fatalf("first visible sheet: %q of %q", tab.Sheet, tab.Sheets)
	}
	if tab.Format != FormatXLSX || tab.Width() != 4 || tab.Headers[1] != "Məbləğ" {
		t.Fatalf("headers: %q", tab.Headers)
	}
	if len(tab.Rows) != 2 || tab.Rows[0].Line != 2 || tab.Rows[1].Line != 4 {
		t.Fatalf("rows: %#v", tab.Rows)
	}
	r := tab.Rows[0].Cells
	if r[0] != "Şəhla İsmayılova" || r[1] != "1234.5" {
		t.Fatalf("raw values: %q", r)
	}
	// Dates are stored as serial numbers; formulas are not evaluated (no cached value written by excelize).
	if d, err := ParseDate(r[2], true, tab.Date1904); err != nil || d != "2026-03-15" {
		t.Fatalf("serial date %q: %v %v", r[2], d, err)
	}
	if r[3] != "" {
		t.Fatalf("formula must not be evaluated: %q", r[3])
	}

	hidden := parse(t, "book.xlsx", data, "Gizli", DefaultLimits)
	if hidden.Headers[0] != "secret" || len(hidden.Rows) != 0 {
		t.Fatalf("chosen sheet: %#v", hidden)
	}
	_, err := Parse(context.Background(), "book.xlsx", data, "Yoxdur", DefaultLimits)
	wantCode(t, err, "sheet_not_found", false)
}

func TestParseNumber(t *testing.T) {
	ok := map[string]float64{
		"1234":        1234,
		"1 234,56":    1234.56,
		"1 234":       1234,
		"1.234,56 ₼":  1234.56,
		"1,234.56":    1234.56,
		"₼ 1 200":     1200,
		"1200 AZN":    1200,
		"250 manat":   250,
		"12,5":        12.5,
		"12.5":        12.5,
		"0.125":       0.125,
		"-45":         -45,
		"−45,5":       -45.5,
		"(300)":       -300,
		"1.234.567":   1234567,
		"1,234,567.5": 1234567.5,
		",5":          0.5,
		"1'234":       1234,
		"$99.99":      99.99,
	}
	for in, want := range ok {
		if got, err := ParseNumber(in, false); err != nil || got != want {
			t.Errorf("ParseNumber(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "12a", "1,2,3", "1.23.4", "1,2345.6", "1..2", "--1", "1e5"} {
		if _, err := ParseNumber(in, false); err == nil {
			t.Errorf("ParseNumber(%q) should fail", in)
		}
	}
	for _, in := range []string{"1.234", "12,500", "999.000"} {
		if _, err := ParseNumber(in, false); !errors.Is(err, ErrAmbiguousNumber) {
			t.Errorf("ParseNumber(%q) must be ambiguous, got %v", in, err)
		}
	}
	// xlsx stored values are machine format: "1.234" is exactly 1.234 there.
	if v, err := ParseNumber("1.234", true); err != nil || v != 1.234 {
		t.Fatalf("canonical: %v %v", v, err)
	}
	if v, err := ParseNumber("1E+3", true); err != nil || v != 1000 {
		t.Fatalf("canonical exponent: %v %v", v, err)
	}
}

func TestParseDates(t *testing.T) {
	ok := map[string]string{
		"2026-03-15":           "2026-03-15",
		"15.03.2026":           "2026-03-15",
		"5.3.2026":             "2026-03-05",
		"15/03/2026":           "2026-03-15",
		"15-03-2026":           "2026-03-15",
		"2026-03-15 10:30":     "2026-03-15",
		"2026-03-15T23:30:00Z": "2026-03-15",
	}
	for in, want := range ok {
		if got, err := ParseDate(in, false, false); err != nil || got != want {
			t.Errorf("ParseDate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "45366", "31.02.2026", "2026/13/01", "tomorrow"} {
		if _, err := ParseDate(in, false, false); err == nil {
			t.Errorf("ParseDate(%q) should fail", in)
		}
	}
	serial := map[string]string{"45366": "2024-03-15", "1": "1900-01-01", "61": "1900-03-01", "45366.75": "2024-03-15"}
	for in, want := range serial {
		if got, err := ParseDate(in, true, false); err != nil || got != want {
			t.Errorf("serial %q = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, _ := ParseDate("0", true, true); got != "" {
		t.Errorf("serial 0 must fail, got %q", got)
	}
	if got, err := ParseDate("43904", true, true); err != nil || got != "2024-03-15" {
		t.Errorf("1904 serial: %q %v", got, err)
	}
	for _, in := range []string{"-1", "3000000"} {
		if _, err := ParseDate(in, true, false); err == nil {
			t.Errorf("serial %q out of range must fail", in)
		}
	}

	dt := map[string]string{
		"2026-03-15 10:30":          "2026-03-15T10:30:00Z",
		"15.03.2026 10:30:15":       "2026-03-15T10:30:15Z",
		"2026-03-15T10:30:00+04:00": "2026-03-15T06:30:00Z",
		"2026-03-15":                "2026-03-15T00:00:00Z",
	}
	for in, want := range dt {
		if got, err := ParseDateTime(in, false, false); err != nil || got != want {
			t.Errorf("ParseDateTime(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := ParseDateTime("45366.5", true, false); err != nil || got != "2024-03-15T12:00:00Z" {
		t.Errorf("serial datetime: %q %v", got, err)
	}
}

func TestParseBoolFoldAndList(t *testing.T) {
	for _, in := range []string{"Bəli", "HƏ", "yes", "TRUE", "1", "✓"} {
		if v, err := ParseBool(in); err != nil || !v {
			t.Errorf("ParseBool(%q) = %v, %v", in, v, err)
		}
	}
	for _, in := range []string{"Xeyr", "YOX", "no", "0", "Yanlış"} {
		if v, err := ParseBool(in); err != nil || v {
			t.Errorf("ParseBool(%q) = %v, %v", in, v, err)
		}
	}
	if _, err := ParseBool("bəlkə"); err == nil {
		t.Error("unknown word must fail")
	}
	pairs := [][2]string{{"İSMAYILLI", "ismayilli"}, {"Gəncə  filialı", "gəncə filiali"}, {"IĞDIR", "iğdir"}}
	for _, p := range pairs {
		if Fold(p[0]) != Fold(p[1]) {
			t.Errorf("Fold(%q)=%q != Fold(%q)=%q", p[0], Fold(p[0]), p[1], Fold(p[1]))
		}
	}
	if got := SplitList(" VIP; Korporativ ,, "); len(got) != 2 || got[0] != "VIP" || got[1] != "Korporativ" {
		t.Errorf("SplitList: %q", got)
	}
}

func TestSuggest(t *testing.T) {
	targets := []Target{
		{ID: "title", Names: []string{"title", "Başlıq"}, Synonyms: []string{"ad", "müştəri", "name"}},
		{ID: "assignee", Names: []string{"assignee", "İcraçı"}, Synonyms: []string{"məsul"}},
		{ID: "field.contract_no", Names: []string{"contract_no", "Müqavilə №"}},
		{ID: "field.branch", Names: []string{"branch", "Filial"}},
		{ID: "field.ad", Names: []string{"ad", "Ad"}},
	}
	got := Suggest([]string{"Müştəri", "FİLİAL", "Müqavilə  №", "Məsul", "Ad", "Naməlum", ""}, targets)
	want := map[int]string{0: "title", 1: "field.branch", 2: "field.contract_no", 3: "assignee", 4: "field.ad"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("column %d: got %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	// A duplicated header maps only once (leftmost).
	got = Suggest([]string{"Filial", "filial"}, targets)
	if len(got) != 1 || got[0] != "field.branch" {
		t.Fatalf("duplicate header: %v", got)
	}
}

func TestColumnName(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 701: "ZZ", 702: "AAA"} {
		if got := ColumnName(i); got != want {
			t.Errorf("ColumnName(%d) = %q, want %q", i, got, want)
		}
	}
}
