package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

const userCols = `id, email, name, password_hash, role, totp_secret, totp_recovery, disabled, created_at, updated_at, last_login_at`

func scanUser(sc interface{ Scan(...any) error }) (*models.User, error) {
	var u models.User
	var totpSecret, totpRecovery, lastLogin sql.NullString
	var created, updated string
	var disabled int
	if err := sc.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &totpSecret, &totpRecovery, &disabled, &created, &updated, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.TOTPSecret = nullStr(totpSecret)
	u.TOTPRecovery = nullStr(totpRecovery)
	u.TOTPEnabled = u.TOTPSecret != nil
	u.Disabled = disabled == 1
	u.CreatedAt = parseTime(created)
	u.UpdatedAt = parseTime(updated)
	u.LastLoginAt = parseTimePtr(lastLogin)
	return &u, nil
}

func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.r.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (d *DB) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := d.r.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='admin' AND disabled=0`).Scan(&n)
	return n, err
}

func (d *DB) CreateUser(ctx context.Context, u *models.User) error {
	ts := now()
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	res, err := d.w.ExecContext(ctx, `INSERT INTO users(email,name,password_hash,role,disabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		u.Email, u.Name, u.PasswordHash, u.Role, boolInt(u.Disabled), fmtTime(ts), fmtTime(ts))
	if err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	u.ID, _ = res.LastInsertId()
	u.CreatedAt, u.UpdatedAt = ts, ts
	return nil
}

func (d *DB) GetUser(ctx context.Context, id int64) (*models.User, error) {
	return scanUser(d.r.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id=?`, id))
}

func (d *DB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return scanUser(d.r.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(email))))
}

func (d *DB) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := d.r.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY name, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (d *DB) UpdateUser(ctx context.Context, id int64, name string, role models.Role, disabled bool) error {
	res, err := d.w.ExecContext(ctx, `UPDATE users SET name=?, role=?, disabled=?, updated_at=? WHERE id=?`, name, role, boolInt(disabled), fmtTime(now()), id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) UpdateUserName(ctx context.Context, id int64, name string) error {
	res, err := d.w.ExecContext(ctx, `UPDATE users SET name=?, updated_at=? WHERE id=?`, name, fmtTime(now()), id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) UpdateUserPassword(ctx context.Context, id int64, hash string) error {
	res, err := d.w.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, fmtTime(now()), id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) UpdateUserTOTP(ctx context.Context, id int64, secret, recovery *string) error {
	res, err := d.w.ExecContext(ctx, `UPDATE users SET totp_secret=?, totp_recovery=?, updated_at=? WHERE id=?`, secret, recovery, fmtTime(now()), id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (d *DB) TouchUserLogin(ctx context.Context, id int64) error {
	_, err := d.w.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, fmtTime(now()), id)
	return err
}

func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	res, err := d.w.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// helpers --------------------------------------------------------------------

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
