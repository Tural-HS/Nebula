package store

import (
	"strings"
	"unicode"
)

// searchFolder mirrors the SQL function search_fold() (migration 00018):
// Azerbaijani letters and common Latin accents fold to ASCII, stray
// combining marks are dropped. The database does the matching; this copy is
// only used to find the matched words again for highlighting, and
// TestSearchFoldMatchesSQL keeps the two in sync.
var searchFolder = strings.NewReplacer(
	"Ə", "e", "ə", "e", "İ", "i", "I", "i", "ı", "i", "Ö", "o", "ö", "o", "Ü", "u",
	"ü", "u", "Ğ", "g", "ğ", "g", "Ş", "s", "ş", "s", "Ç", "c", "ç", "c", "Á", "a",
	"á", "a", "À", "a", "à", "a", "Â", "a", "â", "a", "Ä", "a", "ä", "a", "É", "e",
	"é", "e", "È", "e", "è", "e", "Ê", "e", "ê", "e", "Ë", "e", "ë", "e", "Í", "i",
	"í", "i", "Ì", "i", "ì", "i", "Î", "i", "î", "i", "Ï", "i", "ï", "i", "Ó", "o",
	"ó", "o", "Ò", "o", "ò", "o", "Ô", "o", "ô", "o", "Õ", "o", "õ", "o", "Ú", "u",
	"ú", "u", "Ù", "u", "ù", "u", "Û", "u", "û", "u", "Ñ", "n", "ñ", "n", "\u0300", "",
	"\u0301", "", "\u0302", "", "\u0303", "", "\u0306", "", "\u0307", "", "\u0308", "", "\u0327", "",
)

// searchFold lowercases and folds s the way the search index does.
func searchFold(s string) string {
	return strings.ToLower(searchFolder.Replace(s))
}

// isWordRune reports whether r belongs to a word: the index's 'simple'
// parser splits on everything else (spaces, punctuation, symbols).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

func isMark(r rune) bool { return unicode.Is(unicode.Mn, r) }

// foldWord is a word as the index stores it: folded, without marks.
func foldWord(w string) string {
	return strings.Map(func(r rune) rune {
		if isMark(r) {
			return -1
		}
		return r
	}, searchFold(w))
}

// highlight splits text into segments, marking words that start with one of
// the (folded) terms: the same prefix rule as the tsquery.
func highlight(text string, terms []string) []Segment {
	var out []Segment
	add := func(s string, match bool) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Match == match {
			out[n-1].Text += s
			return
		}
		out = append(out, Segment{Text: s, Match: match})
	}
	rest := text
	for rest != "" {
		i := strings.IndexFunc(rest, isWordRune)
		if i < 0 {
			add(rest, false)
			break
		}
		add(rest[:i], false)
		rest = rest[i:]
		j := strings.IndexFunc(rest, func(r rune) bool { return !isWordRune(r) })
		if j < 0 {
			j = len(rest)
		}
		word := rest[:j]
		add(word, wordMatches(word, terms))
		rest = rest[j:]
	}
	if out == nil {
		out = []Segment{}
	}
	return out
}

func wordMatches(word string, terms []string) bool {
	f := foldWord(word)
	for _, t := range terms {
		if strings.HasPrefix(f, t) {
			return true
		}
	}
	return false
}

// snippet returns an excerpt of about snippetRunes runes around the first
// matching word, highlighted. Without a match it returns nil when
// onlyIfMatch is set, else the start of the text.
func snippet(text string, terms []string, onlyIfMatch bool) []Segment {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) == 0 {
		return nil
	}
	first := -1
	for i := 0; i < len(r); {
		if !isWordRune(r[i]) {
			i++
			continue
		}
		j := i
		for j < len(r) && isWordRune(r[j]) {
			j++
		}
		if wordMatches(string(r[i:j]), terms) {
			first = i
			break
		}
		i = j
	}
	if first < 0 {
		if onlyIfMatch {
			return nil
		}
		first = 0
	}
	start := max(0, first-snippetRunes/4)
	if start > 0 { // begin at a word boundary
		for start < first && r[start-1] != ' ' {
			start++
		}
	}
	end := min(len(r), start+snippetRunes)
	if end < len(r) { // end at a word boundary when there is one
		for k := end; k > first; k-- {
			if r[k-1] == ' ' {
				end = k - 1
				break
			}
		}
	}
	excerpt := string(r[start:end])
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(r) {
		excerpt += "…"
	}
	return highlight(excerpt, terms)
}
