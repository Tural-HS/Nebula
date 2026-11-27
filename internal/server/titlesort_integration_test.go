package server

import (
	"net/url"
	"slices"
	"testing"
)

// ADR-0063: title sorts follow the Azerbaijani alphabet and page exactly.
func TestTitleSort(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	for _, title := range []string{"Şahin", "Əli", "Samir", "Cavid", "Elmar", "Çingiz", "Cavid"} {
		e.must(201, "alice", "POST", items, map[string]any{"title": title})
	}
	collect := func(sort string) []string {
		var out []string
		cursor := ""
		for range 10 {
			q := url.Values{"sort": {sort}, "limit": {"2"}}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			body := e.must(200, "alice", "GET", items+"?"+q.Encode(), nil).Body
			for _, it := range body["items"].([]any) {
				out = append(out, it.(map[string]any)["title"].(string))
			}
			next, _ := body["next_cursor"].(string)
			if next == "" {
				return out
			}
			cursor = next
		}
		t.Fatal("too many pages")
		return nil
	}
	want := []string{"Cavid", "Cavid", "Çingiz", "Elmar", "Əli", "Samir", "Şahin"}
	if got := collect("title"); !slices.Equal(got, want) {
		t.Fatalf("title: %v", got)
	}
	slices.Reverse(want)
	if got := collect("-title"); !slices.Equal(got, want) {
		t.Fatalf("-title: %v", got)
	}
}
