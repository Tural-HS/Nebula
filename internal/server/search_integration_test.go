package server

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const searchPath = "/api/v1/workspaces/acme/search"

type searchHit struct {
	Kind, Match, ItemKey, Title, Snippet string
	Marked                               []string // highlighted words in title + snippet
}

type searchPage struct {
	Hits     []searchHit
	Next     string
	Items    float64
	Comments float64
	Counted  bool
}

func joinSegments(v any) (text string, marked []string) {
	segs, _ := v.([]any)
	var b strings.Builder
	for _, s := range segs {
		m := s.(map[string]any)
		b.WriteString(m["text"].(string))
		if m["match"] == true {
			marked = append(marked, m["text"].(string))
		}
	}
	return b.String(), marked
}

func (e *env) search(user, q string, extra ...string) searchPage {
	e.t.Helper()
	path := searchPath + "?q=" + url.QueryEscape(q)
	for _, x := range extra {
		path += "&" + x
	}
	body := e.must(200, user, "GET", path, nil).Body
	var p searchPage
	for _, h := range body["hits"].([]any) {
		m := h.(map[string]any)
		title, tm := joinSegments(m["title"])
		snip, sm := joinSegments(m["snippet"])
		p.Hits = append(p.Hits, searchHit{Kind: m["kind"].(string), Match: m["match"].(string), ItemKey: m["item_key"].(string),
			Title: title, Snippet: snip, Marked: append(tm, sm...)})
	}
	p.Next, _ = body["next_cursor"].(string)
	if c, ok := body["counts"].(map[string]any); ok {
		p.Counted, p.Items, p.Comments = true, c["items"].(float64), c["comments"].(float64)
	}
	return p
}

func (p searchPage) keys() string {
	var out []string
	for _, h := range p.Hits {
		out = append(out, h.Kind+":"+h.ItemKey)
	}
	return strings.Join(out, ",")
}

