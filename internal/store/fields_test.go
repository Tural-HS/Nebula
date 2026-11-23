package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func choiceDef(typ string) FieldDefinition {
	return FieldDefinition{Key: "f", Type: typ, Options: FieldOptions{Choices: []Choice{
		{ID: "aaa", Label: "High"},
		{ID: "bbb", Label: "Low"},
		{ID: "old", Label: "Legacy", Archived: true},
	}}}
}

func TestCanonicalValue(t *testing.T) {
	tests := []struct {
		name  string
		def   FieldDefinition
		in    string // JSON
		want  any
		empty bool
		err   string
	}{
		{"text trims", FieldDefinition{Type: FieldText}, `"  hi  "`, "hi", false, ""},
		{"text blank clears", FieldDefinition{Type: FieldText}, `"   "`, nil, true, ""},
		{"text too long", FieldDefinition{Type: FieldText}, `"` + strings.Repeat("x", 1001) + `"`, nil, false, "at most 1000"},
		{"text wrong type", FieldDefinition{Type: FieldText}, `5`, nil, false, "must be a string"},
		{"long text limit", FieldDefinition{Type: FieldLongText}, `"` + strings.Repeat("x", 5000) + `"`, strings.Repeat("x", 5000), false, ""},
		{"number", FieldDefinition{Type: FieldNumber}, `3.5`, 3.5, false, ""},
		{"number too big", FieldDefinition{Type: FieldNumber}, `1e16`, nil, false, "between"},
		{"number as string", FieldDefinition{Type: FieldNumber}, `"3"`, nil, false, "must be a number"},
		{"boolean", FieldDefinition{Type: FieldBoolean}, `false`, false, false, ""},
		{"date", FieldDefinition{Type: FieldDate}, `"2026-02-28"`, "2026-02-28", false, ""},
		{"date invalid day", FieldDefinition{Type: FieldDate}, `"2026-02-30"`, nil, false, "YYYY-MM-DD"},
		{"datetime to UTC seconds", FieldDefinition{Type: FieldDateTime}, `"2026-11-23T14:05:09.987+04:00"`, "2026-11-23T10:05:09Z", false, ""},
		{"datetime without zone", FieldDefinition{Type: FieldDateTime}, `"2026-11-23T14:05:09"`, nil, false, "RFC 3339"},
		{"select", choiceDef(FieldSelect), `"aaa"`, "aaa", false, ""},
		{"select unknown", choiceDef(FieldSelect), `"zzz"`, nil, false, "unknown choice"},
		{"select archived", choiceDef(FieldSelect), `"old"`, nil, false, "archived"},
		{"select label is not an id", choiceDef(FieldSelect), `"High"`, nil, false, "unknown choice"},
		{"multi sorts and dedupes", choiceDef(FieldMultiSelect), `["bbb","aaa","bbb"]`, []any{"aaa", "bbb"}, false, ""},
		{"multi empty clears", choiceDef(FieldMultiSelect), `[]`, nil, true, ""},
		{"multi non-string", choiceDef(FieldMultiSelect), `[1]`, nil, false, "list of choice ids"},
		{"user canonical uuid", FieldDefinition{Type: FieldUser}, `"0190F3E2-0000-7000-8000-000000000001"`, "0190f3e2-0000-7000-8000-000000000001", false, ""},
		{"user invalid", FieldDefinition{Type: FieldUser}, `"bob"`, nil, false, "user id"},
		{"url https", FieldDefinition{Type: FieldURL}, `"https://example.com/a?b=1"`, "https://example.com/a?b=1", false, ""},
		{"url javascript rejected", FieldDefinition{Type: FieldURL}, `"javascript:alert(1)"`, nil, false, "http or https"},
		{"url without host", FieldDefinition{Type: FieldURL}, `"https:///x"`, nil, false, "http or https"},
		{"email", FieldDefinition{Type: FieldEmail}, `"a.b@example.com"`, "a.b@example.com", false, ""},
		{"email with display name", FieldDefinition{Type: FieldEmail}, `"Bob <b@example.com>"`, nil, false, "email address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.in), &v); err != nil {
				t.Fatal(err)
			}
			got, empty, err := canonicalValue(tt.def, v, "fields.f")
			if tt.err != "" {
				var ve *ValidationError
				if !errors.As(err, &ve) || !strings.Contains(ve.Message, tt.err) || ve.Field != "fields.f" {
					t.Fatalf("err = %v, want validation error containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if empty != tt.empty || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got (%#v, empty=%v), want (%#v, empty=%v)", got, empty, tt.want, tt.empty)
			}
		})
	}
}

func TestNormalizeChoices(t *testing.T) {
	red := "#ff0000"
	created, err := normalizeChoices(nil, []ChoiceInput{{Label: " High ", Color: &red}, {Label: "Low"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || created[0].Label != "High" || created[0].ID == "" || created[0].ID == created[1].ID {
		t.Fatalf("unexpected choices: %+v", created)
	}

	// Relabel and archive keep ids; a new entry gets a fresh id.
	next, err := normalizeChoices(created, []ChoiceInput{
		{ID: created[0].ID, Label: "Urgent"},
		{ID: created[1].ID, Label: "Low", Archived: true},
		{Label: "Medium"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next[0].ID != created[0].ID || next[0].Label != "Urgent" || !next[1].Archived || next[2].ID == "" {
		t.Fatalf("unexpected update: %+v", next)
	}

	bad := []struct {
		name     string
		existing []Choice
		in       []ChoiceInput
		want     string
	}{
		{"removal", created, []ChoiceInput{{ID: created[0].ID, Label: "High"}}, "cannot be removed"},
		{"duplicate label", nil, []ChoiceInput{{Label: "A"}, {Label: "a"}}, "duplicate label"},
		{"unknown id", nil, []ChoiceInput{{ID: "nope", Label: "A"}}, "unknown choice id"},
		{"duplicate id", created, []ChoiceInput{{ID: created[0].ID, Label: "A"}, {ID: created[0].ID, Label: "B"}, {ID: created[1].ID, Label: "C"}}, "duplicate choice id"},
		{"bad color", nil, []ChoiceInput{{Label: "A", Color: new(string)}}, "hex color"},
		{"blank label", nil, []ChoiceInput{{Label: " "}}, "is required"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeChoices(tt.existing, tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDecodeFieldInput(t *testing.T) {
	in, err := decodeFieldInput(map[string]json.RawMessage{"a": json.RawMessage(`null`), "b": json.RawMessage(`"x"`)})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := in["a"]; !ok || v != nil {
		t.Fatalf("null must decode to a present nil value, got %#v", in)
	}
	if in["b"] != "x" {
		t.Fatalf("b = %#v", in["b"])
	}

	big := map[string]json.RawMessage{}
	for i := range maxFieldsPerWrite + 1 {
		big[strings.Repeat("k", i+1)] = json.RawMessage(`1`)
	}
	if _, err := decodeFieldInput(big); err == nil {
		t.Fatal("expected a limit error")
	}
}
