package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/opt"
	"github.com/Tural-HS/Nebula/internal/safehttp"
	"github.com/Tural-HS/Nebula/internal/store"
)

// automationRoutes manages webhooks and automation rules. Everything here is
// admin-only, reads included: webhook URLs and rules can reveal internal systems.
func (a *api) automationRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(requireRole(store.RoleAdmin))

		r.Get("/webhooks", a.listWebhooks)
		r.Post("/webhooks", a.createWebhook)
		r.Get("/webhooks/{webhookID}", a.getWebhook)
		r.Patch("/webhooks/{webhookID}", a.updateWebhook)
		r.Delete("/webhooks/{webhookID}", a.deleteWebhook)
		r.Post("/webhooks/{webhookID}/rotate-secret", a.rotateWebhookSecret)
		r.Get("/webhooks/{webhookID}/deliveries", a.listDeliveries)

		r.Get("/automations", a.listRules)
		r.Post("/automations", a.createRule)
		r.Get("/automations/{ruleID}", a.getRule)
		r.Patch("/automations/{ruleID}", a.updateRule)
		r.Delete("/automations/{ruleID}", a.deleteRule)
	})
}

func (a *api) checkWebhookURL(raw string) error {
	_, err := safehttp.ValidateURL(raw, a.webhookOpts)
	return err
}

func (a *api) listWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListWebhooks(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": nonNil(list)})
}

func (a *api) createWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL    string   `json:"url"`
		Format string   `json:"format"` // nebula (default) | slack; fixed after creation
		Events []string `json:"events"`
		// Projects to deliver; empty = every workspace-visible project.
		ProjectIDs []uuid.UUID `json:"project_ids"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	hook, secret, err := a.store.CreateWebhook(r.Context(), workspaceFrom(r).ID, principal(r).UserID, body.URL, body.Format, body.Events, body.ProjectIDs, a.checkWebhookURL)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// The secret is returned exactly once; it cannot be read back later.
	// Slack-format webhooks are not signed and have none.
	var shown *string
	if secret != "" {
		shown = &secret
	}
	writeJSON(w, http.StatusCreated, map[string]any{"webhook": hook, "secret": shown})
}

func (a *api) getWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	hook, err := a.store.GetWebhook(r.Context(), workspaceFrom(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (a *api) updateWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	var body struct {
		URL        opt.Field[string]      `json:"url"`
		Events     opt.Field[[]string]    `json:"events"`
		ProjectIDs opt.Field[[]uuid.UUID] `json:"project_ids"`
		Active     opt.Field[bool]        `json:"active"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	hook, err := a.store.UpdateWebhook(r.Context(), store.UpdateWebhookParams{
		WorkspaceID: workspaceFrom(r).ID, ActorID: principal(r).UserID, ID: id,
		URL: body.URL, Events: body.Events, ProjectIDs: body.ProjectIDs, Active: body.Active, Check: a.checkWebhookURL,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (a *api) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	if err := a.store.DeleteWebhook(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) rotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	secret, err := a.store.RotateWebhookSecret(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret})
}

func (a *api) listDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "webhookID")
	if !ok {
		return
	}
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	limit, ok := pageLimit(w, q, 200)
	if !ok {
		return
	}
	list, err := a.store.ListDeliveries(r.Context(), workspaceFrom(r).ID, id, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": nonNil(list)})
}

type ruleBody struct {
	Name       string            `json:"name"`
	Trigger    string            `json:"trigger"`
	Conditions []store.Condition `json:"conditions"`
	Actions    []store.Action    `json:"actions"`
	Enabled    *bool             `json:"enabled"`
}

func (a *api) listRules(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListRules(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"automations": nonNil(list)})
}

func (a *api) createRule(w http.ResponseWriter, r *http.Request) {
	var body ruleBody
	if !a.decode(w, r, &body) {
		return
	}
	enabled := body.Enabled == nil || *body.Enabled
	rule, err := a.store.CreateRule(r.Context(), workspaceFrom(r).ID, principal(r).UserID, store.RuleInput{
		Name: body.Name, Trigger: body.Trigger, Conditions: body.Conditions, Actions: body.Actions, Enabled: enabled,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (a *api) getRule(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	rule, err := a.store.GetRule(r.Context(), workspaceFrom(r).ID, id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (a *api) updateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	var body struct {
		Name       opt.Field[string]            `json:"name"`
		Trigger    opt.Field[string]            `json:"trigger"`
		Conditions opt.Field[[]store.Condition] `json:"conditions"`
		Actions    opt.Field[[]store.Action]    `json:"actions"`
		Enabled    opt.Field[bool]              `json:"enabled"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	rule, err := a.store.UpdateRule(r.Context(), store.UpdateRuleParams{
		WorkspaceID: workspaceFrom(r).ID, ActorID: principal(r).UserID, ID: id,
		Name: body.Name, Trigger: body.Trigger, Conditions: body.Conditions, Actions: body.Actions, Enabled: body.Enabled,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (a *api) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := a.uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	if err := a.store.DeleteRule(r.Context(), workspaceFrom(r).ID, principal(r).UserID, id); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
