package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/importer"
	"github.com/Tural-HS/Nebula/internal/store"
)

// Import limits (ADR-0011). Row, column, cell and uncompressed-size limits
// live in importer.DefaultLimits.
const (
	maxImportFileBytes = 10 << 20
	maxImportBodyBytes = maxImportFileBytes + 256<<10 // multipart framing + mapping
	maxMappingBytes    = 64 << 10
	maxSheetNameBytes  = 256
	importParseTimeout = 15 * time.Second
	importSlotWait     = 5 * time.Second
	previewRowCount    = 20
	maxReportedErrors  = 500
)

// importSlots bounds concurrent imports per process: one import may hold a
// 10 MiB upload plus up to 64 MiB of inflated workbook in memory.
var importSlots = make(chan struct{}, 2)

// importRoutes registers spreadsheet import under /projects/{project}. Imports
// are admin-only: they create many items at once (ADR-0011).
func (a *api) importRoutes(r chi.Router) {
	r.With(requireRole(store.RoleAdmin)).Post("/imports/preview", a.previewImport)
	r.With(requireRole(store.RoleAdmin)).Post("/imports", a.runImport)
}

// ---------------------------------------------------------------------------
// Upload handling: multipart is streamed into memory with hard limits; nothing
// is written to disk (no ParseMultipartForm, which spills to temp files).
// ---------------------------------------------------------------------------

type upload struct {
	name    string
	data    []byte
	sheet   string
	mapping []byte
}

func (a *api) readUpload(w http.ResponseWriter, r *http.Request) (upload, bool) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "send the file as multipart/form-data")
		return upload{}, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body")
		return upload{}, false
	}
	var u upload
	haveFile := false
	for parts := 0; ; parts++ {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || parts >= 4 {
			uploadReadError(w, err)
			return upload{}, false
		}
		limit := int64(0)
		switch part.FormName() {
		case "file":
			limit = maxImportFileBytes
		case "mapping":
			limit = maxMappingBytes
		case "sheet":
			limit = maxSheetNameBytes
		default:
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("unexpected form field %q", part.FormName()))
			return upload{}, false
		}
		b, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			uploadReadError(w, err)
			return upload{}, false
		}
		if int64(len(b)) > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
				fmt.Sprintf("%s exceeds %d KiB", part.FormName(), limit>>10))
			return upload{}, false
		}
		switch part.FormName() {
		case "file":
			if haveFile {
				writeError(w, http.StatusBadRequest, "bad_request", "send exactly one file")
				return upload{}, false
			}
			haveFile, u.data, u.name = true, b, cleanFileName(part.FileName())
		case "mapping":
			u.mapping = b
		case "sheet":
			if !utf8.Valid(b) {
				writeError(w, http.StatusBadRequest, "bad_request", "sheet must be valid UTF-8")
				return upload{}, false
			}
			u.sheet = string(b)
		}
	}
	if !haveFile {
		writeError(w, http.StatusBadRequest, "bad_request", "the file field is required")
		return upload{}, false
	}
	return u, true
}

func uploadReadError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			fmt.Sprintf("the upload exceeds %d MiB", maxImportFileBytes>>20))
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart body")
}

// cleanFileName keeps only the base name, without control characters, at most 200 characters.
func cleanFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	if rs := []rune(strings.TrimSpace(name)); len(rs) > 200 {
		name = string(rs[:200])
	}
	if name == "" || name == "." || name == "/" {
		return "upload"
	}
	return strings.TrimSpace(name)
}

// parseUpload takes an import slot and parses the file under a timeout.
// The returned release must be called when the request is done with the data.
func (a *api) parseUpload(w http.ResponseWriter, r *http.Request, u upload) (*importer.Table, func(), bool) {
	wait, cancelWait := context.WithTimeout(r.Context(), importSlotWait)
	defer cancelWait()
	select {
	case importSlots <- struct{}{}:
	case <-wait.Done():
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusServiceUnavailable, "import_busy", "other imports are running; try again shortly")
		return nil, nil, false
	}
	release := func() { <-importSlots }

	ctx, cancel := context.WithTimeout(r.Context(), importParseTimeout)
	defer cancel()
	table, err := importer.Parse(ctx, u.name, u.data, u.sheet, importer.DefaultLimits)
	if err != nil {
		release()
		var ie *importer.Error
		if errors.As(err, &ie) {
			status := http.StatusBadRequest
			if ie.TooLarge {
				status = http.StatusRequestEntityTooLarge
			}
			writeError(w, status, ie.Code, ie.Message)
			return nil, nil, false
		}
		a.fail(w, r, err)
		return nil, nil, false
	}
	return table, release, true
}

