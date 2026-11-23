package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Tural-HS/Nebula/internal/opt"
)

const (
	maxStatusesPerWorkflow = 50
	maxStatusPosition      = 10_000
)

var statusCategories = map[string]bool{"todo": true, "in_progress": true, "done": true, "canceled": true}

// WorkflowStatus is a status as seen in workflow management.
type WorkflowStatus struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Position  int       `json:"position"`
	IsInitial bool      `json:"is_initial"`
}

// Transition is an allowed status change. A nil From means "from any status".
type Transition struct {
	ID   uuid.UUID  `json:"id"`
	Name string     `json:"name"`
	From *uuid.UUID `json:"from_status_id"`
	To   uuid.UUID  `json:"to_status_id"`
}

// Workflow is an ordered set of statuses shared by one or more item types.
// With EnforceTransitions, items may only move along defined transitions.
type Workflow struct {
	ID                 uuid.UUID        `json:"id"`
	Name               string           `json:"name"`
	EnforceTransitions bool             `json:"enforce_transitions"`
	Statuses           []WorkflowStatus `json:"statuses"`
	Transitions        []Transition     `json:"transitions"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

const maxTransitionsPerWorkflow = 500

// StatusInput is a status in a create request.
type StatusInput struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

func validateStatus(name, category string) (string, error) {
	name, err := text("name", name, 1, 100)
	if err != nil {
		return "", err
	}
	if !statusCategories[category] {
		return "", invalid("category", "must be one of todo, in_progress, done, canceled")
	}
	return name, nil
}

// ListWorkflows returns all workflows with ordered statuses.
func (s *Store) ListWorkflows(ctx context.Context, wsID uuid.UUID) ([]Workflow, error) {
	return listWorkflows(ctx, s.pool, wsID, nil)
}

func listWorkflows(ctx context.Context, q querier, wsID uuid.UUID, only *uuid.UUID) ([]Workflow, error) {
	rows, err := q.Query(ctx, `SELECT id, name, enforce_transitions, created_at, updated_at FROM workflows
		WHERE workspace_id = $1 AND ($2::uuid IS NULL OR id = $2) ORDER BY name, id`, wsID, only)
	if err != nil {
		return nil, err
	}
	wfs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Workflow, error) {
		w := Workflow{Statuses: []WorkflowStatus{}, Transitions: []Transition{}}
		return w, r.Scan(&w.ID, &w.Name, &w.EnforceTransitions, &w.CreatedAt, &w.UpdatedAt)
	})
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT workflow_id, id, name, category, position, is_initial FROM statuses
		WHERE workspace_id = $1 AND ($2::uuid IS NULL OR workflow_id = $2) ORDER BY position, name`, wsID, only)
	if err != nil {
		return nil, err
	}
	byWF := map[uuid.UUID][]WorkflowStatus{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var wf uuid.UUID
		var st WorkflowStatus
		err := r.Scan(&wf, &st.ID, &st.Name, &st.Category, &st.Position, &st.IsInitial)
		byWF[wf] = append(byWF[wf], st)
		return struct{}{}, err
	})
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT t.workflow_id, t.id, t.name, t.from_status_id, t.to_status_id
		FROM workflow_transitions t JOIN workflows w ON w.id = t.workflow_id
		WHERE w.workspace_id = $1 AND ($2::uuid IS NULL OR w.id = $2) ORDER BY t.created_at, t.id`, wsID, only)
	if err != nil {
		return nil, err
	}
	trByWF := map[uuid.UUID][]Transition{}
	_, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (struct{}, error) {
		var wf uuid.UUID
		var tr Transition
		err := r.Scan(&wf, &tr.ID, &tr.Name, &tr.From, &tr.To)
		trByWF[wf] = append(trByWF[wf], tr)
		return struct{}{}, err
	})
	if err != nil {
		return nil, err
	}
	for i := range wfs {
		if sts := byWF[wfs[i].ID]; sts != nil {
			wfs[i].Statuses = sts
		}
		if trs := trByWF[wfs[i].ID]; trs != nil {
			wfs[i].Transitions = trs
		}
	}
	return wfs, nil
}

func getWorkflow(ctx context.Context, q querier, wsID, id uuid.UUID) (Workflow, error) {
	wfs, err := listWorkflows(ctx, q, wsID, &id)
	if err != nil {
		return Workflow{}, err
	}
	if len(wfs) == 0 {
		return Workflow{}, ErrNotFound
	}
	return wfs[0], nil
}

// lockWorkflow serializes structural changes to one workflow.
func lockWorkflow(ctx context.Context, tx pgx.Tx, wsID, id uuid.UUID) error {
	var got uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM workflows WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wsID, id).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// CreateWorkflow creates a workflow; the first status becomes the initial one.
func (s *Store) CreateWorkflow(ctx context.Context, wsID, actorID uuid.UUID, name string, statuses []StatusInput) (Workflow, error) {
	name, err := text("name", name, 1, 200)
	if err != nil {
		return Workflow{}, err
	}
	if len(statuses) == 0 || len(statuses) > maxStatusesPerWorkflow {
		return Workflow{}, invalid("statuses", "must contain 1 to %d statuses", maxStatusesPerWorkflow)
	}
	seen := map[string]bool{}
	for i := range statuses {
		n, err := validateStatus(statuses[i].Name, statuses[i].Category)
		if err != nil {
			return Workflow{}, err
		}
		if seen[strings.ToLower(n)] {
			return Workflow{}, invalid("statuses", "duplicate status name %q", n)
		}
		seen[strings.ToLower(n)] = true
		statuses[i].Name = n
	}

	id := newID()
	var wf Workflow
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO workflows (id, workspace_id, name) VALUES ($1, $2, $3)`, id, wsID, name)
		if isUniqueViolation(err, "workflows_workspace_id_name_key") {
			return &ConflictError{Code: "workflow_name_taken", Message: "a workflow with this name already exists"}
		}
		if err != nil {
			return err
		}
		for i, st := range statuses {
			if _, err := tx.Exec(ctx, `INSERT INTO statuses (id, workspace_id, workflow_id, name, category, position, is_initial)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`, newID(), wsID, id, st.Name, st.Category, i, i == 0); err != nil {
				return err
			}
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "workflow", id, "workflow.created", map[string]any{"name": name}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, id)
		return err
	})
	return wf, err
}

