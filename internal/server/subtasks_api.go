package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// itemRef is a reference to an item of the current project (ADR-0021): a
// JSON number (12) or a string, either the number ("12") or the key ("OPS-12").
type itemRef struct {
	raw string
}

func (r *itemRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		r.raw = s
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	r.raw = n.String()
	return nil
}

// parseItemRef returns the item number, accepting only keys of projectKey:
// a parent always lives in the same project.
func parseItemRef(raw, projectKey, field string) (int64, error) {
	s := strings.TrimSpace(raw)
	if prefix, num, ok := strings.Cut(s, "-"); ok {
		if !strings.EqualFold(prefix, projectKey) {
			return 0, &store.ValidationError{Field: field, Message: "must be an item of this project"}
		}
		s = num
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, &store.ValidationError{Field: field, Message: "must be an item number or key, e.g. 12 or " + projectKey + "-12"}
	}
	return n, nil
}

// number is the parent of a new item (nil: none).
func (r *itemRef) number(projectKey string) (*int64, error) {
	if r == nil {
		return nil, nil
	}
	n, err := parseItemRef(r.raw, projectKey, "parent")
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// parentField converts a PATCH "parent" (absent / null / ref) into the store's form.
func parentField(f opt.Field[itemRef], projectKey string) (opt.Field[int64], error) {
	switch {
	case !f.Set:
		return opt.Field[int64]{}, nil
	case f.Null:
		return opt.Null[int64](), nil
	}
	n, err := parseItemRef(f.Value.raw, projectKey, "parent")
	if err != nil {
		return opt.Field[int64]{}, err
	}
	return opt.Of(n), nil
}

// parseParentFilters reads ?parent=<number|key> and ?top_level=true.
func parseParentFilters(w http.ResponseWriter, q url.Values, projectKey string, p *store.ListItemsParams) bool {
	if v := q.Get("parent"); v != "" {
		n, err := parseItemRef(v, projectKey, "parent")
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "parent must be an item number or key of this project")
			return false
		}
		p.ParentNumber = &n
	}
	switch q.Get("top_level") {
	case "", "false":
	case "true":
		p.TopLevel = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "top_level must be true or false")
		return false
	}
	if p.TopLevel && p.ParentNumber != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "parent and top_level cannot be combined")
		return false
	}
	return true
}
