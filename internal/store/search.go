package store

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Workspace-wide search (ADR-0024). Documents live in search_docs (migration
// 00018), maintained by triggers; this file only reads them.

const (
	// SearchMaxQueryRunes bounds the query text the API accepts.
	SearchMaxQueryRunes = 200
	// SearchMaxLimit is the largest page size.
	SearchMaxLimit = 50
	// searchMaxTerms bounds the tsquery: every term is a GIN lookup.
	searchMaxTerms = 8
	// searchTimeout caps one search; a pathological query fails fast instead
	// of holding a connection.
	searchTimeout = "3s"
	// searchFuzzyThreshold is the pg_trgm word similarity a typo must reach.
	searchFuzzyThreshold = "0.5"
	snippetRunes         = 180
)

// itemKeyRe matches an exact item reference such as QA-12 (case-insensitive).
var itemKeyRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-([1-9][0-9]{0,17})$`)

// SearchParams is one search request.
type SearchParams struct {
	WorkspaceID uuid.UUID
	Viewer      Viewer
	Query       string
	Limit       int
	Cursor      *SearchCursor // nil: first page
}

// SearchCursor continues after the last hit of the previous page.
type SearchCursor struct {
	Rank float32
	ID   uuid.UUID
}

// Encode returns the opaque cursor string.
func (c SearchCursor) Encode() string {
	var b [20]byte
	binary.BigEndian.PutUint32(b[:4], math.Float32bits(c.Rank))
	copy(b[4:], c.ID[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// DecodeSearchCursor parses a cursor from Encode.
func DecodeSearchCursor(s string) (SearchCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) != 20 {
		return SearchCursor{}, errors.New("invalid cursor")
	}
	r := math.Float32frombits(binary.BigEndian.Uint32(b[:4]))
	if math.IsNaN(float64(r)) || math.IsInf(float64(r), 0) || r < 0 {
		return SearchCursor{}, errors.New("invalid cursor")
	}
	id, _ := uuid.FromBytes(b[4:])
	return SearchCursor{Rank: r, ID: id}, nil
}

// Segment is a piece of highlighted text. Text is plain (never HTML): the
// client renders it as text, so no markup ever crosses the API.
type Segment struct {
	Text  string `json:"text"`
	Match bool   `json:"match,omitempty"`
}

// SearchHit is one result: an item, or a comment on an item.
type SearchHit struct {
	Kind           string         `json:"kind"`            // "item" or "comment"
	Match          string         `json:"match"`           // "key" (exact QA-12), "text" or "fuzzy" (typo-tolerant title match)
	ProjectKey     string         `json:"project_key"`     //
	ProjectName    string         `json:"project_name"`    //
	ItemNumber     int64          `json:"item_number"`     //
	ItemKey        string         `json:"item_key"`        // QA-12
	ItemTitle      string         `json:"item_title"`      // plain title
	StatusName     string         `json:"status_name"`     //
	StatusCategory string         `json:"status_category"` //
	Title          []Segment      `json:"title"`           // highlighted title
	Snippet        []Segment      `json:"snippet"`         // description or comment excerpt; empty when nothing matched there
	CommentID      *uuid.UUID     `json:"comment_id"`      //
	Author         *CommentAuthor `json:"author"`          // comments only
	CreatedAt      *time.Time     `json:"created_at"`      // comments only
}

// SearchCounts are the numbers of matches per kind, on the first page only.
type SearchCounts struct {
	Items    int64 `json:"items"`
	Comments int64 `json:"comments"`
}

// SearchResult is one page.
type SearchResult struct {
	Hits   []SearchHit
	Next   *SearchCursor
	Counts *SearchCounts // nil after the first page
}

// searchTerms splits a query into folded words for the tsquery and for
// highlighting. Only letters and digits survive, so the terms can be placed
// into tsquery syntax without escaping. Single letters are dropped (as a
// prefix they would match most of the index).
func searchTerms(q string) []string {
	var out []string
	for w := range strings.FieldsFuncSeq(q, func(r rune) bool { return !isWordRune(r) }) {
		w = searchFold(w)
		w = strings.Map(func(r rune) rune {
			if isWordRune(r) && !isMark(r) {
				return r
			}
			return -1
		}, w)
		if utf8.RuneCountInString(w) < 2 && !isDigits(w) || w == "" {
			continue
		}
		dup := false
		for _, o := range out {
			dup = dup || o == w
		}
		if !dup {
			out = append(out, w)
		}
		if len(out) == searchMaxTerms {
			break
		}
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// tsQuery builds "t1:* & t2:*": every word must appear, as a prefix, so
// results update while the user is still typing.
func tsQuery(terms []string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = t + ":*"
	}
	return strings.Join(parts, " & ")
}

// ValidateSearchQuery normalizes q and reports whether it can be searched.
func ValidateSearchQuery(q string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", invalid("q", "is required")
	}
	if !utf8.ValidString(q) || utf8.RuneCountInString(q) > SearchMaxQueryRunes {
		return "", invalid("q", "must be at most %d characters", SearchMaxQueryRunes)
	}
	if !itemKeyRe.MatchString(q) && len(searchTerms(q)) == 0 {
		return "", invalid("q", "needs a word of at least 2 letters or digits")
	}
	return q, nil
}

type searchCandidate struct {
	id   uuid.UUID
	rank float32
	kind string
}

// Search returns one page of ranked hits across the projects the viewer may
// see. Private projects the viewer cannot see never reach a query: visibility
// is resolved first and every statement is restricted to those project ids.
func (s *Store) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	q, err := ValidateSearchQuery(p.Query)
	if err != nil {
		return SearchResult{}, err
	}
	terms := searchTerms(q)
	res := SearchResult{Hits: []SearchHit{}}
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('statement_timeout', $1, true),
			set_config('pg_trgm.word_similarity_threshold', $2, true)`, searchTimeout, searchFuzzyThreshold); err != nil {
			return err
		}
		visible, err := visibleProjectIDs(ctx, tx, p.WorkspaceID, p.Viewer)
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			if p.Cursor == nil {
				res.Counts = &SearchCounts{}
			}
			return nil
		}

		var exact *searchCandidate
		if p.Cursor == nil {
			if exact, err = exactKeyHit(ctx, tx, p.WorkspaceID, visible, q); err != nil {
				return err
			}
		}
		var cands []searchCandidate
		match := "text"
		if len(terms) > 0 {
			cands, res.Counts, err = ftsPage(ctx, tx, p, visible, tsQuery(terms), exact)
			if err != nil {
				return err
			}
			if p.Cursor == nil && len(cands) == 0 && exact == nil {
				match = "fuzzy"
				if cands, err = fuzzyPage(ctx, tx, p, visible, strings.Join(terms, " ")); err != nil {
					return err
				}
				res.Counts = &SearchCounts{Items: int64(len(cands))}
			}
		}
		if p.Cursor == nil && res.Counts == nil {
			res.Counts = &SearchCounts{}
		}
		if len(cands) > p.Limit {
			cands = cands[:p.Limit]
			last := cands[len(cands)-1]
			res.Next = &SearchCursor{Rank: last.rank, ID: last.id}
		}
		ordered := cands
		if exact != nil {
			ordered = append([]searchCandidate{*exact}, cands...)
			if res.Counts != nil {
				res.Counts.Items++ // excluded from the text matches above
			}
		}
		hits, err := hydrateHits(ctx, tx, p.WorkspaceID, visible, ordered, terms)
		if err != nil {
			return err
		}
		for i := range hits {
			switch {
			case exact != nil && i == 0 && hits[i].Kind == "item" && strings.EqualFold(hits[i].ItemKey, q):
				hits[i].Match = "key"
			default:
				hits[i].Match = match
			}
		}
		res.Hits = hits
		return nil
	})
	return res, err
}