// UpdateWorkflow renames a workflow and/or toggles transition enforcement.
func (s *Store) UpdateWorkflow(ctx context.Context, wsID, actorID, id uuid.UUID, name opt.Field[string], enforce opt.Field[bool]) (Workflow, error) {
	if name.Null || enforce.Null {
		return Workflow{}, invalid("body", "fields cannot be null")
	}
	var wf Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var oldName string
		var oldEnforce bool
		var transitions int
		err := tx.QueryRow(ctx, `SELECT name, enforce_transitions,
				(SELECT count(*) FROM workflow_transitions WHERE workflow_id = w.id)
			FROM workflows w WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, wsID, id).Scan(&oldName, &oldEnforce, &transitions)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		newName, newEnforce := oldName, oldEnforce
		if name.Set {
			if newName, err = text("name", name.Value, 1, 200); err != nil {
				return err
			}
		}
		if enforce.Set {
			newEnforce = enforce.Value
		}
		if newEnforce && !oldEnforce && transitions == 0 {
			return &ConflictError{Code: "no_transitions", Message: "define transitions before enforcing them (otherwise no item could change status)"}
		}
		changes := map[string]any{}
		if newName != oldName {
			changes["name"] = diff(oldName, newName)
		}
		if newEnforce != oldEnforce {
			changes["enforce_transitions"] = diff(oldEnforce, newEnforce)
		}
		if len(changes) > 0 {
			_, err = tx.Exec(ctx, `UPDATE workflows SET name = $1, enforce_transitions = $2 WHERE id = $3`, newName, newEnforce, id)
			if isUniqueViolation(err, "workflows_workspace_id_name_key") {
				return &ConflictError{Code: "workflow_name_taken", Message: "a workflow with this name already exists"}
			}
			if err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, wsID, actorID, "workflow", id, "workflow.updated",
				map[string]any{"changes": changes}); err != nil {
				return err
			}
		}
		wf, err = getWorkflow(ctx, tx, wsID, id)
		return err
	})
	return wf, err
}

// AddTransition allows moving from one status (nil: any) to another.
func (s *Store) AddTransition(ctx context.Context, wsID, actorID, workflowID uuid.UUID, name string, from *uuid.UUID, to uuid.UUID) (Workflow, error) {
	name, err := text("name", name, 1, 100)
	if err != nil {
		return Workflow{}, err
	}
	if from != nil && *from == to {
		return Workflow{}, invalid("to_status_id", "must differ from from_status_id")
	}
	var wf Workflow
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		for field, id := range map[string]*uuid.UUID{"from_status_id": from, "to_status_id": &to} {
			if id == nil {
				continue
			}
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM statuses WHERE id = $1 AND workflow_id = $2)`,
				*id, workflowID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return invalid(field, "status does not belong to this workflow")
			}
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM workflow_transitions WHERE workflow_id = $1`, workflowID).Scan(&n); err != nil {
			return err
		}
		if n >= maxTransitionsPerWorkflow {
			return invalid("name", "a workflow can have at most %d transitions", maxTransitionsPerWorkflow)
		}
		id := newID()
		_, err := tx.Exec(ctx, `INSERT INTO workflow_transitions (id, workflow_id, from_status_id, to_status_id, name)
			VALUES ($1, $2, $3, $4, $5)`, id, workflowID, from, to, name)
		if isUniqueViolation(err, "workflow_transitions_unique_idx") {
			return &ConflictError{Code: "transition_exists", Message: "this transition already exists"}
		}
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "workflow", workflowID, "transition.created",
			map[string]any{"transition_id": id, "from": from, "to": to}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	})
	return wf, err
}

// DeleteTransition removes a transition. The last one cannot be removed while
// enforcement is on, because that would freeze every item in place.
func (s *Store) DeleteTransition(ctx context.Context, wsID, actorID, workflowID, transitionID uuid.UUID) (Workflow, error) {
	var wf Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		var enforce bool
		var n int
		if err := tx.QueryRow(ctx, `SELECT enforce_transitions, (SELECT count(*) FROM workflow_transitions WHERE workflow_id = $1)
			FROM workflows WHERE id = $1`, workflowID).Scan(&enforce, &n); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM workflow_transitions WHERE id = $1 AND workflow_id = $2`, transitionID, workflowID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if enforce && n <= 1 {
			return &ConflictError{Code: "last_transition", Message: "disable enforcement before deleting the last transition"}
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "workflow", workflowID, "transition.deleted",
			map[string]any{"transition_id": transitionID}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	})
	return wf, err
}

