package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
)

const runCols = `s.id, s.store, s.mode, s.trigger, s.requested_by, s.status, s.started_at, s.finished_at, s.range_from, s.range_to,
 s.apps_total, s.apps_done, s.rows_metrics, s.rows_reviews, s.error, s.stats, s.created_at, u.name`

func scanRun(sc interface{ Scan(...any) error }) (*models.SyncRun, error) {
	var r models.SyncRun
	var reqBy sql.NullInt64
	var started, finished, from, to, errS, stats, created, uname sql.NullString
	if err := sc.Scan(&r.ID, &r.Store, &r.Mode, &r.Trigger, &reqBy, &r.Status, &started, &finished, &from, &to,
		&r.AppsTotal, &r.AppsDone, &r.RowsMetrics, &r.RowsReviews, &errS, &stats, &created, &uname); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.RequestedBy = nullInt(reqBy)
	r.StartedAt, r.FinishedAt = parseTimePtr(started), parseTimePtr(finished)
	r.RangeFrom, r.RangeTo, r.Error, r.Stats = nullStr(from), nullStr(to), nullStr(errS), nullStr(stats)
	r.CreatedAt = parseTime(created.String)
	r.RequestedByName = nullStr(uname)
	return &r, nil
}

func (d *DB) CreateSyncRun(ctx context.Context, r *models.SyncRun) error {
	ts := now()
	res, err := d.w.ExecContext(ctx, `INSERT INTO sync_runs(store,mode,trigger,requested_by,status,created_at) VALUES(?,?,?,?,?,?)`,
		r.Store, r.Mode, r.Trigger, r.RequestedBy, models.SyncQueued, fmtTime(ts))
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	r.Status = models.SyncQueued
	r.CreatedAt = ts
	return nil
}

func (d *DB) GetSyncRun(ctx context.Context, id int64) (*models.SyncRun, error) {
	return scanRun(d.r.QueryRowContext(ctx, `SELECT `+runCols+` FROM sync_runs s LEFT JOIN users u ON u.id=s.requested_by WHERE s.id=?`, id))
}

func (d *DB) ListSyncRuns(ctx context.Context, st models.Store, limit, offset int) ([]models.SyncRun, error) {
	q := `SELECT ` + runCols + ` FROM sync_runs s LEFT JOIN users u ON u.id=s.requested_by`
	var args []any
	if st != "" {
		q += ` WHERE s.store=?`
		args = append(args, st)
	}
	q += ` ORDER BY s.id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := d.r.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SyncRun
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (d *DB) LastSyncRun(ctx context.Context, st models.Store) (*models.SyncRun, error) {
	return scanRun(d.r.QueryRowContext(ctx, `SELECT `+runCols+` FROM sync_runs s LEFT JOIN users u ON u.id=s.requested_by WHERE s.store=? AND s.status NOT IN ('queued','running') ORDER BY s.id DESC LIMIT 1`, st))
}

func (d *DB) StartSyncRun(ctx context.Context, id int64, from, to string, appsTotal int) error {
	_, err := d.w.ExecContext(ctx, `UPDATE sync_runs SET status=?, started_at=?, range_from=?, range_to=?, apps_total=? WHERE id=?`,
		models.SyncRunning, fmtTime(now()), from, to, appsTotal, id)
	return err
}

func (d *DB) UpdateSyncProgress(ctx context.Context, id int64, appsDone int, rowsMetrics, rowsReviews int64) error {
	_, err := d.w.ExecContext(ctx, `UPDATE sync_runs SET apps_done=?, rows_metrics=?, rows_reviews=? WHERE id=?`, appsDone, rowsMetrics, rowsReviews, id)
	return err
}

func (d *DB) FinishSyncRun(ctx context.Context, id int64, status models.SyncStatus, errMsg *string, stats *string) error {
	_, err := d.w.ExecContext(ctx, `UPDATE sync_runs SET status=?, finished_at=?, error=?, stats=? WHERE id=?`, status, fmtTime(now()), errMsg, stats, id)
	return err
}

// MarkInterruptedRuns is called at startup.
func (d *DB) MarkInterruptedRuns(ctx context.Context) (int64, error) {
	msg := "process restarted while the run was in progress"
	res, err := d.w.ExecContext(ctx, `UPDATE sync_runs SET status=?, finished_at=?, error=? WHERE status IN ('queued','running')`, models.SyncInterrupted, fmtTime(now()), msg)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) PruneSyncRuns(ctx context.Context, olderThan time.Time) error {
	_, err := d.w.ExecContext(ctx, `DELETE FROM sync_runs WHERE created_at < ? AND status NOT IN ('queued','running')`, fmtTime(olderThan))
	return err
}

// logs -----------------------------------------------------------------------

func (d *DB) AddSyncLog(ctx context.Context, runID int64, level string, appID *int64, msg string) error {
	_, err := d.w.ExecContext(ctx, `INSERT INTO sync_run_logs(run_id,ts,level,app_id,message) VALUES(?,?,?,?,?)`, runID, fmtTime(now()), level, appID, msg)
	return err
}

func (d *DB) ListSyncLogs(ctx context.Context, runID, afterID int64, limit int) ([]models.SyncRunLog, error) {
	rows, err := d.r.QueryContext(ctx, `SELECT id, run_id, ts, level, app_id, message FROM sync_run_logs WHERE run_id=? AND id>? ORDER BY id LIMIT ?`, runID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SyncRunLog
	for rows.Next() {
		var l models.SyncRunLog
		var ts string
		var app sql.NullInt64
		if err := rows.Scan(&l.ID, &l.RunID, &ts, &l.Level, &app, &l.Message); err != nil {
			return nil, err
		}
		l.TS = parseTime(ts)
		l.AppID = nullInt(app)
		out = append(out, l)
	}
	return out, rows.Err()
}

// checkpoints ----------------------------------------------------------------

func (d *DB) GetCheckpoint(ctx context.Context, st models.Store, source string, appID int64) (string, error) {
	var c string
	err := d.r.QueryRowContext(ctx, `SELECT cursor FROM sync_checkpoints WHERE store=? AND source=? AND app_id=?`, st, source, appID).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return c, err
}

func (d *DB) SetCheckpoint(ctx context.Context, tx *sql.Tx, st models.Store, source string, appID int64, cursor string) error {
	exec := d.w.ExecContext
	if tx != nil {
		exec = tx.ExecContext
	}
	_, err := exec(ctx, `INSERT INTO sync_checkpoints(store,source,app_id,cursor,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(store,source,app_id) DO UPDATE SET cursor=excluded.cursor, updated_at=excluded.updated_at`,
		st, source, appID, cursor, fmtTime(now()))
	return err
}

// ingested objects (Google Play) ---------------------------------------------

func (d *DB) IngestedGeneration(ctx context.Context, st models.Store, name string) (string, error) {
	var g string
	err := d.r.QueryRowContext(ctx, `SELECT generation FROM ingested_objects WHERE store=? AND object_name=?`, st, name).Scan(&g)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return g, err
}

func (d *DB) MarkIngested(ctx context.Context, tx *sql.Tx, st models.Store, name, generation, md5 string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO ingested_objects(store,object_name,generation,md5,ingested_at) VALUES(?,?,?,?,?) ON CONFLICT(store,object_name) DO UPDATE SET generation=excluded.generation, md5=excluded.md5, ingested_at=excluded.ingested_at`,
		st, name, generation, md5, fmtTime(now()))
	return err
}
