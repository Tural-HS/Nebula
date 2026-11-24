package formula

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

// Synthetic fields: amount, paid (number), due, start (date), urgent (bool),
// net (a formula of type number), memo (text: not usable).
func testResolver(ref string) (Type, error) {
	switch ref {
	case "amount", "paid", "rate", "net":
		return TypeNumber, nil
	case "due", "start", DueDateRef:
		return TypeDate, nil
	case "urgent":
		return TypeBool, nil
	case "memo":
		return 0, errors.New(`field "memo" is text; formulas can use number, date, boolean and formula fields`)
	}
	return 0, fmt.Errorf("unknown field %q", ref)
}

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func rat(s string) Value {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return Number(r)
}

var testValues = map[string]Value{
	"amount":   rat("1250.50"),
	"paid":     rat("200.25"),
	"rate":     rat("0.18"),
	"due":      Date(d("2026-12-31")),
	"start":    Date(d("2026-11-02")),
	DueDateRef: Date(d("2026-11-30")),
	"urgent":   Bool(true),
	"net":      Null(TypeNumber),
}

func testEnv(vals map[string]Value) Env {
	return Env{
		Ref: func(ref string) Value {
			if v, ok := vals[ref]; ok {
				return v
			}
			t, _ := testResolver(ref)
			return Null(t)
		},
		Today: time.Date(2026, 11, 24, 23, 30, 0, 0, time.UTC),
	}
}

func eval(t *testing.T, src string, vals map[string]Value) (Value, *Program) {
	t.Helper()
	p, err := Compile(src, testResolver)
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}
	return p.Eval(testEnv(vals)), p
}

