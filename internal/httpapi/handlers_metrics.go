package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/permissions"
	"github.com/oxisoft/oxilytics/internal/store"
)

// scope + range parsing ------------------------------------------------------

func scopeParams(r *http.Request) store.Scope {
	q := r.URL.Query()
	var sc store.Scope
	if v := q.Get("product_id"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		sc.ProductID = &id
	}
	if v := q.Get("app_id"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		sc.AppID = &id
	}
	if v := q.Get("platform"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				sc.Platforms = append(sc.Platforms, models.Platform(p))
			}
		}
	}
	if v := models.Store(q.Get("store")); v.Valid() {
		sc.Store = v
	}
	return sc
}

// rangeParams returns from/to (default last 30 days ending yesterday) and the
// equally long preceding period.
func rangeParams(r *http.Request) (from, to, prevFrom, prevTo string) {
	q := r.URL.Query()
	to = q.Get("to")
	from = q.Get("from")
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	if !validDay(to) {
		to = yesterday.Format("2006-01-02")
	}
	if !validDay(from) {
		t, _ := time.Parse("2006-01-02", to)
		from = t.AddDate(0, 0, -29).Format("2006-01-02")
	}
	if from > to {
		from, to = to, from
	}
	f, _ := time.Parse("2006-01-02", from)
	t, _ := time.Parse("2006-01-02", to)
	days := int(t.Sub(f).Hours()/24) + 1
	prevTo = f.AddDate(0, 0, -1).Format("2006-01-02")
	prevFrom = f.AddDate(0, 0, -days).Format("2006-01-02")
	return
}

func validDay(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// metrics --------------------------------------------------------------------

func (a *API) mountMetrics(r chi.Router) {
	r.Get("/metrics/summary", a.metricsSummary)
	r.Get("/metrics/series", a.metricsSeries)
	r.Get("/metrics/countries", a.metricsCountries)
	r.Get("/metrics/days", a.metricsDays)
	r.Get("/metrics/export.csv", a.metricsExport)
}

type summaryOut struct {
	From       string                  `json:"from"`
	To         string                  `json:"to"`
	Totals     store.Totals            `json:"totals"`
	Prev       store.Totals            `json:"prev"`
	ByPlatform map[string]store.Totals `json:"by_platform"`
	Reviews    *store.ReviewStats      `json:"reviews"`
}

func (a *API) metricsSummary(w http.ResponseWriter, r *http.Request) {
	sc := scopeParams(r)
	from, to, pf, pt := rangeParams(r)
	cur, err := a.DB.SumTotals(r.Context(), sc, from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	prev, err := a.DB.SumTotals(r.Context(), sc, pf, pt)
	if err != nil {
		internalErr(w, err)
		return
	}
	byPlat, err := a.DB.SumTotalsByPlatform(r.Context(), sc, from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	rs, err := a.DB.ReviewStats(r.Context(), store.ReviewFilter{Scope: sc, From: from, To: to})
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, summaryOut{From: from, To: to, Totals: cur, Prev: prev, ByPlatform: byPlat, Reviews: rs})
}

type seriesOut struct {
	Buckets []string      `json:"buckets"`
	Series  []seriesEntry `json:"series"`
}

type seriesEntry struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Values []int64 `json:"values"`
}

func (a *API) metricsSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	metric := q.Get("metric")
	if metric == "" {
		metric = "downloads"
	}
	if !store.ValidMetric(metric) {
		badRequest(w, "unknown metric")
		return
	}
	group := q.Get("group")
	bucket := q.Get("bucket")
	if bucket != "week" && bucket != "month" {
		bucket = "day"
	}
	from, to, _, _ := rangeParams(r)
	pts, err := a.DB.Series(r.Context(), scopeParams(r), from, to, metric, group, bucket)
	if err != nil {
		internalErr(w, err)
		return
	}
	// dense buckets
	buckets := bucketsBetween(from, to, bucket)
	idx := map[string]int{}
	for i, b := range buckets {
		idx[b] = i
	}
	series := map[string]*seriesEntry{}
	var order []string
	for _, p := range pts {
		s, ok := series[p.Key]
		if !ok {
			s = &seriesEntry{Key: p.Key, Label: p.Label, Values: make([]int64, len(buckets))}
			series[p.Key] = s
			order = append(order, p.Key)
		}
		if i, ok := idx[p.Bucket]; ok {
			s.Values[i] += p.Value
		}
	}
	out := seriesOut{Buckets: buckets, Series: []seriesEntry{}}
	for _, k := range order {
		out.Series = append(out.Series, *series[k])
	}
	writeJSON(w, 200, out)
}

func bucketsBetween(from, to, bucket string) []string {
	f, _ := time.Parse("2006-01-02", from)
	t, _ := time.Parse("2006-01-02", to)
	var out []string
	switch bucket {
	case "week":
		// Monday of from
		f = f.AddDate(0, 0, -((int(f.Weekday()) + 6) % 7))
		for d := f; !d.After(t); d = d.AddDate(0, 0, 7) {
			out = append(out, d.Format("2006-01-02"))
		}
	case "month":
		f = time.Date(f.Year(), f.Month(), 1, 0, 0, 0, 0, time.UTC)
		for d := f; !d.After(t); d = d.AddDate(0, 1, 0) {
			out = append(out, d.Format("2006-01-02"))
		}
	default:
		for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
			out = append(out, d.Format("2006-01-02"))
		}
	}
	return out
}

