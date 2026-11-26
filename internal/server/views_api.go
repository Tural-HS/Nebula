package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// viewRoutes: saved views and the table's totals row (ADR-0014). Every
// reader of the project may list and save personal views; the store decides
// who may share or change a view.
func (a *api) viewRoutes(r chi.Router) {
	r.Get("/items/sums", a.sumItems)
	r.Get("/views", a.listViews)
	r.Post("/views", a.createView)
	r.Get("/views/{viewID}", a.getView)
	r.Patch("/views/{viewID}", a.updateView)
	r.Delete("/views/{viewID}", a.deleteView)
}

func viewCaller(r *http.Request) store.ViewCaller {
	ws := workspaceFrom(r)
	return store.ViewCaller{WorkspaceID: ws.ID, ProjectID: projectFrom(r).ID, UserID: principal(r).UserID, Role: ws.Role}
}

func (a *api) listViews(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListViews(r.Context(), viewCaller(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": nonNil(list)})
}

func (a *api) getView(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "viewID")
	if !ok {
		return
	}
	v, err := a.store.GetView(r.Context(), viewCaller(r), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// decodeView reads a view body. config is decoded strictly as well, so a
// misspelled key is a 400 instead of being dropped silently.
func (a *api) decodeView(w http.ResponseWriter, r *http.Request) (store.ViewInput, bool) {
	var body struct {
		Name   opt.Field[string]          `json:"name"`
		Kind   opt.Field[string]          `json:"kind"`
		Shared opt.Field[bool]            `json:"shared"`
		Config opt.Field[json.RawMessage] `json:"config"`
	}
	if !a.decode(w, r, &body) {
		return store.ViewInput{}, false
	}
	in := store.ViewInput{Name: body.Name, Kind: body.Kind, Shared: body.Shared}
	if body.Config.Set {
		in.Config.Set, in.Config.Null = true, body.Config.Null
		if !body.Config.Null {
			dec := json.NewDecoder(bytes.NewReader(body.Config.Value))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in.Config.Value); err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid config: "+err.Error())
				return store.ViewInput{}, false
			}
		}
	}
	return in, true
}

func (a *api) createView(w http.ResponseWriter, r *http.Request) {
	in, ok := a.decodeView(w, r)
	if !ok {
		return
	}
	v, err := a.store.CreateView(r.Context(), viewCaller(r), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (a *api) updateView(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "viewID")
	if !ok {
		return
	}
	in, ok := a.decodeView(w, r)
	if !ok {
		return
	}
	v, err := a.store.UpdateView(r.Context(), viewCaller(r), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *api) deleteView(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "viewID")
	if !ok {
		return
	}
	if err := a.store.DeleteView(r.Context(), viewCaller(r), id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sumItems totals number fields over every item matching the list filters
// (?fields=a,b plus the filters of GET …/items), for the table's totals row.
func (a *api) sumItems(w http.ResponseWriter, r *http.Request) {
	p, ok := parseItemFilters(w, r)
	if !ok {
		return
	}
	var keys []string
	for _, raw := range r.URL.Query()["fields"] {
		for k := range strings.SplitSeq(raw, ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
	}
	if len(keys) > store.MaxSumFields {
		writeError(w, http.StatusBadRequest, "bad_request", "at most "+strconv.Itoa(store.MaxSumFields)+" fields")
		return
	}
	// Without fields: only the count (tables with no amount column).
	sums := map[string]store.FieldSum{}
	if len(keys) > 0 {
		var err error
		if sums, err = a.store.SumItemFields(r.Context(), p, keys); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	count, err := a.store.CountItems(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sums": sums, "count": count})
}

// Item list cursors are opaque to clients. Each encodes its sort, so a
// cursor from one order cannot be replayed against another:
//
//	v1:<number>                   number DESC (the original format)
//	d1:<YYYY-MM-DD or empty>:<n>  due_date ASC NULLS LAST, number ASC
//	f1:<sort>:<value or empty>:<n> field sorts (ADR-0030): [-]field.<key>
func encodeItemCursor(c store.ItemCursor) string {
	var s string
	if c.Sort != store.SortDue && c.Sort != store.SortNumber {
		v := ""
		if c.Value != nil {
			v = *c.Value
		}
		s = "f1:" + c.Sort + ":" + v + ":" + strconv.FormatInt(c.Number, 10)
	} else if c.Sort == store.SortDue {
		due := ""
		if c.Due != nil {
			due = *c.Due
		}
		s = "d1:" + due + ":" + strconv.FormatInt(c.Number, 10)
	} else {
		s = "v1:" + strconv.FormatInt(c.Number, 10)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

var errBadCursor = errors.New("invalid cursor")

func decodeItemCursor(s string) (store.ItemCursor, error) {
	if len(s) > 300 {
		return store.ItemCursor{}, errBadCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return store.ItemCursor{}, errBadCursor
	}
	parts := strings.Split(string(b), ":")
	num := func(v string) (int64, bool) {
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil && n >= 1
	}
	switch {
	case len(parts) == 2 && parts[0] == "v1":
		if n, ok := num(parts[1]); ok {
			return store.ItemCursor{Sort: store.SortNumber, Number: n}, nil
		}
	case len(parts) == 3 && parts[0] == "d1":
		n, ok := num(parts[2])
		if !ok {
			break
		}
		c := store.ItemCursor{Sort: store.SortDue, Number: n}
		if parts[1] != "" {
			d, ok := store.ParseCursorDue(parts[1])
			if !ok {
				break
			}
			c.Due = &d
		}
		return c, nil
	case len(parts) == 4 && parts[0] == "f1":
		n, ok := num(parts[3])
		if !ok || parts[1] == store.SortNumber || parts[1] == store.SortDue || !store.ValidSort(parts[1]) {
			break
		}
		c := store.ItemCursor{Sort: parts[1], Number: n}
		if parts[2] != "" {
			v := parts[2]
			c.Value = &v // checked against the field's type by the store
		}
		return c, nil
	}
	return store.ItemCursor{}, errBadCursor
}
