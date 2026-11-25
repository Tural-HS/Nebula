package store

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// FieldMoney is a money field (ADR-0044). Its stored value is an object
// {"amount": "1250.50", "currency": "AZN"}: the amount is exact decimal text
// with exactly the field's scale of decimals (never a float), the currency an
// ISO 4217 code from the field's allowed list. One canonical form per value,
// so containment filters ({"k": {"currency": "USD"}}) stay exact and
// index-backed like every other field type.
const FieldMoney = "money"

const (
	defaultMoneyScale = 2
	maxMoneyScale     = 4
	maxCurrencies     = 30
	// maxMoneyDigits bounds integer + fraction digits so every amount is
	// exact in a float64 and in Excel (15 significant digits): an export
	// never changes a cent.
	maxMoneyDigits = 15
)

var defaultCurrencies = []string{"AZN", "USD", "EUR"}

// iso4217 is the list of active ISO 4217 currency codes (no funds codes,
// precious metals or testing codes). Updated by hand: the list changes a few
// times a decade, and nothing is fetched at runtime.
var iso4217 = func() map[string]bool {
	codes := strings.Fields(`AED AFN ALL AMD AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BRL
		BSD BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD
		FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS
		KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK
		MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD
		SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX
		USD UYU UZS VES VND VUV WST XAF XCD XCG XOF XPF YER ZAR ZMW ZWG`)
	m := make(map[string]bool, len(codes))
	for _, c := range codes {
		m[c] = true
	}
	return m
}()

// IsCurrencyCode reports whether c is an active ISO 4217 code.
func IsCurrencyCode(c string) bool { return iso4217[c] }

// currencySymbols map the symbols accepted on import to the codes they may
// mean. A symbol is used only when exactly one of those codes is allowed by
// the field ("$" is USD unless the field also allows CAD, AUD, ...).
var currencySymbols = map[string][]string{
	"₼": {"AZN"}, "€": {"EUR"}, "£": {"GBP", "GIP", "FKP", "SHP"}, "₽": {"RUB"}, "₺": {"TRY"}, "₾": {"GEL"},
	"₴": {"UAH"}, "₸": {"KZT"}, "¥": {"JPY", "CNY"}, "₹": {"INR"},
	"$": {"USD", "CAD", "AUD", "NZD", "SGD", "HKD", "MXN", "ARS", "CLP", "COP", "TWD", "BSD", "BBD", "BZD", "BMD", "BND",
		"FJD", "GYD", "JMD", "KYD", "LRD", "NAD", "SBD", "SRD", "TTD", "XCD", "UYU", "CUP", "DOP"},
	"manat": {"AZN", "TMT"}, "man.": {"AZN"},
}

// MoneyOptions are the settings of a money field.
type MoneyOptions struct {
	Currencies      []string `json:"currencies"`       // allowed codes, in display order
	DefaultCurrency string   `json:"default_currency"` // one of Currencies
	Scale           int      `json:"scale"`            // decimals, 0..4; fixed at creation
}

// MoneyInput is options.money in a create/update request.
type MoneyInput struct {
	Currencies      []string `json:"currencies"`
	DefaultCurrency string   `json:"default_currency"`
	Scale           *int     `json:"scale"`
}

// normalizeMoneyOptions validates money options. cur is nil on create. The
// scale cannot change later: stored amounts carry exactly that many decimals.
// Currencies may be added or removed; values in a removed currency stay
// readable, only new writes must use an allowed one.
func normalizeMoneyOptions(cur *MoneyOptions, in MoneyInput) (*MoneyOptions, error) {
	out := &MoneyOptions{Scale: defaultMoneyScale}
	if cur != nil {
		*out = *cur
		out.Currencies = slices.Clone(cur.Currencies)
	}
	if in.Scale != nil {
		if *in.Scale < 0 || *in.Scale > maxMoneyScale {
			return nil, invalid("options.money.scale", "must be 0 to %d decimals", maxMoneyScale)
		}
		if cur != nil && *in.Scale != cur.Scale {
			return nil, invalid("options.money.scale", "cannot change after the field is created (stored amounts have %d decimals); create a new field instead", cur.Scale)
		}
		out.Scale = *in.Scale
	}
	switch {
	case in.Currencies != nil:
		if len(in.Currencies) == 0 {
			return nil, invalid("options.money.currencies", "must list at least one currency, e.g. AZN")
		}
		if len(in.Currencies) > maxCurrencies {
			return nil, invalid("options.money.currencies", "at most %d currencies", maxCurrencies)
		}
		out.Currencies = make([]string, 0, len(in.Currencies))
		for i, c := range in.Currencies {
			c = strings.ToUpper(strings.TrimSpace(c))
			if !iso4217[c] {
				return nil, invalid(fmt.Sprintf("options.money.currencies[%d]", i), "%q is not an ISO 4217 currency code (e.g. AZN, USD, EUR)", c)
			}
			if !slices.Contains(out.Currencies, c) {
				out.Currencies = append(out.Currencies, c)
			}
		}
	case cur == nil:
		out.Currencies = slices.Clone(defaultCurrencies)
	}
	if d := strings.ToUpper(strings.TrimSpace(in.DefaultCurrency)); d != "" {
		out.DefaultCurrency = d
	}
	if out.DefaultCurrency == "" || !slices.Contains(out.Currencies, out.DefaultCurrency) {
		if in.DefaultCurrency != "" {
			return nil, invalid("options.money.default_currency", "must be one of the allowed currencies (%s)", strings.Join(out.Currencies, ", "))
		}
		// Unset or no longer allowed: AZN when allowed, else the first.
		out.DefaultCurrency = out.Currencies[0]
		if slices.Contains(out.Currencies, "AZN") {
			out.DefaultCurrency = "AZN"
		}
	}
	return out, nil
}

