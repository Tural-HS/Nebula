package formula

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Error codes of values that could not be computed. They are values, not
// failures: a formula with a division by zero shows an error marker on that
// item and every other item is unaffected.
const (
	ErrDivisionByZero  = "division_by_zero"
	ErrOverflow        = "overflow"          // |value| beyond the limits below
	ErrDateRange       = "date_out_of_range" // outside years 1..9999
	ErrInvalidArgument = "invalid_argument"  // e.g. a date shifted by 1.5 days
)

// Number limits. Intermediate results keep intermediateScale decimals
// (rounded half away from zero), so repeated division cannot grow a
// fraction without bound; a stored result must fit the number field limit
// of ADR-0003.
const (
	intermediateScale = 20
	maxRoundDigits    = 12
	// MaxScale is the largest number of decimals a formula field may store.
	MaxScale = 10
)

var (
	maxIntermediate = new(big.Rat).SetFrac(pow10(30), big.NewInt(1))
	// MaxResult matches the |v| <= 1e15 limit of number fields (ADR-0003).
	maxResult = new(big.Rat).SetFrac(pow10(15), big.NewInt(1))
)

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// Value is the result of evaluating an expression: a number, a date or a
// boolean, empty (Null), or an error marker (Err).
type Value struct {
	Type Type
	Null bool
	Err  string
	num  *big.Rat
	date time.Time // UTC midnight
	b    bool
}

// Null returns the empty value of a type.
func Null(t Type) Value { return Value{Type: t, Null: true} }

// Errorf returns an error value.
func errValue(t Type, code string) Value { return Value{Type: t, Err: code} }

// Number returns a number value. r is not copied.
func Number(r *big.Rat) Value { return Value{Type: TypeNumber, num: r} }

// Date returns a date value for the calendar date of t.
func Date(t time.Time) Value {
	return Value{Type: TypeDate, date: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}
}

// Bool returns a boolean value.
func Bool(b bool) Value { return Value{Type: TypeBool, b: b} }

// Rat returns the number (nil unless a non-empty number).
func (v Value) Rat() *big.Rat {
	if v.Null || v.Err != "" || v.Type != TypeNumber {
		return nil
	}
	return v.num
}

// Time returns the date (zero unless a non-empty date).
func (v Value) Time() time.Time { return v.date }

// BoolValue returns the boolean.
func (v Value) BoolValue() bool { return v.b }

func (v Value) ok() bool { return !v.Null && v.Err == "" }

// parseNumber reads a plain decimal ("12", "-0.18"); no exponent, no
// thousands separators.
func parseNumber(s string) (*big.Rat, error) {
	t := strings.TrimPrefix(s, "-")
	if t == "" || strings.ContainsAny(t, "eE+_ ") || strings.Count(t, ".") > 1 || strings.HasPrefix(t, ".") || strings.HasSuffix(t, ".") {
		return nil, errors.New("invalid number")
	}
	for _, c := range t {
		if (c < '0' || c > '9') && c != '.' {
			return nil, errors.New("invalid number")
		}
	}
	if len(t) > maxNumDigits+1 {
		return nil, errors.New("invalid number")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("invalid number")
	}
	return r, nil
}

// FromJSON converts a value stored in items.fields to a Value of type t.
// Number fields hold JSON numbers decoded as float64 (|v| <= 1e15): the
// shortest representation that round-trips is the decimal the user typed,
// so 0.1 is exactly 1/10, not the binary float. Anything that is not the
// expected stored form is treated as empty; a stored formula error marker
// ({"error": code}) stays an error, so errors propagate like in Excel.
func FromJSON(t Type, v any) Value {
	if m, ok := v.(map[string]any); ok {
		if code, ok := m["error"].(string); ok && code != "" {
			return errValue(t, code)
		}
		return Null(t)
	}
	switch t {
	case TypeNumber:
		var s string
		switch n := v.(type) {
		case float64:
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return Null(t)
			}
			s = strconv.FormatFloat(n, 'f', -1, 64)
		case json.Number:
			s = n.String()
		case int:
			s = strconv.Itoa(n)
		case int64:
			s = strconv.FormatInt(n, 10)
		default:
			return Null(t)
		}
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			return Null(t)
		}
		return Number(r)
	case TypeDate:
		s, ok := v.(string)
		if !ok {
			return Null(t)
		}
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return Null(t)
		}
		return Date(d)
	case TypeBool:
		b, ok := v.(bool)
		if !ok {
			return Null(t)
		}
		return Bool(b)
	}
	return Null(t)
}

// JSON is the stored form of a result: a JSON number rounded to scale
// decimals (half away from zero, trailing zeros removed), "YYYY-MM-DD",
// true/false, nil for empty, or {"error": code}. A number beyond 1e15 after
// rounding is stored as an overflow error.
func (v Value) JSON(scale int) any {
	if v.Err != "" {
		return map[string]any{"error": v.Err}
	}
	if v.Null {
		return nil
	}
	switch v.Type {
	case TypeNumber:
		r := RoundHalfAway(v.num, scale)
		if new(big.Rat).Abs(r).Cmp(maxResult) > 0 {
			return map[string]any{"error": ErrOverflow}
		}
		return json.Number(DecimalString(r, scale))
	case TypeDate:
		return v.date.Format(time.DateOnly)
	case TypeBool:
		return v.b
	}
	return nil
}

// RoundHalfAway rounds r to n decimals (n < 0 rounds to tens, hundreds...),
// halves away from zero: 2.5 -> 3, -2.5 -> -3, like Excel's ROUND and the
// usual commercial rounding (often called "half-up" in finance).
func RoundHalfAway(r *big.Rat, n int) *big.Rat {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	if n >= 0 {
		num.Mul(num, pow10(n))
	} else {
		den.Mul(den, pow10(-n))
	}
	neg := num.Sign() < 0
	num.Abs(num)
	q, m := new(big.Int).QuoRem(num, den, new(big.Int))
	if m.Lsh(m, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	out := new(big.Rat)
	if n >= 0 {
		return out.SetFrac(q, pow10(n))
	}
	return out.SetInt(q.Mul(q, pow10(-n)))
}

// DecimalString formats a value already rounded to at most scale decimals,
// without trailing zeros: "18.36", "-0.5", "1000".
func DecimalString(r *big.Rat, scale int) string {
	s := r.FloatString(max(scale, 0))
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// String formats a value for humans and tests.
func (v Value) String() string {
	switch {
	case v.Err != "":
		return "#" + v.Err
	case v.Null:
		return "(empty)"
	case v.Type == TypeNumber:
		return DecimalString(RoundHalfAway(v.num, intermediateScale), intermediateScale)
	case v.Type == TypeDate:
		return v.date.Format(time.DateOnly)
	case v.Type == TypeBool:
		return strconv.FormatBool(v.b)
	}
	return "?"
}
