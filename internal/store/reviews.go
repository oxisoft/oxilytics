package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

// UpsertReviews inserts or updates reviews by (app_id, store_review_id).
func (d *DB) UpsertReviews(ctx context.Context, tx *sql.Tx, rows []models.Review) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO reviews(app_id,store_review_id,rating,title,body,author,country,language,app_version,device,created_at,edited_at,developer_reply,developer_replied_at,fetched_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(app_id,store_review_id) DO UPDATE SET rating=excluded.rating, title=excluded.title, body=excluded.body, author=COALESCE(excluded.author, reviews.author),
		 country=COALESCE(excluded.country, reviews.country), language=COALESCE(excluded.language, reviews.language), app_version=COALESCE(excluded.app_version, reviews.app_version),
		 device=COALESCE(excluded.device, reviews.device), edited_at=excluded.edited_at, developer_reply=excluded.developer_reply, developer_replied_at=excluded.developer_replied_at, fetched_at=excluded.fetched_at`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var n int64
	ts := fmtTime(now())
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.AppID, r.StoreReviewID, r.Rating, r.Title, r.Body, r.Author, r.Country, r.Language, r.AppVersion, r.Device,
			fmtTime(r.CreatedAt), fmtTimePtr(r.EditedAt), r.DeveloperReply, fmtTimePtr(r.DeveloperRepliedAt), ts); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

type ReviewFilter struct {
	Scope    Scope
	Ratings  []int
	Country  string
	From, To string // YYYY-MM-DD inclusive
	Query    string
	Replied  *bool
	Page     int
	PerPage  int
}

const reviewCols = `r.id, r.app_id, r.store_review_id, r.rating, r.title, r.body, r.author, r.country, r.language, r.app_version, r.device,
 r.created_at, r.edited_at, r.developer_reply, r.developer_replied_at, r.fetched_at, a.name, a.store, a.platform, a.product_id`

func scanReview(sc interface{ Scan(...any) error }) (*models.Review, error) {
	var r models.Review
	var title, body, author, country, lang, ver, dev, created, edited, reply, repliedAt, fetched sql.NullString
	var prod sql.NullInt64
	if err := sc.Scan(&r.ID, &r.AppID, &r.StoreReviewID, &r.Rating, &title, &body, &author, &country, &lang, &ver, &dev,
		&created, &edited, &reply, &repliedAt, &fetched, &r.AppName, &r.Store, &r.Platform, &prod); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	r.Title, r.Body, r.Author, r.Country, r.Language, r.AppVersion, r.Device = nullStr(title), nullStr(body), nullStr(author), nullStr(country), nullStr(lang), nullStr(ver), nullStr(dev)
	r.CreatedAt = parseTime(created.String)
	r.EditedAt, r.DeveloperReply, r.DeveloperRepliedAt = parseTimePtr(edited), nullStr(reply), parseTimePtr(repliedAt)
	r.FetchedAt = parseTime(fetched.String)
	r.ProductID = nullInt(prod)
	return &r, nil
}

func (f ReviewFilter) where() (string, []any) {
	where, args := f.Scope.where("a")
	if len(f.Ratings) > 0 {
		ph := strings.Repeat("?,", len(f.Ratings))
		where = append(where, "r.rating IN ("+ph[:len(ph)-1]+")")
		for _, x := range f.Ratings {
			args = append(args, x)
		}
	}
	if f.Country != "" {
		where = append(where, "r.country=?")
		args = append(args, f.Country)
	}
	if f.From != "" {
		where = append(where, "r.created_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "r.created_at < date(?, '+1 day')")
		args = append(args, f.To)
	}
	if f.Replied != nil {
		if *f.Replied {
			where = append(where, "r.developer_reply IS NOT NULL")
		} else {
			where = append(where, "r.developer_reply IS NULL")
		}
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "r.id IN (SELECT rowid FROM reviews_fts WHERE reviews_fts MATCH ?)")
		args = append(args, ftsQuery(q))
	}
	return strings.Join(where, " AND "), args
}

// ftsQuery turns free text into a safe FTS5 prefix query.
func ftsQuery(q string) string {
	var parts []string
	for _, w := range strings.Fields(q) {
		w = strings.Trim(w, `"'*()`)
		if w == "" {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
	}
	if len(parts) == 0 {
		return `""`
	}
	return strings.Join(parts, " ")
}

func (d *DB) ListReviews(ctx context.Context, f ReviewFilter) ([]models.Review, int, error) {
	where, args := f.where()
	var total int
	if err := d.r.QueryRowContext(ctx, `SELECT COUNT(*) FROM reviews r JOIN apps a ON a.id=r.app_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.PerPage <= 0 || f.PerPage > 200 {
		f.PerPage = 50
	}
	if f.Page < 1 {
		f.Page = 1
	}
	q := `SELECT ` + reviewCols + ` FROM reviews r JOIN apps a ON a.id=r.app_id WHERE ` + where + ` ORDER BY r.created_at DESC, r.id DESC LIMIT ? OFFSET ?`
	rows, err := d.r.QueryContext(ctx, q, append(args, f.PerPage, (f.Page-1)*f.PerPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

func (d *DB) GetReview(ctx context.Context, id int64, adminSeesIgnored bool) (*models.Review, error) {
	q := `SELECT ` + reviewCols + ` FROM reviews r JOIN apps a ON a.id=r.app_id WHERE r.id=?`
	if !adminSeesIgnored {
		q += ` AND a.ignored_at IS NULL`
	}
	return scanReview(d.r.QueryRowContext(ctx, q, id))
}

type ReviewStats struct {
	Count      int64            `json:"count"`
	Avg        *float64         `json:"avg"`
	Histogram  map[int]int64    `json:"histogram"`
	ByPlatform map[string]Stat1 `json:"by_platform"`
}

type Stat1 struct {
	Count int64    `json:"count"`
	Avg   *float64 `json:"avg"`
}

func (d *DB) ReviewStats(ctx context.Context, f ReviewFilter) (*ReviewStats, error) {
	where, args := f.where()
	out := &ReviewStats{Histogram: map[int]int64{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}, ByPlatform: map[string]Stat1{}}
	rows, err := d.r.QueryContext(ctx, `SELECT a.platform, r.rating, COUNT(*) FROM reviews r JOIN apps a ON a.id=r.app_id WHERE `+where+` GROUP BY a.platform, r.rating`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := map[string]float64{}
	var totalSum float64
	for rows.Next() {
		var plat string
		var rating int
		var n int64
		if err := rows.Scan(&plat, &rating, &n); err != nil {
			return nil, err
		}
		out.Count += n
		out.Histogram[rating] += n
		totalSum += float64(rating) * float64(n)
		s := out.ByPlatform[plat]
		s.Count += n
		out.ByPlatform[plat] = s
		sum[plat] += float64(rating) * float64(n)
	}
	if out.Count > 0 {
		a := totalSum / float64(out.Count)
		out.Avg = &a
	}
	for p, s := range out.ByPlatform {
		if s.Count > 0 {
			a := sum[p] / float64(s.Count)
			s.Avg = &a
			out.ByPlatform[p] = s
		}
	}
	return out, rows.Err()
}

// LatestReviewTime returns the newest review created_at for an app (delta cursor).
func (d *DB) LatestReviewTime(ctx context.Context, appID int64) (sql.NullString, error) {
	var s sql.NullString
	err := d.r.QueryRowContext(ctx, `SELECT MAX(created_at) FROM reviews WHERE app_id=?`, appID).Scan(&s)
	return s, err
}