func TestEval(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"amount - paid", "1050.25"},
		{"round(amount * 0.18, 2)", "225.09"}, // 225.09 exactly, no float noise
		{"round(amount * rate, 2)", "225.09"},
		{"0.1 + 0.2", "0.3"},
		{"1 / 3", "0.33333333333333333333"},
		{"(1 / 3) * 3", "0.99999999999999999999"},
		{"round(1 / 3 * 3, 10)", "1"},
		{"2 + 3 * 4", "14"},
		{"(2 + 3) * 4", "20"},
		{"-amount + 1", "-1249.5"},
		{"--1", "1"},
		{"+5", "5"},
		{"10 - 2 - 3", "5"}, // left associative
		{"8 / 2 / 2", "2"},
		{"abs(paid - amount)", "1050.25"},
		{"min(amount, paid, 300)", "200.25"},
		{"max(amount, paid)", "1250.5"},
		{"min(net, 5)", "5"}, // empty arguments are skipped
		{"max(net)", "(empty)"},
		{"coalesce(net, 0)", "0"},
		{"coalesce(net, paid, amount)", "200.25"},
		{"net + 1", "(empty)"}, // empty propagates through arithmetic
		{"if(paid > 0, amount / paid, 0)", "6.24469413233458177278"},
		{"if(urgent, 1, 2)", "1"},
		{"if(net > 0, 1, 2)", "2"}, // an empty condition takes the else branch
		{"if(amount >= 1000 and not urgent, 1, 0)", "0"},
		{"amount > paid", "true"},
		{"amount = 1250.5", "true"},
		{"amount <> 1250.5", "false"},
		{"amount != paid", "true"},
		{"urgent = true", "true"},
		{"net > 0 or urgent", "true"}, // true or empty = true
		{"net > 0 and urgent", "(empty)"},
		{"net > 0 and false", "false"}, // false and empty = false
		{"due - today()", "37"},        // date - date = days
		{"days(today(), due)", "37"},
		{"days(due, today())", "-37"},
		{"due + 30", "2027-01-30"},
		{"due - 1", "2026-12-30"},
		{"30 + start", "2026-12-02"},
		{"@due_date - start", "28"},
		{"today()", "2026-11-24"},
		{"date(2026, 2, 28) + 1", "2026-03-01"},
		{"date(2024, 2, 29)", "2024-02-29"},
		{"max(due, start)", "2026-12-31"},
		{"due > start", "true"},
		{"round(2.5)", "3"},
		{"round(-2.5)", "-3"},     // half away from zero
		{"round(2.45, 1)", "2.5"}, // exact decimal 2.45, not the float 2.4499...
		{"round(1234.5, -2)", "1200"},
		{"round(1250, -2)", "1300"},
		{"round(0.125, 2)", "0.13"},
		{"round(-0.125, 2)", "-0.13"},
		{"999999999999999 + 1", "1000000000000000"},
		{"1000000000000000 + 0.1", "1000000000000000.1"},
		{"workdays(date(2026, 11, 20), date(2026, 11, 27))", "5"}, // Fri -> Fri
		{"workdays(date(2026, 11, 27), date(2026, 11, 20))", "-5"},
		{"workdays(today(), today())", "0"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			v, _ := eval(t, tc.src, testValues)
			if got := v.String(); got != tc.want {
				t.Fatalf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestEvalErrors(t *testing.T) {
	tests := []struct{ src, want string }{
		{"amount / 0", "#" + ErrDivisionByZero},
		{"amount / (paid - paid)", "#" + ErrDivisionByZero},
		{"(amount / 0) + 1", "#" + ErrDivisionByZero},   // errors propagate
		{"min(1, amount / 0)", "#" + ErrDivisionByZero}, // also through functions
		{"if(amount / 0 > 1, 1, 2)", "#" + ErrDivisionByZero},
		{"coalesce(net, amount / 0)", "#" + ErrDivisionByZero},
		{"if(paid = 0, 0, amount / paid) * 0 + 1", "1"}, // the untaken branch is not evaluated
		{"if(true, 1, amount / 0)", "1"},
		{"1000000000000000 * 1000000000000000 * 10", "#" + ErrOverflow},
		{"due + 0.5", "#" + ErrInvalidArgument},
		{"date(2026, 2, 30)", "#" + ErrInvalidArgument},
		{"date(2026, 13, 1)", "#" + ErrInvalidArgument},
		{"date(2026.5, 1, 1)", "#" + ErrInvalidArgument},
		{"date(10000, 1, 1)", "#" + ErrDateRange},
		{"date(9999, 12, 31) + 1", "#" + ErrDateRange},
		{"date(1, 1, 1) - 1", "#" + ErrDateRange},
		{"due + 999999999999", "#" + ErrDateRange},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			v, _ := eval(t, tc.src, testValues)
			if got := v.String(); got != tc.want {
				t.Fatalf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestStoredErrorPropagates(t *testing.T) {
	vals := map[string]Value{"net": FromJSON(TypeNumber, map[string]any{"error": ErrDivisionByZero})}
	v, _ := eval(t, "net + 1", vals)
	if v.Err != ErrDivisionByZero {
		t.Fatalf("got %v", v)
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		src     string
		pos     int
		message string
	}{
		{"", 1, "empty"},
		{"   ", 1, "empty"},
		{"amount +", 9, "end of formula"},
		{"amount paid", 8, `unexpected "paid"`},
		{"amout - paid", 1, `unknown field "amout"`},
		{"amount - memo", 10, "is text"},
		{"round(amount, paid)", 15, "whole number"},
		{"round(amount, 13)", 15, "whole number"},
		{"round(amount, 1.5)", 15, "whole number"},
		{"sum(amount)", 1, "unknown function sum()"},
		{"today(1)", 1, "takes no arguments"},
		{"if(amount, 1, 2)", 4, "must be a boolean"},
		{"if(urgent, 1, due)", 15, "same type"},
		{"due + due", 5, "cannot use +"},
		{"amount * due", 8, "cannot use *"},
		{"amount > due", 8, "two numbers or two dates"},
		{"amount = due", 8, "cannot compare"},
		{"urgent < true", 8, "two numbers or two dates"},
		{"1 < 2 < 3", 7, "chained"},
		{"amount and urgent", 8, "two conditions"},
		{"not amount", 1, "needs a condition"},
		{"-urgent", 1, "needs a number"},
		{"(amount", 8, `expected ")"`},
		{"amount)", 7, `unexpected ")"`},
		{"round(amount 2)", 14, `expected "," or ")"`},
		{"Amount", 1, "lowercase"},
		{`amount = "x"`, 10, "text values"},
		{"amount # 2", 8, "unexpected character"},
		{".5 + 1", 1, "write 0.5"},
		{"5. + 1", 1, "write 0.5"},
		{"1.2.3", 1, "invalid number"},
		{"12abc", 3, `unexpected "a"`},
		{"@", 1, "name after @"},
		{"@created", 1, `unknown field "@created"`},
		{"1234567890123456789012345678901", 1, "more than 30 digits"},
		{"amount + 1 and", 15, "end of formula"},
		{"min()", 1, "at least 1"},
		{"days(due)", 1, "takes 2 arguments, got 1"},
		{"min(due, amount)", 10, "same type"},
		{"abs(due)", 5, "must be a number"},
		{"amount ≥ 1", 8, "unexpected character"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			_, err := Compile(tc.src, testResolver)
			var fe *Error
			if !errors.As(err, &fe) {
				t.Fatalf("Compile(%q) = %v, want an *Error", tc.src, err)
			}
			if fe.Pos != tc.pos || !strings.Contains(fe.Msg, tc.message) {
				t.Fatalf("Compile(%q) = %q at %d, want %q at %d", tc.src, fe.Msg, fe.Pos, tc.message, tc.pos)
			}
		})
	}
}

func TestLimits(t *testing.T) {
	long := strings.Repeat("1+", MaxLength/2) + "1"
	if _, err := Parse(long); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("long formula: %v", err)
	}
	deep := strings.Repeat("(", MaxDepth+1) + "1" + strings.Repeat(")", MaxDepth+1)
	if _, err := Parse(deep); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("deep parens: %v", err)
	}
	neg := strings.Repeat("-", MaxDepth+1) + "1"
	if _, err := Parse(neg); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("deep unary: %v", err)
	}
	// A long flat sum is fine syntactically but deeper than MaxDepth as a tree.
	flat := strings.Repeat("1+", MaxDepth) + "1"
	if _, err := Parse(flat); err == nil || !strings.Contains(err.Error(), "nested too deeply") {
		t.Fatalf("flat sum: %v", err)
	}
	okSum := strings.Repeat("amount+", MaxDepth-2) + "1"
	if _, err := Compile(okSum, testResolver); err != nil {
		t.Fatalf("sum within the depth limit: %v", err)
	}
	args := "min(" + strings.Repeat("1,", MaxArgs) + "1)"
	if _, err := Parse(args); err == nil || !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("args: %v", err)
	}
	refs := make([]string, MaxRefs+1)
	for i := range refs {
		refs[i] = fmt.Sprintf("f%d", i)
	}
	_, err := Compile("max("+strings.Join(refs[:20], ",")+") + max("+strings.Join(refs[20:40], ",")+") + max("+strings.Join(refs[40:], ",")+")",
		func(string) (Type, error) { return TypeNumber, nil })
	if err == nil || !strings.Contains(err.Error(), "too many different fields") {
		t.Fatalf("refs: %v", err)
	}
}