// ---------------------------------------------------------------------------
// Targets: what a column can be mapped to.
// ---------------------------------------------------------------------------

const (
	targetTitle       = "title"
	targetDescription = "description"
	targetType        = "type"
	targetStatus      = "status"
	targetAssignee    = "assignee"
	targetDueDate     = "due_date"
	fieldTargetPrefix = "field."
)

type importTarget struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // builtin | field
	Required bool   `json:"required,omitempty"`
	Key      string `json:"key,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"` // custom field type
	field    *store.FieldDefinition
	hints    importer.Target
}

var builtinTargets = []importTarget{
	{ID: targetTitle, Kind: "builtin", Required: true, hints: importer.Target{
		Names:    []string{"title", "başlıq"},
		Synonyms: []string{"ad", "adı", "ad soyad", "adı soyadı", "name", "full name", "müştəri", "müştərinin adı", "customer", "client", "summary", "mövzu", "subject", "tapşırıq"},
	}},
	{ID: targetDescription, Kind: "builtin", hints: importer.Target{
		Names:    []string{"description", "təsvir"},
		Synonyms: []string{"açıqlama", "qeyd", "qeydlər", "notes", "note", "comment", "şərh", "details"},
	}},
	{ID: targetType, Kind: "builtin", hints: importer.Target{
		Names: []string{"type", "növ"}, Synonyms: []string{"tip", "item type", "kind"},
	}},
	{ID: targetStatus, Kind: "builtin", hints: importer.Target{
		Names: []string{"status"}, Synonyms: []string{"vəziyyət", "state", "mərhələ", "stage"},
	}},
	{ID: targetAssignee, Kind: "builtin", hints: importer.Target{
		Names: []string{"assignee", "icraçı"}, Synonyms: []string{"məsul", "məsul şəxs", "responsible"},
	}},
	{ID: targetDueDate, Kind: "builtin", hints: importer.Target{
		Names:    []string{"due date", "due_date", "son tarix"},
		Synonyms: []string{"deadline", "son müddət", "ödəniş tarixi", "ödəniş günü", "müddət", "due"},
	}},
}

// importContext is the workspace configuration an import converts against.
type importContext struct {
	targets  []importTarget
	byID     map[string]importTarget
	types    []store.ItemType
	typeKeys map[string]*store.ItemType // folded key and name
	members  map[string]uuid.UUID       // folded email
}

func (a *api) loadImportContext(ctx context.Context, wsID uuid.UUID) (*importContext, error) {
	types, err := a.store.ListItemTypes(ctx, wsID)
	if err != nil {
		return nil, err
	}
	fields, err := a.store.ListFields(ctx, wsID)
	if err != nil {
		return nil, err
	}
	members, err := a.store.ListMembers(ctx, wsID)
	if err != nil {
		return nil, err
	}
	ic := &importContext{types: types, byID: map[string]importTarget{},
		typeKeys: map[string]*store.ItemType{}, members: map[string]uuid.UUID{}}
	ic.targets = append(ic.targets, builtinTargets...)
	for i := range fields {
		f := &fields[i]
		if f.Archived {
			continue
		}
		ic.targets = append(ic.targets, importTarget{ID: fieldTargetPrefix + f.Key, Kind: "field", Key: f.Key, Name: f.Name,
			Type: f.Type, field: f, hints: importer.Target{Names: []string{f.Key, f.Name}}})
	}
	for i := range ic.targets {
		ic.targets[i].hints.ID = ic.targets[i].ID
		ic.byID[ic.targets[i].ID] = ic.targets[i]
	}
	for i := range types {
		ic.typeKeys[importer.Fold(types[i].Key)] = &types[i]
		if _, taken := ic.typeKeys[importer.Fold(types[i].Name)]; !taken {
			ic.typeKeys[importer.Fold(types[i].Name)] = &types[i]
		}
	}
	for _, m := range members {
		ic.members[importer.Fold(m.Email)] = m.UserID
	}
	return ic, nil
}

