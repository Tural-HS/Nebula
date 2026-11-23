package engine

import "maps"

// FilterDigest narrows an overdue digest payload to the given project keys:
// items and totals of other projects are dropped. ok is false when nothing is
// left, in which case nothing is sent.
func FilterDigest(payload map[string]any, keys map[string]bool) (map[string]any, bool) {
	out := maps.Clone(payload)
	var items []any
	if list, ok := payload["items"].([]any); ok {
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				if k, _ := m["project"].(string); keys[k] {
					items = append(items, m)
				}
			}
		}
	}
	total := 0
	byProject := map[string]any{}
	if counts, ok := payload["by_project"].(map[string]any); ok {
		for k, v := range counts {
			if n, ok := v.(float64); ok && keys[k] {
				total += int(n)
				byProject[k] = v
			}
		}
	} else {
		total = len(items) // digests written before per-project counts existed
	}
	if total == 0 || len(items) == 0 {
		return nil, false
	}
	if items == nil {
		items = []any{}
	}
	out["items"], out["total"], out["by_project"], out["truncated"] = items, total, byProject, total > len(items)
	return out, true
}