// checkTransition enforces the workflow's transition rules for a status change.
func checkTransition(ctx context.Context, q querier, workflowID, from, to uuid.UUID) error {
	var enforce, allowed bool
	err := q.QueryRow(ctx, `SELECT w.enforce_transitions, EXISTS (
			SELECT 1 FROM workflow_transitions t
			WHERE t.workflow_id = w.id AND t.to_status_id = $3 AND (t.from_status_id = $2 OR t.from_status_id IS NULL))
		FROM workflows w WHERE w.id = $1`, workflowID, from, to).Scan(&enforce, &allowed)
	if err != nil {
		return err
	}
	if enforce && !allowed {
		return invalid("status_id", "this status change is not allowed by the workflow")
	}
	return nil
}

// AllowedStatuses returns the statuses an item can move to next.
func (s *Store) AllowedStatuses(ctx context.Context, wsID, projectID uuid.UUID, number int64) ([]Status, error) {
	rows, err := s.pool.Query(ctx, `SELECT st.id, st.name, st.category
		FROM items i
		JOIN item_types it ON it.id = i.item_type_id
		JOIN workflows w ON w.id = it.workflow_id
		JOIN statuses st ON st.workflow_id = w.id AND st.id <> i.status_id
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND i.number = $3 AND i.deleted_at IS NULL
		  AND (NOT w.enforce_transitions OR EXISTS (
			SELECT 1 FROM workflow_transitions t WHERE t.workflow_id = w.id AND t.to_status_id = st.id
			  AND (t.from_status_id = i.status_id OR t.from_status_id IS NULL)))
		ORDER BY st.position, st.name`, wsID, projectID, number)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Status])
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		// Distinguish "item missing" from "no way forward".
		if _, err := s.GetItem(ctx, wsID, projectID, number); err != nil {
			return nil, err
		}
	}
	return nonNilSlice(out), nil
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// AddStatus appends a status to a workflow.
func (s *Store) AddStatus(ctx context.Context, wsID, actorID, workflowID uuid.UUID, in StatusInput) (Workflow, error) {
	name, err := validateStatus(in.Name, in.Category)
	if err != nil {
		return Workflow{}, err
	}
	var wf Workflow
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		var count, nextPos int
		if err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(max(position) + 1, 0) FROM statuses WHERE workflow_id = $1`,
			workflowID).Scan(&count, &nextPos); err != nil {
			return err
		}
		if count >= maxStatusesPerWorkflow {
			return invalid("name", "a workflow can have at most %d statuses", maxStatusesPerWorkflow)
		}
		id := newID()
		_, err := tx.Exec(ctx, `INSERT INTO statuses (id, workspace_id, workflow_id, name, category, position)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, wsID, workflowID, name, in.Category, min(nextPos, maxStatusPosition))
		if isUniqueViolation(err, "statuses_workflow_id_name_key") {
			return &ConflictError{Code: "status_name_taken", Message: "a status with this name already exists in the workflow"}
		}
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "status", id, "status.created",
			map[string]any{"workflow_id": workflowID, "name": name}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	})
	return wf, err
}

