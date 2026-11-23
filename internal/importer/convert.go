package importer

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// Fold normalizes text for case-insensitive matching of labels, names and
// emails: Unicode lower case, dotless ı treated as i (Azerbaijani/Turkish
// I/İ/ı/i all match), whitespace runs collapsed.
func Fold(s string) string {
	var b strings.Builder
	for i, w := range strings.Fields(s) {
		if i > 0 {
			b.WriteByte(' ')
		}
		for _, r := range w {
			r = unicode.ToLower(r)
			if r == 'ı' {
				r = 'i'
			}
			if r == '̇' { // combining dot left by some İ lowercasings
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// currencyTokens are stripped from number cells (compared after Fold).
var currencyTokens = []string{"manat", "man.", "azn", "₼", "usd", "$", "eur", "€", "rub", "₽", "gbp", "£", "try", "₺"}

// ErrAmbiguousNumber means a value like "1.234" could be read either way.
var ErrAmbiguousNumber = errors.New(`ambiguous number: write it without a thousands separator (1234 or 1234,5)`)

var errNotNumber = errors.New("must be a number")

// ParseNumber reads a number cell. canonical is true for xlsx, whose stored
// values are always machine format ("1234.5"); those are tried first.
// Otherwise thousands separators (space, dot, comma, apostrophe), currency
// symbols such as ₼ and either decimal separator are accepted. A single
// separator followed by exactly three digits ("1.234", "12,500") is rejected
// as ambiguous rather than guessed: silently importing 1.234 instead of 1234
// manat is worse than asking.
func ParseNumber(s string, canonical bool) (float64, error) {
	s = strings.TrimSpace(s)
	if canonical {
		if v, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return v, nil
		}
	}
	s = Fold(s)
	for _, tok := range currencyTokens {
		s = strings.ReplaceAll(s, tok, "")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\'' || r == '’' {
			return -1
		}
		if r == '−' { // U+2212 minus sign
			return '-'
		}
		return r
	}, s)
	neg := false
	switch {
	case strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"):
		neg, s = true, s[1:len(s)-1]
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	if s == "" {
		return 0, errNotNumber
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != ',' && r != '.' {
			return 0, errNotNumber
		}
	}

	intPart, frac, err := splitDecimal(s)
	if err != nil {
		return 0, err
	}
	norm := intPart
	if frac != "" {
		norm += "." + frac
	}
	v, err := strconv.ParseFloat(norm, 64)
	if err != nil || math.IsInf(v, 0) {
		return 0, errNotNumber
	}
	if neg {
		v = -v
	}
	return v, nil
}

// splitDecimal separates the integer digits (thousands separators removed and
// validated) from the fraction digits.
func splitDecimal(s string) (intPart, frac string, err error) {
	commas, dots := strings.Count(s, ","), strings.Count(s, ".")
	var dec, thou string
	switch {
	case commas > 0 && dots > 0:
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			dec, thou = ",", "."
		} else {
			dec, thou = ".", ","
		}
		if strings.Count(s, dec) > 1 {
			return "", "", errNotNumber
		}
	case commas > 1:
		thou = ","
	case dots > 1:
		thou = "."
	case commas == 1 || dots == 1:
		sep := ","
		if dots == 1 {
			sep = "."
		}
		before, after, _ := strings.Cut(s, sep)
		if len(after) == 3 && before != "" && before != "0" && len(before) <= 3 {
			return "", "", ErrAmbiguousNumber
		}
		dec = sep
	}

	intPart = s
	if dec != "" {
		var ok bool
		intPart, frac, ok = strings.Cut(s, dec)
		if !ok || frac == "" || strings.ContainsAny(frac, ",.") {
			return "", "", errNotNumber
		}
	}
	if thou != "" && strings.Contains(intPart, thou) {
		groups := strings.Split(intPart, thou)
		if len(groups[0]) < 1 || len(groups[0]) > 3 {
			return "", "", errNotNumber
		}
		for _, g := range groups[1:] {
			if len(g) != 3 {
				return "", "", errNotNumber
			}
		}
		intPart = strings.Join(groups, "")
	}
	if intPart == "" {
		intPart = "0" // ",5"
	}
	if strings.ContainsAny(intPart, ",.") {
		return "", "", errNotNumber
	}
	return intPart, frac, nil
}

// Excel serial date range: 1900-01-01 .. 9999-12-31.
const maxExcelSerial = 2_958_465

// Date layouts accepted for text cells, day before month (Azerbaijani usage).
var dateLayouts = []string{
	time.DateOnly,
	"2.1.2006", "2/1/2006", "2-1-2006",
}

var dateTimeLayouts = []string{
	"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04",
	"2.1.2006 15:04:05", "2.1.2006 15:04", "2/1/2006 15:04:05", "2/1/2006 15:04",
}

var errDate = errors.New("must be a date: YYYY-MM-DD or DD.MM.YYYY")

// ParseDate returns a date as YYYY-MM-DD. serials enables Excel serial
// numbers (only meaningful for xlsx cells, where dates are stored that way);
// a time of day, if present, is dropped.
func ParseDate(s string, serials, date1904 bool) (string, error) {
	t, err := parseTime(s, serials, date1904)
	if err != nil {
		return "", errDate
	}
	return t.Format(time.DateOnly), nil
}

// ParseDateTime returns an RFC 3339 UTC timestamp. Values without a time
// zone (and Excel serials) are taken as UTC.
func ParseDateTime(s string, serials, date1904 bool) (string, error) {
	t, err := parseTime(s, serials, date1904)
	if err != nil {
		return "", errors.New("must be a date and time: YYYY-MM-DD HH:MM or DD.MM.YYYY HH:MM")
	}
	return t.UTC().Truncate(time.Second).Format(time.RFC3339), nil
}

func parseTime(s string, serials, date1904 bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if serials {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			if v < 1 || v > maxExcelSerial {
				return time.Time{}, errDate
			}
			t, err := excelize.ExcelDateToTime(v, date1904)
			if err != nil {
				return time.Time{}, errDate
			}
			return t.UTC().Round(time.Second), nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	for _, l := range dateTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errDate
}

var (
	trueWords  = []string{"true", "yes", "y", "1", "bəli", "beli", "hə", "he", "var", "doğru", "dogru", "x", "✓", "✔"}
	falseWords = []string{"false", "no", "n", "0", "xeyr", "yox", "yanlış", "yanlis"}
)

// ParseBool reads yes/no style cells, Azerbaijani words included.
func ParseBool(s string) (bool, error) {
	f := Fold(s)
	for _, w := range trueWords {
		if f == Fold(w) {
			return true, nil
		}
	}
	for _, w := range falseWords {
		if f == Fold(w) {
			return false, nil
		}
	}
	return false, errors.New("must be yes/no (bəli/xeyr, true/false, 1/0)")
}

// SplitList splits a multi-value cell on commas and semicolons.
func SplitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
