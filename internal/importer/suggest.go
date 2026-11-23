package importer

import (
	"strings"
	"unicode"
)

// Target is something a column can be mapped to, with the header names that
// suggest it. Names are tried for every target before any Synonyms, so a
// custom field literally named "Ad" wins over the built-in "ad" → title synonym.
type Target struct {
	ID       string
	Names    []string
	Synonyms []string
}

// NormalizeHeader folds a header for comparison and drops everything that is
// not a letter or digit ("Müqavilə №" → "müqavilə", "start_date" → "startdate").
func NormalizeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, Fold(s))
}

// Suggest maps column indexes to target IDs by header name. Each target and
// each column is used at most once; the leftmost matching column wins.
func Suggest(headers []string, targets []Target) map[int]string {
	norm := make([]string, len(headers))
	for i, h := range headers {
		norm[i] = NormalizeHeader(h)
	}
	out := map[int]string{}
	used := map[string]bool{}
	pass := func(names func(Target) []string) {
		for _, t := range targets {
			if used[t.ID] {
				continue
			}
			want := map[string]bool{}
			for _, n := range names(t) {
				if n = NormalizeHeader(n); n != "" {
					want[n] = true
				}
			}
			for i, h := range norm {
				if _, taken := out[i]; !taken && h != "" && want[h] {
					out[i] = t.ID
					used[t.ID] = true
					break
				}
			}
		}
	}
	pass(func(t Target) []string { return t.Names })
	pass(func(t Target) []string { return t.Synonyms })
	return out
}
