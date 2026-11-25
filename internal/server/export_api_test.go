package server

import (
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/tableexport"
)

func TestExportDisposition(t *testing.T) {
	cases := map[string]string{
		"Hüquq şöbəsi": `attachment; filename="SEC-2026-11-25.xlsx"; filename*=UTF-8''H%C3%BCquq%20%C5%9F%C3%B6b%C9%99si-2026-11-25.xlsx`,
		// Quotes, separators and line breaks never reach the header raw.
		"a\"b\r\nX-Evil: 1;c/d": `attachment; filename="SEC-2026-11-25.xlsx"; filename*=UTF-8''a_b__X-Evil_%201%3Bc_d-2026-11-25.xlsx`,
		"   ":                   `attachment; filename="SEC-2026-11-25.xlsx"; filename*=UTF-8''SEC-2026-11-25.xlsx`,
	}
	for name, want := range cases {
		if got := exportDisposition("SEC", name, "2026-11-25", "xlsx"); got != want {
			t.Errorf("%q:\n got %s\nwant %s", name, got, want)
		}
	}
}

func TestResolveExportColumns(t *testing.T) {
	two := 2
	defs := []store.FieldDefinition{
		{Key: "amount", Name: "Məbləğ", Type: store.FieldNumber},
		{Key: "left", Name: "Qalıq", Type: store.FieldFormula, Options: store.FieldOptions{Formula: &store.FormulaOptions{ResultType: "number", Scale: &two}}},
		{Key: "old", Name: "Köhnə", Type: store.FieldText, Archived: true},
		{Key: "paid_on", Name: "Ödənilib", Type: store.FieldDate},
	}
	cols := resolveExportColumns([]string{"field.amount", "status", "field.old", "field.gone", "field.left", "field.paid_on", "due_date"}, defs, "az")
	var got []string
	for _, c := range cols {
		got = append(got, c.spec.Header)
	}
	want := "#|Başlıq|Məbləğ|Status|Qalıq|Ödənilib|Son tarix"
	if s := strings.Join(got, "|"); s != want {
		t.Fatalf("headers %s, want %s", s, want)
	}
	if cols[2].spec.Kind != tableexport.Number || cols[4].spec.Scale == nil || *cols[4].spec.Scale != 2 ||
		cols[5].spec.Kind != tableexport.Date || cols[6].spec.Kind != tableexport.Date {
		t.Fatalf("kinds: %+v", cols)
	}
	for in, want := range map[string]string{"key,type": "key,title,type", "type": "key,title,type", "type,key": "title,type,key"} {
		var ids []string
		for _, c := range resolveExportColumns(strings.Split(in, ","), nil, "en") {
			ids = append(ids, c.id)
		}
		if strings.Join(ids, ",") != want {
			t.Errorf("%s: %v, want %s", in, ids, want)
		}
	}
}
