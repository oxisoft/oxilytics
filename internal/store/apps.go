package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

const appCols = `id, store, store_app_id, name, bundle_id, platform, icon_url, product_id, suggested_product_id,
 rating_avg, rating_count, rating_updated_at, ignored_at, ignored_by, ignored_reason, first_seen_at, last_synced_at`

func scanApp(sc interface{ Scan(...any) error }) (*models.App, error) {
	var a models.App
	var bundle, icon, ratingUpd, ignAt, ignReason, firstSeen, lastSynced sql.NullString
	var prod, sugg, ratingCount, ignBy sql.NullInt64
	var ratingAvg sql.NullFloat64
	if err := sc.Scan(&a.ID, &a.Store, &a.StoreAppID, &a.Name, &bundle, &a.Platform, &icon, &prod, &sugg,
		&ratingAvg, &ratingCount, &ratingUpd, &ignAt, &ignBy, &ignReason, &firstSeen, &lastSynced); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.BundleID, a.IconURL = nullStr(bundle), nullStr(icon)
	a.ProductID, a.SuggestedProductID = nullInt(prod), nullInt(sugg)
	a.RatingAvg, a.RatingCount = nullFloat(ratingAvg), nullInt(ratingCount)
	a.RatingUpdatedAt = parseTimePtr(ratingUpd)
	a.IgnoredAt, a.IgnoredBy, a.IgnoredReason = parseTimePtr(ignAt), nullInt(ignBy), nullStr(ignReason)
	a.FirstSeenAt = parseTime(firstSeen.String)
	a.LastSyncedAt = parseTimePtr(lastSynced)
	return &a, nil
}

