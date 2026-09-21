package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
)

const tokenCols = `id, user_id, name, token_hash, prefix, created_at, last_used_at, revoked_at, can_run_sync`

func scanToken(sc interface{ Scan(...any) error }) (*models.APIToken, error) {
	var t models.APIToken
	var created string
	var lastUsed, revoked sql.NullString
	if err := sc.Scan(&t.ID, &t.UserID, &t.Name, &t.Hash, &t.Prefix, &created, &lastUsed, &revoked, &t.CanRunSync); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.CreatedAt = parseTime(created)
	t.LastUsedAt = parseTimePtr(lastUsed)
	t.RevokedAt = parseTimePtr(revoked)
	return &t, nil
}

func (d *DB) CreateAPIToken(ctx context.Context, t *models.APIToken) error {
	now := fmtTime(time.Now())
	res, err := d.w.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, prefix, created_at, can_run_sync) VALUES (?,?,?,?,?,?)`,
		t.UserID, t.Name, t.Hash, t.Prefix, now, t.CanRunSync)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	t.CreatedAt = parseTime(now)
	return nil
}

// ListAPITokens returns a user's tokens, newest first. Revoked tokens are kept
// so the list doubles as an audit trail of what was issued.
func (d *DB) ListAPITokens(ctx context.Context, userID int64) ([]models.APIToken, error) {
	rows, err := d.r.QueryContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE user_id=? ORDER BY revoked_at IS NOT NULL, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetAPITokenByHash resolves a presented token. Revoked tokens and tokens whose
// owner is gone or disabled must not authenticate, so the join enforces both.
//
// The column list is derived from tokenCols rather than written out again: the
// two drifted apart once already when a column was added, and the symptom was
// every token failing to authenticate.
func (d *DB) GetAPITokenByHash(ctx context.Context, hash string) (*models.APIToken, error) {
	return scanToken(d.r.QueryRowContext(ctx,
		`SELECT `+prefixCols("t.", tokenCols)+`
		 FROM api_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash=? AND t.revoked_at IS NULL AND u.disabled=0`, hash))
}

// RevokeAPIToken is idempotent and scoped to the owner, so one user cannot
// revoke another's token by guessing an id.
func (d *DB) RevokeAPIToken(ctx context.Context, id, userID int64) error {
	res, err := d.w.ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`,
		fmtTime(time.Now()), id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchAPIToken records usage. Deliberately best-effort and coarse: it is
// written at most once a minute per token so a busy script does not turn every
// read into a database write.
func (d *DB) TouchAPIToken(ctx context.Context, id int64) error {
	_, err := d.w.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at=? WHERE id=? AND (last_used_at IS NULL OR last_used_at < ?)`,
		fmtTime(time.Now()), id, fmtTime(time.Now().Add(-time.Minute)))
	return err
}
