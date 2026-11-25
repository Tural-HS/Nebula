package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/tableexport"
)

// cycleRoutes: the monthly period close of a project (ADR-0051). Members
// can read the configuration and the closed months; changing it, the
// preview and closing a month by hand are admin+, like recurrences.
func (a *api) cycleRoutes(r chi.Router) {
	member := r.With(requireRole(store.RoleMember))
	admin := r.With(requireRole(store.RoleAdmin))
	member.Get("/cycle", a.getCycle)
	member.Get("/cycle/closes", a.listCloses)
	member.Get("/cycle/closes/{period}", a.getClose)
	member.Get("/cycle/closes/{period}/export", a.exportClose)
	admin.Put("/cycle", a.saveCycle)
	admin.Post("/cycle/preview", a.previewCycle)
	admin.Post("/cycle/close", a.closeCycle)
}

func (a *api) getCycle(w http.ResponseWriter, r *http.Request) {
	c, err := a.store.GetCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *api) saveCycle(w http.ResponseWriter, r *http.Request) {
	var body store.CycleInput
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.SaveCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type periodBody struct {
	Period string `json:"period"` // YYYY-MM; empty = last month
}

func (a *api) previewCycle(w http.ResponseWriter, r *http.Request) {
	var body periodBody
	if !a.decode(w, r, &body) {
		return
	}
	p, err := a.store.PreviewCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, body.Period)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *api) closeCycle(w http.ResponseWriter, r *http.Request) {
	var body periodBody
	if !a.decode(w, r, &body) {
		return
	}
	c, err := a.store.CloseCycle(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, principal(r).UserID, body.Period)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (a *api) listCloses(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListCloses(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closes": list})
}

func (a *api) getClose(w http.ResponseWriter, r *http.Request) {
	c, items, err := a.store.GetCloseSnapshot(r.Context(), workspaceFrom(r).ID, projectFrom(r).ID, chi.URLParam(r, "period"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"close": c, "items": items})
}

// exportClose downloads one closed month as xlsx or CSV, with the same
// parameters, limits, cell rules (formula neutralising) and audit record as
// the item table export (ADR-0040). Without cols: key, title, status and
// every field the month's items had.
func (a *api) exportClose(w http.ResponseWriter, r *http.Request) {
	explicitCols := r.URL.Query().Get("cols") != ""
	req, ok := parseExportRequest(w, r)
	if !ok {
		return
	}
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
	ws, proj := workspaceFrom(r), projectFrom(r)
	c, rows, err := a.store.GetCloseSnapshot(ctx, ws.ID, proj.ID, chi.URLParam(r, "period"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defs, err := a.store.ListFields(ctx, ws.ID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !explicitCols {
		req.cols = []string{"key", "title", "status"}
		for _, d := range defs {
			if slices.ContainsFunc(rows, func(row store.PeriodSnapshotRow) bool { _, has := row.Fields[d.Key]; return has }) {
				req.cols = append(req.cols, "field."+d.Key)
			}
		}
		req.cols = req.cols[:min(len(req.cols), maxExportColumns)]
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
	for i, col := range cols {
		specs[i] = col.spec
	}
	row := &exportRow{cols: cols, names: names, loc: loc, cells: make([]tableexport.Cell, len(cols))}
	labels := tableexport.Labels{Yes: exportWords[req.lang].yes, No: exportWords[req.lang].no}

	// The CSV writer writes as it goes: collect it in memory first (a month
	// is at most a few thousand rows), so errors still get a proper response.
	var buf bytesSink
	var sink deferredWriter
	if req.format == "csv" {
		sink.w = &buf
	}
	var out tableexport.Writer
	var contentType string
	if req.format == "csv" {
		out, err = tableexport.NewCSV(&sink, tableexport.CSVOptions{Semicolon: req.semicolon, Labels: labels})
		contentType = "text/csv; charset=utf-8"
	} else {
		out, err = tableexport.NewXLSX(&sink, tableexport.XLSXOptions{Sheet: proj.Key + " " + c.Period, Labels: labels, MaxBytes: ex.maxBytes})
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := out.Header(specs); err != nil {
		tableexport.Abort(out)
		a.fail(w, r, err)
		return
	}
	for _, sr := range rows {
		it := store.Item{Key: fmt.Sprintf("%s-%d", proj.Key, sr.Number), Title: sr.Title, AssigneeID: sr.AssigneeID, Fields: sr.Fields}
		it.Status.Name = sr.StatusName
		if err := out.Row(row.build(it)); err != nil {
			tableexport.Abort(out)
			if errors.Is(err, tableexport.ErrTooLarge) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
					"code": "export_too_large", "message": "too much data for one Excel file; export CSV",
					"field": "bytes", "limit": ex.maxBytes}})
				return
			}
			a.fail(w, r, err)
			return
		}
	}
	payload := map[string]any{"format": req.format, "rows": len(rows), "columns": req.cols, "language": req.lang, "period": c.Period}
	if err := a.store.RecordItemsExport(ctx, ws.ID, me.UserID, proj.ID, payload); err != nil {
		tableexport.Abort(out)
		a.fail(w, r, err)
		return
	}
	setExportHeaders(w.Header(), contentType, proj.Key, proj.Name+" "+c.Period, c.Period, map[bool]string{true: "csv", false: "xlsx"}[req.format == "csv"])
	if req.format == "csv" {
		if err := out.Close(); err != nil {
			a.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.b)
		return
	}
	sink.w = w
	w.WriteHeader(http.StatusOK)
	if err := out.Close(); err != nil {
		a.logger.WarnContext(ctx, "month export failed mid-stream", "error", err)
		panic(http.ErrAbortHandler)
	}
}

// bytesSink collects a small export in memory.
type bytesSink struct{ b []byte }

func (s *bytesSink) Header() http.Header         { return http.Header{} }
func (s *bytesSink) WriteHeader(int)             {}
func (s *bytesSink) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
