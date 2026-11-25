package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/tableexport"
)

// Item table export (ADR-0040): GET /items/export takes the list's query
// parameters plus format, cols, lang and delimiter, and returns every item
// the list would show, over all pages, as CSV or xlsx. Rows come from the
// list's own code path (parseItemFilters, store.ListItems), so privacy and
// filters are exactly the list's.

const (
	// MaxExportRows caps one export; above it the user narrows the filters.
	MaxExportRows = 50_000
	exportPage    = 500
	// maxXLSXBytes bounds the sheet XML of one xlsx export: the stream
	// writer spills past 16 MiB to /tmp (a 64 MiB tmpfs in production,
	// shared with imports), and at most exportSlots run at once.
	maxXLSXBytes = 24 << 20
	exportSlots  = 2
)

// itemExports bounds the work exports can cause.
type itemExports struct {
	limiter  *ratelimit.Limiter
	slots    chan struct{}
	maxRows  int
	maxBytes int64
}

func newItemExports(maxRows int) *itemExports {
	if maxRows <= 0 {
		maxRows = MaxExportRows
	}
	return &itemExports{
		// 30 per hour per user, at most 10 back to back.
		limiter:  ratelimit.New(30.0/3600, 10),
		slots:    make(chan struct{}, exportSlots),
		maxRows:  maxRows,
		maxBytes: maxXLSXBytes,
	}
}

// Same rule as the web's column list (lib/views.ts COLUMN).
var exportColumnRe = regexp.MustCompile(`^(key|title|type|status|assignee|due_date|created_at|updated_at|field\.[a-z][a-z0-9_]{0,62})$`)

const maxExportColumns = 30

var defaultExportColumns = []string{"key", "title", "type", "status", "assignee", "updated_at"}

// exportWords are the built-in headers and yes/no in each UI language,
// the same words as the table (web/src/locales/*/views.ts, core.ts).
var exportWords = map[string]struct {
	cols    map[string]string
	yes, no string
}{
	"az": {map[string]string{"key": "#", "title": "Başlıq", "type": "Tip", "status": "Status", "assignee": "İcraçı",
		"due_date": "Son tarix", "created_at": "Yaradılıb", "updated_at": "Yenilənib", "currency": "valyuta"}, "Bəli", "Xeyr"},
	"en": {map[string]string{"key": "#", "title": "Title", "type": "Type", "status": "Status", "assignee": "Assignee",
		"due_date": "Due date", "created_at": "Created", "updated_at": "Updated", "currency": "currency"}, "Yes", "No"},
	"tr": {map[string]string{"key": "#", "title": "Başlık", "type": "Tür", "status": "Durum", "assignee": "Sorumlu",
		"due_date": "Bitiş tarihi", "created_at": "Oluşturuldu", "updated_at": "Güncellendi", "currency": "para birimi"}, "Evet", "Hayır"},
	"ru": {map[string]string{"key": "#", "title": "Заголовок", "type": "Тип", "status": "Статус", "assignee": "Исполнитель",
		"due_date": "Срок", "created_at": "Создано", "updated_at": "Обновлено", "currency": "валюта"}, "Да", "Нет"},
}

type exportRequest struct {
	format    string // "xlsx" or "csv"
	semicolon bool
	lang      string
	cols      []string
}

