package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Tural-HS/Nebula/internal/formula"
)

func TestFormulaGraph(t *testing.T) {
	defs := map[string]formulaDef{
		"amount": {key: "amount", typ: FieldNumber},
		"note":   {key: "note", typ: FieldText},
		"old":    {key: "old", typ: FieldNumber, archived: true},
	}
	add := func(key, expr string) error {
		opts, _, err := compileFormulaDef(defs, key, FormulaInput{Expression: expr})
		if err == nil {
			defs[key] = formulaDef{key: key, typ: FieldFormula, formula: opts}
		}
		return err
	}
	if err := add("a", "amount + 1"); err != nil {
		t.Fatal(err)
	}
	if err := add("b", "a * 2"); err != nil {
		t.Fatal(err)
	}
	for expr, want := range map[string]string{
		"b + 1":      "circular reference: a → b → a",
		"a":          "cannot use its own value",
		"old":        `field "old" is archived`,
		"note":       "is a text field",
		"@created":   `unknown field "@created"`,
		"amount + x": `unknown field "x"`,
	} {
		err := add("a", expr)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a = %s: %v, want %q", expr, err, want)
		}
	}
	// Chains: c1 -> c2 -> ... are bounded, from either end.
	prev := "amount"
	for i := range maxFormulaChain {
		key := fmt.Sprintf("c%d", i)
		if err := add(key, prev+" + 1"); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		prev = key
	}
	if err := add("too_deep", prev+" + 1"); err == nil || !strings.Contains(err.Error(), "levels deep") {
		t.Fatalf("chain limit: %v", err)
	}
}

func TestSameStoredValue(t *testing.T) {
	for _, tc := range []struct {
		a, b any
		want bool
	}{
		{18.36, json.Number("18.36"), true},
		{1000000000000000.1, json.Number("1000000000000000.1"), true},
		{18.0, json.Number("18"), true},
		{18.36, json.Number("18.37"), false},
		{nil, nil, true},
		{nil, 1.0, false},
		{map[string]any{"error": "division_by_zero"}, map[string]any{"error": "division_by_zero"}, true},
		{map[string]any{"error": "overflow"}, 1.0, false},
	} {
		if got := sameStoredValue(formula.TypeNumber, tc.a, tc.b); got != tc.want {
			t.Errorf("sameStoredValue(%v, %v) = %v", tc.a, tc.b, got)
		}
	}
}
