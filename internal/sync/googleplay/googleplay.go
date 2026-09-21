// Package googleplay ingests Play Console reports and reviews into the store.
package googleplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/storeclient"
	"github.com/oxisoft/oxilytics/internal/storeclient/googleplay"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

const (
	srcMetrics = "metrics"
	srcReviews = "reviews"
)

type Client interface {
	ListObjects(ctx context.Context, prefix string) ([]googleplay.Object, error)
	Download(ctx context.Context, name string) ([]byte, error)
	Reviews(ctx context.Context, pkg string) ([]googleplay.Review, error)
	Listing(ctx context.Context, pkg string) (*googleplay.Listing, error)
}

type Ingester struct {
	c    Client
	objs []googleplay.Object // cached listing for the run
	byPkg map[string][]googleplay.Object
}

func New(c Client) *Ingester { return &Ingester{c: c} }

func (in *Ingester) Store() models.Store { return models.StoreGooglePlay }

func (in *Ingester) DiscoverApps(ctx context.Context, rc *osync.RunContext) ([]models.App, error) {
	in.objs = nil
	in.byPkg = map[string][]googleplay.Object{}
	for _, prefix := range []string{"stats/installs/", "stats/ratings/", "stats/crashes/", "reviews/"} {
		objs, err := in.c.ListObjects(ctx, prefix)
		if err != nil {
			if errors.Is(err, storeclient.ErrAuth) || errors.Is(err, storeclient.ErrForbidden) {
				return nil, &osync.FatalError{Err: fmt.Errorf("list %s: %w", prefix, err)}
			}
			return nil, fmt.Errorf("list %s: %w", prefix, err)
		}
		for _, o := range objs {
			if _, pkg, _, ok := googleplay.ClassifyObject(o.Name); ok {
				in.objs = append(in.objs, o)
				in.byPkg[pkg] = append(in.byPkg[pkg], o)
			}
		}
	}
	pkgs := make([]string, 0, len(in.byPkg))
	for p := range in.byPkg {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		existing, err := rc.DB.GetAppByStoreID(ctx, models.StoreGooglePlay, pkg)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		name := pkg
		var icon *string
		// Hit the Publisher API for new apps, full runs, or whenever we still
		// have no icon for an app we already know: a delta run would otherwise
		// never backfill an icon that an earlier bug failed to store.
		if existing == nil || rc.Mode == models.SyncFull || existing.IconURL == nil || *existing.IconURL == "" {
			if l, err := in.c.Listing(ctx, pkg); err == nil {
				if l.Title != "" {
					name = l.Title
				}
				if l.IconURL != "" {
					icon = &l.IconURL
				} else if l.IconErr != nil {
					rc.Log("warn", nil, "%s: icon lookup failed (%v)", pkg, l.IconErr)
				}
			} else {
				rc.Log("warn", nil, "%s: listing lookup failed (%v); using package name", pkg, err)
				if existing != nil {
					name = existing.Name
				}
			}
		} else {
			name = existing.Name
		}
		bundle := pkg
		row, created, err := rc.DB.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: pkg, Name: name, BundleID: &bundle, Platform: models.PlatformAndroid, IconURL: icon})
		if err != nil {
			return nil, err
		}
		if created {
			rc.Log("info", &row.ID, "discovered new app %s (%s)", name, pkg)
		}
		osync.SuggestProduct(ctx, rc, row)
	}
	return rc.DB.ListApps(ctx, store.AppFilter{Store: models.StoreGooglePlay})
}