// parseExportRequest reads the export's own parameters (400 when invalid).
func parseExportRequest(w http.ResponseWriter, r *http.Request) (exportRequest, bool) {
	q := r.URL.Query()
	var req exportRequest
	for _, k := range []string{"format", "delimiter", "lang", "cols"} {
		if len(q[k]) > 1 {
			writeError(w, http.StatusBadRequest, "bad_request", k+" may be given only once")
			return req, false
		}
	}
	switch req.format = q.Get("format"); req.format {
	case "xlsx", "csv":
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "format must be xlsx or csv")
		return req, false
	}
	switch q.Get("delimiter") {
	case "", "comma":
	case "semicolon":
		req.semicolon = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "delimiter must be comma or semicolon")
		return req, false
	}
	req.lang = q.Get("lang")
	if req.lang == "" {
		req.lang = "az"
	}
	if _, ok := exportWords[req.lang]; !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "lang must be az, en, tr or ru")
		return req, false
	}
	seen := map[string]bool{}
	if raw := q.Get("cols"); raw != "" {
		for c := range strings.SplitSeq(raw, ",") {
			c = strings.TrimSpace(c)
			if !exportColumnRe.MatchString(c) {
				writeError(w, http.StatusBadRequest, "bad_request", "cols must be a list of table columns")
				return req, false
			}
			if !seen[c] {
				seen[c] = true
				req.cols = append(req.cols, c)
			}
		}
	}
	if len(req.cols) > maxExportColumns {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("at most %d columns", maxExportColumns))
		return req, false
	}
	if len(req.cols) == 0 {
		req.cols = append(req.cols, defaultExportColumns...)
	}
	return req, true
}

// exportColumn is one resolved column: a built-in or an active field.
type exportColumn struct {
	id    string
	field *store.FieldDefinition
	spec  tableexport.Column
	// currency is the second column of a money field (ADR-0044): the
	// amount stays a numeric cell, its currency code goes next to it.
	currency bool
}

// resolveExportColumns drops unknown and archived fields (as the table
// does) and puts the key and the title first when they are missing.
func resolveExportColumns(ids []string, defs []store.FieldDefinition, lang string) []exportColumn {
	byKey := make(map[string]*store.FieldDefinition, len(defs))
	for i := range defs {
		byKey[defs[i].Key] = &defs[i]
	}
	// The title where the table puts it (lib/views.ts visibleColumns), then
	// the key in front when it is missing.
	order := slices.Clone(ids)
	if !slices.Contains(order, "title") {
		at := 0
		if len(order) > 0 && order[0] == "key" {
			at = 1
		}
		order = slices.Insert(order, at, "title")
	}
	if !slices.Contains(order, "key") {
		order = slices.Insert(order, 0, "key")
	}
	words := exportWords[lang].cols
	out := make([]exportColumn, 0, len(order))
	for _, id := range order {
		key, isField := strings.CutPrefix(id, "field.")
		if !isField {
			kind := tableexport.Text
			switch id {
			case "due_date":
				kind = tableexport.Date
			case "created_at", "updated_at":
				kind = tableexport.DateTime
			}
			out = append(out, exportColumn{id: id, spec: tableexport.Column{Header: words[id], Kind: kind}})
			continue
		}
		def := byKey[key]
		if def == nil || def.Archived {
			continue
		}
		spec := tableexport.Column{Header: def.Name}
		switch fieldValueType(def) {
		case store.FieldNumber:
			spec.Kind = tableexport.Number
			if def.Options.Formula != nil {
				spec.Scale = def.Options.Formula.Scale
			}
		case store.FieldDate:
			spec.Kind = tableexport.Date
		case store.FieldDateTime:
			spec.Kind = tableexport.DateTime
		case store.FieldBoolean:
			spec.Kind = tableexport.Bool
		case store.FieldMoney:
			spec.Kind = tableexport.Number
			scale := 2
			if def.Options.Money != nil {
				scale = def.Options.Money.Scale
			}
			spec.Scale = &scale
			out = append(out, exportColumn{id: id, field: def, spec: spec},
				exportColumn{id: id, field: def, currency: true,
					spec: tableexport.Column{Header: def.Name + " (" + words["currency"] + ")", Kind: tableexport.Text}})
			continue
		}
		out = append(out, exportColumn{id: id, field: def, spec: spec})
	}
	return out
}

// fieldValueType: a formula exports as its result type (ADR-0030).
func fieldValueType(def *store.FieldDefinition) string {
	if def.Type != store.FieldFormula {
		return def.Type
	}
	if def.Options.Formula == nil {
		return store.FieldText
	}
	switch def.Options.Formula.ResultType {
	case "number":
		return store.FieldNumber
	case "date":
		return store.FieldDate
	case "boolean":
		return store.FieldBoolean
	}
	return store.FieldText
}

