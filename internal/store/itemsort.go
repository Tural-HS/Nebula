package store

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Item list orders (ADR-0014). Each has a total order (number breaks ties),
// so keyset pagination neither skips nor repeats rows, including rows
// inserted while a client pages: new items only ever get higher numbers.
const (
	SortNumber = "number" // number DESC: newest first (default)
	SortDue    = "due"    // due_date ASC NULLS LAST, number ASC: soonest first, undated last
)

// ItemCursor is the position after the last item of a page. It belongs to
// one sort: Due is the last item's due date (nil when it had none) and is
// only meaningful for SortDue.
type ItemCursor struct {
	Sort   string
	Number int64
	Due    *string // YYYY-MM-DD
}

// ValidSort reports whether s is a known sort ("" means SortNumber).
func ValidSort(s string) bool { return s == "" || s == SortNumber || s == SortDue }

func normSort(s string) string {
	if s == "" {
		return SortNumber
	}
	return s
}

func cursorAfter(sort string, last Item) *ItemCursor {
	c := &ItemCursor{Sort: normSort(sort), Number: last.Number}
	if c.Sort == SortDue {
		c.Due = last.DueDate
	}
	return c
}

// sortAndSeek returns the ORDER BY clause and adds the keyset condition for
// p.Cursor. Values are bound as parameters.
func sortAndSeek(p ListItemsParams, where []string, args []any) (string, []string, []any, error) {
	sort := normSort(p.Sort)
	if !ValidSort(sort) {
		return "", nil, nil, invalid("sort", "must be number or due")
	}
	c := p.Cursor
	if c != nil && normSort(c.Sort) != sort {
		return "", nil, nil, invalid("cursor", "belongs to another sort")
	}
	param := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	switch sort {
	case SortDue:
		if c != nil {
			if c.Due == nil {
				where = append(where, "(i.due_date IS NULL AND i.number > "+param(c.Number)+")")
			} else {
				// Row comparison matches the index order (due_date, number);
				// undated items sort after every dated one.
				where = append(where, "((i.due_date, i.number) > ("+param(*c.Due)+"::date, "+param(c.Number)+") OR i.due_date IS NULL)")
			}
		}
		return "i.due_date ASC NULLS LAST, i.number ASC", where, args, nil
	default:
		if c != nil {
			where = append(where, "i.number < "+param(c.Number))
		}
		return "i.number DESC", where, args, nil
	}
}

// ParseCursorDue validates a cursor's due date part.
func ParseCursorDue(s string) (string, bool) {
	d, err := time.Parse(time.DateOnly, s)
	return d.Format(time.DateOnly), err == nil && d.Format(time.DateOnly) == s
}

// MaxSumFields bounds the number of fields one sums request may total.
const MaxSumFields = 10

// FieldSum is the total of one number field over the filtered items. Count
// is how many of them have a numeric value for the field.
type FieldSum struct {
	Sum   string `json:"sum"` // exact decimal, as text: never rounded through float64
	Count int64  `json:"count"`
}

// SumItemFields totals number fields over every item matching the same
// filters as ListItems (status included), not just one page. Only fields of
// type number may be summed; values that are not JSON numbers (data written
// before a type change, imports, manual SQL) are ignored instead of failing.
func (s *Store) SumItemFields(ctx context.Context, p ListItemsParams, keys []string) (map[string]FieldSum, error) {
	if len(keys) == 0 {
		return nil, invalid("fields", "is required")
	}
	if len(keys) > MaxSumFields {
		return nil, invalid("fields", "at most %d fields", MaxSumFields)
	}
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	types, err := fieldTypesByKey(ctx, s.pool, p.WorkspaceID, keys)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		t, ok := types[k]
		if !ok {
			return nil, invalid("fields", "unknown field %q", k)
		}
		if t != FieldNumber {
			return nil, invalid("fields", "field %q is not a number field", k)
		}
	}

	where, args, err := s.itemFilter(ctx, p, true)
	if err != nil {
		return nil, err
	}
	// One pass over the filtered items with two aggregates per key. Keys are
	// bound as parameters; a value is cast to numeric only when jsonb_typeof
	// says it is a number (CASE guards the cast), so the query cannot fail on
	// bad data.
	cols := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		args = append(args, k)
		ph := "$" + strconv.Itoa(len(args))
		isNum := "jsonb_typeof(i.fields -> " + ph + "::text) = 'number'"
		cols = append(cols,
			"COALESCE(sum(CASE WHEN "+isNum+" THEN (i.fields ->> "+ph+"::text)::numeric END), 0)::text",
			"count(*) FILTER (WHERE "+isNum+")")
	}
	vals := make([]any, 0, 2*len(keys))
	out := make(map[string]FieldSum, len(keys))
	sums := make([]FieldSum, len(keys))
	for i := range sums {
		vals = append(vals, &sums[i].Sum, &sums[i].Count)
	}
	err = s.pool.QueryRow(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM items i WHERE "+strings.Join(where, " AND "), args...).Scan(vals...)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		out[k] = sums[i]
	}
	return out, nil
}

// fieldTypesByKey returns key -> type for the workspace's field definitions
// among keys (archived ones included: old items still carry their values).
func fieldTypesByKey(ctx context.Context, q querier, wsID uuid.UUID, keys []string) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT key, field_type FROM field_definitions WHERE workspace_id = $1 AND key = ANY($2)`, wsID, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, t string
		if err := rows.Scan(&k, &t); err != nil {
			return nil, err
		}
		out[k] = t
	}
	return out, rows.Err()
}