// UpdateStatusParams is a partial status update.
type UpdateStatusParams struct {
	WorkspaceID, ActorID, WorkflowID, StatusID uuid.UUID
	Name                                       opt.Field[string]
	Category                                   opt.Field[string]
	Position                                   opt.Field[int]
	IsInitial                                  opt.Field[bool]
}

// UpdateStatus renames, recategorizes, reorders or makes a status initial.
func (s *Store) UpdateStatus(ctx context.Context, p UpdateStatusParams) (Workflow, error) {
	if p.Name.Null || p.Category.Null || p.Position.Null || p.IsInitial.Null {
		return Workflow{}, invalid("body", "fields cannot be null")
	}
	if p.IsInitial.Set && !p.IsInitial.Value {
		return Workflow{}, invalid("is_initial", "cannot be unset; mark another status as initial instead")
	}
	if p.Position.Set && (p.Position.Value < 0 || p.Position.Value > maxStatusPosition) {
		return Workflow{}, invalid("position", "must be between 0 and %d", maxStatusPosition)
	}
	var wf Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, p.WorkspaceID, p.WorkflowID); err != nil {
			return err
		}
		var cur WorkflowStatus
		err := tx.QueryRow(ctx, `SELECT id, name, category, position, is_initial FROM statuses
			WHERE id = $1 AND workflow_id = $2`, p.StatusID, p.WorkflowID).
			Scan(&cur.ID, &cur.Name, &cur.Category, &cur.Position, &cur.IsInitial)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		next := cur
		if p.Name.Set {
			if next.Name, err = text("name", p.Name.Value, 1, 100); err != nil {
				return err
			}
		}
		if p.Category.Set {
			if !statusCategories[p.Category.Value] {
				return invalid("category", "must be one of todo, in_progress, done, canceled")
			}
			next.Category = p.Category.Value
		}
		if p.Position.Set {
			next.Position = p.Position.Value
		}
		if p.IsInitial.Set && !cur.IsInitial {
			// The partial unique index allows one initial status: clear the old one first.
			if _, err := tx.Exec(ctx, `UPDATE statuses SET is_initial = false WHERE workflow_id = $1 AND is_initial`,
				p.WorkflowID); err != nil {
				return err
			}
			next.IsInitial = true
		}
		if next != cur {
			_, err = tx.Exec(ctx, `UPDATE statuses SET name = $1, category = $2, position = $3, is_initial = $4 WHERE id = $5`,
				next.Name, next.Category, next.Position, next.IsInitial, cur.ID)
			if isUniqueViolation(err, "statuses_workflow_id_name_key") {
				return &ConflictError{Code: "status_name_taken", Message: "a status with this name already exists in the workflow"}
			}
			if err != nil {
				return err
			}
			if err := appendEvent(ctx, tx, p.WorkspaceID, p.ActorID, "status", cur.ID, "status.updated",
				map[string]any{"changes": diff(cur, next)}); err != nil {
				return err
			}
		}
		wf, err = getWorkflow(ctx, tx, p.WorkspaceID, p.WorkflowID)
		return err
	})
	return wf, err
}

// DeleteStatus removes an unused, non-initial status.
func (s *Store) DeleteStatus(ctx context.Context, wsID, actorID, workflowID, statusID uuid.UUID) (Workflow, error) {
	var wf Workflow
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := lockWorkflow(ctx, tx, wsID, workflowID); err != nil {
			return err
		}
		var isInitial bool
		var total int
		err := tx.QueryRow(ctx, `SELECT s.is_initial, (SELECT count(*) FROM statuses WHERE workflow_id = $2)
			FROM statuses s WHERE s.id = $1 AND s.workflow_id = $2`, statusID, workflowID).Scan(&isInitial, &total)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if total <= 1 {
			return &ConflictError{Code: "last_status", Message: "a workflow must keep at least one status"}
		}
		if isInitial {
			return &ConflictError{Code: "status_is_initial", Message: "mark another status as initial before deleting this one"}
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM items WHERE status_id = $1)`, statusID).Scan(&used); err != nil {
			return err
		}
		if used {
			return &ConflictError{Code: "status_in_use", Message: "items still use this status; move them first"}
		}
		_, err = tx.Exec(ctx, `DELETE FROM statuses WHERE id = $1`, statusID)
		if isForeignKeyViolation(err) { // an item raced in after the check
			return &ConflictError{Code: "status_in_use", Message: "items still use this status; move them first"}
		}
		if err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, wsID, actorID, "status", statusID, "status.deleted",
			map[string]any{"workflow_id": workflowID}); err != nil {
			return err
		}
		wf, err = getWorkflow(ctx, tx, wsID, workflowID)
		return err
	})
	return wf, err
}

func isForeignKeyViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23503"
}