// visibleProjectIDs is the access predicate of ADR-0013 for the viewer.
func visibleProjectIDs(ctx context.Context, q querier, wsID uuid.UUID, v Viewer) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT id FROM projects WHERE workspace_id = $1 AND `+visibleToViewer,
		wsID, v.UserID, RoleAtLeast(v.Role, RoleAdmin))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// exactKeyHit finds the item QA-12 when the whole query is an item key.
func exactKeyHit(ctx context.Context, q querier, wsID uuid.UUID, visible []uuid.UUID, query string) (*searchCandidate, error) {
	m := itemKeyRe.FindStringSubmatch(query)
	if m == nil {
		return nil, nil
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return nil, nil //nolint:nilerr // out of range: simply no such item
	}
	var id uuid.UUID
	err = q.QueryRow(ctx, `SELECT i.id FROM items i JOIN projects p ON p.id = i.project_id
		WHERE i.workspace_id = $1 AND p.workspace_id = $1 AND p.id = ANY($2) AND p.key = $3 AND i.number = $4 AND i.deleted_at IS NULL`,
		wsID, visible, strings.ToUpper(m[1]), n).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &searchCandidate{id: id, kind: "item"}, nil
}

// ftsPage ranks full-text matches. Weights: title A (1.0), description B
// (0.4), comment C (0.2), so an item whose title matches outranks a comment
// that mentions the word. Normalization 1 divides by 1 + log(length), so long
// texts do not win by repetition. Counts come from the same scan (window
// aggregates over every match, before LIMIT).
func ftsPage(ctx context.Context, q querier, p SearchParams, visible []uuid.UUID, tsq string, exact *searchCandidate) ([]searchCandidate, *SearchCounts, error) {
	exclude := uuid.Nil
	if exact != nil {
		exclude = exact.id
	}
	args := []any{p.WorkspaceID, visible, tsq, exclude}
	seek := ""
	countCols := "count(*) FILTER (WHERE m.kind = 'item') OVER (), count(*) FILTER (WHERE m.kind = 'comment') OVER ()"
	if p.Cursor != nil {
		args = append(args, p.Cursor.Rank, p.Cursor.ID)
		seek = "WHERE (m.rank < $5::real OR (m.rank = $5::real AND m.id > $6))"
		countCols = "0::bigint, 0::bigint" // counts are a first-page thing
	}
	args = append(args, p.Limit+1)
	rows, err := q.Query(ctx, `WITH m AS (
			SELECT d.id, d.kind, ts_rank(d.doc, query, 1) AS rank
			FROM search_docs d, to_tsquery('simple', search_fold($3)) query
			WHERE d.workspace_id = $1 AND d.project_id = ANY($2) AND d.doc @@ query AND d.id <> $4
		)
		SELECT m.id, m.kind, m.rank, `+countCols+`
		FROM m `+seek+`
		ORDER BY m.rank DESC, m.id
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []searchCandidate
	counts := &SearchCounts{}
	for rows.Next() {
		var c searchCandidate
		if err := rows.Scan(&c.id, &c.kind, &c.rank, &counts.Items, &counts.Comments); err != nil {
			return nil, nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if p.Cursor != nil {
		counts = nil // after the seek they would count only the rest
	}
	return out, counts, nil
}

// fuzzyPage is the typo fallback: titles whose words are similar to the
// query (pg_trgm word similarity), when full-text search found nothing. One
// page only: the trigram index serves it, but it is not worth paging.
func fuzzyPage(ctx context.Context, q querier, p SearchParams, visible []uuid.UUID, text string) ([]searchCandidate, error) {
	rows, err := q.Query(ctx, `SELECT d.id, word_similarity(search_fold($3), d.title_fold) AS rank
		FROM search_docs d
		WHERE d.kind = 'item' AND d.workspace_id = $1 AND d.project_id = ANY($2) AND search_fold($3) <% d.title_fold
		ORDER BY rank DESC, d.id
		LIMIT $4`, p.WorkspaceID, visible, text, p.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []searchCandidate
	for rows.Next() {
		c := searchCandidate{kind: "item"}
		if err := rows.Scan(&c.id, &c.rank); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// hydrateHits loads what the page shows. It re-checks, from the items
// table itself, that each item is live and in a visible project, so a stale
// search document can never reveal anything.
func hydrateHits(ctx context.Context, q querier, wsID uuid.UUID, visible []uuid.UUID, cands []searchCandidate, terms []string) ([]SearchHit, error) {
	hits := []SearchHit{}
	if len(cands) == 0 {
		return hits, nil
	}
	ids := make([]uuid.UUID, len(cands))
	for i, c := range cands {
		ids[i] = c.id
	}
	rows, err := q.Query(ctx, `SELECT d.kind, p.key, p.name, i.number, i.title, i.description, s.name, s.category,
			c.id, c.body, c.created_at, u.id, u.display_name
		FROM unnest($3::uuid[]) WITH ORDINALITY AS x(id, ord)
		JOIN search_docs d ON d.id = x.id
		JOIN items i ON i.id = d.item_id AND i.workspace_id = $1 AND i.deleted_at IS NULL
		JOIN projects p ON p.id = i.project_id AND p.workspace_id = $1 AND p.id = ANY($2)
		JOIN statuses s ON s.id = i.status_id
		LEFT JOIN comments c ON c.id = d.comment_id AND c.item_id = i.id AND c.deleted_at IS NULL
		LEFT JOIN users u ON u.id = c.author_id
		WHERE d.kind = 'item' OR c.id IS NOT NULL
		ORDER BY x.ord`, wsID, visible, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mentioned := map[uuid.UUID]bool{}
	for rows.Next() {
		var h SearchHit
		var desc, body, authorName *string
		var authorID *uuid.UUID
		if err := rows.Scan(&h.Kind, &h.ProjectKey, &h.ProjectName, &h.ItemNumber, &h.ItemTitle, &desc,
			&h.StatusName, &h.StatusCategory, &h.CommentID, &body, &h.CreatedAt, &authorID, &authorName); err != nil {
			return nil, err
		}
		h.ItemKey = h.ProjectKey + "-" + strconv.FormatInt(h.ItemNumber, 10)
		h.Title = highlight(h.ItemTitle, terms)
		if h.Kind == "comment" && body != nil {
			for _, m := range MentionRe.FindAllStringSubmatch(*body, -1) {
				if id, err := uuid.Parse(m[1]); err == nil {
					mentioned[id] = true
				}
			}
			h.Snippet = []Segment{{Text: *body}} // excerpted below, after mentions are resolved
			if authorID != nil && authorName != nil {
				h.Author = &CommentAuthor{ID: *authorID, DisplayName: *authorName}
			}
		} else {
			h.CreatedAt = nil
			if desc != nil {
				h.Snippet = snippet(*desc, terms, true)
			}
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names, err := mentionNames(ctx, q, wsID, mentioned)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		if hits[i].Kind != "comment" || len(hits[i].Snippet) == 0 {
			continue
		}
		body := MentionRe.ReplaceAllStringFunc(hits[i].Snippet[0].Text, func(tok string) string {
			id, _ := uuid.Parse(MentionRe.FindStringSubmatch(tok)[1])
			if n, ok := names[id]; ok {
				return "@" + n
			}
			return "@?"
		})
		hits[i].Snippet = snippet(body, terms, false)
	}
	return hits, nil
}

// mentionNames resolves mentioned users, limited to members of the
// workspace (a mention never reveals who exists elsewhere).
func mentionNames(ctx context.Context, q querier, wsID uuid.UUID, ids map[uuid.UUID]bool) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	list := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	rows, err := q.Query(ctx, `SELECT u.id, u.display_name FROM users u
		JOIN workspace_members m ON m.user_id = u.id AND m.workspace_id = $1 WHERE u.id = ANY($2)`, wsID, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}
