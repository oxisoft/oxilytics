package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

// Totals is one aggregate row.
type Totals struct {
	Downloads   int64 `json:"downloads"`
	Redownloads int64 `json:"redownloads"`
	Updates     int64 `json:"updates"`
	Uninstalls  int64 `json:"uninstalls"`
	Crashes     int64 `json:"crashes"`
	ANRs        int64 `json:"anrs"`
}

const totalsSel = `COALESCE(SUM(m.downloads),0), COALESCE(SUM(m.redownloads),0), COALESCE(SUM(m.updates),0), COALESCE(SUM(m.uninstalls),0), COALESCE(SUM(m.crashes),0), COALESCE(SUM(m.anrs),0)`

// SumTotals sums the "*" country rows in [from,to] under scope.
func (d *DB) SumTotals(ctx context.Context, sc Scope, from, to string) (Totals, error) {
	where, args := sc.where("a")
	q := `SELECT ` + totalsSel + ` FROM metric_days m JOIN apps a ON a.id=m.app_id WHERE m.country='*' AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ")
	var t Totals
	err := d.r.QueryRowContext(ctx, q, append([]any{from, to}, args...)...).Scan(&t.Downloads, &t.Redownloads, &t.Updates, &t.Uninstalls, &t.Crashes, &t.ANRs)
	return t, err
}

// SumTotalsByPlatform returns totals keyed by platform.
func (d *DB) SumTotalsByPlatform(ctx context.Context, sc Scope, from, to string) (map[string]Totals, error) {
	where, args := sc.where("a")
	q := `SELECT a.platform, ` + totalsSel + ` FROM metric_days m JOIN apps a ON a.id=m.app_id WHERE m.country='*' AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ") + ` GROUP BY a.platform`
	rows, err := d.r.QueryContext(ctx, q, append([]any{from, to}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Totals{}
	for rows.Next() {
		var p string
		var t Totals
		if err := rows.Scan(&p, &t.Downloads, &t.Redownloads, &t.Updates, &t.Uninstalls, &t.Crashes, &t.ANRs); err != nil {
			return nil, err
		}
		out[p] = t
	}
	return out, rows.Err()
}

// Series returns bucketed values for one metric grouped by a dimension.
type SeriesPoint struct {
	Bucket string
	Key    string
	Label  string
	Value  int64
}

var metricCol = map[string]string{
	"downloads": "m.downloads", "redownloads": "m.redownloads", "updates": "m.updates", "uninstalls": "m.uninstalls",
	"crashes": "m.crashes", "anrs": "m.anrs", "active_devices": "m.active_devices",
}

func ValidMetric(m string) bool { _, ok := metricCol[m]; return ok }

func bucketExpr(bucket string) string {
	switch bucket {
	case "week":
		// ISO-ish week starting Monday
		return `date(m.day, '-' || ((strftime('%w', m.day) + 6) % 7) || ' days')`
	case "month":
		return `substr(m.day, 1, 7) || '-01'`
	}
	return "m.day"
}

func (d *DB) Series(ctx context.Context, sc Scope, from, to, metric, group, bucket string) ([]SeriesPoint, error) {
	col, ok := metricCol[metric]
	if !ok {
		return nil, fmt.Errorf("unknown metric %q", metric)
	}
	agg := "SUM(" + col + ")"
	if metric == "active_devices" {
		agg = "MAX(" + col + ")" // snapshot, not summable across days
	}
	var keyExpr, labelExpr, join string
	country := "m.country='*'"
	switch group {
	case "product":
		keyExpr, labelExpr = "COALESCE(a.product_id, 0)", "COALESCE(p.name, 'Unassigned')"
		join = " LEFT JOIN products p ON p.id=a.product_id"
	case "platform":
		keyExpr, labelExpr = "a.platform", "a.platform"
	case "store":
		keyExpr, labelExpr = "a.store", "a.store"
	case "app":
		keyExpr, labelExpr = "a.id", "a.name"
	case "country":
		keyExpr, labelExpr = "m.country", "m.country"
		country = "m.country<>'*'"
	default:
		keyExpr, labelExpr = "'total'", "'Total'"
	}
	where, args := sc.where("a")
	q := `SELECT ` + bucketExpr(bucket) + ` AS b, ` + keyExpr + `, ` + labelExpr + `, COALESCE(` + agg + `,0)
		FROM metric_days m JOIN apps a ON a.id=m.app_id` + join + `
		WHERE ` + country + ` AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ") + `
		GROUP BY b, ` + keyExpr + ` ORDER BY b`
	rows, err := d.r.QueryContext(ctx, q, append([]any{from, to}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesPoint
	for rows.Next() {
		var p SeriesPoint
		var key any
		if err := rows.Scan(&p.Bucket, &key, &p.Label, &p.Value); err != nil {
			return nil, err
		}
		p.Key = fmt.Sprint(key)
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountryRow for the top-countries breakdown.
type CountryRow struct {
	Country string `json:"country"`
	Value   int64  `json:"value"`
}

func (d *DB) TopCountries(ctx context.Context, sc Scope, from, to, metric string, limit int) ([]CountryRow, int64, error) {
	col, ok := metricCol[metric]
	if !ok {
		return nil, 0, fmt.Errorf("unknown metric %q", metric)
	}
	where, args := sc.where("a")
	q := `SELECT m.country, COALESCE(SUM(` + col + `),0) v FROM metric_days m JOIN apps a ON a.id=m.app_id
		WHERE m.country<>'*' AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ") + ` GROUP BY m.country ORDER BY v DESC LIMIT ?`
	rows, err := d.r.QueryContext(ctx, q, append(append([]any{from, to}, args...), limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []CountryRow
	var sum int64
	for rows.Next() {
		var r CountryRow
		if err := rows.Scan(&r.Country, &r.Value); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
		sum += r.Value
	}
	return out, sum, rows.Err()
}

// ProductRow feeds the dashboard products table.
type ProductRow struct {
	Product    models.Product     `json:"product"`
	Apps       []models.App       `json:"apps"`
	Totals     Totals             `json:"totals"`
	Prev       Totals             `json:"prev"`
	ByPlatform map[string]Totals  `json:"by_platform"`
	LastReview *string            `json:"last_review_at"`
}

func (d *DB) ProductRows(ctx context.Context, from, to, prevFrom, prevTo string, includeArchived bool) ([]ProductRow, error) {
	products, err := d.ListProducts(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	apps, err := d.ListApps(ctx, AppFilter{})
	if err != nil {
		return nil, err
	}
	byProduct := map[int64][]models.App{}
	for _, a := range apps {
		if a.ProductID != nil {
			byProduct[*a.ProductID] = append(byProduct[*a.ProductID], a)
		}
	}
	out := make([]ProductRow, 0, len(products))
	for _, p := range products {
		sc := Scope{ProductID: &p.ID}
		cur, err := d.SumTotals(ctx, sc, from, to)
		if err != nil {
			return nil, err
		}
		prev, err := d.SumTotals(ctx, sc, prevFrom, prevTo)
		if err != nil {
			return nil, err
		}
		byPlat, err := d.SumTotalsByPlatform(ctx, sc, from, to)
		if err != nil {
			return nil, err
		}
		var last sql.NullString
		_ = d.r.QueryRowContext(ctx, `SELECT MAX(r.created_at) FROM reviews r JOIN apps a ON a.id=r.app_id WHERE a.product_id=? AND a.ignored_at IS NULL`, p.ID).Scan(&last)
		row := ProductRow{Product: p, Apps: byProduct[p.ID], Totals: cur, Prev: prev, ByPlatform: byPlat, LastReview: nullStr(last)}
		if row.Apps == nil {
			row.Apps = []models.App{}
		}
		out = append(out, row)
	}
	return out, nil
}

// DayRow is one row of the per-day table export.
type DayRow struct {
	Day      string `json:"day"`
	Platform string `json:"platform"`
	Totals
	ActiveDevices *int64 `json:"active_devices"`
}

func (d *DB) DayRows(ctx context.Context, sc Scope, from, to string) ([]DayRow, error) {
	where, args := sc.where("a")
	q := `SELECT m.day, a.platform, ` + totalsSel + `, MAX(m.active_devices) FROM metric_days m JOIN apps a ON a.id=m.app_id
		WHERE m.country='*' AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ") + ` GROUP BY m.day, a.platform ORDER BY m.day DESC, a.platform`
	rows, err := d.r.QueryContext(ctx, q, append([]any{from, to}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayRow
	for rows.Next() {
		var r DayRow
		var ad sql.NullInt64
		if err := rows.Scan(&r.Day, &r.Platform, &r.Downloads, &r.Redownloads, &r.Updates, &r.Uninstalls, &r.Crashes, &r.ANRs, &ad); err != nil {
			return nil, err
		}
		r.ActiveDevices = nullInt(ad)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountryRows: per-country per-platform totals for the Countries tab.
type CountryPlatformRow struct {
	Country  string `json:"country"`
	Platform string `json:"platform"`
	Value    int64  `json:"value"`
}

func (d *DB) CountryRows(ctx context.Context, sc Scope, from, to, metric string) ([]CountryPlatformRow, error) {
	col, ok := metricCol[metric]
	if !ok {
		return nil, fmt.Errorf("unknown metric %q", metric)
	}
	where, args := sc.where("a")
	q := `SELECT m.country, a.platform, COALESCE(SUM(` + col + `),0) FROM metric_days m JOIN apps a ON a.id=m.app_id
		WHERE m.country<>'*' AND m.day BETWEEN ? AND ? AND ` + strings.Join(where, " AND ") + ` GROUP BY m.country, a.platform ORDER BY 3 DESC`
	rows, err := d.r.QueryContext(ctx, q, append([]any{from, to}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CountryPlatformRow
	for rows.Next() {
		var r CountryPlatformRow
		if err := rows.Scan(&r.Country, &r.Platform, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
