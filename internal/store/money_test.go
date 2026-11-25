package store

import (
	"reflect"
	"testing"
)

func TestCanonicalAmount(t *testing.T) {
	for _, c := range []struct {
		in    string
		scale int
		want  string
	}{
		{"1250.5", 2, "1250.50"}, {"1250", 2, "1250.00"}, {"0", 2, "0.00"}, {"-0", 2, "0.00"},
		{"-0.10", 2, "-0.10"}, {"+12", 0, "12"}, {"007.5", 1, "7.5"}, {".5", 2, "0.50"},
		{"1.2300", 2, "1.23"}, {"9999999999999", 2, "9999999999999.00"}, {"1.2345", 4, "1.2345"},
	} {
		got, err := CanonicalAmount(c.in, c.scale)
		if err != nil || got != c.want {
			t.Errorf("CanonicalAmount(%q, %d) = %q, %v; want %q", c.in, c.scale, got, err, c.want)
		}
	}
	for _, c := range []struct {
		in    string
		scale int
	}{
		{"1.005", 2}, {"1.5", 0}, {"1,5", 2}, {"1 000", 2}, {"1e3", 2}, {"", 2}, {"-", 2}, {"1.", 2},
		{"abc", 2}, {"10000000000000", 2}, {"0x10", 2}, {"١٢", 2},
	} {
		if got, err := CanonicalAmount(c.in, c.scale); err == nil {
			t.Errorf("CanonicalAmount(%q, %d) = %q, want an error", c.in, c.scale, got)
		}
	}
}

func TestNormalizeMoneyOptions(t *testing.T) {
	two := 2
	o, err := normalizeMoneyOptions(nil, MoneyInput{})
	if err != nil || !reflect.DeepEqual(o, &MoneyOptions{Currencies: []string{"AZN", "USD", "EUR"}, DefaultCurrency: "AZN", Scale: 2}) {
		t.Fatalf("defaults %+v %v", o, err)
	}
	o, err = normalizeMoneyOptions(nil, MoneyInput{Currencies: []string{" usd", "EUR", "USD"}, Scale: &two})
	if err != nil || !reflect.DeepEqual(o.Currencies, []string{"USD", "EUR"}) || o.DefaultCurrency != "USD" {
		t.Fatalf("dedupe %+v %v", o, err)
	}
	// Removing the default currency falls back to AZN when allowed.
	o, err = normalizeMoneyOptions(&MoneyOptions{Currencies: []string{"USD", "AZN"}, DefaultCurrency: "USD", Scale: 2},
		MoneyInput{Currencies: []string{"EUR", "AZN"}})
	if err != nil || o.DefaultCurrency != "AZN" || o.Scale != 2 {
		t.Fatalf("update %+v %v", o, err)
	}
	three, five, neg := 3, 5, -1
	for name, c := range map[string]struct {
		cur *MoneyOptions
		in  MoneyInput
	}{
		"bad code":      {nil, MoneyInput{Currencies: []string{"ABC"}}},
		"fund code":     {nil, MoneyInput{Currencies: []string{"XAU"}}},
		"empty":         {nil, MoneyInput{Currencies: []string{}}},
		"scale 5":       {nil, MoneyInput{Scale: &five}},
		"scale -1":      {nil, MoneyInput{Scale: &neg}},
		"scale changed": {&MoneyOptions{Currencies: []string{"AZN"}, DefaultCurrency: "AZN", Scale: 2}, MoneyInput{Scale: &three}},
		"bad default":   {nil, MoneyInput{Currencies: []string{"USD"}, DefaultCurrency: "AZN"}},
	} {
		if _, err := normalizeMoneyOptions(c.cur, c.in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCanonicalMoney(t *testing.T) {
	def := FieldDefinition{Key: "p", Type: FieldMoney, Options: FieldOptions{Money: &MoneyOptions{
		Currencies: []string{"AZN", "USD"}, DefaultCurrency: "AZN", Scale: 2}}}
	v, empty, err := canonicalValue(def, map[string]any{"amount": 12.5, "currency": "usd"}, "fields.p")
	if err != nil || empty || !reflect.DeepEqual(v, map[string]any{"amount": "12.50", "currency": "USD"}) {
		t.Fatalf("got %v %v %v", v, empty, err)
	}
	if _, empty, err := canonicalValue(def, map[string]any{"amount": ""}, "fields.p"); err != nil || !empty {
		t.Fatalf("empty amount: %v %v", empty, err)
	}
	for _, in := range []any{"12", 12.0, map[string]any{"amount": true}, map[string]any{"amount": "1", "currency": "EUR"},
		map[string]any{"amount": "1", "currency": 5}, map[string]any{"amount": "1", "x": 1}} {
		if _, _, err := canonicalValue(def, in, "fields.p"); err == nil {
			t.Errorf("%v: no error", in)
		}
	}
}

func TestResolveCurrency(t *testing.T) {
	def := FieldDefinition{Type: FieldMoney, Options: FieldOptions{Money: &MoneyOptions{
		Currencies: []string{"AZN", "USD", "EUR"}, DefaultCurrency: "AZN", Scale: 2}}}
	for token, want := range map[string]string{"": "AZN", "usd": "USD", "₼": "AZN", "$": "USD", "€": "EUR", "manat": "AZN"} {
		if got, err := ResolveCurrency(def, token); err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", token, got, err, want)
		}
	}
	for _, token := range []string{"GBP", "£", "₽", "XYZ", "dollars"} {
		if got, err := ResolveCurrency(def, token); err == nil {
			t.Errorf("%q = %q, want an error", token, got)
		}
	}
	// "$" is ambiguous once a second dollar currency is allowed.
	def.Options.Money.Currencies = append(def.Options.Money.Currencies, "CAD")
	if _, err := ResolveCurrency(def, "$"); err == nil {
		t.Error("$ with USD and CAD: no error")
	}
}