func TestProgramMetadata(t *testing.T) {
	_, p := eval(t, "if(urgent, workdays(today(), due), days(start, @due_date)) + amount + amount", testValues)
	if want := "@due_date,amount,due,start,urgent"; strings.Join(p.Refs, ",") != want {
		t.Fatalf("refs = %v, want %s", p.Refs, want)
	}
	if !p.Volatile || !p.Calendar || p.Result != TypeNumber {
		t.Fatalf("got volatile=%v calendar=%v result=%v", p.Volatile, p.Calendar, p.Result)
	}
	_, p = eval(t, "amount > 1", testValues)
	if p.Volatile || p.Calendar || p.Result != TypeBool {
		t.Fatalf("got volatile=%v calendar=%v result=%v", p.Volatile, p.Calendar, p.Result)
	}
}

func TestWorkdaysUsesEnv(t *testing.T) {
	p, err := Compile("workdays(start, due)", testResolver)
	if err != nil {
		t.Fatal(err)
	}
	env := testEnv(testValues)
	var gotFrom, gotTo time.Time
	env.Workdays = func(from, to time.Time) int { gotFrom, gotTo = from, to; return 42 }
	if v := p.Eval(env); v.String() != "42" || !gotFrom.Equal(d("2026-11-02")) || !gotTo.Equal(d("2026-12-31")) {
		t.Fatalf("got %v (%v, %v)", v, gotFrom, gotTo)
	}
}

func TestMondayToFriday(t *testing.T) {
	brute := func(from, to time.Time) int {
		sign := 1
		if to.Before(from) {
			from, to, sign = to, from, -1
		}
		n := 0
		for x := from.AddDate(0, 0, 1); !x.After(to); x = x.AddDate(0, 0, 1) {
			if x.Weekday() != time.Saturday && x.Weekday() != time.Sunday {
				n++
			}
		}
		return sign * n
	}
	base := d("2026-01-01")
	for i := range 40 {
		for j := range 40 {
			a, b := base.AddDate(0, 0, i), base.AddDate(0, 0, j)
			if got, want := MondayToFriday(a, b), brute(a, b); got != want {
				t.Fatalf("MondayToFriday(%s, %s) = %d, want %d", a.Format(time.DateOnly), b.Format(time.DateOnly), got, want)
			}
		}
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		v     Value
		scale int
		want  string
	}{
		{rat("225.09"), 2, "225.09"},
		{rat("225.095"), 2, "225.1"}, // half away from zero, trailing zero dropped
		{rat("-225.095"), 2, "-225.1"},
		{rat("18"), 2, "18"},
		{rat("-0.001"), 2, "0"}, // no "-0"
		{rat("1000000000000000.004"), 2, "1000000000000000"},
		{rat("1000000000000000.005"), 2, `{"error":"overflow"}`}, // rounds past the limit
		{rat("2.5"), 0, "3"},
		{rat("123456789.123456789"), 10, "123456789.123456789"},
		{Date(d("2026-01-31")), 2, `"2026-01-31"`},
		{Bool(false), 2, "false"},
		{Null(TypeNumber), 2, "null"},
		{errValue(TypeNumber, ErrDivisionByZero), 2, `{"error":"division_by_zero"}`},
	}
	for _, tc := range tests {
		b, err := json.Marshal(tc.v.JSON(tc.scale))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tc.want {
			t.Errorf("JSON(%v, %d) = %s, want %s", tc.v, tc.scale, b, tc.want)
		}
	}
}

