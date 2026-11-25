package engine

import "maps"

// FilterDigest narrows a daily digest payload (overdue items and, since
// ADR-0051, items due today) to the given project keys: items and totals of
// other projects are dropped. ok is false when nothing is left, in which
// case nothing is sent.
func FilterDigest(payload map[string]any, keys map[string]bool) (map[string]any, bool) {
	out := maps.Clone(payload)
	items, total := filterList(payload, "items", "by_project", keys)
	today, todayTotal := filterList(payload, "today", "today_by_project", keys)
	if (total == 0 || len(items) == 0) && (todayTotal == 0 || len(today) == 0) {
		return nil, false
	}
	out["items"], out["total"], out["by_project"], out["truncated"] = items, total, countsFor(payload, "by_project", keys), total > len(items)
	if _, has := payload["today"]; has {
		out["today"], out["today_total"], out["today_by_project"] = today, todayTotal, countsFor(payload, "today_by_project", keys)
	}
	return out, true
}

// filterList keeps the entries of payload[list] whose project is in keys and
// sums payload[counts] over keys.
func filterList(payload map[string]any, list, counts string, keys map[string]bool) ([]any, int) {
	items := []any{}
	if l, ok := payload[list].([]any); ok {
		for _, it := range l {
			if m, ok := it.(map[string]any); ok {
				if k, _ := m["project"].(string); keys[k] {
					items = append(items, m)
				}
			}
		}
	}
	c, ok := payload[counts].(map[string]any)
	if !ok {
		return items, len(items) // digests written before per-project counts existed
	}
	total := 0
	for k, v := range c {
		if n, ok := v.(float64); ok && keys[k] {
			total += int(n)
		}
	}
	return items, total
}

func countsFor(payload map[string]any, counts string, keys map[string]bool) map[string]any {
	out := map[string]any{}
	if c, ok := payload[counts].(map[string]any); ok {
		for k, v := range c {
			if keys[k] {
				out[k] = v
			}
		}
	}
	return out
}
