package evidence

import "testing"

func TestValueRendersFormulaErrors(t *testing.T) {
	r := resolver{}
	for in, want := range map[string]struct {
		v    any
		want string
	}{
		"formula error": {map[string]any{"error": "division_by_zero"}, "⚠ division_by_zero"},
		"other object":  {map[string]any{"a": 1.0}, `{"a":1}`},
		"number":        {12.5, "12.5"},
	} {
		if got := r.value(want.v); got != want.want {
			t.Errorf("%s: %q, want %q", in, got, want.want)
		}
	}
}
