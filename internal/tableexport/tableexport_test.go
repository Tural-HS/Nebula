package tableexport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestNeutralize(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"Ödəniş":               "Ödəniş",
		"=1+1":                 "'=1+1",
		"+994 50 000 00 00":    "'+994 50 000 00 00",
		"-ödənilməyib":         "'-ödənilməyib",
		"@SUM(A1)":             "'@SUM(A1)",
		"\tx":                  "'\tx",
		"\rx":                  "'\rx",
		"  =HYPERLINK(\"x\")":  "'  =HYPERLINK(\"x\")",
		"\n=cmd|' /C calc'!A0": "'\n=cmd|' /C calc'!A0",
		"＝1+1":                 "'＝1+1",
		"a=b":                  "a=b",
		"-12.50":               "-12.50", // a plain number evaluates to itself
		"-12,5":                "-12,5",
		"+5":                   "+5",
		"-":                    "'-",
		"-1+2":                 "'-1+2",
		"-12.":                 "'-12.",
		"#div0":                "#div0",
		"ACME-1":               "ACME-1",
	}
	for in, want := range cases {
		if got := Neutralize(in); got != want {
			t.Errorf("Neutralize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	two := 2
	cases := []struct {
		v     float64
		scale *int
		comma bool
		want  string
	}{
		{1234567.5, nil, false, "1234567.5"},
		{1234567.5, nil, true, "1234567,5"},
		{-0.25, nil, false, "-0.25"},
		{-1500, &two, true, "-1500,00"},
		{1e15, nil, false, "1000000000000000"},
		{-0.001, &two, false, "0.00"},
	}
	for _, c := range cases {
		if got := FormatNumber(c.v, c.scale, c.comma); got != c.want {
			t.Errorf("FormatNumber(%v, %v, %v) = %q, want %q", c.v, c.scale, c.comma, got, c.want)
		}
	}
}

var (
	day   = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	stamp = time.Date(2026, 11, 25, 14, 30, 5, 0, time.FixedZone("AZT", 4*3600))
)

func sample(t *testing.T, w Writer) {
	t.Helper()
	two := 2
	cols := []Column{{Header: "#"}, {Header: "Başlıq"}, {Header: "Məbləğ", Kind: Number, Scale: &two},
		{Header: "Son tarix", Kind: Date}, {Header: "Yenilənib", Kind: DateTime}, {Header: "Təsdiq", Kind: Bool}}
	if err := w.Header(cols); err != nil {
		t.Fatal(err)
	}
	rows := [][]Cell{
		{TextCell("ACME-1"), TextCell("=HYPERLINK(\"http://x\")"), NumberCell(-1500.5), DateCell(day), DateTimeCell(stamp), BoolCell(true)},
		{TextCell("ACME-2"), TextCell("Şirkət; \"ödəniş\""), {}, {}, DateTimeCell(stamp), BoolCell(false)},
	}
	for _, r := range rows {
		if err := w.Row(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCSV(t *testing.T) {
	for _, semi := range []bool{false, true} {
		var buf bytes.Buffer
		w, err := NewCSV(&buf, CSVOptions{Semicolon: semi, Labels: Labels{Yes: "Bəli", No: "Xeyr"}})
		if err != nil {
			t.Fatal(err)
		}
		sample(t, w)
		raw := buf.String()
		if !strings.HasPrefix(raw, "\xef\xbb\xbf") || !strings.Contains(raw, "\r\n") {
			t.Fatalf("BOM and CRLF expected: %q", raw[:20])
		}
		r := csv.NewReader(strings.NewReader(strings.TrimPrefix(raw, "\xef\xbb\xbf")))
		if semi {
			r.Comma = ';'
		}
		recs, err := r.ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		amount := "-1500.50"
		if semi {
			amount = "-1500,50"
		}
		want := [][]string{
			{"#", "Başlıq", "Məbləğ", "Son tarix", "Yenilənib", "Təsdiq"},
			{"ACME-1", "'=HYPERLINK(\"http://x\")", amount, "2026-11-25", "2026-11-25 14:30:05", "Bəli"},
			{"ACME-2", "Şirkət; \"ödəniş\"", "", "", "2026-11-25 14:30:05", "Xeyr"},
		}
		if len(recs) != len(want) {
			t.Fatalf("rows: %v", recs)
		}
		for i := range want {
			if strings.Join(recs[i], "|") != strings.Join(want[i], "|") {
				t.Errorf("semicolon=%v row %d: %q, want %q", semi, i, recs[i], want[i])
			}
		}
	}
}

func TestXLSX(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewXLSX(&buf, XLSXOptions{Sheet: "ACME", Labels: Labels{Yes: "Bəli", No: "Xeyr"}})
	if err != nil {
		t.Fatal(err)
	}
	sample(t, w)
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if got := f.GetSheetList(); len(got) != 1 || got[0] != "ACME" {
		t.Fatalf("sheets: %v", got)
	}
	rows, err := f.GetRows("ACME", excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	// Text is kept verbatim (no apostrophe) as an inline string, never a formula.
	if len(rows) != 3 || rows[0][1] != "Başlıq" || rows[1][1] != "=HYPERLINK(\"http://x\")" {
		t.Fatalf("rows: %q", rows)
	}
	if typ, _ := f.GetCellType("ACME", "B2"); typ != excelize.CellTypeInlineString {
		t.Errorf("text cell type %v, want inline string", typ)
	}
	if formula, err := f.GetCellFormula("ACME", "B2"); err != nil || formula != "" {
		t.Errorf("text cell has a formula: %q %v", formula, err)
	}
	// Typed cells: the amount is a number, the dates are date serials.
	if typ, _ := f.GetCellType("ACME", "C2"); typ != excelize.CellTypeUnset && typ != excelize.CellTypeNumber {
		t.Errorf("amount cell type %v", typ)
	}
	if rows[1][2] != "-1500.5" {
		t.Errorf("amount raw %q", rows[1][2])
	}
	if rows[1][3] != "46351" { // 2026-11-25 as an Excel serial
		t.Errorf("date raw %q", rows[1][3])
	}
	if !strings.HasPrefix(rows[1][4], "46351.60") { // 14:30:05 local wall time
		t.Errorf("datetime raw %q", rows[1][4])
	}
	if rows[1][5] != "Bəli" || rows[2][5] != "Xeyr" {
		t.Errorf("booleans %q %q", rows[1][5], rows[2][5])
	}
	shown, _ := f.GetCellValue("ACME", "C2")
	if shown != "-1,500.50" {
		t.Errorf("amount shown as %q", shown)
	}
	if formula, _ := f.GetCellFormula("ACME", "B2"); formula != "" {
		t.Errorf("text became a formula: %q", formula)
	}
}

func TestXLSXBudget(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewXLSX(&buf, XLSXOptions{Sheet: "X", MaxBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer Abort(w)
	if err := w.Header([]Column{{Header: "a"}}); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if err = w.Row([]Cell{TextCell(strings.Repeat("x", 50))}); err != nil {
			break
		}
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSheetName(t *testing.T) {
	if got := SheetName("a/b:c" + strings.Repeat("ş", 40)); got != "a_b_c"+strings.Repeat("ş", 26) {
		t.Errorf("SheetName = %q", got)
	}
	if SheetName("''") != "Sheet1" {
		t.Error("empty name")
	}
}