// moneyOptionsOf returns the field's money options with defaults filled in.
func moneyOptionsOf(def FieldDefinition) MoneyOptions {
	if def.Options.Money != nil {
		return *def.Options.Money
	}
	return MoneyOptions{Currencies: slices.Clone(defaultCurrencies), DefaultCurrency: "AZN", Scale: defaultMoneyScale}
}

// CanonicalAmount turns decimal text ("1250.5", "-0", "+12") into the
// stored form with exactly scale decimals ("1250.50"). It never rounds:
// more decimals than the scale is an error, as silently dropping a fraction
// of a cent is worse than asking.
func CanonicalAmount(s string, scale int) (string, error) {
	s = strings.TrimSpace(s)
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" && hasDot {
		intPart = "0"
	}
	if intPart == "" || !allDigits(intPart) || (hasDot && (frac == "" || !allDigits(frac))) {
		return "", fmt.Errorf("must be a decimal amount with a dot, like 1250.50")
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	frac = strings.TrimRight(frac, "0")
	if len(frac) > scale {
		if scale == 0 {
			return "", fmt.Errorf("must be a whole amount (this field has no decimals)")
		}
		return "", fmt.Errorf("has more than %d decimals; round it to %d", scale, scale)
	}
	if len(intPart)+scale > maxMoneyDigits {
		return "", fmt.Errorf("is too large: at most %d digits before the decimal mark", maxMoneyDigits-scale)
	}
	out := intPart
	if scale > 0 {
		out += "." + frac + strings.Repeat("0", scale-len(frac))
	}
	if neg && strings.Trim(out, "0.") != "" {
		out = "-" + out
	}
	return out, nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// canonicalMoney validates a money value: {"amount": "1250.50" or 1250.5,
// "currency": "AZN"}. A missing or empty currency means the field's default.
// An empty amount (null or "") removes the value.
func canonicalMoney(def FieldDefinition, v any, path string) (any, bool, error) {
	opts := moneyOptionsOf(def)
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false, invalid(path, `must be an object like {"amount": "1250.50", "currency": "%s"}`, opts.DefaultCurrency)
	}
	for k := range m {
		if k != "amount" && k != "currency" {
			return nil, false, invalid(path, "unknown key %q: a money value has only amount and currency", k)
		}
	}
	var text string
	switch a := m["amount"].(type) {
	case nil:
		return nil, true, nil
	case string:
		if strings.TrimSpace(a) == "" {
			return nil, true, nil
		}
		text = a
	case float64:
		if math.IsNaN(a) || math.IsInf(a, 0) {
			return nil, false, invalid(path+".amount", "must be a decimal amount like 1250.50")
		}
		text = strconv.FormatFloat(a, 'f', -1, 64)
	default:
		return nil, false, invalid(path+".amount", "must be a decimal amount like \"1250.50\"")
	}
	amount, err := CanonicalAmount(text, opts.Scale)
	if err != nil {
		return nil, false, invalid(path+".amount", "%s", err.Error())
	}
	cur := opts.DefaultCurrency
	if c, ok := m["currency"]; ok && c != nil {
		s, ok := c.(string)
		if !ok {
			return nil, false, invalid(path+".currency", "must be a currency code such as %s", opts.DefaultCurrency)
		}
		if s = strings.ToUpper(strings.TrimSpace(s)); s != "" {
			cur = s
		}
	}
	if !slices.Contains(opts.Currencies, cur) {
		return nil, false, invalid(path+".currency", "%q is not allowed for this field; use one of %s", cur, strings.Join(opts.Currencies, ", "))
	}
	return map[string]any{"amount": amount, "currency": cur}, false, nil
}

// ResolveCurrency maps an import token (an ISO code, a symbol such as ₼ or
// $, or "" for none) to one of the field's currencies. Ambiguity is an
// error that says how to fix the cell.
func ResolveCurrency(def FieldDefinition, token string) (string, error) {
	opts := moneyOptionsOf(def)
	if token == "" {
		return opts.DefaultCurrency, nil
	}
	if up := strings.ToUpper(token); iso4217[up] {
		if !slices.Contains(opts.Currencies, up) {
			return "", fmt.Errorf("currency %s is not allowed for this field; use one of %s", up, strings.Join(opts.Currencies, ", "))
		}
		return up, nil
	}
	cands, ok := currencySymbols[strings.ToLower(token)]
	if !ok {
		return "", fmt.Errorf("unknown currency %q: write an ISO code such as %s", token, opts.DefaultCurrency)
	}
	var match []string
	for _, c := range cands {
		if slices.Contains(opts.Currencies, c) {
			match = append(match, c)
		}
	}
	switch len(match) {
	case 1:
		return match[0], nil
	case 0:
		return "", fmt.Errorf("currency %s is not allowed for this field; use one of %s", token, strings.Join(opts.Currencies, ", "))
	}
	return "", fmt.Errorf("the symbol %s is ambiguous here (%s): write the currency code instead", token, strings.Join(match, ", "))
}
