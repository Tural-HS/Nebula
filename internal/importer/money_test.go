package importer

import (
	"errors"
	"testing"
)

func TestParseMoney(t *testing.T) {
	for _, c := range []struct {
		in, amount, cur string
		canonical       bool
	}{
		{"1250.50 AZN", "1250.50", "AZN", false},
		{"1 250,50 ₼", "1250.50", "₼", false},
		{"$300", "300", "$", false},
		{"AZN 1250", "1250", "AZN", false},
		{"usd 1,250.50", "1250.50", "USD", false},
		{"-12,5 EUR", "-12.5", "EUR", false},
		{"(100) manat", "-100", "manat", false},
		{"1.200,00 ₼", "1200.00", "₼", false},
		{"1250.5", "1250.5", "", true},
		{"0,5", "0.5", "", false},
	} {
		amount, cur, err := ParseMoney(c.in, c.canonical)
		if err != nil || amount != c.amount || cur != c.cur {
			t.Errorf("ParseMoney(%q) = %q, %q, %v; want %q, %q", c.in, amount, cur, err, c.amount, c.cur)
		}
	}
	for _, in := range []string{"", "AZN", "abc", "12 AZN USD", "₼12$", "1.234 AZN", "12,500", "1e5 USD", "12 dollars"} {
		if a, c, err := ParseMoney(in, false); err == nil {
			t.Errorf("ParseMoney(%q) = %q, %q; want an error", in, a, c)
		}
	}
	if _, _, err := ParseMoney("1.234 AZN", false); !errors.Is(err, ErrAmbiguousNumber) {
		t.Errorf("ambiguous: %v", err)
	}
}
