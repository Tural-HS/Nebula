package store

import "testing"

func TestLockedChange(t *testing.T) {
	for name, tc := range map[string]struct {
		changes map[string]any
		locked  bool
	}{
		"none":        {map[string]any{}, false},
		"assignee":    {map[string]any{"assignee_id": 1}, false},
		"due date":    {map[string]any{"due_date": 1, "parent": 1}, false},
		"title":       {map[string]any{"title": 1}, true},
		"description": {map[string]any{"description": 1}, true},
		"status":      {map[string]any{"status_id": 1}, true},
		"field":       {map[string]any{"assignee_id": 1, "fields.amount": 1}, true},
	} {
		if got := lockedChange(tc.changes); got != tc.locked {
			t.Errorf("%s: locked = %v, want %v", name, got, tc.locked)
		}
	}
}

func TestApprovalPayload(t *testing.T) {
	r := approvalRow{approvals: 1, minApprovals: 2}
	p := approvalPayload(r, map[string]any{"reason": "x"})
	for _, k := range []string{"request_id", "requested_by", "from_status_id", "to_status_id", "approvals", "min_approvals", "reason"} {
		if _, ok := p[k]; !ok {
			t.Errorf("payload lacks %s: %v", k, p)
		}
	}
}
