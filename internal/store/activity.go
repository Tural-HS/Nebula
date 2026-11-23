package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxActivityPage = 100

// Activity is one audit event as shown in feeds.
type Activity struct {
	ID         int64          `json:"id"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   uuid.UUID      `json:"entity_id"`
	Actor      *CommentAuthor `json:"actor"` // nil for automation/system changes
	Source     string         `json:"source"`
	RuleID     *uuid.UUID     `json:"automation_rule_id"`
	Data       map[string]any `json:"data"`
	OccurredAt time.Time      `json:"occurred_at"`
}

const activitySelect = `SELECT e.id, e.action, e.entity_type, e.entity_id, e.actor_id, u.display_name, e.source,
	e.automation_rule_id, e.payload, e.occurred_at
	FROM events e LEFT JOIN users u ON u.id = e.actor_id`

func scanActivities(rows pgx.Rows, limit int) ([]Activity, int64, error) {
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		var actorID *uuid.UUID
		var actorName *string
		if err := rows.Scan(&a.ID, &a.Action, &a.EntityType, &a.EntityID, &actorID, &actorName, &a.Source,
			&a.RuleID, &a.Data, &a.OccurredAt); err != nil {
			return nil, 0, err
		}
		if actorID != nil && actorName != nil {
			a.Actor = &CommentAuthor{ID: *actorID, DisplayName: *actorName}
		}
		// The rule-evaluation snapshot is internal and large; feeds don't need it.
		delete(a.Data, "snapshot")
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// ItemActivity returns an item's history (changes and comments), newest first.
func (s *Store) ItemActivity(ctx context.Context, wsID, projectID uuid.UUID, number int64, before int64, limit int) ([]Activity, int64, error) {
	item, err := liveItem(ctx, s.pool, wsID, projectID, number)
	if err != nil {
		return nil, 0, err
	}
	limit = min(max(limit, 1), maxActivityPage)
	rows, err := s.pool.Query(ctx, activitySelect+`
		WHERE e.workspace_id = $1 AND e.entity_type = 'item' AND e.entity_id = $2 AND ($3 = 0 OR e.id < $3)
		ORDER BY e.id DESC LIMIT $4`, wsID, item.id, before, limit+1)
	if err != nil {
		return nil, 0, err
	}
	return scanActivities(rows, limit)
}

// AuditLogParams filter the workspace audit log.
type AuditLogParams struct {
	WorkspaceID uuid.UUID
	Before      int64
	Limit       int
	// ActionPrefix filters by action, e.g. "webhook." or "item.updated".
	ActionPrefix string
	ActorID      *uuid.UUID
}

// AuditLog returns all workspace events newest first (admins only at the API).
func (s *Store) AuditLog(ctx context.Context, p AuditLogParams) ([]Activity, int64, error) {
	limit := min(max(p.Limit, 1), maxActivityPage)
	where := []string{"e.workspace_id = $1"}
	args := []any{p.WorkspaceID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if p.Before > 0 {
		add("e.id < ?", p.Before)
	}
	if p.ActionPrefix != "" {
		if len(p.ActionPrefix) > 100 || strings.ContainsAny(p.ActionPrefix, `%_\`) {
			return nil, 0, invalid("action", "invalid action filter")
		}
		add("e.action LIKE ?", p.ActionPrefix+"%")
	}
	if p.ActorID != nil {
		add("e.actor_id = ?", *p.ActorID)
	}
	args = append(args, limit+1)
	rows, err := s.pool.Query(ctx, activitySelect+" WHERE "+strings.Join(where, " AND ")+
		fmt.Sprintf(" ORDER BY e.id DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	return scanActivities(rows, limit)
}
