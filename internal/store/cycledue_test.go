package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMonthDayClamps(t *testing.T) {
	for _, tc := range []struct {
		month string
		day   int
		want  string
	}{
		{"2026-11-01", 31, "2026-11-30"},
		{"2026-12-01", 31, "2026-12-31"},
		{"2027-02-01", 31, "2027-02-28"},
		{"2028-02-01", 30, "2028-02-29"},
		{"2026-11-01", 5, "2026-11-05"},
	} {
		m, _ := time.Parse(time.DateOnly, tc.month)
		if got := monthDay(m, tc.day).Format(time.DateOnly); got != tc.want {
			t.Errorf("monthDay(%s, %d) = %s, want %s", tc.month, tc.day, got, tc.want)
		}
	}
}

func TestCycleDueInAndCarries(t *testing.T) {
	field, paid := "loan_date", "paid"
	c := CycleInput{DueDayField: &field, PaidField: &paid}
	nov, _ := time.Parse(time.DateOnly, "2026-11-01")
	item := func(loan string, paidV any, due *string) cycleItem {
		f := map[string]json.RawMessage{}
		if loan != "" {
			f[field], _ = json.Marshal(loan)
		}
		if paidV != nil {
			f[paid], _ = json.Marshal(paidV)
		}
		return cycleItem{open: true, fields: f, due: due}
	}
	for _, tc := range []struct {
		loan, want string
		ok         bool
	}{
		{"2025-01-31", "2026-11-30", true},
		{"2025-03-10", "2026-11-10", true},
		{"2026-10-20", "2026-11-20", true}, // given last month: due this month
		{"2026-11-20", "2026-12-20", true}, // given this month: first due next month
		{"", "", false},
		{"not a date", "", false},
	} {
		got, ok := c.dueIn(item(tc.loan, nil, nil), nov)
		if got != tc.want || ok != tc.ok {
			t.Errorf("dueIn(%q) = %q, %v; want %q, %v", tc.loan, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := (CycleInput{}).dueIn(item("2025-01-31", nil, nil), nov); ok {
		t.Error("dueIn without a due day field")
	}

	today, _ := time.Parse(time.DateOnly, "2026-11-01")
	past, same := "2026-10-15", "2026-11-01"
	for i, tc := range []struct {
		it   cycleItem
		want bool
	}{
		{item("2025-01-15", false, &past), true},
		{item("2025-01-15", nil, &past), true}, // no mark = unpaid
		{item("2025-01-15", true, &past), false},
		{item("2025-01-15", false, &same), false}, // due today: not late yet
		{item("2025-01-15", false, nil), false},
	} {
		if got := c.carries(tc.it, today); got != tc.want {
			t.Errorf("case %d: carries = %v, want %v", i, got, tc.want)
		}
	}
	done := item("2025-01-15", nil, &past)
	done.open = false
	if c.carries(done, today) {
		t.Error("carried a done item")
	}
	noPaid := CycleInput{DueDayField: &field}
	if noPaid.carries(item("2025-01-15", false, &past), today) {
		t.Error("carried without a paid field")
	}
}

// ADR-0060: a payment-day override plus the loan date as the start.
func TestCycleDueInWithStartField(t *testing.T) {
	day, start := "payment_day", "loan_date"
	c := CycleInput{DueDayField: &day, DueStartField: &start}
	nov, _ := time.Parse(time.DateOnly, "2026-11-01")
	item := func(payDay, loan string) cycleItem {
		f := map[string]json.RawMessage{}
		if payDay != "" {
			f[day], _ = json.Marshal(payDay)
		}
		if loan != "" {
			f[start], _ = json.Marshal(loan)
		}
		return cycleItem{open: true, fields: f}
	}
	for _, tc := range []struct {
		payDay, loan, want string
		ok                 bool
	}{
		{"2026-01-01", "2026-07-20", "2026-11-01", true}, // override day, old loan
		{"2026-01-31", "2025-03-31", "2026-11-30", true}, // clamped
		{"", "2026-10-05", "2026-11-05", true},           // no override: the loan day
		{"", "2026-11-09", "2026-12-09", true},           // loan this month: first due next month
		{"2026-01-12", "2026-11-12", "2026-12-12", true}, // override, but the loan is this month
		{"", "", "", false},
	} {
		got, ok := c.dueIn(item(tc.payDay, tc.loan), nov)
		if got != tc.want || ok != tc.ok {
			t.Errorf("dueIn(%q, %q) = %q, %v; want %q, %v", tc.payDay, tc.loan, got, ok, tc.want, tc.ok)
		}
	}
}
