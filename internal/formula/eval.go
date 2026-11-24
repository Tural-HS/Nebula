package formula

import (
	"math/big"
	"time"
)

// Env supplies what a formula reads while it is evaluated.
type Env struct {
	// Ref returns the value of a reference; an absent value is Null of the
	// reference's type. It must agree with the Resolver used to compile.
	Ref func(ref string) Value
	// Today is the workspace's local date (the time of day is ignored).
	Today time.Time
	// Workdays counts the working days d with from < d <= to (negative when
	// to is before from). Nil uses Monday to Friday without holidays.
	Workdays func(from, to time.Time) int
}

var (
	minDate = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	maxDate = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	// maxShiftDays keeps date + n inside time.Duration arithmetic.
	maxShiftDays = big.NewRat(3_700_000, 1)
)

// Eval computes the formula. It never panics on any input the compiler
// accepted and never fails: problems become error values (see Err*).
func (p *Program) Eval(env Env) Value {
	e := evaluator{env: env}
	return e.eval(p.root)
}

type evaluator struct{ env Env }

// normalize keeps intermediate numbers bounded: intermediateScale decimals
// and |v| <= 1e30.
func normalize(r *big.Rat) Value {
	if !r.IsInt() {
		r = RoundHalfAway(r, intermediateScale)
	}
	if new(big.Rat).Abs(r).Cmp(maxIntermediate) > 0 {
		return errValue(TypeNumber, ErrOverflow)
	}
	return Number(r)
}

func dateValue(t time.Time) Value {
	if t.Before(minDate) || t.After(maxDate) {
		return errValue(TypeDate, ErrDateRange)
	}
	return Date(t)
}

// daysBetween is b - a in days. Unix seconds cover years 1..9999 exactly,
// unlike time.Duration (which saturates after 292 years).
func daysBetween(a, b time.Time) int64 { return (b.Unix() - a.Unix()) / 86400 }

// shift adds n whole days to d.
func shift(d time.Time, n *big.Rat) Value {
	if !n.IsInt() {
		return errValue(TypeDate, ErrInvalidArgument)
	}
	if new(big.Rat).Abs(n).Cmp(maxShiftDays) > 0 {
		return errValue(TypeDate, ErrDateRange)
	}
	return dateValue(d.AddDate(0, 0, int(n.Num().Int64())))
}

func (e *evaluator) eval(n *node) Value {
	switch n.kind {
	case nNum:
		r, err := parseNumber(n.text)
		if err != nil {
			return errValue(TypeNumber, ErrInvalidArgument)
		}
		return Number(r)
	case nBool:
		return Bool(n.text == "true")
	case nRef:
		v := e.env.Ref(n.text)
		if v.Type != n.typ { // defensive: a resolver/env mismatch reads as empty
			return Null(n.typ)
		}
		return v
	case nNeg:
		x := e.eval(n.args[0])
		if !x.ok() {
			return x
		}
		return Number(new(big.Rat).Neg(x.num))
	case nNot:
		x := e.eval(n.args[0])
		if !x.ok() {
			return x
		}
		return Bool(!x.b)
	case nBin:
		if n.op == "and" || n.op == "or" {
			return e.logic(n)
		}
		l := e.eval(n.args[0])
		if l.Err != "" {
			return errValue(n.typ, l.Err)
		}
		r := e.eval(n.args[1])
		if r.Err != "" {
			return errValue(n.typ, r.Err)
		}
		if l.Null || r.Null {
			return Null(n.typ)
		}
		return binary(n.op, l, r)
	case nCall:
		return e.call(n)
	}
	return errValue(n.typ, ErrInvalidArgument)
}

// logic is three-valued (SQL): false and x = false, true or x = true, and an
// empty operand otherwise makes the result empty. Errors propagate.
func (e *evaluator) logic(n *node) Value {
	l := e.eval(n.args[0])
	if l.Err != "" {
		return l
	}
	short := n.op == "or" // the value that decides the result alone
	if l.ok() && l.b == short {
		return Bool(short)
	}
	r := e.eval(n.args[1])
	if r.Err != "" {
		return r
	}
	if r.ok() && r.b == short {
		return Bool(short)
	}
	if l.Null || r.Null {
		return Null(TypeBool)
	}
	return Bool(!short)
}