func (ic *importContext) suggest(headers []string) []mappedColumn {
	hints := make([]importer.Target, len(ic.targets))
	for i, t := range ic.targets {
		hints[i] = t.hints
	}
	out := []mappedColumn{}
	for col, target := range importer.Suggest(headers, hints) {
		out = append(out, mappedColumn{Column: col, Target: target})
	}
	slices.SortFunc(out, func(a, b mappedColumn) int { return a.Column - b.Column })
	return out
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

type previewRow struct {
	Row   int      `json:"row"`
	Cells []string `json:"cells"`
}

func (a *api) previewImport(w http.ResponseWriter, r *http.Request) {
	u, ok := a.readUpload(w, r)
	if !ok {
		return
	}
	if u.mapping != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "preview takes no mapping")
		return
	}
	table, release, ok := a.parseUpload(w, r, u)
	if !ok {
		return
	}
	defer release()
	ic, err := a.loadImportContext(r.Context(), workspaceFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	rows := make([]previewRow, 0, previewRowCount)
	for _, row := range table.Rows[:min(previewRowCount, len(table.Rows))] {
		rows = append(rows, previewRow{Row: row.Line, Cells: row.Cells})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file_name": u.name,
		"format":    table.Format,
		"sheets":    nonNil(table.Sheets),
		"sheet":     table.Sheet,
		"delimiter": table.Delimiter,
		"headers":   table.Headers,
		"rows":      rows,
		"row_count": len(table.Rows),
		"targets":   ic.targets,
		"suggested": ic.suggest(table.Headers),
		"limits": map[string]any{
			"max_file_bytes": maxImportFileBytes,
			"max_rows":       importer.DefaultLimits.MaxRows,
			"max_columns":    importer.DefaultLimits.MaxColumns,
		},
	})
}

// ---------------------------------------------------------------------------
// Import (dry run or real)
// ---------------------------------------------------------------------------

type mappedColumn struct {
	Column int    `json:"column"`
	Target string `json:"target"`
}

type importMapping struct {
	Columns     []mappedColumn `json:"columns"`
	DefaultType string         `json:"default_type"`
}

