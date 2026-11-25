package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Tural-HS/Nebula/internal/ratelimit"
	"github.com/Tural-HS/Nebula/internal/store"
	"github.com/Tural-HS/Nebula/internal/tableexport"
)

// Workspace reports "Hesabatlar" (ADR-0041). Every member, guests included,
// may read them: the numbers only ever cover projects the viewer can see
// (store.ReportsFor), so a guest without projects simply gets zeros.

// newReportsLimiter: the reports batch scans a workspace's items and
// events; 60 per minute per user (30 back to back) is far above a person
// clicking filters and well below a scraper.
func newReportsLimiter() *ratelimit.Limiter { return ratelimit.New(1, 30) }

func (a *api) reportsRoutes(r chi.Router) {
	r.Get("/reports", a.reports)
	r.Get("/reports/workload.csv", a.reportWorkloadCSV)
}

// reportWords are the CSV headers in each UI language
// (web/src/locales/*/reports.ts).
var reportWords = map[string]struct{ assignee, open, overdue, completed, unassigned string }{
	"az": {"İcraçı", "Açıq", "Gecikən", "Dövrdə bitən", "Təyin olunmayıb"},
	"en": {"Assignee", "Open", "Overdue", "Completed in period", "Unassigned"},
	"tr": {"Sorumlu", "Açık", "Geciken", "Dönemde tamamlanan", "Atanmamış"},
	"ru": {"Исполнитель", "Открыто", "Просрочено", "Завершено за период", "Не назначено"},
}

// parseReportsParams reads from, to, projects, assignee and type (400 when
// invalid). Without from/to the period is the current month up to today,
// in the workspace's time zone.
func parseReportsParams(w http.ResponseWriter, r *http.Request, q url.Values) (store.ReportsParams, bool) {
	ws := workspaceFrom(r)
	p := store.ReportsParams{WorkspaceID: ws.ID, Viewer: viewer(r)}
	for _, k := range []string{"from", "to", "assignee", "type"} {
		if len(q[k]) > 1 {
			writeError(w, http.StatusBadRequest, "bad_request", k+" may be given only once")
			return p, false
		}
	}
	p.From, p.To = q.Get("from"), q.Get("to")
	switch {
	case p.From == "" && p.To == "":
		loc, err := time.LoadLocation(ws.Timezone)
		if err != nil {
			loc = time.UTC
		}
		now := time.Now().In(loc)
		p.From = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		p.To = now.Format(time.DateOnly)
	case p.From == "" || p.To == "":
		writeError(w, http.StatusBadRequest, "bad_request", "give both from and to, or neither")
		return p, false
	}
	for _, raw := range q["projects"] {
		for part := range strings.SplitSeq(raw, ",") {
			if k := strings.TrimSpace(part); k != "" {
				p.Projects = append(p.Projects, k)
			}
		}
	}
	switch v := q.Get("assignee"); v {
	case "":
	case "me":
		id := principal(r).UserID
		p.AssigneeID = &id
	case "none":
		p.Unassigned = true
	default:
		id, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "assignee must be a user id, 'me' or 'none'")
			return p, false
		}
		p.AssigneeID = &id
	}
	p.TypeKey = q.Get("type")
	return p, true
}

func (a *api) reportsAllowed(w http.ResponseWriter, r *http.Request) bool {
	if ok, wait := a.reportsLimiter.Allow(principal(r).UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many report requests; try again in a moment")
		return false
	}
	return true
}

// reports returns every widget's data in one response.
func (a *api) reports(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	p, ok := parseReportsParams(w, r, q)
	if !ok || !a.reportsAllowed(w, r) {
		return
	}
	rep, err := a.store.ReportsFor(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	// Short and private: reloading or going back within a minute need not
	// rerun the batch. Vary keeps a cached answer from reaching another
	// user signed in on the same browser.
	h := w.Header()
	h.Set("Cache-Control", "private, max-age=60")
	h.Set("Vary", "Authorization")
	writeJSON(w, http.StatusOK, rep)
}

// reportWorkloadCSV downloads the workload table as CSV (ADR-0040 writer:
// BOM, formula neutralization, optional semicolon). Same parameters as
// /reports plus lang and delimiter; limited and audited like item exports.
func (a *api) reportWorkloadCSV(w http.ResponseWriter, r *http.Request) {
	q, ok := strictQuery(w, r)
	if !ok {
		return
	}
	p, ok := parseReportsParams(w, r, q)
	if !ok {
		return
	}
	for _, k := range []string{"lang", "delimiter"} {
		if len(q[k]) > 1 {
			writeError(w, http.StatusBadRequest, "bad_request", k+" may be given only once")
			return
		}
	}
	lang := q.Get("lang")
	if lang == "" {
		lang = "az"
	}
	words, ok := reportWords[lang]
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "lang must be az, en, tr or ru")
		return
	}
	semicolon := false
	switch q.Get("delimiter") {
	case "", "comma":
	case "semicolon":
		semicolon = true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "delimiter must be comma or semicolon")
		return
	}
	me := principal(r)
	if ok, wait := a.exports.limiter.Allow(me.UserID.String()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait.Seconds()+0.999))))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many exports; try again later")
		return
	}
	ctx := r.Context()
	rep, err := a.store.ReportsFor(ctx, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	ws := workspaceFrom(r)
	assignee := "all"
	switch {
	case p.Unassigned:
		assignee = "none"
	case p.AssigneeID != nil:
		assignee = "user"
	}
	payload := map[string]any{"report": "workload", "format": "csv", "rows": len(rep.Workload), "language": lang,
		"from": p.From, "to": p.To, "projects": len(p.Projects), "assignee": assignee, "type": p.TypeKey != "",
		"delimiter": map[bool]string{false: "comma", true: "semicolon"}[semicolon]}
	if err := a.store.RecordReportExport(ctx, ws.ID, me.UserID, payload); err != nil {
		a.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", exportDisposition(ws.Slug+"-workload", ws.Slug+"-workload", p.From+"_"+p.To, "csv"))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	out, err := tableexport.NewCSV(w, tableexport.CSVOptions{Semicolon: semicolon})
	if err == nil {
		err = out.Header([]tableexport.Column{{Header: words.assignee}, {Header: words.open, Kind: tableexport.Number},
			{Header: words.overdue, Kind: tableexport.Number}, {Header: words.completed, Kind: tableexport.Number}})
	}
	for _, row := range rep.Workload {
		if err != nil {
			break
		}
		name := words.unassigned
		if row.Name != nil {
			name = *row.Name
		}
		err = out.Row([]tableexport.Cell{tableexport.TextCell(name), tableexport.NumberCell(float64(row.Open)),
			tableexport.NumberCell(float64(row.Overdue)), tableexport.NumberCell(float64(row.Completed))})
	}
	if err == nil {
		err = out.Close()
	}
	if err != nil {
		a.logger.WarnContext(ctx, "workload export failed mid-stream", "error", err)
		panic(http.ErrAbortHandler)
	}
}
