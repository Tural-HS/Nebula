package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Automation triggers.
const (
	TriggerItemCreated = "item.created"
	TriggerItemUpdated = "item.updated"
	TriggerItemOverdue = "item.overdue" // emitted by the scheduler (ADR-0009)
)

// Action types.
const (
	ActionSetStatus   = "set_status"
	ActionSetAssignee = "set_assignee"
	ActionSetField    = "set_field"
	ActionSendEmail   = "send_email"
)

const (
	maxRulesPerWorkspace = 50
	maxConditions        = 20
	maxActions           = 10
	maxEmailRecipients   = 20
	maxConditionValue    = 4096
)

var (
	conditionFieldRe = regexp.MustCompile(`^(type|project|status|status_category|assignee|reporter|title|due_date|parent|changed|fields\.[a-z][a-z0-9_]{0,62})$`)
	conditionOps     = map[string]bool{"eq": true, "neq": true, "in": true, "not_in": true, "is_empty": true, "is_not_empty": true, "contains": true}
)

// Condition is one predicate; a rule matches when all conditions hold.
//
//	{"field": "type", "op": "eq", "value": "bug"}
//	{"field": "fields.severity", "op": "in", "value": ["c1", "c2"]}
//	{"field": "changed", "op": "includes", "value": "status_id"}   (item.updated only)
type Condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Action is one effect of a rule.
//
//	{"type": "set_status", "status_id": "..."}
//	{"type": "set_assignee", "user_id": "..." | null}
//	{"type": "set_field", "key": "severity", "value": "c1"}
//	{"type": "send_email", "to": ["assignee", "reporter", "ops@example.com"], "subject": "...", "body": "..."}
type Action struct {
	Type     string          `json:"type"`
	StatusID *uuid.UUID      `json:"status_id,omitempty"`
	UserID   *uuid.UUID      `json:"user_id,omitempty"`
	Key      string          `json:"key,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	To       []string        `json:"to,omitempty"`
	Subject  string          `json:"subject,omitempty"`
	Body     string          `json:"body,omitempty"`
}

// Rule is an automation rule.
type Rule struct {
	ID         uuid.UUID   `json:"id"`
	Name       string      `json:"name"`
	Trigger    string      `json:"trigger"`
	Conditions []Condition `json:"conditions"`
	Actions    []Action    `json:"actions"`
	Enabled    bool        `json:"enabled"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// RuleInput is a full rule definition from a client.
type RuleInput struct {
	Name       string
	Trigger    string
	Conditions []Condition
	Actions    []Action
	Enabled    bool
}

func (s *Store) validateRule(ctx context.Context, q querier, wsID uuid.UUID, in *RuleInput) error {
	name, err := text("name", in.Name, 1, 200)
	if err != nil {
		return err
	}
	in.Name = name
	if in.Trigger != TriggerItemCreated && in.Trigger != TriggerItemUpdated && in.Trigger != TriggerItemOverdue {
		return invalid("trigger", "must be item.created, item.updated or item.overdue")
	}
	if in.Conditions == nil {
		in.Conditions = []Condition{}
	}
	if len(in.Conditions) > maxConditions {
		return invalid("conditions", "at most %d conditions", maxConditions)
	}
	for i, c := range in.Conditions {
		path := fmt.Sprintf("conditions[%d]", i)
		if !conditionFieldRe.MatchString(c.Field) {
			return invalid(path+".field", "unknown field %q", c.Field)
		}
		if len(c.Value) > maxConditionValue {
			return invalid(path+".value", "too large")
		}
		if c.Field == "changed" {
			if in.Trigger != TriggerItemUpdated {
				return invalid(path+".field", "\"changed\" is only available for item.updated")
			}
			var f string
			if c.Op != "includes" || json.Unmarshal(c.Value, &f) != nil || f == "" {
				return invalid(path, "use {\"field\": \"changed\", \"op\": \"includes\", \"value\": \"<field name>\"}")
			}
			continue
		}
		if !conditionOps[c.Op] {
			return invalid(path+".op", "must be one of eq, neq, in, not_in, contains, is_empty, is_not_empty")
		}
		hasValue := len(c.Value) > 0 && string(c.Value) != "null"
		switch c.Op {
		case "is_empty", "is_not_empty":
			if hasValue {
				return invalid(path+".value", "must be omitted for %s", c.Op)
			}
		case "in", "not_in":
			var list []any
			if json.Unmarshal(c.Value, &list) != nil || len(list) == 0 || len(list) > 100 {
				return invalid(path+".value", "must be an array of 1 to 100 values")
			}
		default:
			if !hasValue {
				return invalid(path+".value", "is required")
			}
		}
		if c.Field == "status_category" && c.Op == "eq" {
			var cat string
			if json.Unmarshal(c.Value, &cat) != nil || !statusCategories[cat] {
				return invalid(path+".value", "must be one of todo, in_progress, done, canceled")
			}
		}
	}

	if len(in.Actions) == 0 || len(in.Actions) > maxActions {
		return invalid("actions", "must contain 1 to %d actions", maxActions)
	}
	seen := map[string]bool{}
	for i, a := range in.Actions {
		path := fmt.Sprintf("actions[%d]", i)
		dup := a.Type
		if a.Type == ActionSetField {
			dup += ":" + a.Key
		}
		if a.Type != ActionSendEmail && seen[dup] {
			return invalid(path, "duplicate %s action", a.Type)
		}
		seen[dup] = true
		switch a.Type {
		case ActionSetStatus:
			if a.StatusID == nil {
				return invalid(path+".status_id", "is required")
			}
			var ok bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE workspace_id = $1 AND id = $2)`, wsID, *a.StatusID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return invalid(path+".status_id", "unknown status")
			}
		case ActionSetAssignee:
			if a.UserID != nil {
				ok, err := isMember(ctx, q, wsID, *a.UserID)
				if err != nil {
					return err
				}
				if !ok {
					return invalid(path+".user_id", "user is not a member of this workspace")
				}
			}
		case ActionSetField:
			if !fieldKeyRe.MatchString(a.Key) || len(a.Value) == 0 || len(a.Value) > maxConditionValue {
				return invalid(path, "set_field needs a valid key and a value")
			}
			var exists bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM field_definitions WHERE workspace_id = $1 AND key = $2 AND archived_at IS NULL)`,
				wsID, a.Key).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return invalid(path+".key", "unknown field")
			}
		case ActionSendEmail:
			if len(a.To) == 0 || len(a.To) > maxEmailRecipients {
				return invalid(path+".to", "must contain 1 to %d recipients", maxEmailRecipients)
			}
			for _, to := range a.To {
				if to == "assignee" || to == "reporter" {
					continue
				}
				// Only workspace members: automation must not become a spam relay.
				var ok bool
				if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users u JOIN workspace_members m ON m.user_id = u.id
					WHERE m.workspace_id = $1 AND u.email = $2 AND u.is_active)`, wsID, to).Scan(&ok); err != nil {
					return err
				}
				if !ok {
					return invalid(path+".to", "%q must be \"assignee\", \"reporter\" or the email of a workspace member", to)
				}
			}
			if _, err := text(path+".subject", a.Subject, 1, 200); err != nil {
				return err
			}
			if strings.ContainsAny(a.Subject, "\r\n") {
				return invalid(path+".subject", "must be a single line")
			}
			if _, err := text(path+".body", a.Body, 1, 10_000); err != nil {
				return err
			}
		default:
			return invalid(path+".type", "must be one of set_status, set_assignee, set_field, send_email")
		}
	}
	return nil
}

const ruleSelect = `SELECT id, name, trigger, conditions, actions, is_enabled, created_at, updated_at FROM automation_rules`

func scanRule(row pgx.Row) (Rule, error) {
	var r Rule
	err := row.Scan(&r.ID, &r.Name, &r.Trigger, &r.Conditions, &r.Actions, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	return r, err
}

// CreateRule validates and stores an automation rule.
func (s *Store) CreateRule(ctx context.Context, wsID, actorID uuid.UUID, in RuleInput) (Rule, error) {
	var r Rule
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := s.validateRule(ctx, tx, wsID, &in); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR NO KEY UPDATE`, wsID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM automation_rules WHERE workspace_id = $1`, wsID).Scan(&n); err != nil {
			return err
		}
		if n >= maxRulesPerWorkspace {
			return &ConflictError{Code: "rule_limit_reached", Message: "a workspace can have at most 50 automation rules"}
		}
		var err error
		r, err = insertRule(ctx, tx, wsID, actorID, in)
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "automation_rule", r.ID, "automation_rule.created", map[string]any{"name": r.Name})
	})
	return r, err
}

func insertRule(ctx context.Context, q querier, wsID, actorID uuid.UUID, in RuleInput) (Rule, error) {
	var creator any
	if actorID != uuid.Nil {
		creator = actorID
	}
	r, err := scanRule(q.QueryRow(ctx, `INSERT INTO automation_rules (id, workspace_id, name, trigger, conditions, actions, is_enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, trigger, conditions, actions, is_enabled, created_at, updated_at`,
		newID(), wsID, in.Name, in.Trigger, in.Conditions, in.Actions, in.Enabled, creator))
	if isUniqueViolation(err, "automation_rules_workspace_id_name_key") {
		return Rule{}, &ConflictError{Code: "rule_name_taken", Message: "a rule with this name already exists"}
	}
	return r, err
}

// ListRules returns all rules of a workspace.
func (s *Store) ListRules(ctx context.Context, wsID uuid.UUID) ([]Rule, error) {
	rows, err := s.pool.Query(ctx, ruleSelect+` WHERE workspace_id = $1 ORDER BY name, id`, wsID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Rule, error) { return scanRule(r) })
}

// GetRule returns one rule of a workspace.
func (s *Store) GetRule(ctx context.Context, wsID, id uuid.UUID) (Rule, error) {
	return scanRule(s.pool.QueryRow(ctx, ruleSelect+` WHERE workspace_id = $1 AND id = $2`, wsID, id))
}

// UpdateRuleParams replaces any of the rule's parts.
type UpdateRuleParams struct {
	WorkspaceID, ActorID, ID uuid.UUID
	Name                     opt.Field[string]
	Trigger                  opt.Field[string]
	Conditions               opt.Field[[]Condition]
	Actions                  opt.Field[[]Action]
	Enabled                  opt.Field[bool]
}

// UpdateRule revalidates and saves a rule.
func (s *Store) UpdateRule(ctx context.Context, p UpdateRuleParams) (Rule, error) {
	if p.Name.Null || p.Trigger.Null || p.Actions.Null || p.Enabled.Null {
		return Rule{}, invalid("body", "name, trigger, actions and enabled cannot be null")
	}
	var r Rule
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanRule(tx.QueryRow(ctx, ruleSelect+` WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, p.WorkspaceID, p.ID))
		if err != nil {
			return err
		}
		in := RuleInput{Name: cur.Name, Trigger: cur.Trigger, Conditions: cur.Conditions, Actions: cur.Actions, Enabled: cur.Enabled}
		if p.Name.Set {
			in.Name = p.Name.Value
		}
		if p.Trigger.Set {
			in.Trigger = p.Trigger.Value
		}
		if p.Conditions.Set {
			in.Conditions = p.Conditions.Value
		}
		if p.Actions.Set {
			in.Actions = p.Actions.Value
		}
		if p.Enabled.Set {
			in.Enabled = p.Enabled.Value
		}
		if err := s.validateRule(ctx, tx, p.WorkspaceID, &in); err != nil {
			return err
		}
		r, err = scanRule(tx.QueryRow(ctx, `UPDATE automation_rules SET name = $1, trigger = $2, conditions = $3, actions = $4, is_enabled = $5
			WHERE id = $6 RETURNING id, name, trigger, conditions, actions, is_enabled, created_at, updated_at`,
			in.Name, in.Trigger, in.Conditions, in.Actions, in.Enabled, cur.ID))
		if isUniqueViolation(err, "automation_rules_workspace_id_name_key") {
			return &ConflictError{Code: "rule_name_taken", Message: "a rule with this name already exists"}
		}
		if err != nil {
			return err
		}
		return appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "automation_rule", r.ID, "automation_rule.updated",
			map[string]any{"name": r.Name, "enabled": r.Enabled})
	})
	return r, err
}