// exportRow turns an item into cells, in the columns' order.
type exportRow struct {
	cols  []exportColumn
	names map[uuid.UUID]string // member display names
	loc   *time.Location
	cells []tableexport.Cell
}

func (e *exportRow) build(it store.Item) []tableexport.Cell {
	for i, c := range e.cols {
		e.cells[i] = e.cell(c, it)
	}
	return e.cells
}

func (e *exportRow) cell(c exportColumn, it store.Item) tableexport.Cell {
	switch c.id {
	case "key":
		return tableexport.TextCell(it.Key)
	case "title":
		return tableexport.TextCell(it.Title)
	case "type":
		return tableexport.TextCell(it.Type.Name)
	case "status":
		return tableexport.TextCell(it.Status.Name)
	case "assignee":
		if it.AssigneeID == nil {
			return tableexport.Cell{}
		}
		return tableexport.TextCell(e.names[*it.AssigneeID])
	case "due_date":
		if it.DueDate == nil {
			return tableexport.Cell{}
		}
		return dateCell(*it.DueDate)
	case "created_at":
		return tableexport.DateTimeCell(wallClock(it.CreatedAt, e.loc))
	case "updated_at":
		return tableexport.DateTimeCell(wallClock(it.UpdatedAt, e.loc))
	}
	if c.field == nil {
		return tableexport.Cell{}
	}
	if c.field.Type == store.FieldMoney {
		return moneyCell(it.Fields[c.field.Key], c.currency)
	}
	return e.fieldCell(c.field, it.Fields[c.field.Key])
}

// moneyCell is the amount (numeric: amounts have at most 15 digits, exact
// in a float64 and in Excel) or the currency code of a money value.
func moneyCell(v any, currency bool) tableexport.Cell {
	m, ok := v.(map[string]any)
	if !ok {
		return tableexport.Cell{}
	}
	if currency {
		c, _ := m["currency"].(string)
		return tableexport.TextCell(c)
	}
	a, _ := m["amount"].(string)
	n, err := strconv.ParseFloat(a, 64)
	if err != nil || a == "" {
		return tableexport.Cell{}
	}
	return tableexport.NumberCell(n)
}

func (e *exportRow) fieldCell(def *store.FieldDefinition, v any) tableexport.Cell {
	if v == nil {
		return tableexport.Cell{}
	}
	if m, ok := v.(map[string]any); ok { // a formula that cannot be computed: {"error": code}
		if code, ok := m["error"].(string); ok {
			return tableexport.TextCell("#" + code)
		}
		return tableexport.Cell{}
	}
	switch fieldValueType(def) {
	case store.FieldNumber:
		if n, ok := number(v); ok {
			return tableexport.NumberCell(n)
		}
	case store.FieldDate:
		if s, ok := v.(string); ok {
			return dateCell(s)
		}
	case store.FieldDateTime:
		if s, ok := v.(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return tableexport.DateTimeCell(wallClock(t, e.loc))
			}
		}
	case store.FieldBoolean:
		if b, ok := v.(bool); ok {
			return tableexport.BoolCell(b)
		}
	case store.FieldSelect:
		if s, ok := v.(string); ok {
			return tableexport.TextCell(choiceLabel(def, s))
		}
	case store.FieldMultiSelect:
		if list, ok := v.([]any); ok {
			labels := make([]string, 0, len(list))
			for _, x := range list {
				if s, ok := x.(string); ok {
					labels = append(labels, choiceLabel(def, s))
				}
			}
			return tableexport.TextCell(strings.Join(labels, ", "))
		}
	case store.FieldUser:
		if s, ok := v.(string); ok {
			if id, err := uuid.Parse(s); err == nil {
				return tableexport.TextCell(e.names[id])
			}
		}
	}
	switch x := v.(type) {
	case string:
		return tableexport.TextCell(x)
	case []any:
		parts := make([]string, len(x))
		for i, p := range x {
			parts[i] = fmt.Sprint(p)
		}
		return tableexport.TextCell(strings.Join(parts, ", "))
	}
	return tableexport.TextCell(fmt.Sprint(v))
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func choiceLabel(def *store.FieldDefinition, id string) string {
	for _, c := range def.Options.Choices {
		if c.ID == id {
			return c.Label
		}
	}
	return "?"
}

