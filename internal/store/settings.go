package store

import (
	"context"
	"database/sql"
)

func (d *DB) GetSettings(ctx context.Context) (map[string]string, error) {
	rows, err := d.r.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (d *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := d.r.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v, err
}

func (d *DB) SetSettings(ctx context.Context, kv map[string]string) error {
	ts := fmtTime(now())
	return d.Tx(ctx, func(tx *sql.Tx) error {
		for k, v := range kv {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, k, v, ts); err != nil {
				return err
			}
		}
		return nil
	})
}