func TestSearchAzerbaijaniFolding(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "Ödəniş sistemi işləmir", "description": "Müştəri kartla ödəyə bilmir."})
	e.must(201, "alice", "POST", items, map[string]any{"title": "odenis hesabati", "description": "ASCII ilə yazılıb"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Giriş səhifəsi"})
	e.must(201, "carol", "POST", items+"/3/comments", map[string]any{"body": "Ödənişdən sonra yönləndirmə yoxdur, " + mention(e.userID("alice"))})

	// Folded both ways: ə/ö/ş in the text or in the query, any case.
	for _, q := range []string{"odenis", "ödəniş", "ÖDƏNİŞ", "Odənis", "ödən"} {
		got := e.search("bob", q) // bob is a guest: guests read workspace projects
		if got.keys() != "item:ACME-1,item:ACME-2,comment:ACME-3" && got.keys() != "item:ACME-2,item:ACME-1,comment:ACME-3" {
			t.Errorf("%q: %s", q, got.keys())
		}
		if got.Items != 2 || got.Comments != 1 {
			t.Errorf("%q counts: %v/%v", q, got.Items, got.Comments)
		}
	}
	// Every word must match (AND), as a prefix.
	if got := e.search("bob", "ödəniş müştəri"); got.keys() != "item:ACME-1" {
		t.Errorf("two words: %s", got.keys())
	}
	if got := e.search("bob", "musteri kart"); got.keys() != "item:ACME-1" {
		t.Errorf("description words: %s", got.keys())
	}
	// ı/i, ğ/g, ç/c.
	e.must(201, "alice", "POST", items, map[string]any{"title": "Ağac çıxışı"})
	if got := e.search("bob", "agac cixisi"); got.keys() != "item:ACME-4" {
		t.Errorf("ğ/ç/ı: %s", got.keys())
	}

	// Highlighting marks the original words, in title and snippet.
	got := e.search("bob", "odenis musteri")
	if len(got.Hits) != 1 || got.Hits[0].Title != "Ödəniş sistemi işləmir" || strings.Join(got.Hits[0].Marked, "|") != "Ödəniş|Müştəri" {
		t.Fatalf("highlight: %+v", got.Hits)
	}
	// Comment snippets show mentions by name, never the raw token.
	c := e.search("bob", "yönləndirmə").Hits
	if len(c) != 1 || c[0].Kind != "comment" || !strings.Contains(c[0].Snippet, "@Alice") || strings.Contains(c[0].Snippet, "<@") {
		t.Fatalf("comment snippet: %+v", c)
	}
}

func TestSearchRankingKeyAndPaging(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "Server yenilənməsi", "description": "Sonra hesabat göndər"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Hesabat aylıq"})
	e.must(201, "alice", "POST", items+"/1/comments", map[string]any{"body": "hesabat hazırdır"})

	// Title (weight A) beats description (B) beats comment (C).
	if got := e.search("carol", "hesabat"); got.keys() != "item:ACME-2,item:ACME-1,comment:ACME-1" {
		t.Fatalf("ranking: %s", got.keys())
	}

	// An exact key comes first, with its comments still found by text.
	got := e.search("carol", "acme-1")
	if len(got.Hits) == 0 || got.Hits[0].ItemKey != "ACME-1" || got.Hits[0].Match != "key" {
		t.Fatalf("key match: %+v", got.Hits)
	}
	for _, h := range got.Hits[1:] {
		if h.Kind == "item" && h.ItemKey == "ACME-1" {
			t.Fatalf("key hit repeated: %s", got.keys())
		}
	}
	if e.search("carol", "ACME-99").keys() != "" {
		t.Fatal("missing key must find nothing")
	}

	// Keyset paging: every hit exactly once, counts only on the first page.
	for i := range 23 {
		e.must(201, "carol", "POST", items, map[string]any{"title": fmt.Sprintf("Planlama mərhələsi %d", i)})
	}
	seen := map[string]bool{}
	cursor, pages := "", 0
	for {
		extra := []string{"limit=10"}
		if cursor != "" {
			extra = append(extra, "cursor="+url.QueryEscape(cursor))
		}
		p := e.search("carol", "planlama", extra...)
		if pages == 0 && (!p.Counted || p.Items != 23) {
			t.Fatalf("first page counts: %+v", p)
		}
		if pages > 0 && p.Counted {
			t.Fatal("counts after the first page")
		}
		for _, h := range p.Hits {
			if seen[h.ItemKey] {
				t.Fatalf("duplicate %s on page %d", h.ItemKey, pages)
			}
			seen[h.ItemKey] = true
		}
		pages++
		if cursor = p.Next; cursor == "" {
			break
		}
	}
	if len(seen) != 23 || pages != 3 {
		t.Fatalf("paged %d hits in %d pages", len(seen), pages)
	}

	// Typo tolerance: no full-text match, so similar titles are offered.
	got = e.search("carol", "plenlama")
	if len(got.Hits) == 0 || got.Hits[0].Match != "fuzzy" || !strings.HasPrefix(got.Hits[0].Title, "Planlama") {
		t.Fatalf("fuzzy: %+v", got.Hits)
	}
	// Every word must match, also in the fallback: a word that appears in no
	// title is not outweighed by another word that does.
	if got = e.search("carol", "gizli merhelesi"); len(got.Hits) != 0 {
		t.Fatalf("fuzzy with an unmatched word: %+v", got.Hits)
	}
	if got = e.search("carol", "plenlama"); len(got.Hits) == 0 {
		t.Fatal("single-word typo stopped matching")
	}
}

func TestSearchFollowsEditsAndDeletes(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "Köhnə ad"})
	c := e.must(201, "alice", "POST", items+"/1/comments", map[string]any{"body": "birinci qeyd"}).Body
	commentPath := items + "/1/comments/" + c["id"].(string)

	e.must(200, "alice", "PATCH", items+"/1", map[string]any{"version": 1, "title": "Təzə ad", "description": "Anbar sayımı"})
	if e.search("carol", "kohne").keys() != "" || e.search("carol", "teze anbar").keys() != "item:ACME-1" {
		t.Fatal("title/description edit not indexed")
	}
	e.must(200, "alice", "PATCH", commentPath, map[string]any{"body": "ikinci qeyd"})
	if e.search("carol", "birinci").keys() != "" || e.search("carol", "ikinci").keys() != "comment:ACME-1" {
		t.Fatal("comment edit not indexed")
	}
	e.must(204, "alice", "DELETE", commentPath, nil)
	if e.search("carol", "ikinci").keys() != "" {
		t.Fatal("deleted comment still found")
	}
	e.must(201, "alice", "POST", items+"/1/comments", map[string]any{"body": "üçüncü qeyd"})
	e.must(204, "alice", "DELETE", items+"/1", nil)
	for _, q := range []string{"teze", "ucuncu", "ACME-1"} {
		if got := e.search("carol", q); got.keys() != "" || got.Items+got.Comments != 0 {
			t.Fatalf("deleted item still found by %q: %s", q, got.keys())
		}
	}
}