func (in *Ingester) SyncApp(ctx context.Context, rc *osync.RunContext, app models.App) error {
	objs := in.byPkg[app.StoreAppID]
	if len(objs) == 0 {
		rc.Log("warn", &app.ID, "no report files for %s", app.StoreAppID)
		return nil
	}
	cp, _ := rc.DB.GetCheckpoint(ctx, models.StoreGooglePlay, srcMetrics, app.ID)
	from := osync.DeltaFrom(rc, cp)
	fromMonth := strings.ReplaceAll(from[:7], "-", "")
	now := time.Now().UTC()
	curMonth := now.Format("200601")
	prevMonth := now.AddDate(0, -1, 0).Format("200601")

	maxDay := ""
	var latestRating *float64
	var latestRatingDay string
	var errs []error

	sort.Slice(objs, func(i, j int) bool { return objs[i].Name < objs[j].Name })
	for _, o := range objs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		kind, _, month, _ := googleplay.ClassifyObject(o.Name)
		if month < fromMonth {
			continue
		}
		// delta: skip unchanged old months; always re-read current & previous month
		if rc.Mode == models.SyncDelta && month != curMonth && month != prevMonth {
			if g, _ := rc.DB.IngestedGeneration(ctx, models.StoreGooglePlay, o.Name); g == o.Generation {
				continue
			}
		}
		data, err := in.c.Download(ctx, o.Name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", o.Name, err))
			continue
		}
		rc.Stats.AddBytes(int64(len(data)))
		var n int64
		switch kind {
		case googleplay.KindInstallsOverview, googleplay.KindInstallsCountry, googleplay.KindCrashesOverview:
			rows, err := googleplay.ParseStats(data)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", o.Name, err))
				continue
			}
			mds, cols := toMetricDays(app.ID, kind, rows, from)
			n, err = in.upsertMetrics(ctx, rc, mds, cols, o)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", o.Name, err))
				continue
			}
			for _, m := range mds {
				maxDay = osync.MaxDay(maxDay, m.Day)
			}
			rc.AddRows(n, 0)
		case googleplay.KindRatingsOverview:
			rows, err := googleplay.ParseStats(data)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, r := range rows {
				if r.TotalAvgRating != nil && r.Date >= latestRatingDay {
					latestRating, latestRatingDay = r.TotalAvgRating, r.Date
				}
			}
			_ = rc.DB.Tx(ctx, func(tx *sql.Tx) error { return rc.DB.MarkIngested(ctx, tx, models.StoreGooglePlay, o.Name, o.Generation, o.MD5) })
		case googleplay.KindReviews:
			rows, err := googleplay.ParseReviews(data)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			revs := csvReviews(app.ID, rows)
			n, err = in.upsertReviews(ctx, rc, revs, &o)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			rc.AddRows(0, n)
		default:
			continue
		}
		rc.Stats.Steps[string(kind)] += n
	}
	if maxDay != "" {
		if err := rc.DB.SetCheckpoint(ctx, nil, models.StoreGooglePlay, srcMetrics, app.ID, osync.MaxDay(maxDay, cp)); err != nil {
			errs = append(errs, err)
		}
		rc.Log("info", &app.ID, "metrics through %s", maxDay)
	}
	if latestRating != nil {
		_ = rc.DB.UpdateAppRating(ctx, app.ID, latestRating, nil)
	}

	// live reviews (last 7 days) — also fills reviews created today
	live, err := in.c.Reviews(ctx, app.StoreAppID)
	if err != nil {
		errs = append(errs, fmt.Errorf("reviews api: %w", err))
	} else if len(live) > 0 {
		n, err := in.upsertReviews(ctx, rc, apiReviews(app.ID, live), nil)
		if err != nil {
			errs = append(errs, err)
		} else {
			rc.AddRows(0, n)
			rc.Log("info", &app.ID, "reviews api: %d recent", n)
		}
	}
	return errors.Join(errs...)
}

