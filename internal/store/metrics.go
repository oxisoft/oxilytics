package store

import (
	"context"
	"database/sql"

	"github.com/oxisoft/oxilytics/internal/models"
)

// UpsertMetricDays merges rows: fields given as non-zero/non-nil overwrite;
// the caller supplies whole rows per (app, day, country) for one source, so
// we use per-column COALESCE-style updates to let different sources
// (downloads vs crashes) coexist in the same row.
func (d *DB) UpsertMetricDays(ctx context.Context, tx *sql.Tx, rows []models.MetricDay, cols MetricCols) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	set := ""
	sep := ""
	add := func(c string) {
		set += sep + c + "=excluded." + c
		sep = ", "
	}
	if cols.Downloads {
		add("downloads")
		add("redownloads")
		add("updates")
	}
	if cols.Uninstalls {
		add("uninstalls")
	}
	if cols.ActiveDevices {
		add("active_devices")
	}
	if cols.Crashes {
		add("crashes")
		add("anrs")
	}
	add("source_updated_at")
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO metric_days(app_id,day,country,downloads,redownloads,updates,uninstalls,active_devices,crashes,anrs,source_updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(app_id,day,country) DO UPDATE SET `+set)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var n int64
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.AppID, r.Day, r.Country, r.Downloads, r.Redownloads, r.Updates, r.Uninstalls, r.ActiveDevices, r.Crashes, r.ANRs, r.SourceUpdatedAt); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// MetricCols says which column groups an upsert batch is authoritative for.
type MetricCols struct {
	Downloads     bool
	Uninstalls    bool
	ActiveDevices bool
	Crashes       bool
}

// DeleteStoreData wipes metrics, reviews, checkpoints and ingested objects for a store.
func (d *DB) DeleteStoreData(ctx context.Context, st models.Store) error {
	return d.Tx(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM metric_days WHERE app_id IN (SELECT id FROM apps WHERE store=?)`,
			`DELETE FROM reviews WHERE app_id IN (SELECT id FROM apps WHERE store=?)`,
			`DELETE FROM sync_checkpoints WHERE store=?`,
			`DELETE FROM ingested_objects WHERE store=?`,
			`UPDATE apps SET rating_avg=NULL, rating_count=NULL, rating_updated_at=NULL, last_synced_at=NULL WHERE store=?`,
		} {
			if _, err := tx.ExecContext(ctx, q, st); err != nil {
				return err
			}
		}
		return nil
	})
}

// PruneMetrics deletes metric rows older than `beforeDay`.
func (d *DB) PruneMetrics(ctx context.Context, beforeDay string) (int64, error) {
	res, err := d.w.ExecContext(ctx, `DELETE FROM metric_days WHERE day < ?`, beforeDay)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
