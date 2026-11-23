package engine

import (
	"encoding/json"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/nebula/nebula/internal/store"
)

func cond(field, op string, value any) store.Condition {
	c := store.Condition{Field: field, Op: op}
	if value != nil {
		c.Value, _ = json.Marshal(value)
	}
	return c
}

func TestEvaluate(t *testing.T) {
	s := Snapshot{
		Values: map[string]any{
			"type": "bug", "project": "OPS", "status_category": "todo", "assignee": nil, "title": "Login broken on Safari",
			"fields.severity": "c_high", "fields.tags": []any{"c_a", "c_b"}, "fields.score": 5.0,
		},
		Changed: map[string]bool{"title": true},
	}
	cases := []struct {
		c    store.Condition
		want bool
	}{
		{cond("type", "eq", "bug"), true},
		{cond("type", "eq", "task"), false},
		{cond("type", "neq", "task"), true},
		{cond("project", "in", []string{"DEV", "OPS"}), true},
		{cond("project", "not_in", []string{"DEV", "OPS"}), false},
		{cond("assignee", "is_empty", nil), true},
		{cond("assignee", "is_not_empty", nil), false},
		{cond("title", "contains", "SAFARI"), true},
		{cond("fields.tags", "contains", "c_b"), true},
		{cond("fields.tags", "contains", "c_z"), false},
		{cond("fields.score", "eq", 5), true},
		{cond("fields.missing", "is_empty", nil), true},
		{cond("changed", "includes", "title"), true},
		{cond("changed", "includes", "status_id"), false},
		{cond("type", "unknown_op", "bug"), false},
	}
	for _, tc := range cases {
		if got := Evaluate([]store.Condition{tc.c}, s); got != tc.want {
			t.Errorf("%s %s %s: got %v want %v", tc.c.Field, tc.c.Op, tc.c.Value, got, tc.want)
		}
	}
	if !Evaluate(nil, s) {
		t.Error("no conditions must match")
	}
	if Evaluate([]store.Condition{cond("type", "eq", "bug"), cond("project", "eq", "DEV")}, s) {
		t.Error("conditions are ANDed")
	}
}

func TestRender(t *testing.T) {
	it := store.Item{Key: "OPS-12", Title: "Disk full", Type: store.ItemTypeRef{Name: "Bug"}, Status: store.Status{Name: "Done"}}
	got := Render("[{{item.key}}] {{item.title}} ({{item.type}}/{{item.status}}) in {{item.project}} by {{rule.name}} {{unknown}}", it, "Triage", "acme")
	want := "[OPS-12] Disk full (Bug/Done) in OPS by Triage {{unknown}}"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestSignIsDeterministicHMAC(t *testing.T) {
	ts := time.Unix(1_800_000_000, 0)
	a := Sign("whsec_x", ts, []byte(`{"a":1}`))
	if a != Sign("whsec_x", ts, []byte(`{"a":1}`)) {
		t.Fatal("not deterministic")
	}
	if a == Sign("whsec_y", ts, []byte(`{"a":1}`)) || a == Sign("whsec_x", ts, []byte(`{"a":2}`)) || a == Sign("whsec_x", ts.Add(time.Second), []byte(`{"a":1}`)) {
		t.Fatal("signature must depend on secret, body and timestamp")
	}
	if a[:13] != "t=1800000000," || len(a) != len("t=1800000000,v1=")+64 {
		t.Fatalf("format: %s", a)
	}
}

func TestTruncateKeepsUTF8Valid(t *testing.T) {
	s := "Əliyev şəhər"
	for n := 0; n <= len(s); n++ {
		if !utf8.ValidString(truncate(s, n)) {
			t.Fatalf("n=%d produced invalid UTF-8", n)
		}
	}
	_ = uuid.Nil
}
