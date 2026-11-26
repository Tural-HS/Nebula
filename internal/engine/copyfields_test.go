package engine

import (
	"testing"
)

func TestCopyPatch(t *testing.T) {
	src := map[string]any{
		"balance": 1200.5, "phone": "+994 50 000 00 00",
		"price": map[string]any{"amount": "10.00", "currency": "AZN"}, "note": "same",
	}
	dst := map[string]any{
		"balance": 900.0, "phone": "+994 50 000 00 00", "monthly_payment": 50.0,
		"price": map[string]any{"currency": "AZN", "amount": "10.00"}, "note": "same",
	}
	bound := map[string]bool{"balance": true, "phone": true, "price": true, "note": true}
	keys := []string{"balance", "phone", "monthly_payment", "price", "note", "unbound", "absent"}
	src["unbound"] = "x"

	patch, err := CopyPatch(src, dst, keys, bound)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"balance": "1200.5", "monthly_payment": "null"}
	if len(patch) != len(want) {
		t.Fatalf("patch: %v", stringify(patch))
	}
	for k, v := range want {
		if string(patch[k]) != v {
			t.Errorf("%s: got %s, want %s", k, patch[k], v)
		}
	}

	// Converged: nothing to write (no event, no version bump).
	if patch, _ := CopyPatch(src, src, []string{"balance", "phone", "price"}, bound); len(patch) != 0 {
		t.Fatalf("converged patch: %v", stringify(patch))
	}
}

func stringify[V ~[]byte](m map[string]V) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = string(v)
	}
	return out
}
