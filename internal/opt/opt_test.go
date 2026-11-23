package opt

import (
	"encoding/json"
	"testing"
)

func TestField(t *testing.T) {
	var body struct {
		A Field[string] `json:"a"`
		B Field[string] `json:"b"`
		C Field[string] `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"x","b":null}`), &body); err != nil {
		t.Fatal(err)
	}
	if !body.A.Set || body.A.Null || body.A.Value != "x" || *body.A.Ptr() != "x" {
		t.Errorf("a = %+v", body.A)
	}
	if !body.B.Set || !body.B.Null || body.B.Ptr() != nil {
		t.Errorf("b = %+v", body.B)
	}
	if body.C.Set {
		t.Errorf("c must be unset: %+v", body.C)
	}
	if err := json.Unmarshal([]byte(`{"a":1}`), &body); err == nil {
		t.Error("type mismatch must fail")
	}
}
