// Package opt provides a JSON field type for PATCH requests that distinguishes
// "absent" (leave unchanged), "null" (clear) and a value (set).
package opt

import "encoding/json"

// Field is an optional, nullable JSON value.
type Field[T any] struct {
	Set   bool // key was present in the JSON body
	Null  bool // value was JSON null
	Value T
}

// UnmarshalJSON is only invoked when the key is present.
func (f *Field[T]) UnmarshalJSON(b []byte) error {
	f.Set = true
	if string(b) == "null" {
		f.Null = true
		var zero T
		f.Value = zero
		return nil
	}
	f.Null = false
	return json.Unmarshal(b, &f.Value)
}

// Ptr returns nil for null and a pointer to the value otherwise.
func (f Field[T]) Ptr() *T {
	if f.Null {
		return nil
	}
	v := f.Value
	return &v
}

// Of returns a set, non-null field (useful in tests).
func Of[T any](v T) Field[T] { return Field[T]{Set: true, Value: v} }

// Null returns a set, null field.
func Null[T any]() Field[T] { return Field[T]{Set: true, Null: true} }
