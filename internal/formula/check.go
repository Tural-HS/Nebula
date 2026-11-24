package formula

import (
	"fmt"
	"slices"
)

// Type is the static type of an expression.
type Type uint8

// Types. A formula field stores its result in the stored form of the
// matching plain field type: JSON number, "YYYY-MM-DD", true/false.
const (
	TypeNumber Type = iota + 1
	TypeDate
	TypeBool
)

func (t Type) String() string {
	switch t {
	case TypeNumber:
		return "number"
	case TypeDate:
		return "date"
	case TypeBool:
		return "boolean"
	}
	return "unknown"
}

// ParseType is the inverse of Type.String.
func ParseType(s string) (Type, bool) {
	for _, t := range []Type{TypeNumber, TypeDate, TypeBool} {
		if t.String() == s {
			return t, true
		}
	}
	return 0, false
}

// Resolver gives the type of a reference (a field key or a built-in such as
// DueDateRef). A returned error's text is reported at the reference.
type Resolver func(ref string) (Type, error)

// Program is a compiled, type-checked formula, safe for concurrent use.
type Program struct {
	root *node
	// Result is the type of the value the formula produces.
	Result Type
	// Refs are the distinct references (field keys and built-ins), sorted.
	Refs []string
	// Volatile formulas use today(): their value changes with the date.
	Volatile bool
	// Calendar formulas use workdays(): their value changes with the
	// workspace's working-day calendar (ADR-0027).
	Calendar bool
}

// Compile parses src and checks every reference and type.
func Compile(src string, resolve Resolver) (*Program, error) {
	tree, err := Parse(src)
	if err != nil {
		return nil, err
	}
	c := &checker{resolve: resolve, refs: map[string]bool{}}
	t, err := c.check(tree.root)
	if err != nil {
		return nil, err
	}
	p := &Program{root: tree.root, Result: t, Volatile: c.volatile, Calendar: c.calendar}
	for r := range c.refs {
		p.Refs = append(p.Refs, r)
	}
	slices.Sort(p.Refs)
	return p, nil
}

type checker struct {
	resolve  Resolver
	refs     map[string]bool
	volatile bool
	calendar bool
}

func errAt(n *node, format string, args ...any) error {
	return &Error{Pos: n.pos, Msg: fmt.Sprintf(format, args...)}
}

func (c *checker) check(n *node) (Type, error) {
	t, err := c.typeOf(n)
	n.typ = t
	return t, err
}

func (c *checker) typeOf(n *node) (Type, error) {
	switch n.kind {
	case nNum:
		if _, err := parseNumber(n.text); err != nil {
			return 0, errAt(n, "invalid number %q", n.text)
		}
		return TypeNumber, nil
	case nBool:
		return TypeBool, nil
	case nRef:
		if !c.refs[n.text] && len(c.refs) >= MaxRefs {
			return 0, errAt(n, "too many different fields (at most %d)", MaxRefs)
		}
		t, err := c.resolve(n.text)
		if err != nil {
			return 0, errAt(n, "%s", err.Error())
		}
		c.refs[n.text] = true
		return t, nil
	case nNeg:
		t, err := c.check(n.args[0])
		if err != nil {
			return 0, err
		}
		if t != TypeNumber {
			return 0, errAt(n, "minus needs a number, not a %s", t)
		}
		return TypeNumber, nil
	case nNot:
		t, err := c.check(n.args[0])
		if err != nil {
			return 0, err
		}
		if t != TypeBool {
			return 0, errAt(n, "not needs a condition, not a %s", t)
		}
		return TypeBool, nil
	case nBin:
		return c.binary(n)
	case nCall:
		return c.call(n)
	}
	return 0, errAt(n, "invalid expression")
}

func (c *checker) binary(n *node) (Type, error) {
	l, err := c.check(n.args[0])
	if err != nil {
		return 0, err
	}
	r, err := c.check(n.args[1])
	if err != nil {
		return 0, err
	}
	switch n.op {
	case "+":
		switch {
		case l == TypeNumber && r == TypeNumber:
			return TypeNumber, nil
		case l == TypeDate && r == TypeNumber, l == TypeNumber && r == TypeDate:
			return TypeDate, nil // a date shifted by a number of days
		}
	case "-":
		switch {
		case l == TypeNumber && r == TypeNumber:
			return TypeNumber, nil
		case l == TypeDate && r == TypeNumber:
			return TypeDate, nil
		case l == TypeDate && r == TypeDate:
			return TypeNumber, nil // days between
		}
	case "*", "/":
		if l == TypeNumber && r == TypeNumber {
			return TypeNumber, nil
		}
	case "=", "!=":
		if l == r {
			return TypeBool, nil
		}
		return 0, errAt(n, "cannot compare a %s with a %s", l, r)
	case "<", "<=", ">", ">=":
		if l == r && l != TypeBool {
			return TypeBool, nil
		}
		return 0, errAt(n, "%s needs two numbers or two dates, not a %s and a %s", n.op, l, r)
	case "and", "or":
		if l == TypeBool && r == TypeBool {
			return TypeBool, nil
		}
		return 0, errAt(n, "%s needs two conditions, not a %s and a %s", n.op, l, r)
	}
	return 0, errAt(n, "cannot use %s with a %s and a %s", n.op, l, r)
}

