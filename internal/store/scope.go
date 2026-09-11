package store

import (
	"strings"

	"github.com/oxisoft/oxilytics/internal/models"
)

// Scope is the common data filter: product / platform / store / app.
// Ignored apps are always excluded.
type Scope struct {
	ProductID *int64
	Platforms []models.Platform
	Store     models.Store
	AppID     *int64
}

// where returns conditions against the apps table aliased as `alias`.
func (s Scope) where(alias string) ([]string, []any) {
	where := []string{alias + ".ignored_at IS NULL"}
	var args []any
	if s.ProductID != nil {
		where = append(where, alias+".product_id=?")
		args = append(args, *s.ProductID)
	}
	if len(s.Platforms) > 0 {
		ph := strings.Repeat("?,", len(s.Platforms))
		where = append(where, alias+".platform IN ("+ph[:len(ph)-1]+")")
		for _, p := range s.Platforms {
			args = append(args, p)
		}
	}
	if s.Store != "" {
		where = append(where, alias+".store=?")
		args = append(args, s.Store)
	}
	if s.AppID != nil {
		where = append(where, alias+".id=?")
		args = append(args, *s.AppID)
	}
	return where, args
}