func binary(op string, l, r Value) Value {
	switch op {
	case "+":
		switch {
		case l.Type == TypeNumber && r.Type == TypeNumber:
			return normalize(new(big.Rat).Add(l.num, r.num))
		case l.Type == TypeDate:
			return shift(l.date, r.num)
		default:
			return shift(r.date, l.num)
		}
	case "-":
		switch {
		case l.Type == TypeNumber:
			return normalize(new(big.Rat).Sub(l.num, r.num))
		case r.Type == TypeNumber:
			return shift(l.date, new(big.Rat).Neg(r.num))
		default:
			return Number(new(big.Rat).SetInt64(daysBetween(r.date, l.date)))
		}
	case "*":
		return normalize(new(big.Rat).Mul(l.num, r.num))
	case "/":
		if r.num.Sign() == 0 {
			return errValue(TypeNumber, ErrDivisionByZero)
		}
		return normalize(new(big.Rat).Quo(l.num, r.num))
	}
	c := compare(l, r)
	switch op {
	case "=":
		return Bool(c == 0)
	case "!=":
		return Bool(c != 0)
	case "<":
		return Bool(c < 0)
	case "<=":
		return Bool(c <= 0)
	case ">":
		return Bool(c > 0)
	case ">=":
		return Bool(c >= 0)
	}
	return errValue(TypeBool, ErrInvalidArgument)
}

func compare(l, r Value) int {
	switch l.Type {
	case TypeNumber:
		return l.num.Cmp(r.num)
	case TypeDate:
		return l.date.Compare(r.date)
	default:
		switch {
		case l.b == r.b:
			return 0
		case r.b:
			return -1
		}
		return 1
	}
}

func (e *evaluator) call(n *node) Value {
	args := n.args
	switch n.op {
	case "if":
		c := e.eval(args[0])
		if c.Err != "" {
			return errValue(n.typ, c.Err)
		}
		// Only the chosen branch is evaluated, so if(paid = 0, 0, amount / paid)
		// never divides by zero. An empty condition takes the else branch.
		if c.ok() && c.b {
			return e.eval(args[1])
		}
		return e.eval(args[2])
	case "round":
		x := e.eval(args[0])
		if !x.ok() {
			return x
		}
		digits := 0
		if len(args) == 2 {
			digits, _ = constInt(args[1])
		}
		return normalize(RoundHalfAway(x.num, digits))
	case "abs":
		x := e.eval(args[0])
		if !x.ok() {
			return x
		}
		return Number(new(big.Rat).Abs(x.num))
	case "min", "max":
		// Empty arguments are skipped, like blank cells in Excel's MIN/MAX.
		var best Value
		found := false
		for _, a := range args {
			v := e.eval(a)
			if v.Err != "" {
				return v
			}
			if v.Null {
				continue
			}
			c := 0
			if found {
				c = compare(v, best)
			}
			if !found || (n.op == "min" && c < 0) || (n.op == "max" && c > 0) {
				best, found = v, true
			}
		}
		if !found {
			return Null(n.typ)
		}
		return best
	case "coalesce":
		for _, a := range args {
			v := e.eval(a)
			if !v.Null {
				return v // a value or an error
			}
		}
		return Null(n.typ)
	case "today":
		return Date(e.env.Today)
	case "days", "workdays":
		a := e.eval(args[0])
		if !a.ok() {
			return Value{Type: TypeNumber, Null: a.Null, Err: a.Err}
		}
		b := e.eval(args[1])
		if !b.ok() {
			return Value{Type: TypeNumber, Null: b.Null, Err: b.Err}
		}
		if n.op == "days" {
			return Number(new(big.Rat).SetInt64(daysBetween(a.date, b.date)))
		}
		count := e.env.Workdays
		if count == nil {
			count = MondayToFriday
		}
		return Number(new(big.Rat).SetInt64(int64(count(a.date, b.date))))
	case "date":
		parts := [3]int{}
		for i, a := range args {
			v := e.eval(a)
			if !v.ok() {
				return Value{Type: TypeDate, Null: v.Null, Err: v.Err}
			}
			if !v.num.IsInt() || !v.num.Num().IsInt64() {
				return errValue(TypeDate, ErrInvalidArgument)
			}
			x := v.num.Num().Int64()
			if x < 0 || x > 9999 {
				return errValue(TypeDate, ErrDateRange)
			}
			parts[i] = int(x)
		}
		y, m, d := parts[0], parts[1], parts[2]
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		if y < 1 || t.Year() != y || int(t.Month()) != m || t.Day() != d {
			return errValue(TypeDate, ErrInvalidArgument) // no 31 February
		}
		return Date(t)
	}
	return errValue(n.typ, ErrInvalidArgument)
}

// MondayToFriday counts the weekdays d with from < d <= to (negative when
// to is before from): the default working-day calendar.
func MondayToFriday(from, to time.Time) int {
	if to.Before(from) {
		return -MondayToFriday(to, from)
	}
	n := int(daysBetween(from, to))
	count := n / 7 * 5
	wd := from.Weekday()
	for range n % 7 {
		wd = (wd + 1) % 7
		if wd != time.Saturday && wd != time.Sunday {
			count++
		}
	}
	return count
}