// functions lists the built-in functions for error messages and the UI.
var functions = []string{"abs", "coalesce", "date", "days", "if", "max", "min", "round", "today", "workdays"}

// Functions returns the names of the built-in functions.
func Functions() []string { return slices.Clone(functions) }

func (c *checker) call(n *node) (Type, error) {
	argc := len(n.args)
	want := func(lo, hi int) error {
		if argc >= lo && argc <= hi {
			return nil
		}
		switch {
		case lo == hi && lo == 0:
			return errAt(n, "%s() takes no arguments", n.op)
		case lo == hi:
			return errAt(n, "%s() takes %d arguments, got %d", n.op, lo, argc)
		case hi >= MaxArgs:
			return errAt(n, "%s() takes at least %d argument(s)", n.op, lo)
		}
		return errAt(n, "%s() takes %d to %d arguments, got %d", n.op, lo, hi, argc)
	}
	types := make([]Type, argc)
	checkArgs := func() error {
		for i, a := range n.args {
			t, err := c.check(a)
			if err != nil {
				return err
			}
			types[i] = t
		}
		return nil
	}
	need := func(i int, t Type) error {
		if types[i] != t {
			return errAt(n.args[i], "argument %d of %s() must be a %s, not a %s", i+1, n.op, t, types[i])
		}
		return nil
	}
	same := func(from int, allowed ...Type) (Type, error) {
		t := types[from]
		if !slices.Contains(allowed, t) {
			return 0, errAt(n.args[from], "%s() does not accept a %s", n.op, t)
		}
		for i := from + 1; i < argc; i++ {
			if types[i] != t {
				return 0, errAt(n.args[i], "all arguments of %s() must have the same type (%s), not a %s", n.op, t, types[i])
			}
		}
		return t, nil
	}

	switch n.op {
	case "if":
		if err := want(3, 3); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		if err := need(0, TypeBool); err != nil {
			return 0, err
		}
		return same(1, TypeNumber, TypeDate, TypeBool)
	case "round":
		if err := want(1, 2); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		if err := need(0, TypeNumber); err != nil {
			return 0, err
		}
		if argc == 2 {
			if _, ok := constInt(n.args[1]); !ok {
				return 0, errAt(n.args[1], "the number of decimals must be a whole number from %d to %d", -maxRoundDigits, maxRoundDigits)
			}
		}
		return TypeNumber, nil
	case "abs":
		if err := want(1, 1); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		return TypeNumber, need(0, TypeNumber)
	case "min", "max":
		if err := want(1, MaxArgs); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		return same(0, TypeNumber, TypeDate)
	case "coalesce":
		if err := want(1, MaxArgs); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		return same(0, TypeNumber, TypeDate, TypeBool)
	case "today":
		if err := want(0, 0); err != nil {
			return 0, err
		}
		c.volatile = true
		return TypeDate, nil
	case "days", "workdays":
		if err := want(2, 2); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		if err := need(0, TypeDate); err != nil {
			return 0, err
		}
		if err := need(1, TypeDate); err != nil {
			return 0, err
		}
		if n.op == "workdays" {
			c.calendar = true
		}
		return TypeNumber, nil
	case "date":
		if err := want(3, 3); err != nil {
			return 0, err
		}
		if err := checkArgs(); err != nil {
			return 0, err
		}
		for i := range 3 {
			if err := need(i, TypeNumber); err != nil {
				return 0, err
			}
		}
		return TypeDate, nil
	}
	return 0, errAt(n, "unknown function %s() (available: %s)", n.op, joinNames(functions))
}

func joinNames(names []string) string {
	s := ""
	for i, n := range names {
		if i > 0 {
			s += ", "
		}
		s += n
	}
	return s
}

// constInt reports the value of an integer literal, optionally negated,
// within the round() range.
func constInt(n *node) (int, bool) {
	neg := false
	if n.kind == nNeg {
		neg, n = true, n.args[0]
	}
	if n.kind != nNum {
		return 0, false
	}
	v := 0
	for _, ch := range n.text {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + int(ch-'0')
		if v > maxRoundDigits {
			return 0, false
		}
	}
	if neg {
		v = -v
	}
	return v, true
}
