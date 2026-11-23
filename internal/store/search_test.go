package store

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSearchTerms(t *testing.T) {
	for q, want := range map[string]string{
		"Ödəniş sistemi":        "odenis sistemi",
		"  ÇIXIŞ  çıxış ":       "cixis",           // folded duplicates collapse
		"a b QA-12":             "qa 12",           // single letters dropped, digits kept
		"x 7":                   "7",               // a lone digit is a term
		"foo:* | !bar & (baz)":  "foo bar baz",     // tsquery syntax cannot get through
		"ödénis":              "odenis",          // decomposed accents
		"1 2 3 4 5 6 7 8 9 10 ": "1 2 3 4 5 6 7 8", // at most 8 terms
		"!!":                    "",
	} {
		if got := strings.Join(searchTerms(q), " "); got != want {
			t.Errorf("searchTerms(%q) = %q, want %q", q, got, want)
		}
	}
	if got := tsQuery([]string{"odenis", "12"}); got != "odenis:* & 12:*" {
		t.Errorf("tsQuery: %q", got)
	}
}

func TestValidateSearchQuery(t *testing.T) {
	for q, ok := range map[string]bool{
		"":                       false,
		"a":                      false,
		"QA-12":                  true,
		"ab":                     true,
		strings.Repeat("ş", 200): true,
		strings.Repeat("ş", 201): false,
		"\xff\xfe":               false,
	} {
		if _, err := ValidateSearchQuery(q); (err == nil) != ok {
			t.Errorf("ValidateSearchQuery(%.12q): %v", q, err)
		}
	}
}

func render(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		if s.Match {
			b.WriteString("[" + s.Text + "]")
		} else {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func TestHighlight(t *testing.T) {
	terms := searchTerms("odenis musteri")
	for text, want := range map[string]string{
		"Ödəniş sistemi: müştərilər <b>gözləyir</b>": "[Ödəniş] sistemi: [müştərilər] <b>gözləyir</b>",
		"Ödənişdən sonra": "[Ödənişdən] sonra", // prefix, like the tsquery
		"yenidənödəniş":   "yenidənödəniş",     // not a word start
		"":                "",
	} {
		if got := render(highlight(text, terms)); got != want {
			t.Errorf("highlight(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestSnippet(t *testing.T) {
	terms := searchTerms("anbar")
	long := strings.Repeat("söz ", 100) + "Anbar\nsayımı " + strings.Repeat("son ", 100)
	got := render(snippet(long, terms, true))
	if !strings.HasPrefix(got, "…söz") || !strings.Contains(got, "[Anbar] sayımı") || !strings.HasSuffix(got, "son…") {
		t.Fatalf("snippet: %q", got)
	}
	if n := len([]rune(got)); n > snippetRunes+4 {
		t.Fatalf("snippet too long: %d", n)
	}
	if snippet("heç nə", terms, true) != nil {
		t.Fatal("no match must give no description snippet")
	}
	if got := render(snippet("qısa şərh", terms, false)); got != "qısa şərh" {
		t.Fatalf("comment without a highlighted word: %q", got)
	}
}

func TestSearchCursorRoundTrip(t *testing.T) {
	c := SearchCursor{Rank: 0.0607927, ID: uuid.New()}
	got, err := DecodeSearchCursor(c.Encode())
	if err != nil || got != c {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	for _, bad := range []string{"", "x", c.Encode() + "AA", "_____________________________"} {
		if _, err := DecodeSearchCursor(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
