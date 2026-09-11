package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

const productCols = `id, name, slug, icon_url, description, archived, created_at, updated_at`

func scanProduct(sc interface{ Scan(...any) error }) (*models.Product, error) {
	var p models.Product
	var icon, desc sql.NullString
	var created, updated string
	var archived int
	if err := sc.Scan(&p.ID, &p.Name, &p.Slug, &icon, &desc, &archived, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.IconURL, p.Description = nullStr(icon), nullStr(desc)
	p.Archived = archived == 1
	p.CreatedAt, p.UpdatedAt = parseTime(created), parseTime(updated)
	return &p, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify makes a URL-safe identifier from a name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "product"
	}
	return s
}

// NormalizeName is the key used for product suggestions.
func NormalizeName(name string) string {
	s := strings.ToLower(name)
	s = nonSlug.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func (d *DB) CreateProduct(ctx context.Context, p *models.Product) error {
	ts := now()
	p.Name = strings.TrimSpace(p.Name)
	if p.Slug == "" {
		p.Slug = Slugify(p.Name)
	}
	// ensure unique slug
	base := p.Slug
	for i := 2; ; i++ {
		var n int
		if err := d.r.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE slug=?`, p.Slug).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			break
		}
		p.Slug = base + "-" + itoa(i)
	}
	res, err := d.w.ExecContext(ctx, `INSERT INTO products(name,slug,icon_url,description,archived,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		p.Name, p.Slug, p.IconURL, p.Description, boolInt(p.Archived), fmtTime(ts), fmtTime(ts))
	if err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	p.ID, _ = res.LastInsertId()
	p.CreatedAt, p.UpdatedAt = ts, ts
	return nil
}

func (d *DB) GetProduct(ctx context.Context, id int64) (*models.Product, error) {
	return scanProduct(d.r.QueryRowContext(ctx, `SELECT `+productCols+` FROM products WHERE id=?`, id))
}

func (d *DB) GetProductBySlug(ctx context.Context, slug string) (*models.Product, error) {
	return scanProduct(d.r.QueryRowContext(ctx, `SELECT `+productCols+` FROM products WHERE slug=?`, slug))
}

func (d *DB) ListProducts(ctx context.Context, includeArchived bool) ([]models.Product, error) {
	q := `SELECT ` + productCols + ` FROM products`
	if !includeArchived {
		q += ` WHERE archived=0`
	}
	q += ` ORDER BY name`
	rows, err := d.r.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (d *DB) UpdateProduct(ctx context.Context, id int64, name string, icon, desc *string, archived bool) error {
	res, err := d.w.ExecContext(ctx, `UPDATE products SET name=?, icon_url=?, description=?, archived=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(name), icon, desc, boolInt(archived), fmtTime(now()), id)
	if err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	return affected(res)
}

// DeleteProduct refuses when apps are linked.
func (d *DB) DeleteProduct(ctx context.Context, id int64) error {
	var n int
	if err := d.r.QueryRowContext(ctx, `SELECT COUNT(*) FROM apps WHERE product_id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	res, err := d.w.ExecContext(ctx, `DELETE FROM products WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// FindProductByNormalizedName returns the product whose normalised name matches.
func (d *DB) FindProductByNormalizedName(ctx context.Context, name string) (*models.Product, error) {
	ps, err := d.ListProducts(ctx, true)
	if err != nil {
		return nil, err
	}
	want := NormalizeName(name)
	for i := range ps {
		if NormalizeName(ps[i].Name) == want {
			return &ps[i], nil
		}
	}
	return nil, ErrNotFound
}

func itoa(i int) string { return strconv.Itoa(i) }
