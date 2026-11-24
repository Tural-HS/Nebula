package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/store"
)

// fieldRoutes registers custom field definitions under /workspaces/{workspace}.
// Reads are open to every member; changes need admin (ADR-0002, ADR-0003).
func (a *api) fieldRoutes(r chi.Router) {
	r.Get("/fields", a.listFields)
	r.With(requireRole(store.RoleAdmin)).Post("/fields", a.createField)
	// Registered before /fields/{field}: a field key cannot contain "-".
	r.With(requireRole(store.RoleAdmin)).Post("/fields/formula-preview", a.previewFormula)
	r.Get("/fields/{field}", a.getField)
	r.With(requireRole(store.RoleAdmin)).Patch("/fields/{field}", a.updateField)
}

// itemTypeFieldRoutes registers field bindings relative to an item type route
// (/workspaces/{workspace}/item-types/{type}); the router must define {type}.
func (a *api) itemTypeFieldRoutes(r chi.Router) {
	r.With(requireRole(store.RoleAdmin)).Put("/fields/{field}", a.bindField)
	r.With(requireRole(store.RoleAdmin)).Delete("/fields/{field}", a.unbindField)
}

type fieldOptionsBody struct {
	Choices *[]store.ChoiceInput `json:"choices"`
	Formula *store.FormulaInput  `json:"formula"`
}

func (a *api) listFields(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListFields(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": nonNil(list)})
}

func (a *api) createField(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key     string            `json:"key"`
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		Options *fieldOptionsBody `json:"options"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	var choices []store.ChoiceInput
	var formula *store.FormulaInput
	if body.Options != nil {
		if body.Options.Choices != nil {
			choices = *body.Options.Choices
		}
		formula = body.Options.Formula
	}
	ws := workspaceFrom(r)
	f, err := a.store.CreateField(r.Context(), store.CreateFieldParams{
		WorkspaceID: ws.ID, ActorID: principal(r).UserID,
		Key: body.Key, Name: body.Name, Type: body.Type, Choices: choices, Formula: formula,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/v1/workspaces/%s/fields/%s", ws.Slug, f.Key))
	writeJSON(w, http.StatusCreated, f)
}

func (a *api) getField(w http.ResponseWriter, r *http.Request) {
	f, err := a.store.GetField(r.Context(), workspaceFrom(r).ID, chi.URLParam(r, "field"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// updateField: key and type are immutable, so the strict decoder rejects them.
// options.choices, when sent, is the complete list (choices can be archived, not removed).
func (a *api) updateField(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     opt.Field[string] `json:"name"`
		Options  *fieldOptionsBody `json:"options"`
		Archived opt.Field[bool]   `json:"archived"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	p := store.UpdateFieldParams{
		WorkspaceID: workspaceFrom(r).ID, ActorID: principal(r).UserID, Key: chi.URLParam(r, "field"),
		Name: body.Name, Archived: body.Archived,
	}
	if body.Options != nil {
		p.Choices = body.Options.Choices
		p.Formula = body.Options.Formula
	}
	f, err := a.store.UpdateField(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (a *api) bindField(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Required bool `json:"required"`
		Position int  `json:"position"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	b, err := a.store.BindField(r.Context(), workspaceFrom(r).ID, principal(r).UserID,
		chi.URLParam(r, "type"), chi.URLParam(r, "field"), body.Required, body.Position)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (a *api) unbindField(w http.ResponseWriter, r *http.Request) {
	if err := a.store.UnbindField(r.Context(), workspaceFrom(r).ID, principal(r).UserID,
		chi.URLParam(r, "type"), chi.URLParam(r, "field")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// previewFormula compiles a draft formula and computes it for a sample item
// and/or sample values, without saving anything (ADR-0030). Compile errors
// are a normal 200 response with valid=false and the error position, so the
// field editor can validate while the admin types.
func (a *api) previewFormula(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key        string                     `json:"key"`
		Expression string                     `json:"expression"`
		Scale      *int                       `json:"scale"`
		Item       string                     `json:"item"`
		Values     map[string]json.RawMessage `json:"values"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	res, err := a.store.PreviewFormula(r.Context(), store.FormulaPreviewParams{
		WorkspaceID: workspaceFrom(r).ID, Viewer: viewer(r), Key: body.Key,
		Formula: store.FormulaInput{Expression: body.Expression, Scale: body.Scale},
		Item:    body.Item, Values: body.Values,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
