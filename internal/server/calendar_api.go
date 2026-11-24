package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Tural-HS/Nebula/internal/store"
)

// calendarRoutes: the workspace's working-day calendar (ADR-0027). Every
// member (guests too) reads it: the calendar view shades holidays. Changing
// it is admin+, because it moves the dates of recurring items.
func (a *api) calendarRoutes(r chi.Router) {
	admin := r.With(requireRole(store.RoleAdmin))
	r.Get("/calendar", a.getCalendar)
	admin.Put("/calendar/weekend", a.setWeekend)
	admin.Put("/calendar/days/{date}", a.setCalendarDay)
	admin.Delete("/calendar/days/{date}", a.deleteCalendarDay)
	admin.Post("/calendar/presets/az-fixed", a.addAzerbaijanHolidays)
}

// getCalendar: ?year=YYYY, or ?from=&to= (YYYY-MM-DD, inclusive, at most 400 days).
func (a *api) getCalendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for _, k := range []string{"year", "from", "to"} {
		if len(q[k]) > 1 {
			writeError(w, http.StatusBadRequest, "bad_request", k+" may be given only once")
			return
		}
	}
	var from, to time.Time
	switch year, f, t := q.Get("year"), q.Get("from"), q.Get("to"); {
	case year != "" && f == "" && t == "":
		y, err := store.ParseCalendarYear(year)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		from, to = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
	case year == "" && f != "" && t != "":
		var err1, err2 error
		from, err1 = time.Parse(time.DateOnly, f)
		to, err2 = time.Parse(time.DateOnly, t)
		if err1 != nil || err2 != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "from and to must be dates YYYY-MM-DD")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "give either year or both from and to")
		return
	}
	cal, err := a.store.GetCalendar(r.Context(), workspaceFrom(r).ID, from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cal)
}

func (a *api) setWeekend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WeekendDays []int `json:"weekend_days"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	ws := workspaceFrom(r)
	if err := a.store.SetWeekend(r.Context(), ws.ID, principal(r).UserID, body.WeekendDays); err != nil {
		a.fail(w, r, err)
		return
	}
	today := time.Now().UTC()
	cal, err := a.store.GetCalendar(r.Context(), ws.ID, today, today)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"weekend_days": cal.WeekendDays})
}

func (a *api) setCalendarDay(w http.ResponseWriter, r *http.Request) {
	var body store.CalendarDayInput
	if !a.decode(w, r, &body) {
		return
	}
	day, err := a.store.SetCalendarDay(r.Context(), workspaceFrom(r).ID, principal(r).UserID, chi.URLParam(r, "date"), body)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, day)
}

func (a *api) deleteCalendarDay(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteCalendarDay(r.Context(), workspaceFrom(r).ID, principal(r).UserID, chi.URLParam(r, "date")); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) addAzerbaijanHolidays(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Year int `json:"year"`
	}
	if !a.decode(w, r, &body) {
		return
	}
	added, err := a.store.AddAzerbaijanFixedHolidays(r.Context(), workspaceFrom(r).ID, principal(r).UserID, body.Year)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}