func (a *API) metricsCountries(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "downloads"
	}
	if !store.ValidMetric(metric) {
		badRequest(w, "unknown metric")
		return
	}
	from, to, _, _ := rangeParams(r)
	if r.URL.Query().Get("by_platform") == "1" {
		rows, err := a.DB.CountryRows(r.Context(), scopeParams(r), from, to, metric)
		if err != nil {
			internalErr(w, err)
			return
		}
		if rows == nil {
			rows = []store.CountryPlatformRow{}
		}
		writeJSON(w, 200, rows)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 250 {
		limit = 10
	}
	rows, sum, err := a.DB.TopCountries(r.Context(), scopeParams(r), from, to, metric, limit)
	if err != nil {
		internalErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.CountryRow{}
	}
	writeJSON(w, 200, map[string]any{"rows": rows, "total": sum})
}

func (a *API) metricsDays(w http.ResponseWriter, r *http.Request) {
	from, to, _, _ := rangeParams(r)
	rows, err := a.DB.DayRows(r.Context(), scopeParams(r), from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	if rows == nil {
		rows = []store.DayRow{}
	}
	writeJSON(w, 200, rows)
}

func (a *API) metricsExport(w http.ResponseWriter, r *http.Request) {
	from, to, _, _ := rangeParams(r)
	rows, err := a.DB.DayRows(r.Context(), scopeParams(r), from, to)
	if err != nil {
		internalErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="oxilytics-%s-%s.csv"`, from, to))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"day", "platform", "downloads", "redownloads", "updates", "uninstalls", "crashes", "anrs", "active_devices"})
	for _, d := range rows {
		ad := ""
		if d.ActiveDevices != nil {
			ad = strconv.FormatInt(*d.ActiveDevices, 10)
		}
		_ = cw.Write([]string{d.Day, d.Platform, i64(d.Downloads), i64(d.Redownloads), i64(d.Updates), i64(d.Uninstalls), i64(d.Crashes), i64(d.ANRs), ad})
	}
	cw.Flush()
}

func i64(n int64) string { return strconv.FormatInt(n, 10) }

// reviews --------------------------------------------------------------------

func (a *API) mountReviews(r chi.Router) {
	r.Get("/reviews", a.listReviews)
	r.Get("/reviews/stats", a.reviewStats)
	r.Get("/reviews/export.csv", a.reviewsExport)
	r.Get("/reviews/{id}", a.getReview)
}

func reviewFilter(r *http.Request) store.ReviewFilter {
	q := r.URL.Query()
	f := store.ReviewFilter{Scope: scopeParams(r), Country: strings.ToUpper(q.Get("country")), Query: q.Get("q")}
	f.From, f.To = q.Get("from"), q.Get("to")
	if !validDay(f.From) {
		f.From = ""
	}
	if !validDay(f.To) {
		f.To = ""
	}
	for _, s := range strings.Split(q.Get("rating"), ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 1 && n <= 5 {
			f.Ratings = append(f.Ratings, n)
		}
	}
	switch q.Get("replied") {
	case "1", "true":
		t := true
		f.Replied = &t
	case "0", "false":
		fl := false
		f.Replied = &fl
	}
	f.Page, _ = strconv.Atoi(q.Get("page"))
	f.PerPage, _ = strconv.Atoi(q.Get("per_page"))
	return f
}

func (a *API) listReviews(w http.ResponseWriter, r *http.Request) {
	f := reviewFilter(r)
	rows, total, err := a.DB.ListReviews(r.Context(), f)
	if err != nil {
		internalErr(w, err)
		return
	}
	if rows == nil {
		rows = []models.Review{}
	}
	per := f.PerPage
	if per <= 0 || per > 200 {
		per = 50
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	writeJSON(w, 200, map[string]any{"rows": rows, "total": total, "page": page, "per_page": per})
}

func (a *API) reviewStats(w http.ResponseWriter, r *http.Request) {
	st, err := a.DB.ReviewStats(r.Context(), reviewFilter(r))
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (a *API) getReview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rv, err := a.DB.GetReview(r.Context(), id, permissions.Can(auth.UserFromContext(r.Context()), permissions.ViewIgnoredApps))
	if err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, rv)
}

func (a *API) reviewsExport(w http.ResponseWriter, r *http.Request) {
	f := reviewFilter(r)
	f.Page, f.PerPage = 1, 200
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="oxilytics-reviews.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"created_at", "app", "store", "platform", "rating", "title", "body", "author", "country", "language", "app_version", "developer_reply"})
	for {
		rows, _, err := a.DB.ListReviews(r.Context(), f)
		if err != nil || len(rows) == 0 {
			break
		}
		for _, rv := range rows {
			_ = cw.Write([]string{rv.CreatedAt.Format(time.RFC3339), rv.AppName, string(rv.Store), string(rv.Platform), strconv.Itoa(rv.Rating), deref(rv.Title), deref(rv.Body), deref(rv.Author), deref(rv.Country), deref(rv.Language), deref(rv.AppVersion), deref(rv.DeveloperReply)})
		}
		if len(rows) < f.PerPage || f.Page > 200 {
			break
		}
		f.Page++
	}
	cw.Flush()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