func toMetricDays(appID int64, kind googleplay.ObjectKind, rows []googleplay.StatRow, from string) ([]models.MetricDay, store.MetricCols) {
	var out []models.MetricDay
	var cols store.MetricCols
	for _, r := range rows {
		if r.Date < from {
			continue
		}
		country := "*"
		if kind == googleplay.KindInstallsCountry {
			country = strings.ToUpper(r.Country)
			if country == "" {
				country = "ZZ"
			}
		}
		md := models.MetricDay{AppID: appID, Day: r.Date, Country: country}
		switch kind {
		case googleplay.KindInstallsOverview, googleplay.KindInstallsCountry:
			md.Downloads = r.DailyUserInstalls
			md.Updates = r.DailyDeviceUpgrades
			md.Uninstalls = r.DailyUserUninstalls
			md.ActiveDevices = r.ActiveDeviceInstalls
			cols = store.MetricCols{Downloads: true, Uninstalls: true, ActiveDevices: r.ActiveDeviceInstalls != nil}
		case googleplay.KindCrashesOverview:
			md.Crashes = r.DailyCrashes
			md.ANRs = r.DailyANRs
			cols = store.MetricCols{Crashes: true}
		}
		out = append(out, md)
	}
	return out, cols
}

func (in *Ingester) upsertMetrics(ctx context.Context, rc *osync.RunContext, rows []models.MetricDay, cols store.MetricCols, o googleplay.Object) (int64, error) {
	var n int64
	err := rc.DB.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = rc.DB.UpsertMetricDays(ctx, tx, rows, cols)
		if err != nil {
			return err
		}
		return rc.DB.MarkIngested(ctx, tx, models.StoreGooglePlay, o.Name, o.Generation, o.MD5)
	})
	return n, err
}

func (in *Ingester) upsertReviews(ctx context.Context, rc *osync.RunContext, rows []models.Review, o *googleplay.Object) (int64, error) {
	var n int64
	err := rc.DB.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = rc.DB.UpsertReviews(ctx, tx, rows)
		if err != nil {
			return err
		}
		if o != nil {
			return rc.DB.MarkIngested(ctx, tx, models.StoreGooglePlay, o.Name, o.Generation, o.MD5)
		}
		return nil
	})
	return n, err
}

func csvReviews(appID int64, rows []googleplay.ReviewRow) []models.Review {
	out := make([]models.Review, 0, len(rows))
	for _, r := range rows {
		created, err := time.Parse(time.RFC3339, r.SubmittedAt)
		if err != nil {
			created, err = time.Parse("2006-01-02T15:04:05Z", r.SubmittedAt)
			if err != nil {
				continue
			}
		}
		rv := models.Review{AppID: appID, StoreReviewID: r.ID, Rating: r.Rating, Title: nz(r.Title), Body: nz(r.Text), Language: nz(r.Language), AppVersion: nz(r.AppVersionName), Device: nz(r.Device), CreatedAt: created}
		if lang := strings.ToUpper(r.Language); len(lang) >= 2 {
			// reviewer language like "en" or "pt_BR"; country only known when a region is given
			if i := strings.IndexAny(lang, "_-"); i > 0 && len(lang) >= i+3 {
				c := lang[i+1 : i+3]
				rv.Country = &c
			}
		}
		if r.ReplyText != "" {
			rt := r.ReplyText
			rv.DeveloperReply = &rt
			if at, err := time.Parse(time.RFC3339, r.ReplyAt); err == nil {
				rv.DeveloperRepliedAt = &at
			}
		}
		out = append(out, rv)
	}
	return out
}

func apiReviews(appID int64, rows []googleplay.Review) []models.Review {
	out := make([]models.Review, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" || r.Rating == 0 {
			continue
		}
		rv := models.Review{AppID: appID, StoreReviewID: r.ID, Rating: r.Rating, Body: nz(r.Text), Author: nz(r.Author), Language: nz(r.Language), Device: nz(r.Device), AppVersion: nz(r.AppVersion), CreatedAt: r.CreatedAt}
		if !r.ModifiedAt.IsZero() && r.ModifiedAt.After(r.CreatedAt) {
			m := r.ModifiedAt
			rv.EditedAt = &m
		}
		if r.ReplyText != "" {
			rt := r.ReplyText
			rv.DeveloperReply = &rt
			if !r.ReplyAt.IsZero() {
				at := r.ReplyAt
				rv.DeveloperRepliedAt = &at
			}
		}
		out = append(out, rv)
	}
	return out
}

func nz(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
