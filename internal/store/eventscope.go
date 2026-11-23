package store

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EventScope is the project an event belongs to, for webhook project filters
// (ADR-0013).
type EventScope struct {
	ProjectID  *uuid.UUID // nil: a workspace-level event (fields, workflows, members, …)
	ProjectKey string
	Private    bool
	// Unknown: the event belongs to a project that can no longer be resolved.
	// Such events are delivered to no webhook (fail closed).
	Unknown bool
}

// projectScoped are the entity types whose events belong to one project.
var projectScoped = map[string]bool{"item": true, "recurrence": true, "project": true, "view": true}

// EventScopeOf resolves an event's project. A project_id in the payload wins
// (deletion events carry it because the entity row is gone); otherwise the
// entity is looked up. Comments are recorded on their item.
func (s *Store) EventScopeOf(ctx context.Context, e Event) (EventScope, error) {
	return eventScope(ctx, s.pool, e)
}

func eventScope(ctx context.Context, q querier, e Event) (EventScope, error) {
	if !projectScoped[e.EntityType] {
		return EventScope{}, nil
	}
	var projectID uuid.UUID
	if raw, ok := e.Payload["project_id"].(string); ok {
		id, err := uuid.Parse(raw)
		if err != nil {
			return EventScope{Unknown: true}, nil
		}
		projectID = id
	} else {
		var lookup string
		switch e.EntityType {
		case "item":
			lookup = `SELECT project_id FROM items WHERE workspace_id = $1 AND id = $2`
		case "recurrence":
			lookup = `SELECT project_id FROM recurrences WHERE workspace_id = $1 AND id = $2`
		case "view":
			lookup = `SELECT project_id FROM saved_views WHERE workspace_id = $1 AND id = $2`
		case "project":
			projectID = e.EntityID
		}
		if lookup != "" {
			err := q.QueryRow(ctx, lookup, e.WorkspaceID, e.EntityID).Scan(&projectID)
			if errors.Is(err, pgx.ErrNoRows) {
				return EventScope{Unknown: true}, nil
			}
			if err != nil {
				return EventScope{}, err
			}
		}
	}
	sc := EventScope{ProjectID: &projectID}
	var visibility string
	err := q.QueryRow(ctx, `SELECT key, visibility FROM projects WHERE workspace_id = $1 AND id = $2`, e.WorkspaceID, projectID).
		Scan(&sc.ProjectKey, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return EventScope{Unknown: true}, nil
	}
	sc.Private = visibility == "private"
	return sc, err
}

// Allows reports whether a webhook with this project filter may receive an
// event of this scope. An empty filter means every workspace-visible project:
// a private project is delivered only to webhooks that list it.
func (sc EventScope) Allows(filter []uuid.UUID) bool {
	switch {
	case sc.Unknown:
		return false
	case sc.ProjectID == nil:
		return true
	case len(filter) == 0:
		return !sc.Private
	default:
		return slices.Contains(filter, *sc.ProjectID)
	}
}

// WebhookProjectKeys returns the keys of the projects a webhook filter covers,
// for filtering aggregate events (the overdue digest) at delivery time.
func (s *Store) WebhookProjectKeys(ctx context.Context, wsID uuid.UUID, filter []uuid.UUID) (map[string]bool, error) {
	var rows pgx.Rows
	var err error
	if len(filter) == 0 {
		rows, err = s.pool.Query(ctx, `SELECT key FROM projects WHERE workspace_id = $1 AND visibility = 'workspace'`, wsID)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT key FROM projects WHERE workspace_id = $1 AND id = ANY($2::uuid[])`, wsID, filter)
	}
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out, nil
}

// normalizeProjectFilter validates a webhook's project list: distinct
// projects of this workspace, at most 50.
func normalizeProjectFilter(ctx context.Context, q querier, wsID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) > 50 {
		return nil, invalid("project_ids", "at most 50 projects")
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	var n int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM projects WHERE workspace_id = $1 AND id = ANY($2::uuid[])`, wsID, out).Scan(&n); err != nil {
		return nil, err
	}
	if n != len(out) {
		return nil, invalid("project_ids", "unknown project")
	}
	return out, nil
}
