package importer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Mattermost Boards (Focalboard) archives, format version 2 (ADR-0020):
//
//	version.json                {"version":2,"date":<ms>}
//	<boardID>/board.jsonl       {"type":"board"|"block"|"boardMember","data":{...}} per line
//	<boardID>/<fileID>          images and attachments (ignored here)
//
// Only board.jsonl entries are read. People appear as opaque Mattermost user
// ids; Table.ApplyPeople maps them to email addresses chosen by the admin.

// FormatBoards is a Mattermost Boards archive.
const FormatBoards Format = "boards"

const (
	boardsArchiveVersion = 2
	// The plugin caps block fields at 800k runes; allow that plus framing.
	maxBoardLineBytes = 4 << 20
	// Description = card content + old comments; the store limit is 100k.
	maxBoardDescriptionRunes = 100_000
)

// Board is one board read from an archive.
type Board struct {
	ID         string
	Title      string
	Properties []BoardProperty
	Cards      []BoardCard
}

// BoardProperty is a card property definition.
type BoardProperty struct {
	ID      string
	Name    string
	Type    string
	Options map[string]string // option id -> display value
}

// BoardCard is a card with its content and comments, in display order.
type BoardCard struct {
	Title      string
	CreatedBy  string
	CreateAt   int64
	Properties map[string][]string // property id -> raw values (ids for options and people)
	Content    []string            // Markdown lines/paragraphs
	Comments   []BoardComment
}

// BoardComment is a comment on a card.
type BoardComment struct {
	UserID   string
	CreateAt int64
	Text     string
}

type archiveLine struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rawBoard struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	IsTemplate     bool   `json:"isTemplate"`
	DeleteAt       int64  `json:"deleteAt"`
	CardProperties []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Options []struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		} `json:"options"`
	} `json:"cardProperties"`
}

