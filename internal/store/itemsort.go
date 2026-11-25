package store

import (
	"context"
	"math/big"
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

// Field sorts (ADR-0030): "field.<key>" ascending, "-field.<key>"
// descending, for number, date and formula (number or date result) fields.
// Empty values come last in both directions; the item number breaks ties
// in the sort's direction, so the order is total and keyset pagination is
// exact.
const (
	sortFieldPrefix     = "field."
	sortFieldDescPrefix = "-field."
)

// ItemCursor is the position after the last item of a page. It belongs to
// one sort: Due is the last item's due date (nil when it had none) and is
// only meaningful for SortDue; Value is the last item's sort value for a
// field sort (nil when it had none), as exact text.
type ItemCursor struct {
	Sort   string
	Number int64
	Due    *string // YYYY-MM-DD
	Value  *string
}

// ValidSort reports whether s is a known sort ("" means SortNumber). Field
// sorts are checked against the field definitions when used.
func ValidSort(s string) bool {
	if s == "" || s == SortNumber || s == SortDue {
		return true
	}
	_, _, ok := parseFieldSort(s)
	return ok
}

// parseFieldSort splits "field.<key>" / "-field.<key>".
func parseFieldSort(s string) (key string, desc, ok bool) {
	if k, found := strings.CutPrefix(s, sortFieldDescPrefix); found {
		return k, true, fieldKeyRe.MatchString(k)
	}
	if k, found := strings.CutPrefix(s, sortFieldPrefix); found {
		return k, false, fieldKeyRe.MatchString(k)
	}
	return "", false, false
}

// fieldSort is a resolved field sort.
type fieldSort struct {
	key     string
	desc    bool
	numeric bool // number (else date as text)
	money   bool // money: the amount, whatever the currency (ADR-0044)
}

// resolveFieldSort checks that a field sort names a sortable field.
func resolveFieldSort(ctx context.Context, q querier, wsID uuid.UUID, sort, path string) (*fieldSort, error) {
	key, desc, ok := parseFieldSort(sort)
	if !ok {
		return nil, nil
	}
	types, err := fieldTypesByKey(ctx, q, wsID, []string{key})
	if err != nil {
		return nil, err
	}
	switch types[key] {
	case FieldNumber, "formula:number":
		return &fieldSort{key: key, desc: desc, numeric: true}, nil
	case FieldMoney:
		return &fieldSort{key: key, desc: desc, numeric: true, money: true}, nil
	case FieldDate, "formula:date":
		return &fieldSort{key: key, desc: desc}, nil
	case "":
		return nil, invalid(path, "unknown field %q", key)
	}
	return nil, invalid(path, "field %q cannot be sorted: only number, money, date and formula fields can", key)
}

// expr is the sort value; values not in the field's stored form sort as empty.
func (f *fieldSort) expr(param string) string {
	if f.money {
		return moneyAmountExpr(param)
	}
	if f.numeric {
		return "(CASE WHEN jsonb_typeof(i.fields -> " + param + "::text) = 'number' THEN (i.fields ->> " + param + "::text)::numeric END)"
	}
	return "(CASE WHEN jsonb_typeof(i.fields -> " + param + "::text) = 'string' THEN (i.fields ->> " + param + "::text) END) COLLATE \"C\""
}

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

// SortError is the message for an unknown sort.
const SortError = "must be number, due, field.<key> or -field.<key>"

// sortAndSeek returns the ORDER BY clause and adds the keyset condition for
// p.Cursor. Values are bound as parameters. fs is the resolved field sort,
// if p.Sort is one.
func sortAndSeek(p ListItemsParams, fs *fieldSort, where []string, args []any) (string, []string, []any, error) {
	sort := normSort(p.Sort)
	if !ValidSort(sort) {
		return "", nil, nil, invalid("sort", SortError)
	}
	c := p.Cursor
	if c != nil && normSort(c.Sort) != sort {
		return "", nil, nil, invalid("cursor", "belongs to another sort")
	}
	param := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if fs != nil {
		e := fs.expr(param(fs.key))
		cmp, dir := ">", "ASC"
		if fs.desc {
			cmp, dir = "<", "DESC"
		}
		if c != nil {
			if c.Value == nil {
				where = append(where, "("+e+" IS NULL AND i.number "+cmp+" "+param(c.Number)+")")
			} else {
				cast := "::numeric"
				if !fs.numeric {
					cast = "::text COLLATE \"C\""
				}
				v, n := param(*c.Value)+cast, param(c.Number)
				where = append(where, "("+e+" "+cmp+" "+v+" OR ("+e+" = "+v+" AND i.number "+cmp+" "+n+") OR "+e+" IS NULL)")
			}
		}
		return e + " " + dir + " NULLS LAST, i.number " + dir, where, args, nil
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

// moneyAmountPattern is the stored amount text (CanonicalAmount); the cast
// to numeric runs only on values that match it, so bad data cannot fail a query.
const moneyAmountPattern = `^-?[0-9]{1,15}(\.[0-9]{1,4})?$`

// moneyAmountExpr is the amount of a money field as numeric, NULL when the
// value is not in the stored form.
func moneyAmountExpr(param string) string {
	a := "(i.fields -> " + param + "::text ->> 'amount')"
	return "(CASE WHEN jsonb_typeof(i.fields -> " + param + "::text -> 'amount') = 'string' AND " + a +
		" ~ '" + moneyAmountPattern + "' THEN " + a + "::numeric END)"
}

// MaxSumFields bounds the number of fields one sums request may total.
const MaxSumFields = 10

// FieldSum is the total of one number field over the filtered items. Count
// is how many of them have a numeric value for the field.
type FieldSum struct {
	Sum   string `json:"sum,omitempty"` // exact decimal, as text: never rounded through float64 (not for money)
	Count int64  `json:"count"`
	// Currencies are the totals of a money field, one per currency in code
	// order: amounts in different currencies are never added (ADR-0044).
	Currencies []CurrencySum `json:"currencies,omitempty"`
}

// CurrencySum is the total of a money field's values in one currency.
type CurrencySum struct {
	Currency string `json:"currency"`
	Sum      string `json:"sum"`
	Count    int64  `json:"count"`
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
		if t != FieldNumber && t != "formula:number" && t != FieldMoney {
			return nil, invalid("fields", "field %q is not a number or money field", k)
		}
	}
	var money []string
	keys = slices.DeleteFunc(keys, func(k string) bool {
		if types[k] == FieldMoney {
			money = append(money, k)
			return true
		}
		return false
	})

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
	out := make(map[string]FieldSum, len(keys)+len(money))
	sums := make([]FieldSum, len(keys))
	for i := range sums {
		vals = append(vals, &sums[i].Sum, &sums[i].Count)
	}
	if len(keys) > 0 {
		err = s.pool.QueryRow(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM items i WHERE "+strings.Join(where, " AND "), args...).Scan(vals...)
		if err != nil {
			return nil, err
		}
	}
	for i, k := range keys {
		out[k] = sums[i]
	}
	// Money: one grouped pass per field (at most MaxSumFields), per currency.
	base := len(args) - len(keys) // the filter's own parameters
	for _, k := range money {
		margs := append(slices.Clone(args[:base]), k)
		ph := "$" + strconv.Itoa(len(margs))
		amount := moneyAmountExpr(ph)
		rows, err := s.pool.Query(ctx, "SELECT i.fields -> "+ph+"::text ->> 'currency', sum("+amount+")::text, count(*)"+
			" FROM items i WHERE "+strings.Join(where, " AND ")+" AND "+amount+" IS NOT NULL"+
			" AND jsonb_typeof(i.fields -> "+ph+"::text -> 'currency') = 'string'"+
			" GROUP BY 1", margs...)
		if err != nil {
			return nil, err
		}
		fs := FieldSum{Currencies: []CurrencySum{}}
		for rows.Next() {
			var c CurrencySum
			if err := rows.Scan(&c.Currency, &c.Sum, &c.Count); err != nil {
				rows.Close()
				return nil, err
			}
			fs.Count += c.Count
			fs.Currencies = append(fs.Currencies, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		slices.SortFunc(fs.Currencies, func(a, b CurrencySum) int { return strings.Compare(a.Currency, b.Currency) })
		out[k] = fs
	}
	return out, nil
}

// fieldTypesByKey returns key -> type for the workspace's field definitions
// among keys (archived ones included: old items still carry their values).
func fieldTypesByKey(ctx context.Context, q querier, wsID uuid.UUID, keys []string) (map[string]string, error) {
	// Formula fields report "formula:<result type>" (ADR-0030).
	rows, err := q.Query(ctx, `SELECT key, CASE WHEN field_type = 'formula'
			THEN 'formula:' || coalesce(options -> 'formula' ->> 'result_type', '') ELSE field_type END
		FROM field_definitions WHERE workspace_id = $1 AND key = ANY($2)`, wsID, keys)
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

// checkCursorValue validates a field sort cursor's value for its field.
func checkCursorValue(fs *fieldSort, v string) error {
	if fs.numeric {
		if _, ok := new(big.Rat).SetString(v); !ok || strings.ContainsAny(v, "eE/") || len(v) > 64 {
			return invalid("cursor", "invalid cursor")
		}
		return nil
	}
	if len(v) > 64 {
		return invalid("cursor", "invalid cursor")
	}
	return nil
}
