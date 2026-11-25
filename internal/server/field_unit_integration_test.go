package server

import "testing"

// A number field's display unit (options.unit): number fields only, short,
// no digits; admins set, change and clear it; members and guests cannot.
func TestNumberFieldUnit(t *testing.T) {
	e := newEnv(t)
	e.setup()

	body := map[string]any{"key": "balance", "name": "Qalıq", "type": "number", "options": map[string]any{"unit": " ₼ "}}
	e.must(403, "carol", "POST", fieldsPath, body) // member
	e.must(403, "bob", "POST", fieldsPath, body)   // guest
	f := e.must(201, "alice", "POST", fieldsPath, body)
	if got := f.Body["options"].(map[string]any)["unit"]; got != "₼" {
		t.Fatalf("unit = %v, want trimmed ₼", got)
	}

	// Only number fields; validation.
	e.must(422, "alice", "POST", fieldsPath, map[string]any{"key": "nm", "name": "Ad", "type": "text", "options": map[string]any{"unit": "₼"}})
	for _, bad := range []string{"123456789", "AZN2", "abcdefghi", "a\tb"} {
		e.must(422, "alice", "PATCH", fieldsPath+"/balance", map[string]any{"options": map[string]any{"unit": bad}})
	}

	// Change, keep (options without unit), clear.
	e.must(403, "carol", "PATCH", fieldsPath+"/balance", map[string]any{"options": map[string]any{"unit": "AZN"}})
	f = e.must(200, "alice", "PATCH", fieldsPath+"/balance", map[string]any{"options": map[string]any{"unit": "AZN"}})
	if got := f.Body["options"].(map[string]any)["unit"]; got != "AZN" {
		t.Fatalf("unit after change = %v", got)
	}
	f = e.must(200, "alice", "PATCH", fieldsPath+"/balance", map[string]any{"name": "Qalıq (AZN)"})
	if got := f.Body["options"].(map[string]any)["unit"]; got != "AZN" {
		t.Fatalf("rename dropped the unit: %v", got)
	}
	f = e.must(200, "alice", "PATCH", fieldsPath+"/balance", map[string]any{"options": map[string]any{"unit": ""}})
	if _, has := f.Body["options"].(map[string]any)["unit"]; has {
		t.Fatalf("unit not cleared: %v", f.Body["options"])
	}

	// A text field cannot get one later either.
	e.must(201, "alice", "POST", fieldsPath, map[string]any{"key": "note2", "name": "Qeyd", "type": "text"})
	e.must(422, "alice", "PATCH", fieldsPath+"/note2", map[string]any{"options": map[string]any{"unit": "₼"}})
}
