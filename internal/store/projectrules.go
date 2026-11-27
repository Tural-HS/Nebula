package store

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	maxCompleteFields = 5
	maxLateFields     = 10
	maxSummaryItems   = 6
)

// SummaryItem is one total of the summary bar above a project's table
// (ADR-0064): the sum of Field over the listed items, minus the sum of
// Minus when set, shown as "Label: value Unit".
type SummaryItem struct {
	Label string `json:"label"`
	Field string `json:"field"`
	Minus string `json:"minus,omitempty"`
	Unit  string `json:"unit,omitempty"`
}

// ProjectRules are a project's item rules (ADR-0061).
//
// CompleteFields: boolean fields that must all be true before people move an
// item to a done status, and that keep a closed item closed while all are
// true. LateFields: fields whose table cells take the overdue band colour.
type ProjectRules struct {
	CompleteFields []string      `json:"complete_fields"`
	LateFields     []string      `json:"late_fields"`
	Summary        []SummaryItem `json:"summary"`
}

// GetProjectRules returns the project's rules (empty lists when none).
func (s *Store) GetProjectRules(ctx context.Context, wsID, projectID uuid.UUID) (ProjectRules, error) {
	out := ProjectRules{CompleteFields: []string{}, LateFields: []string{}, Summary: []SummaryItem{}}
	err := s.pool.QueryRow(ctx, `SELECT complete_fields, late_fields, summary FROM project_rules WHERE workspace_id = $1 AND project_id = $2`,
		wsID, projectID).Scan(&out.CompleteFields, &out.LateFields, &out.Summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// SaveProjectRules replaces the rules. Complete fields must be active boolean
// fields; late fields any active fields; both unique.
func (s *Store) SaveProjectRules(ctx context.Context, wsID, projectID, actorID uuid.UUID, in ProjectRules) (ProjectRules, error) {
	if in.CompleteFields == nil {
		in.CompleteFields = []string{}
	}
	if in.LateFields == nil {
		in.LateFields = []string{}
	}
	if in.Summary == nil {
		in.Summary = []SummaryItem{}
	}
	if err := s.checkSummary(ctx, wsID, in.Summary); err != nil {
		return ProjectRules{}, err
	}
	if err := s.checkRuleFields(ctx, wsID, "complete_fields", in.CompleteFields, maxCompleteFields, FieldBoolean); err != nil {
		return ProjectRules{}, err
	}
	if err := s.checkRuleFields(ctx, wsID, "late_fields", in.LateFields, maxLateFields, ""); err != nil {
		return ProjectRules{}, err
	}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO project_rules (project_id, workspace_id, complete_fields, late_fields, summary, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (project_id) DO UPDATE SET complete_fields = EXCLUDED.complete_fields,
				late_fields = EXCLUDED.late_fields, summary = EXCLUDED.summary, updated_by = EXCLUDED.updated_by`,
			projectID, wsID, in.CompleteFields, in.LateFields, in.Summary, nullUUID(actorID)); err != nil {
			return err
		}
		return appendEvent(ctx, tx, wsID, actorID, "project", projectID, "project_rules.updated",
			map[string]any{"complete_fields": in.CompleteFields, "late_fields": in.LateFields, "summary": in.Summary})
	})
	if err != nil {
		return ProjectRules{}, err
	}
	return in, nil
}

func (s *Store) checkRuleFields(ctx context.Context, wsID uuid.UUID, path string, keys []string, limit int, typ string) error {
	if len(keys) > limit {
		return invalid(path, "at most %d fields", limit)
	}
	seen := map[string]bool{}
	for i, k := range keys {
		if seen[k] {
			return invalid(path, "%q is listed twice", k)
		}
		seen[k] = true
		var t string
		var archived bool
		err := s.pool.QueryRow(ctx, `SELECT field_type, archived_at IS NOT NULL FROM field_definitions WHERE workspace_id = $1 AND key = $2`,
			wsID, k).Scan(&t, &archived)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (archived || (typ != "" && t != typ))) {
			if typ != "" {
				return invalid(path, "%s[%d] must be an active %s field", path, i, typ)
			}
			return invalid(path, "%s[%d] must be an active field", path, i)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// checkSummary validates the summary bar: labels of 1-60 characters, number,
// money or number-formula fields, units of up to 8 characters.
func (s *Store) checkSummary(ctx context.Context, wsID uuid.UUID, items []SummaryItem) error {
	if len(items) > maxSummaryItems {
		return invalid("summary", "at most %d entries", maxSummaryItems)
	}
	for i := range items {
		it := &items[i]
		path := "summary[" + strconv.Itoa(i) + "]"
		label, err := text(path+".label", it.Label, 1, 60)
		if err != nil {
			return err
		}
		it.Label = label
		if it.Unit, err = text(path+".unit", it.Unit, 0, 8); err != nil {
			return err
		}
		keys := []string{it.Field}
		if it.Minus != "" {
			keys = append(keys, it.Minus)
		}
		types, err := fieldTypesByKey(ctx, s.pool, wsID, keys)
		if err != nil {
			return err
		}
		for _, k := range keys {
			switch types[k] {
			case FieldNumber, FieldMoney, "formula:number":
			default:
				return invalid(path, "%q must be a number, money or number formula field", k)
			}
		}
	}
	return nil
}

// checkCompleteFields applies a project's complete fields to a status move
// by a person (ADR-0061), on the item's new values: a move into a done
// status needs every field true; a move from done/canceled to an open
// status is refused while every field is true (clear one first, possibly
// in the same change). A zero actor is the system: always allowed.
func checkCompleteFields(ctx context.Context, q querier, wsID, projectID, from, to, actor uuid.UUID, fields map[string]any) error {
	if actor == uuid.Nil || from == to {
		return nil
	}
	var keys []string
	var fromCat, toCat string
	err := q.QueryRow(ctx, `SELECT r.complete_fields,
			(SELECT category FROM statuses WHERE id = $3), (SELECT category FROM statuses WHERE id = $4)
		FROM project_rules r WHERE r.workspace_id = $1 AND r.project_id = $2`, wsID, projectID, from, to).Scan(&keys, &fromCat, &toCat)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(keys) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	all := true
	for _, k := range keys {
		if v, _ := fields[k].(bool); !v {
			all = false
			break
		}
	}
	closed := func(c string) bool { return c == "done" || c == "canceled" }
	switch {
	case toCat == "done" && fromCat != "done" && !all:
		return &ConflictError{Code: "complete_requires", Message: "tick every one of these fields before completing: " + strings.Join(keys, ", ")}
	case closed(fromCat) && !closed(toCat) && all:
		return &ConflictError{Code: "close_lock", Message: "these fields are all set, clear one before reopening: " + strings.Join(keys, ", ")}
	}
	return nil
}