// UpsertApp inserts a discovered store app or refreshes name/bundle/icon.
// Ignored apps stay ignored. Returns the row and whether it was newly created.
func (d *DB) UpsertApp(ctx context.Context, a *models.App) (*models.App, bool, error) {
	existing, err := d.GetAppByStoreID(ctx, a.Store, a.StoreAppID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	if existing != nil {
		name := existing.Name
		if a.Name != "" && a.Name != a.StoreAppID {
			name = a.Name
		}
		icon := existing.IconURL
		if a.IconURL != nil && *a.IconURL != "" {
			icon = a.IconURL
		}
		bundle := existing.BundleID
		if a.BundleID != nil {
			bundle = a.BundleID
		}
		_, err := d.w.ExecContext(ctx, `UPDATE apps SET name=?, bundle_id=?, icon_url=?, platform=? WHERE id=?`, name, bundle, icon, a.Platform, existing.ID)
		if err != nil {
			return nil, false, err
		}
		fresh, err := d.GetApp(ctx, existing.ID)
		return fresh, false, err
	}
	ts := fmtTime(now())
	res, err := d.w.ExecContext(ctx, `INSERT INTO apps(store,store_app_id,name,bundle_id,platform,icon_url,first_seen_at) VALUES(?,?,?,?,?,?,?)`,
		a.Store, a.StoreAppID, a.Name, a.BundleID, a.Platform, a.IconURL, ts)
	if err != nil {
		return nil, false, err
	}
	id, _ := res.LastInsertId()
	fresh, err := d.GetApp(ctx, id)
	return fresh, true, err
}

func (d *DB) GetApp(ctx context.Context, id int64) (*models.App, error) {
	return scanApp(d.r.QueryRowContext(ctx, `SELECT `+appCols+` FROM apps WHERE id=?`, id))
}

func (d *DB) GetAppByStoreID(ctx context.Context, st models.Store, storeAppID string) (*models.App, error) {
	return scanApp(d.r.QueryRowContext(ctx, `SELECT `+appCols+` FROM apps WHERE store=? AND store_app_id=?`, st, storeAppID))
}

type AppFilter struct {
	Store      models.Store
	Platform   models.Platform
	ProductID  *int64
	Unassigned bool
	Ignored    bool // true = only ignored; false = only active
}

func (d *DB) ListApps(ctx context.Context, f AppFilter) ([]models.App, error) {
	var where []string
	var args []any
	if f.Ignored {
		where = append(where, "ignored_at IS NOT NULL")
	} else {
		where = append(where, "ignored_at IS NULL")
	}
	if f.Store != "" {
		where = append(where, "store=?")
		args = append(args, f.Store)
	}
	if f.Platform != "" {
		where = append(where, "platform=?")
		args = append(args, f.Platform)
	}
	if f.ProductID != nil {
		where = append(where, "product_id=?")
		args = append(args, *f.ProductID)
	}
	if f.Unassigned {
		where = append(where, "product_id IS NULL")
	}
	q := `SELECT ` + appCols + ` FROM apps WHERE ` + strings.Join(where, " AND ") + ` ORDER BY name, store`
	rows, err := d.r.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (d *DB) UpdateAppMeta(ctx context.Context, id int64, name string, iconURL *string) error {
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET name=?, icon_url=? WHERE id=?`, name, iconURL, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) UpdateAppRating(ctx context.Context, id int64, avg *float64, count *int64) error {
	_, err := d.w.ExecContext(ctx, `UPDATE apps SET rating_avg=?, rating_count=?, rating_updated_at=? WHERE id=?`, avg, count, fmtTime(now()), id)
	return err
}

func (d *DB) TouchAppSynced(ctx context.Context, id int64) error {
	_, err := d.w.ExecContext(ctx, `UPDATE apps SET last_synced_at=? WHERE id=?`, fmtTime(now()), id)
	return err
}

// IgnoreApp unlinks from its product and marks ignored. by=0 → NULL.
func (d *DB) IgnoreApp(ctx context.Context, id, by int64, reason *string) error {
	var byArg any
	if by > 0 {
		byArg = by
	}
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET product_id=NULL, suggested_product_id=NULL, ignored_at=?, ignored_by=?, ignored_reason=? WHERE id=? AND ignored_at IS NULL`, fmtTime(now()), byArg, reason, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) RestoreApp(ctx context.Context, id int64) error {
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET ignored_at=NULL, ignored_by=NULL, ignored_reason=NULL WHERE id=? AND ignored_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) UpdateAppIgnoreReason(ctx context.Context, id int64, reason *string) error {
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET ignored_reason=? WHERE id=? AND ignored_at IS NOT NULL`, reason, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// LinkApp attaches an app to a product (one per platform).
func (d *DB) LinkApp(ctx context.Context, appID, productID int64) error {
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET product_id=?, suggested_product_id=NULL WHERE id=? AND ignored_at IS NULL`, productID, appID)
	if err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	return affected(res)
}

func (d *DB) UnlinkApp(ctx context.Context, appID int64) error {
	res, err := d.w.ExecContext(ctx, `UPDATE apps SET product_id=NULL WHERE id=?`, appID)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) SetAppSuggestion(ctx context.Context, appID int64, productID *int64) error {
	_, err := d.w.ExecContext(ctx, `UPDATE apps SET suggested_product_id=? WHERE id=? AND product_id IS NULL`, productID, appID)
	return err
}

// ignoredAppRow is ListIgnoredApps' richer row.
type IgnoredAppRow struct {
	models.App
	IgnoredByName *string `json:"ignored_by_name"`
	LastDataDay   *string `json:"last_data_day"`
	MetricRows    int64   `json:"metric_rows"`
	ReviewRows    int64   `json:"review_rows"`
}

func (d *DB) ListIgnoredApps(ctx context.Context) ([]IgnoredAppRow, error) {
	rows, err := d.r.QueryContext(ctx, `SELECT `+prefixCols("a.", appCols)+`, u.name,
		(SELECT MAX(day) FROM metric_days m WHERE m.app_id=a.id),
		(SELECT COUNT(*) FROM metric_days m WHERE m.app_id=a.id),
		(SELECT COUNT(*) FROM reviews r WHERE r.app_id=a.id)
		FROM apps a LEFT JOIN users u ON u.id=a.ignored_by WHERE a.ignored_at IS NOT NULL ORDER BY a.ignored_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IgnoredAppRow
	for rows.Next() {
		var r IgnoredAppRow
		var bundle, icon, ratingUpd, ignAt, ignReason, firstSeen, lastSynced, byName, lastDay sql.NullString
		var prod, sugg, ratingCount, ignBy sql.NullInt64
		var ratingAvg sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Store, &r.StoreAppID, &r.Name, &bundle, &r.Platform, &icon, &prod, &sugg,
			&ratingAvg, &ratingCount, &ratingUpd, &ignAt, &ignBy, &ignReason, &firstSeen, &lastSynced,
			&byName, &lastDay, &r.MetricRows, &r.ReviewRows); err != nil {
			return nil, err
		}
		r.BundleID, r.IconURL = nullStr(bundle), nullStr(icon)
		r.IgnoredAt, r.IgnoredBy, r.IgnoredReason = parseTimePtr(ignAt), nullInt(ignBy), nullStr(ignReason)
		r.FirstSeenAt = parseTime(firstSeen.String)
		r.IgnoredByName, r.LastDataDay = nullStr(byName), nullStr(lastDay)
		out = append(out, r)
	}
	return out, rows.Err()
}

func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}