func dateCell(s string) tableexport.Cell {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return tableexport.TextCell(s)
	}
	return tableexport.DateCell(t)
}

// wallClock: the time as the workspace's clock shows it (spreadsheets have
// no time zones).
func wallClock(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second(), 0, time.UTC)
}

// exportFilterSummary describes the filters without their values where
// those could be data: whether there was a search, not its text.
func exportFilterSummary(p store.ListItemsParams) map[string]any {
	s := map[string]any{"search": p.Query != "", "statuses": len(p.StatusIDs)}
	if len(p.FieldFilters) > 0 {
		keys := make([]string, 0, len(p.FieldFilters))
		for k := range p.FieldFilters {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		s["fields"] = keys
	}
	switch {
	case p.Unassigned:
		s["assignee"] = "none"
	case p.AssigneeID != nil:
		s["assignee"] = "user"
	}
	if p.Due != "" {
		s["due"] = p.Due
	}
	if p.DueFrom != nil || p.DueTo != nil {
		s["due_range"] = true
	}
	if p.Approval != "" {
		s["approval"] = p.Approval
	}
	if p.Blocked {
		s["blocked"] = true
	}
	if p.ParentNumber != nil {
		s["parent"] = true
	}
	if p.Sort != "" {
		s["sort"] = p.Sort
	}
	return s
}

func (a *api) exportItems(w http.ResponseWriter, r *http.Request) {
	req, ok := parseExportRequest(w, r)
	if !ok {
		return
	}
	p, ok := parseItemFilters(w, r)
	if !ok {
		return
	}
	p.Cursor, p.Limit = nil, exportPage // every page, from the start

	ex := a.exports
	me := principal(r)
	if ok, wait := ex.limiter.Allow(me.UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many exports; try again later")
		return
	}
	select {
	case ex.slots <- struct{}{}:
		defer func() { <-ex.slots }()
	default:
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusTooManyRequests, "export_busy", "other exports are running; try again shortly")
		return
	}

	ctx := r.Context()
	total, err := a.store.CountItemsUpTo(ctx, p, ex.maxRows)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if total > ex.maxRows {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
			"code": "export_too_large", "message": fmt.Sprintf("more than %d items match; narrow the filters", ex.maxRows),
			"field": "rows", "limit": ex.maxRows}})
		return
	}
	ws, proj := workspaceFrom(r), projectFrom(r)
	defs, err := a.store.ListFields(ctx, ws.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	members, err := a.store.ListMembers(ctx, ws.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	names := make(map[uuid.UUID]string, len(members))
	for _, m := range members {
		names[m.UserID] = m.DisplayName
	}
	loc, err := time.LoadLocation(ws.Timezone)
	if err != nil {
		loc = time.UTC
	}
	cols := resolveExportColumns(req.cols, defs, req.lang)
	specs := make([]tableexport.Column, len(cols))
	ids := make([]string, len(cols))
	for i, c := range cols {
		specs[i], ids[i] = c.spec, c.id
	}
	row := &exportRow{cols: cols, names: names, loc: loc, cells: make([]tableexport.Cell, len(cols))}
	labels := tableexport.Labels{Yes: exportWords[req.lang].yes, No: exportWords[req.lang].no}

	// Recorded before a single byte leaves: no unaudited export. Column ids
	// and filter kinds only, never cell values or the search text.
	payload := map[string]any{"format": req.format, "rows": total, "columns": ids,
		"filters": exportFilterSummary(p), "language": req.lang}
	if req.format == "csv" {
		payload["delimiter"] = map[bool]string{false: "comma", true: "semicolon"}[req.semicolon]
	}

	date := time.Now().In(loc).Format(time.DateOnly)
	h := w.Header()
	if req.format == "csv" {
		if err := a.store.RecordItemsExport(ctx, ws.ID, me.UserID, proj.ID, payload); err != nil {
			a.fail(w, r, err)
			return
		}
		setExportHeaders(h, "text/csv; charset=utf-8", proj.Key, proj.Name, date, "csv")
		w.WriteHeader(http.StatusOK)
		out, err := tableexport.NewCSV(w, tableexport.CSVOptions{Semicolon: req.semicolon, Labels: labels})
		if err == nil {
			err = a.writeExportRows(r, out, specs, p, row)
		}
		if err == nil {
			err = out.Close()
		}
		if err != nil {
			// Headers are gone: break the transfer so the file is not
			// mistaken for a complete one.
			a.logger.WarnContext(ctx, "item export failed mid-stream", "error", err)
			panic(http.ErrAbortHandler)
		}
		return
	}

	// xlsx: the workbook is assembled first (memory, then /tmp past 16 MiB),
	// so a failure or the size budget still gets a proper error response.
	var sink deferredWriter
	out, err := tableexport.NewXLSX(&sink, tableexport.XLSXOptions{Sheet: proj.Key, Labels: labels, MaxBytes: ex.maxBytes})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.writeExportRows(r, out, specs, p, row); err != nil {
		tableexport.Abort(out)
		if errors.Is(err, tableexport.ErrTooLarge) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
				"code": "export_too_large", "message": "too much data for one Excel file; export CSV or choose fewer columns",
				"field": "bytes", "limit": ex.maxBytes}})
			return
		}
		a.fail(w, r, err)
		return
	}
	if err := a.store.RecordItemsExport(ctx, ws.ID, me.UserID, proj.ID, payload); err != nil {
		tableexport.Abort(out)
		a.fail(w, r, err)
		return
	}
	setExportHeaders(h, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", proj.Key, proj.Name, date, "xlsx")
	sink.w = w // Close writes the zip straight to the client
	w.WriteHeader(http.StatusOK)
	if err := out.Close(); err != nil {
		a.logger.WarnContext(ctx, "item export failed mid-stream", "error", err)
		panic(http.ErrAbortHandler)
	}
}