// DeleteRule removes a rule. Pending runs of it become no-ops.
func (s *Store) DeleteRule(ctx context.Context, wsID, actorID, id uuid.UUID) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM automation_rules WHERE workspace_id = $1 AND id = $2`, wsID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return appendEvent(ctx, tx, wsID, actorID, "automation_rule", id, "automation_rule.deleted", nil)
	})
}

// RuleByID loads a rule for execution (any workspace).
func (s *Store) RuleByID(ctx context.Context, id uuid.UUID) (Rule, uuid.UUID, error) {
	var wsID uuid.UUID
	var r Rule
	err := s.pool.QueryRow(ctx, `SELECT workspace_id, id, name, trigger, conditions, actions, is_enabled, created_at, updated_at
		FROM automation_rules WHERE id = $1`, id).
		Scan(&wsID, &r.ID, &r.Name, &r.Trigger, &r.Conditions, &r.Actions, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, uuid.Nil, ErrNotFound
	}
	return r, wsID, err
}

// defaultRules notify assignees by email; installed with every new workspace.
func defaultRules() []RuleInput {
	return assigneeRules("Notify assignee on create", "Notify assignee on reassignment",
		"[{{item.key}}] Assigned to you: {{item.title}}",
		"{{item.key}} \"{{item.title}}\" is now assigned to you.\n\nType: {{item.type}}\nStatus: {{item.status}}\n")
}

// defaultRulesAZ, defaultRulesTR and defaultRulesRU are defaultRules with names
// and email text in the workspace template's language (ADR-0020, ADR-0025).
func defaultRulesAZ() []RuleInput {
	return assigneeRules("Yaradılanda icraçıya bildir", "İcraçı dəyişəndə bildir",
		"[{{item.key}}] Sizə təyin edildi: {{item.title}}",
		"{{item.key}} \"{{item.title}}\" sizə təyin edildi.\n\nNövü: {{item.type}}\nStatus: {{item.status}}\n")
}

func defaultRulesTR() []RuleInput {
	return assigneeRules("Oluşturulunca atanan kişiye bildir", "Atanan kişi değişince bildir",
		"[{{item.key}}] Size atandı: {{item.title}}",
		"{{item.key}} \"{{item.title}}\" size atandı.\n\nTür: {{item.type}}\nDurum: {{item.status}}\n")
}

func defaultRulesRU() []RuleInput {
	return assigneeRules("Уведомить исполнителя при создании", "Уведомить исполнителя при переназначении",
		"[{{item.key}}] Вам назначено: {{item.title}}",
		"Вам назначено: {{item.key}} «{{item.title}}».\n\nТип: {{item.type}}\nСтатус: {{item.status}}\n")
}

// assigneeRules emails the assignee when an item is created with one and when
// the assignee changes.
func assigneeRules(onCreate, onReassign, subject, body string) []RuleInput {
	notify := Action{Type: ActionSendEmail, To: []string{"assignee"}, Subject: subject, Body: body}
	assigned := Condition{Field: "assignee", Op: "is_not_empty"}
	return []RuleInput{
		{Name: onCreate, Trigger: TriggerItemCreated, Enabled: true,
			Conditions: []Condition{assigned}, Actions: []Action{notify}},
		{Name: onReassign, Trigger: TriggerItemUpdated, Enabled: true,
			Conditions: []Condition{{Field: "changed", Op: "includes", Value: json.RawMessage(`"assignee_id"`)}, assigned},
			Actions:    []Action{notify}},
	}
}