type importRowError struct {
	Row     int    `json:"row"`
	Column  *int   `json:"column"` // nil: not tied to a mapped column (e.g. a required field that is not mapped)
	Header  string `json:"header,omitempty"`
	Target  string `json:"target,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (a *api) runImport(w http.ResponseWriter, r *http.Request) {
	dryRun := false
	switch v := r.URL.Query().Get("dry_run"); v {
	case "", "false", "0":
	case "true", "1":
		dryRun = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "dry_run must be true or false")
		return
	}
	u, ok := a.readUpload(w, r)
	if !ok {
		return
	}
	if u.mapping == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "the mapping field is required")
		return
	}
	var m importMapping
	dec := json.NewDecoder(bytes.NewReader(u.mapping))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "bad_request", "mapping must be a single JSON object")
		return
	}

	table, release, ok := a.parseUpload(w, r, u)
	if !ok {
		return
	}
	defer release()
	if len(table.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "empty_file", "the sheet has no data rows")
		return
	}
	ws, proj := workspaceFrom(r), projectFrom(r)
	ic, err := a.loadImportContext(r.Context(), ws.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	conv, err := ic.newConverter(table, m)
	if err != nil {
		a.fail(w, r, err)
		return
	}

	// Rows with conversion errors still go through store validation (minus the
	// cells that failed), so one dry run reports every problem in every row.
	rows := make([]store.ImportRow, 0, len(table.Rows))
	var report []importRowError
	failed := map[[2]string]bool{} // {row, target} already reported
	for _, row := range table.Rows {
		item, errs := conv.convert(row)
		for _, e := range errs {
			failed[[2]string{fmt.Sprint(e.Row), e.Target}] = true
		}
		report = append(report, errs...)
		rows = append(rows, store.ImportRow{Line: row.Line, Item: item})
	}
	res, err := a.store.ImportItems(r.Context(), store.ImportParams{
		WorkspaceID: ws.ID, ProjectID: proj.ID, ActorID: principal(r).UserID,
		FieldKeys: conv.fieldKeys, Rows: rows,
		DryRun: dryRun || len(report) > 0, // validate the rest so one report lists every problem
		Audit: map[string]any{"file_name": u.name, "format": table.Format, "sheet": table.Sheet,
			"rows": len(table.Rows), "mapping": m.Columns},
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for _, e := range res.Errors {
		if se := conv.storeError(e); !failed[[2]string{fmt.Sprint(se.Row), se.Target}] {
			report = append(report, se)
		}
	}
	slices.SortStableFunc(report, func(a, b importRowError) int { return a.Row - b.Row })

	body := map[string]any{
		"dry_run":     dryRun,
		"file_name":   u.name,
		"row_count":   len(table.Rows),
		"valid":       len(table.Rows) - countRows(report),
		"created":     res.Created,
		"error_count": len(report),
		"errors":      nonNil(report[:min(len(report), maxReportedErrors)]),
		"truncated":   len(report) > maxReportedErrors,
	}
	if res.Created > 0 {
		body["first_key"] = fmt.Sprintf("%s-%d", proj.Key, res.FirstNumber)
		body["last_key"] = fmt.Sprintf("%s-%d", proj.Key, res.LastNumber)
		a.logger.InfoContext(r.Context(), "items imported", "workspace", ws.Slug, "project", proj.Key,
			"rows", res.Created, "format", table.Format, "request_id", middleware.GetReqID(r.Context()))
	}
	status := http.StatusOK
	switch {
	case !dryRun && len(report) > 0:
		status = http.StatusUnprocessableEntity
		body["error"] = map[string]string{"code": "import_invalid",
			"message": fmt.Sprintf("%d rows have errors; nothing was imported", countRows(report))}
	case !dryRun:
		status = http.StatusCreated
	}
	writeJSON(w, status, body)
}

// countRows counts distinct rows in a report sorted by row.
func countRows(report []importRowError) int {
	n, last := 0, -1
	for _, e := range report {
		if e.Row != last {
			n, last = n+1, e.Row
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Row conversion
// ---------------------------------------------------------------------------

type converter struct {
	ic          *importContext
	table       *importer.Table
	columns     []mappedColumn
	byTarget    map[string]int
	defaultType *store.ItemType
	fieldKeys   []string
}

func (ic *importContext) newConverter(t *importer.Table, m importMapping) (*converter, error) {
	if len(m.Columns) > importer.DefaultLimits.MaxColumns {
		return nil, &store.ValidationError{Field: "mapping.columns", Message: "too many mapped columns"}
	}
	c := &converter{ic: ic, table: t, byTarget: map[string]int{}, fieldKeys: []string{}}
	usedCol := map[int]bool{}
	for i, mc := range m.Columns {
		path := fmt.Sprintf("mapping.columns[%d]", i)
		if mc.Target == "" {
			continue // explicitly ignored column
		}
		if mc.Column < 0 || mc.Column >= t.Width() {
			return nil, &store.ValidationError{Field: path + ".column", Message: "no such column in the sheet"}
		}
		target, ok := ic.byID[mc.Target]
		if !ok {
			return nil, &store.ValidationError{Field: path + ".target", Message: fmt.Sprintf("unknown target %q", mc.Target)}
		}
		if _, dup := c.byTarget[mc.Target]; dup {
			return nil, &store.ValidationError{Field: path + ".target", Message: fmt.Sprintf("%q is mapped twice", mc.Target)}
		}
		if usedCol[mc.Column] {
			return nil, &store.ValidationError{Field: path + ".column", Message: "column is mapped twice"}
		}
		usedCol[mc.Column] = true
		c.byTarget[mc.Target] = mc.Column
		c.columns = append(c.columns, mc)
		if target.field != nil {
			c.fieldKeys = append(c.fieldKeys, target.Key)
		}
	}
	if _, ok := c.byTarget[targetTitle]; !ok {
		return nil, &store.ValidationError{Field: "mapping.columns", Message: "a column must be mapped to the title"}
	}
	switch {
	case m.DefaultType != "":
		for i := range ic.types {
			if ic.types[i].Key == m.DefaultType {
				c.defaultType = &ic.types[i]
			}
		}
		if c.defaultType == nil {
			return nil, &store.ValidationError{Field: "mapping.default_type", Message: "unknown item type"}
		}
	default:
		// Like single creates, "task" if it exists, else the first type (by key).
		// Resolved here (not left to the store) so statuses can be matched by name.
		for i := range ic.types {
			if ic.types[i].Key == "task" {
				c.defaultType = &ic.types[i]
			}
		}
		if c.defaultType == nil && len(ic.types) > 0 {
			c.defaultType = &ic.types[0]
		}
	}
	return c, nil
}

func (c *converter) rowError(row importer.Row, col int, target, msg string) importRowError {
	e := importRowError{Row: row.Line, Target: target, Message: msg}
	if col >= 0 {
		e.Column = &col
		e.Header = c.table.Headers[col]
	}
	return e
}

// convert turns one row into create parameters. Blank cells leave the value
// unset (title stays empty so the store reports it as required).
func (c *converter) convert(row importer.Row) (store.CreateItemParams, []importRowError) {
	var p store.CreateItemParams
	var errs []importRowError
	cell := func(target string) (string, int) {
		col, ok := c.byTarget[target]
		if !ok {
			return "", -1
		}
		return strings.TrimSpace(row.Cells[col]), col
	}

	p.Title, _ = cell(targetTitle)
	if v, _ := cell(targetDescription); v != "" {
		p.Description = &v
	}
	typ := c.defaultType
	if v, col := cell(targetType); v != "" {
		if typ = c.ic.typeKeys[importer.Fold(v)]; typ == nil {
			errs = append(errs, c.rowError(row, col, targetType, fmt.Sprintf("unknown item type %q", v)))
		}
	}
	if typ != nil {
		p.TypeKey = typ.Key
	}
	if v, col := cell(targetStatus); v != "" && typ != nil {
		for _, s := range typ.Statuses {
			if importer.Fold(s.Name) == importer.Fold(v) {
				id := s.ID
				p.StatusID = &id
				break
			}
		}
		if p.StatusID == nil {
			errs = append(errs, c.rowError(row, col, targetStatus, fmt.Sprintf("status %q does not exist in the workflow of %q", v, typ.Name)))
		}
	}
	if v, col := cell(targetDueDate); v != "" {
		// Same parser as date fields: ISO, dd.mm.yyyy and (xlsx) serial dates.
		if d, err := importer.ParseDate(v, c.table.Format == importer.FormatXLSX, c.table.Date1904); err != nil {
			errs = append(errs, c.rowError(row, col, targetDueDate, err.Error()))
		} else {
			p.DueDate = &d
		}
	}
	if v, col := cell(targetAssignee); v != "" {
		if id, ok := c.ic.members[importer.Fold(v)]; ok {
			p.AssigneeID = &id
		} else {
			errs = append(errs, c.rowError(row, col, targetAssignee, fmt.Sprintf("no workspace member has the email %q", v)))
		}
	}

	for _, mc := range c.columns {
		t := c.ic.byID[mc.Target]
		if t.field == nil {
			continue
		}
		v := strings.TrimSpace(row.Cells[mc.Column])
		if v == "" {
			continue
		}
		val, err := c.fieldValue(*t.field, v)
		if err != nil {
			errs = append(errs, c.rowError(row, mc.Column, mc.Target, err.Error()))
			continue
		}
		raw, err := json.Marshal(val)
		if err != nil {
			errs = append(errs, c.rowError(row, mc.Column, mc.Target, "value cannot be stored"))
			continue
		}
		if p.Fields == nil {
			p.Fields = map[string]json.RawMessage{}
		}
		p.Fields[t.Key] = raw
	}
	return p, errs
}

// fieldValue converts cell text into the field type's API input; the store
// then validates it exactly as for a single create.
func (c *converter) fieldValue(def store.FieldDefinition, v string) (any, error) {
	xlsx := c.table.Format == importer.FormatXLSX
	switch def.Type {
	case store.FieldNumber:
		return importer.ParseNumber(v, xlsx)
	case store.FieldDate:
		return importer.ParseDate(v, xlsx, c.table.Date1904)
	case store.FieldDateTime:
		return importer.ParseDateTime(v, xlsx, c.table.Date1904)
	case store.FieldBoolean:
		return importer.ParseBool(v)
	case store.FieldSelect:
		return matchChoice(def, v)
	case store.FieldMultiSelect:
		ids := []string{}
		for _, part := range importer.SplitList(v) {
			id, err := matchChoice(def, part)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	case store.FieldUser:
		if id, ok := c.ic.members[importer.Fold(v)]; ok {
			return id.String(), nil
		}
		return nil, fmt.Errorf("no workspace member has the email %q", v)
	}
	return v, nil // text, long_text, url, email: validated by the store
}

// matchChoice finds an active choice by label (case-insensitive) or by id.
func matchChoice(def store.FieldDefinition, v string) (string, error) {
	want := importer.Fold(v)
	found := ""
	for _, ch := range def.Options.Choices {
		if ch.Archived {
			continue
		}
		if ch.ID == v {
			return ch.ID, nil
		}
		if importer.Fold(ch.Label) == want {
			if found != "" {
				return "", fmt.Errorf("%q matches more than one choice", v)
			}
			found = ch.ID
		}
	}
	if found == "" {
		return "", fmt.Errorf("%q is not a choice of this field", v)
	}
	return found, nil
}

// storeError maps a store validation error back to the column it came from.
func (c *converter) storeError(e store.ImportRowError) importRowError {
	target := ""
	switch {
	case e.Field == "title", e.Field == "description", e.Field == "type":
		target = e.Field
	case e.Field == "status_id":
		target = targetStatus
	case e.Field == "assignee_id":
		target = targetAssignee
	case e.Field == "due_date":
		target = targetDueDate
	case strings.HasPrefix(e.Field, "fields."):
		target = fieldTargetPrefix + strings.TrimPrefix(e.Field, "fields.")
	}
	col := -1
	if i, ok := c.byTarget[target]; ok {
		col = i
	}
	out := c.rowError(importer.Row{Line: e.Line}, col, target, e.Message)
	out.Field = e.Field
	return out
}