// Private projects (ADR-0013) must not leak through search in any form: hits,
// counts, exact keys, typo matches or mention names.
func TestSearchPrivacy(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member, dave outsider
	e.must(201, "alice", "POST", projectsPath, map[string]any{"key": "SEC", "name": "Hüquq", "visibility": "private"})
	e.must(201, "alice", "POST", secItems, map[string]any{"title": "Gizli ödəniş müqaviləsi", "description": "məxfi məbləğ"})
	e.must(201, "alice", "POST", secItems+"/1/comments", map[string]any{"body": "ödəniş sirri"})
	e.must(201, "alice", "POST", items, map[string]any{"title": "Açıq ödəniş"})

	for _, user := range []string{"bob", "carol"} {
		for _, q := range []string{"odenis", "ödəniş", "gizli", "mexfi", "sirri", "muqavile"} {
			got := e.search(user, q)
			for _, h := range got.Hits {
				if strings.HasPrefix(h.ItemKey, "SEC-") {
					t.Fatalf("%s sees %s via %q", user, h.ItemKey, q)
				}
			}
			if q == "odenis" && (got.keys() != "item:ACME-1" || got.Items != 1 || got.Comments != 0) {
				t.Fatalf("%s %q: %s counts %v/%v", user, q, got.keys(), got.Items, got.Comments)
			}
			if q != "odenis" && q != "ödəniş" && (got.Items != 0 || got.Comments != 0) {
				t.Fatalf("%s %q: counts leak %v/%v", user, q, got.Items, got.Comments)
			}
		}
		if got := e.search(user, "SEC-1"); got.keys() != "" {
			t.Fatalf("%s finds SEC-1 by key", user)
		}
		if got := e.search(user, "gizly"); got.keys() != "" { // typo path
			t.Fatalf("%s finds SEC by typo: %s", user, got.keys())
		}
	}
	if got := e.search("alice", "odenis"); got.Items != 2 || got.Comments != 1 {
		t.Fatalf("owner counts: %v/%v (%s)", got.Items, got.Comments, got.keys())
	}
	if got := e.search("alice", "gizly"); len(got.Hits) == 0 || got.Hits[0].ItemKey != "SEC-1" {
		t.Fatalf("owner typo match: %s", got.keys())
	}
	if r := e.call("dave", "GET", searchPath+"?q=odenis", nil); r.Status != 404 {
		t.Fatalf("outsider: %d", r.Status)
	}

	// Granting project access makes it searchable; revoking hides it again.
	carol := e.userID("carol")
	e.must(204, "alice", "PUT", secPath+"/members/"+carol, nil)
	if got := e.search("carol", "gizli"); got.keys() != "item:SEC-1" {
		t.Fatalf("project member: %s", got.keys())
	}
	e.must(204, "alice", "DELETE", secPath+"/members/"+carol, nil)
	if got := e.search("carol", "gizli"); got.keys() != "" {
		t.Fatalf("after removal: %s", got.keys())
	}
	// Making the project workspace-visible opens it to everyone.
	e.must(200, "alice", "PATCH", secPath, map[string]any{"visibility": "workspace"})
	if got := e.search("bob", "gizli"); got.keys() != "item:SEC-1" {
		t.Fatalf("workspace-visible: %s", got.keys())
	}
}

func TestSearchValidationAndLimits(t *testing.T) {
	e := newP1Env(t)
	e.setup()
	e.must(201, "alice", "POST", items, map[string]any{"title": "<script>alert(1)</script> hesab"})

	for q, want := range map[string]int{
		"":                       422,
		"   ":                    422,
		"a":                      422,
		"!!":                     422,
		strings.Repeat("ə", 201): 422,
		strings.Repeat("ə", 200): 200,
	} {
		if r := e.call("carol", "GET", searchPath+"?q="+url.QueryEscape(q), nil); r.Status != want {
			t.Errorf("q=%.20q: %d, want %d", q, r.Status, want)
		}
	}
	for _, qs := range []string{"q=hesab&limit=0", "q=hesab&limit=51", "q=hesab&cursor=nope", "q=hesab&q=x", "q=%zz"} {
		if r := e.call("carol", "GET", searchPath+"?"+qs, nil); r.Status != 400 {
			t.Errorf("%s: %d", qs, r.Status)
		}
	}
	// Text is returned as plain segments: no server-made HTML at all.
	r := e.must(200, "carol", "GET", searchPath+"?q=hesab", nil)
	h := e.search("carol", "hesab").Hits
	if len(h) != 1 || h[0].Title != "<script>alert(1)</script> hesab" || strings.Join(h[0].Marked, "|") != "hesab" || strings.Contains(r.Raw, "<mark") {
		t.Fatalf("segments: %+v %s", h, r.Raw)
	}

	// The per-user search budget: a burst, then 429 with Retry-After; other
	// users are unaffected.
	limited := false
	for range searchBurst + 5 {
		if r := e.call("bob", "GET", searchPath+"?q=hesab", nil); r.Status == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("search was not rate limited")
	}
	e.must(200, "alice", "GET", searchPath+"?q=hesab", nil)
}