// deferredWriter forwards to w once it is set; the xlsx writer only writes
// on Close, after the response headers are decided.
type deferredWriter struct{ w http.ResponseWriter }

func (d *deferredWriter) Write(b []byte) (int, error) {
	if d.w == nil {
		return 0, errors.New("export: write before the response is ready")
	}
	return d.w.Write(b)
}

// writeExportRows pages through the list (the same query as GET /items)
// and writes each row as it comes.
func (a *api) writeExportRows(r *http.Request, out tableexport.Writer, specs []tableexport.Column, p store.ListItemsParams, row *exportRow) error {
	if err := out.Header(specs); err != nil {
		return err
	}
	written := 0
	for {
		items, next, err := a.store.ListItems(r.Context(), p)
		if err != nil {
			return err
		}
		for _, it := range items {
			if written >= a.exports.maxRows {
				return nil // items created since the count
			}
			if err := out.Row(row.build(it)); err != nil {
				return err
			}
			written++
		}
		if next == nil {
			return nil
		}
		p.Cursor = next
	}
}

func setExportHeaders(h http.Header, contentType, key, name, date, ext string) {
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", exportDisposition(key, name, date, ext))
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
}

// exportDisposition: an ASCII filename (project key) for old clients and
// the project's name as an RFC 5987 UTF-8 filename* for the rest.
func exportDisposition(key, name, date, ext string) string {
	ascii := asciiFileName(key + "-" + date + "." + ext)
	clean := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" {
		clean = key
	}
	if len(clean) > 120 {
		clean = strings.ToValidUTF8(clean[:120], "")
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, rfc5987(clean+"-"+date+"."+ext))
}

// rfc5987 percent-encodes everything but attr-char (RFC 5987 §3.2.1).
func rfc5987(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