func TestFromJSON(t *testing.T) {
	// 0.1 and 0.2 are stored as float64 JSON numbers; the shortest
	// representation recovers the exact decimals.
	a, b := FromJSON(TypeNumber, 0.1), FromJSON(TypeNumber, 0.2)
	sum := new(big.Rat).Add(a.Rat(), b.Rat())
	if sum.Cmp(big.NewRat(3, 10)) != 0 {
		t.Fatalf("0.1 + 0.2 = %v", sum)
	}
	if v := FromJSON(TypeNumber, 1e15); v.String() != "1000000000000000" {
		t.Fatalf("1e15 = %v", v)
	}
	if v := FromJSON(TypeNumber, json.Number("12.30")); v.String() != "12.3" {
		t.Fatalf("json.Number = %v", v)
	}
	for _, bad := range []any{"12", true, nil, []any{1}, map[string]any{"x": 1}} {
		if v := FromJSON(TypeNumber, bad); !v.Null {
			t.Fatalf("FromJSON(%v) = %v, want empty", bad, v)
		}
	}
	if v := FromJSON(TypeDate, "2026-02-30"); !v.Null {
		t.Fatalf("bad date = %v", v)
	}
	if v := FromJSON(TypeDate, "2026-02-28"); v.String() != "2026-02-28" {
		t.Fatalf("date = %v", v)
	}
	if v := FromJSON(TypeBool, true); !v.BoolValue() || v.Null {
		t.Fatalf("bool = %v", v)
	}
}

func TestLargeNumbersStayExact(t *testing.T) {
	vals := map[string]Value{"amount": FromJSON(TypeNumber, 999999999999999.0), "paid": FromJSON(TypeNumber, 0.01)}
	v, _ := eval(t, "amount + paid", vals)
	if got := DecimalString(v.Rat(), 2); got != "999999999999999.01" {
		t.Fatalf("got %s", got)
	}
}

// FuzzCompile checks that no input makes the compiler or the evaluator
// panic or run away, and that accepted formulas respect the limits.
func FuzzCompile(f *testing.F) {
	for _, s := range []string{
		"amount - paid", "round(amount * 0.18, 2)", "if(paid = 0, 0, amount / paid)", "due - today()",
		"workdays(today(), @due_date)", "min(1, 2, net)", "not (urgent and amount > 1)", "date(2026, 2, 29) + 1",
		"((((1))))", "-(-(-1))", "1 <> 2", "coalesce(net, 0) / 3", "round(1/3, -12)", "@due_date + 99999999",
		"max(due, start, today())", "amount ≥ 1", "\xff", "round(", ")(", "1..2",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		p, err := Compile(src, testResolver)
		if err != nil {
			var fe *Error
			if !errors.As(err, &fe) {
				t.Fatalf("Compile(%q) returned %T, want *Error", src, err)
			}
			if fe.Pos < 1 || fe.Pos > len([]rune(src))+1 {
				t.Fatalf("Compile(%q): position %d out of range", src, fe.Pos)
			}
			return
		}
		if len(src) > MaxLength {
			t.Fatalf("accepted a formula of %d bytes", len(src))
		}
		if depthOf(p.root) > MaxDepth {
			t.Fatalf("accepted depth %d", depthOf(p.root))
		}
		v := p.Eval(testEnv(testValues))
		if v.Type != p.Result {
			t.Fatalf("Eval(%q) has type %v, compiled as %v", src, v.Type, p.Result)
		}
		if _, err := json.Marshal(v.JSON(2)); err != nil {
			t.Fatalf("JSON(%q): %v", src, err)
		}
	})
}
