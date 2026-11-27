package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// QueueCounts are the item list's quick-filter counts (ADR-0035): one number
// per preset, each the size of the list that clicking it shows.
type QueueCounts struct {
	All      int64 `json:"all"`
	Mine     int64 `json:"mine"`
	Overdue  int64 `json:"overdue"`
	Approval int64 `json:"approval"` // any open approval request
	Blocked  int64 `json:"blocked"`
	Week     int64 `json:"week"` // due in the next 7 days
	// SLA queues (ADR-0045); always 0 in projects without a policy.
	SLABreached int64 `json:"sla_breached"`
	SLAAtRisk   int64 `json:"sla_at_risk"`
	// ADR-0065: status categories, and this month's due dates still to do.
	Todo  int64 `json:"todo"`
	Done  int64 `json:"done"`
	Month int64 `json:"month"`
}

// CountQueues counts the presets in one round trip (a pgx batch). The other
// filters of base (search, statuses, fields, parent) apply to every count;
// the preset dimensions (assignee, due, approval, blocked) are replaced.
// base.Viewer is required: "mine" is the viewer's, and blockers in projects
// the viewer cannot see never count (ADR-0013).
func (s *Store) CountQueues(ctx context.Context, base ListItemsParams) (QueueCounts, error) {
	if base.Viewer == nil {
		return QueueCounts{}, errors.New("queue counts need a viewer")
	}
	base.AssigneeID, base.Unassigned, base.Due, base.Approval, base.Blocked, base.SLA, base.Category = nil, false, "", "", false, "", ""
	base.Cursor = nil
	me := base.Viewer.UserID
	presets := []func(p *ListItemsParams){
		func(*ListItemsParams) {},
		func(p *ListItemsParams) { p.AssigneeID = &me },
		func(p *ListItemsParams) { p.Due = "overdue" },
		func(p *ListItemsParams) { p.Approval = "pending" },
		func(p *ListItemsParams) { p.Blocked = true },
		func(p *ListItemsParams) { p.Due = "week" },
		func(p *ListItemsParams) { p.SLA = SLABreached },
		func(p *ListItemsParams) { p.SLA = SLAAtRisk },
		func(p *ListItemsParams) { p.Category = "todo" },
		func(p *ListItemsParams) { p.Category = "done" },
		// "Due this month" counts what is still to do (ADR-0065): not done, not late-and-in-progress.
		func(p *ListItemsParams) { p.Due, p.Category = "month", "todo" },
	}
	batch := &pgx.Batch{}
	for _, set := range presets {
		p := base
		set(&p)
		where, args, err := s.itemFilter(ctx, p, true)
		if err != nil {
			return QueueCounts{}, err
		}
		batch.Queue("SELECT count(*) FROM items i WHERE "+strings.Join(where, " AND "), args...)
	}
	var out QueueCounts
	dst := []*int64{&out.All, &out.Mine, &out.Overdue, &out.Approval, &out.Blocked, &out.Week, &out.SLABreached, &out.SLAAtRisk, &out.Todo, &out.Done, &out.Month}
	res := s.pool.SendBatch(ctx, batch)
	for _, d := range dst {
		if err := res.QueryRow().Scan(d); err != nil {
			_ = res.Close()
			return QueueCounts{}, err
		}
	}
	return out, res.Close()
}