type rawBlock struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId"`
	CreatedBy string `json:"createdBy"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Fields    struct {
		Properties   map[string]json.RawMessage `json:"properties"`
		ContentOrder []json.RawMessage          `json:"contentOrder"`
		IsTemplate   bool                       `json:"isTemplate"`
		Value        bool                       `json:"value"`
	} `json:"fields"`
	CreateAt int64 `json:"createAt"`
	DeleteAt int64 `json:"deleteAt"`
}

// IsBoardsArchive reports whether a file name looks like a Boards export.
func IsBoardsArchive(fileName string) bool {
	return strings.EqualFold(path.Ext(fileName), ".boardarchive")
}

// parseBoardsArchive reads every board in the archive and returns the one
// titled sheet (or the only/first one) as a Table.
func parseBoardsArchive(ctx context.Context, data []byte, sheet string, lim Limits) (*Table, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fail("malformed_file", "the file is not a valid Mattermost Boards archive")
	}
	if len(zr.File) > lim.MaxZipEntries {
		return nil, tooLarge("file_too_large", "the archive has too many files")
	}
	var version *zip.File
	var boardFiles []*zip.File
	var total uint64
	for _, f := range zr.File {
		switch {
		case f.Name == "version.json":
			version = f
		case path.Base(f.Name) == "board.jsonl" && strings.Count(f.Name, "/") == 1 && !strings.HasPrefix(f.Name, "/") && !strings.Contains(f.Name, ".."):
			boardFiles = append(boardFiles, f)
		default:
			continue // images and attachments are not imported
		}
		total += f.UncompressedSize64
		if total > uint64(max(lim.MaxUnzippedBytes, 0)) || total < f.UncompressedSize64 {
			return nil, tooLarge("file_too_large", "the boards are larger than %d MiB uncompressed", lim.MaxUnzippedBytes>>20)
		}
	}
	if version == nil {
		return nil, fail("malformed_file", "version.json is missing: export the board again from Mattermost Boards")
	}
	var hdr struct {
		Version int `json:"version"`
	}
	if b, err := readZipEntry(version, 4<<10); err != nil || json.Unmarshal(b, &hdr) != nil {
		return nil, fail("malformed_file", "version.json is not valid")
	}
	if hdr.Version != boardsArchiveVersion {
		return nil, fail("unsupported_format", "archive version %d is not supported (expected %d)", hdr.Version, boardsArchiveVersion)
	}
	if len(boardFiles) == 0 {
		return nil, fail("malformed_file", "the archive contains no boards")
	}

	var boards []*Board
	for _, f := range boardFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fail("malformed_file", "cannot read %s", f.Name)
		}
		b, err := readBoard(ctx, rc, lim)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		boards = append(boards, b)
	}
	slices.SortStableFunc(boards, func(a, b *Board) int { return strings.Compare(a.Title, b.Title) })

	names := make([]string, len(boards))
	chosen := boards[0]
	for i, b := range boards {
		names[i] = b.Title
		if sheet != "" && b.Title == sheet {
			chosen = b
		}
	}
	if sheet != "" && chosen.Title != sheet {
		return nil, fail("sheet_not_found", "the archive has no board titled %q", sheet)
	}
	t := &Table{Format: FormatBoards, Sheets: names, Sheet: chosen.Title, Board: chosen}
	if err := t.ApplyPeople(nil, lim); err != nil {
		return nil, err
	}
	return t, nil
}

func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = errors.New("too large")
	}
	return b, err
}

// readBoard parses one board.jsonl stream.
func readBoard(ctx context.Context, r io.Reader, lim Limits) (*Board, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	var board *Board
	cards := map[string]*rawBlock{}
	var cardOrder []string
	children := map[string][]*rawBlock{} // card id -> content and comments
	blocks := 0
	for line := 1; ; line++ {
		if line%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		raw, err := readLine(br, maxBoardLineBytes)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fail("malformed_file", "board.jsonl line %d: %v", line, err)
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var al archiveLine
		if err := json.Unmarshal(raw, &al); err != nil {
			return nil, fail("malformed_file", "board.jsonl line %d is not valid JSON", line)
		}
		switch al.Type {
		case "board":
			var rb rawBoard
			if err := json.Unmarshal(al.Data, &rb); err != nil || board != nil {
				return nil, fail("malformed_file", "board.jsonl line %d: invalid or repeated board", line)
			}
			if rb.IsTemplate {
				return nil, fail("unsupported_format", "%q is a board template, not a board", rb.Title)
			}
			board = &Board{ID: rb.ID, Title: clean(rb.Title, 200)}
			if board.Title == "" {
				board.Title = "Board"
			}
			for _, p := range rb.CardProperties {
				bp := BoardProperty{ID: p.ID, Name: clean(p.Name, 200), Type: p.Type, Options: map[string]string{}}
				for _, o := range p.Options {
					bp.Options[o.ID] = clean(o.Value, 500)
				}
				board.Properties = append(board.Properties, bp)
			}
		case "block":
			blocks++
			if blocks > lim.MaxRows*20 {
				return nil, tooLarge("too_many_rows", "the board has too many blocks")
			}
			var b rawBlock
			if err := json.Unmarshal(al.Data, &b); err != nil {
				return nil, fail("malformed_file", "board.jsonl line %d: invalid block", line)
			}
			if b.DeleteAt != 0 {
				continue
			}
			switch b.Type {
			case "card":
				if b.Fields.IsTemplate {
					continue
				}
				if _, dup := cards[b.ID]; !dup {
					cardOrder = append(cardOrder, b.ID)
				}
				cards[b.ID] = &b
				if len(cards) > lim.MaxRows {
					return nil, tooLarge("too_many_rows", "the board has more than %d cards", lim.MaxRows)
				}
			case "text", "checkbox", "divider", "comment", "image", "attachment":
				children[b.ParentID] = append(children[b.ParentID], &b)
			}
		case "boardMember":
			// Membership is set up in Nebula itself (project members).
		default:
			return nil, fail("malformed_file", "board.jsonl line %d: unknown line type %q", line, al.Type)
		}
	}
	if board == nil {
		return nil, fail("malformed_file", "board.jsonl has no board line")
	}
	for _, id := range cardOrder {
		board.Cards = append(board.Cards, buildCard(cards[id], children[id], board.Properties))
	}
	// Oldest first, like the board's creation order.
	slices.SortStableFunc(board.Cards, func(a, b BoardCard) int { return int(min(max(a.CreateAt-b.CreateAt, -1), 1)) })
	return board, nil
}

// readLine reads one '\n'-terminated line of at most limit bytes.
func readLine(br *bufio.Reader, limit int) ([]byte, error) {
	var out []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) && len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if len(out)+len(chunk) > limit {
			return nil, fmt.Errorf("line longer than %d MiB", limit>>20)
		}
		out = append(out, chunk...)
		if !isPrefix {
			return out, nil
		}
	}
}

func buildCard(c *rawBlock, kids []*rawBlock, props []BoardProperty) BoardCard {
	card := BoardCard{Title: clean(c.Title, 500), CreatedBy: c.CreatedBy, CreateAt: c.CreateAt, Properties: map[string][]string{}}
	for _, p := range props {
		raw, ok := c.Fields.Properties[p.ID]
		if !ok {
			continue
		}
		var one string
		var many []string
		switch {
		case json.Unmarshal(raw, &one) == nil:
			if one != "" {
				card.Properties[p.ID] = []string{one}
			}
		case json.Unmarshal(raw, &many) == nil:
			card.Properties[p.ID] = slices.DeleteFunc(many, func(s string) bool { return s == "" })
		}
	}
	byID := map[string]*rawBlock{}
	for _, k := range kids {
		byID[k.ID] = k
	}
	used := map[string]bool{}
	for _, id := range flattenOrder(c.Fields.ContentOrder) {
		if k, ok := byID[id]; ok && !used[id] && k.Type != "comment" {
			used[id] = true
			if s := contentText(k); s != "" {
				card.Content = append(card.Content, s)
			}
		}
	}
	var comments []BoardComment
	for _, k := range kids {
		switch {
		case k.Type == "comment":
			if s := strings.TrimSpace(k.Title); s != "" {
				comments = append(comments, BoardComment{UserID: k.CreatedBy, CreateAt: k.CreateAt, Text: s})
			}
		case !used[k.ID]: // content missing from contentOrder: keep it, after the ordered part
			if s := contentText(k); s != "" {
				card.Content = append(card.Content, s)
			}
		}
	}
	slices.SortStableFunc(comments, func(a, b BoardComment) int { return int(min(max(a.CreateAt-b.CreateAt, -1), 1)) })
	card.Comments = comments
	return card
}

// flattenOrder flattens contentOrder, whose entries are ids or arrays of ids
// (blocks placed side by side).
func flattenOrder(order []json.RawMessage) []string {
	var out []string
	for _, raw := range order {
		var id string
		if json.Unmarshal(raw, &id) == nil {
			out = append(out, id)
			continue
		}
		var ids []string
		if json.Unmarshal(raw, &ids) == nil {
			out = append(out, ids...)
		}
	}
	return out
}

func contentText(b *rawBlock) string {
	s := strings.TrimSpace(b.Title)
	switch b.Type {
	case "text":
		return s
	case "checkbox":
		if b.Fields.Value {
			return "- [x] " + s
		}
		return "- [ ] " + s
	case "divider":
		return "---"
	case "image", "attachment":
		return "_(fayl Mattermost Boards-da qaldı)_"
	}
	return ""
}

// boardColumns lists the property columns that become table columns.
func boardColumns(b *Board) []BoardProperty {
	var out []BoardProperty
	for _, p := range b.Properties {
		switch p.Type {
		case "file", "updatedTime", "updatedBy", "unknown":
			continue // nothing importable
		}
		out = append(out, p)
	}
	return out
}

// People lists the Mattermost user ids that appear in person columns,
// creators and comments of the chosen board, with sample card titles so an
// admin can tell who is who (the archive has no names or emails).
func (t *Table) People() []BoardPerson {
	if t.Board == nil {
		return nil
	}
	seen := map[string]*BoardPerson{}
	counted := map[[2]string]bool{} // {person, card index}: count each card once
	var order []string
	var cardIdx int
	add := func(id, card string) {
		if id == "" || id == "system" {
			return
		}
		p, ok := seen[id]
		if !ok {
			p = &BoardPerson{ID: id}
			seen[id] = p
			order = append(order, id)
		}
		if k := [2]string{id, strconv.Itoa(cardIdx)}; !counted[k] {
			counted[k] = true
			p.Cards++
		}
		if len(p.Samples) < 3 && card != "" && !slices.Contains(p.Samples, card) {
			p.Samples = append(p.Samples, card)
		}
	}
	for i, c := range t.Board.Cards {
		cardIdx = i
		for _, p := range t.Board.Properties {
			if p.Type == "person" || p.Type == "multiPerson" {
				for _, id := range c.Properties[p.ID] {
					add(id, c.Title)
				}
			}
			if p.Type == "createdBy" {
				add(c.CreatedBy, c.Title)
			}
		}
		for _, cm := range c.Comments {
			add(cm.UserID, c.Title)
		}
	}
	out := make([]BoardPerson, 0, len(order))
	for _, id := range order {
		out = append(out, *seen[id])
	}
	slices.SortStableFunc(out, func(a, b BoardPerson) int { return b.Cards - a.Cards })
	return out
}

// BoardPerson is a Mattermost user id found in a board.
type BoardPerson struct {
	ID      string   `json:"id"`
	Cards   int      `json:"cards"`
	Samples []string `json:"samples"`
}

// ApplyPeople (re)builds the table rows of a Boards archive, writing people
// as the email the admin mapped them to (unmapped people stay empty in person
// columns and appear as "Mattermost istifadəçisi" in comments).
func (t *Table) ApplyPeople(people map[string]string, lim Limits) error {
	if t.Board == nil {
		return nil
	}
	cols := boardColumns(t.Board)
	// A card's description (content + old comments) may be longer than a
	// spreadsheet cell; the store's own limit still applies.
	lim.MaxCellRunes = max(lim.MaxCellRunes, maxBoardDescriptionRunes)
	b := &builder{ctx: context.Background(), lim: lim, table: &Table{}}
	headers := []string{"Başlıq"}
	for _, p := range cols {
		headers = append(headers, p.Name)
	}
	headers = append(headers, "Təsvir")
	if err := b.add(1, headers); err != nil {
		return err
	}
	who := func(id string) string {
		if e := people[id]; e != "" {
			return e
		}
		return ""
	}
	for i, c := range t.Board.Cards {
		cells := []string{c.Title}
		for _, p := range cols {
			cells = append(cells, propertyText(p, c, who))
		}
		cells = append(cells, cardDescription(c, who))
		if err := b.add(i+2, cells); err != nil {
			return err
		}
	}
	built, err := b.finish()
	if err != nil {
		return err
	}
	// The header row always has the title and description columns, so the
	// width is fixed even when trailing cells are blank.
	t.Headers, t.Rows = built.Headers, built.Rows
	return nil
}

func propertyText(p BoardProperty, c BoardCard, who func(string) string) string {
	vals := c.Properties[p.ID]
	switch p.Type {
	case "createdTime":
		if c.CreateAt <= 0 {
			return ""
		}
		return msDate(c.CreateAt, true)
	case "createdBy":
		return who(c.CreatedBy)
	}
	if len(vals) == 0 {
		return ""
	}
	switch p.Type {
	case "select", "multiSelect":
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			if name, ok := p.Options[v]; ok && name != "" {
				out = append(out, name)
			}
		}
		return strings.Join(out, ", ")
	case "person", "multiPerson":
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			if e := who(v); e != "" {
				out = append(out, e)
			}
		}
		return strings.Join(out, ", ")
	case "date":
		return boardDate(vals[0])
	case "checkbox":
		if vals[0] == "true" {
			return "true"
		}
		return "false"
	}
	return vals[0] // text, number, url, email, phone
}

// boardDate converts the date property's JSON ({"from":ms,"includeTime":b})
// or a bare ms number into ISO text.
func boardDate(v string) string {
	var d struct {
		From        *int64 `json:"from"`
		IncludeTime bool   `json:"includeTime"`
	}
	if err := json.Unmarshal([]byte(v), &d); err != nil || d.From == nil {
		ms, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return v // left as is: the converter reports it
		}
		d.From = &ms
	}
	// Dates before 1970 (birthdays) are negative milliseconds.
	return msDate(*d.From, d.IncludeTime)
}

func msDate(ms int64, withTime bool) string {
	t := time.UnixMilli(ms).UTC()
	if withTime {
		return t.Format(time.RFC3339)
	}
	return t.Format(time.DateOnly) // date-only values are stored as UTC midnight
}

func cardDescription(c BoardCard, who func(string) string) string {
	var sb strings.Builder
	sb.WriteString(strings.Join(c.Content, "\n\n"))
	if len(c.Comments) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("---\n**Mattermost Boards-dan köçürülmüş şərhlər:**\n")
		for _, cm := range c.Comments {
			author := who(cm.UserID)
			if author == "" {
				author = "Mattermost istifadəçisi"
			}
			fmt.Fprintf(&sb, "\n**%s** (%s):\n%s\n", author, msDate(cm.CreateAt, true), cm.Text)
		}
	}
	s := sb.String()
	if utf8.RuneCountInString(s) > maxBoardDescriptionRunes {
		r := []rune(s)
		s = string(r[:maxBoardDescriptionRunes-40]) + "\n\n…(qısaldılıb)"
	}
	return s
}

func clean(s string, n int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}
