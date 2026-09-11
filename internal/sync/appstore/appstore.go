// Package appstore ingests App Store Connect data into the store.
package appstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/storeclient"
	"github.com/oxisoft/oxilytics/internal/storeclient/appstoreconnect"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

const (
	srcMetrics = "metrics"
	srcCrashes = "crashes"
	srcReviews = "reviews"
	srcRequest = "asc_report_request" // cursor = ONGOING request id
)

// Client is the subset of appstoreconnect.Client we use (mockable).
type Client interface {
	Apps(ctx context.Context) ([]appstoreconnect.App, error)
	Lookup(ctx context.Context, appID, country string) (*appstoreconnect.Lookup, error)
	Reviews(ctx context.Context, appID string, since time.Time, fn func([]appstoreconnect.Review) bool) error
	ReportRequests(ctx context.Context, appID string) ([]appstoreconnect.ReportRequest, error)
	CreateReportRequest(ctx context.Context, appID string, access appstoreconnect.AccessType) (*appstoreconnect.ReportRequest, error)
	Reports(ctx context.Context, requestID, name string) ([]appstoreconnect.Report, error)
	Instances(ctx context.Context, reportID string) ([]appstoreconnect.Instance, error)
	SegmentURLs(ctx context.Context, instanceID string) ([]string, error)
	DownloadSegment(ctx context.Context, url string) ([]byte, error)
}

type Ingester struct {
	c Client
	// SnapshotWait bounds how long a full run waits for Apple to produce a
	// ONE_TIME_SNAPSHOT before giving up on that app for this run.
	SnapshotWait time.Duration
	PollEvery    time.Duration
}

func New(c Client) *Ingester {
	return &Ingester{c: c, SnapshotWait: 4 * time.Hour, PollEvery: 2 * time.Minute}
}

func (in *Ingester) Store() models.Store { return models.StoreAppStore }

func (in *Ingester) DiscoverApps(ctx context.Context, rc *osync.RunContext) ([]models.App, error) {
	apps, err := in.c.Apps(ctx)
	if err != nil {
		return nil, &osync.FatalError{Err: fmt.Errorf("list apps: %w", err)}
	}
	for _, a := range apps {
		platform := models.PlatformIOS
		var icon *string
		if lk, err := in.c.Lookup(ctx, a.ID, "us"); err == nil {
			if lk.ArtworkURL != "" {
				icon = &lk.ArtworkURL
			}
			if lk.Kind == "mac-software" {
				platform = models.PlatformMacOS
			}
		}
		bundle := a.BundleID
		row, created, err := rc.DB.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: a.ID, Name: a.Name, BundleID: &bundle, Platform: platform, IconURL: icon})
		if err != nil {
			return nil, err
		}
		if created {
			rc.Log("info", &row.ID, "discovered new app %s (%s)", a.Name, a.BundleID)
		}
		osync.SuggestProduct(ctx, rc, row)
	}
	return rc.DB.ListApps(ctx, store.AppFilter{Store: models.StoreAppStore})
}

func (in *Ingester) SyncApp(ctx context.Context, rc *osync.RunContext, app models.App) error {
	var errs []error
	if err := in.syncReports(ctx, rc, app); err != nil {
		if isFatal(err) {
			return err
		}
		errs = append(errs, fmt.Errorf("reports: %w", err))
	}
	if err := in.syncReviews(ctx, rc, app); err != nil {
		if isFatal(err) {
			return err
		}
		errs = append(errs, fmt.Errorf("reviews: %w", err))
	}
	if err := in.syncRating(ctx, rc, app); err != nil {
		errs = append(errs, fmt.Errorf("rating: %w", err))
	}
	return errors.Join(errs...)
}

func isFatal(err error) bool {
	return errors.Is(err, storeclient.ErrAuth)
}

// reports --------------------------------------------------------------------

func (in *Ingester) syncReports(ctx context.Context, rc *osync.RunContext, app models.App) error {
	reqID, err := in.reportRequest(ctx, rc, app)
	if err != nil {
		return err
	}
	if reqID == "" {
		rc.Log("warn", &app.ID, "no analytics report request available yet; skipping reports")
		return nil
	}
	cpMetrics, _ := rc.DB.GetCheckpoint(ctx, models.StoreAppStore, srcMetrics, app.ID)
	cpCrashes, _ := rc.DB.GetCheckpoint(ctx, models.StoreAppStore, srcCrashes, app.ID)
	fromMetrics := osync.DeltaFrom(rc, cpMetrics)
	fromCrashes := osync.DeltaFrom(rc, cpCrashes)

	type spec struct {
		name   string
		source string
		from   string
		cols   store.MetricCols
	}
	specs := []spec{
		{appstoreconnect.ReportDownloads, srcMetrics, fromMetrics, store.MetricCols{Downloads: true}},
		{appstoreconnect.ReportInstalls, srcMetrics, fromMetrics, store.MetricCols{Uninstalls: true}},
		{appstoreconnect.ReportCrashes, srcCrashes, fromCrashes, store.MetricCols{Crashes: true}},
	}
	for _, s := range specs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		maxDay, n, err := in.ingestReport(ctx, rc, app, reqID, s.name, s.from, s.cols)
		if err != nil {
			if errors.Is(err, storeclient.ErrNotFound) {
				rc.Log("warn", &app.ID, "report %q not available", s.name)
				continue
			}
			return fmt.Errorf("%s: %w", s.name, err)
		}
		rc.Log("info", &app.ID, "%s: %d rows through %s", s.name, n, maxDay)
		if maxDay != "" {
			if err := rc.DB.SetCheckpoint(ctx, nil, models.StoreAppStore, s.source, app.ID, osync.MaxDay(maxDay, checkpointFor(s.source, cpMetrics, cpCrashes))); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkpointFor(source, cpMetrics, cpCrashes string) string {
	if source == srcCrashes {
		return cpCrashes
	}
	return cpMetrics
}

// reportRequest finds or creates the request we read from. Full runs create a
// ONE_TIME_SNAPSHOT and wait for it; delta runs use (or create) ONGOING.
func (in *Ingester) reportRequest(ctx context.Context, rc *osync.RunContext, app models.App) (string, error) {
	reqs, err := in.c.ReportRequests(ctx, app.StoreAppID)
	if err != nil {
		if errors.Is(err, storeclient.ErrForbidden) {
			return "", &osync.FatalError{Err: fmt.Errorf("analytics reports forbidden — API key needs App Manager/Admin role: %w", err)}
		}
		return "", err
	}
	var ongoing, snapshot string
	for _, r := range reqs {
		if r.Stopped {
			continue
		}
		switch r.AccessType {
		case appstoreconnect.AccessOngoing:
			ongoing = r.ID
		case appstoreconnect.AccessOneTime:
			snapshot = r.ID
		}
	}
	if rc.Mode == models.SyncFull {
		if snapshot == "" {
			r, err := in.c.CreateReportRequest(ctx, app.StoreAppID, appstoreconnect.AccessOneTime)
			if err != nil {
				return "", fmt.Errorf("create snapshot request: %w", err)
			}
			snapshot = r.ID
			rc.Log("info", &app.ID, "requested ONE_TIME_SNAPSHOT %s; Apple may take hours to produce it", snapshot)
		}
		// wait for the snapshot to have any instances
		deadline := time.Now().Add(in.SnapshotWait)
		for {
			reps, err := in.c.Reports(ctx, snapshot, appstoreconnect.ReportDownloads)
			if err == nil && len(reps) > 0 {
				if insts, err := in.c.Instances(ctx, reps[0].ID); err == nil && len(insts) > 0 {
					break
				}
			}
			if time.Now().After(deadline) {
				rc.Log("warn", &app.ID, "snapshot not ready after %s; will use ONGOING data if any and retry next run", in.SnapshotWait)
				snapshot = ""
				break
			}
			rc.Log("info", &app.ID, "waiting for Apple to generate the snapshot…")
			select {
			case <-time.After(in.PollEvery):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	if ongoing == "" {
		r, err := in.c.CreateReportRequest(ctx, app.StoreAppID, appstoreconnect.AccessOngoing)
		if err != nil {
			rc.Log("warn", &app.ID, "create ONGOING request: %v", err)
		} else {
			ongoing = r.ID
			rc.Log("info", &app.ID, "created ONGOING report request %s", ongoing)
		}
	}
	if ongoing != "" {
		_ = rc.DB.SetCheckpoint(ctx, nil, models.StoreAppStore, srcRequest, app.ID, ongoing)
	}
	if rc.Mode == models.SyncFull && snapshot != "" {
		return snapshot, nil
	}
	return ongoing, nil
}

// ingestReport downloads every daily instance of a report newer than `from`.
func (in *Ingester) ingestReport(ctx context.Context, rc *osync.RunContext, app models.App, reqID, name, from string, cols store.MetricCols) (string, int64, error) {
	reps, err := in.c.Reports(ctx, reqID, name)
	if err != nil {
		return "", 0, err
	}
	if len(reps) == 0 {
		return "", 0, storeclient.ErrNotFound
	}
	insts, err := in.c.Instances(ctx, reps[0].ID)
	if err != nil {
		return "", 0, err
	}
	var total int64
	maxDay := ""
	for _, inst := range insts {
		if ctx.Err() != nil {
			return maxDay, total, ctx.Err()
		}
		// processingDate is when Apple produced it; data inside is ≤ that day.
		if inst.ProcessingDate != "" && inst.ProcessingDate < from {
			continue
		}
		urls, err := in.c.SegmentURLs(ctx, inst.ID)
		if err != nil {
			return maxDay, total, err
		}
		var rows []models.MetricDay
		for _, u := range urls {
			data, err := in.c.DownloadSegment(ctx, u)
			if err != nil {
				return maxDay, total, err
			}
			rc.Stats.AddBytes(int64(len(data)))
			parsed, err := appstoreconnect.ParseReport(data)
			if err != nil {
				return maxDay, total, err
			}
			rows = append(rows, toMetricDays(app.ID, parsed, from)...)
		}
		if len(rows) == 0 {
			continue
		}
		n, err := in.upsert(ctx, rc, rows, cols)
		if err != nil {
			return maxDay, total, err
		}
		total += n
		rc.AddRows(n, 0)
		for _, r := range rows {
			maxDay = osync.MaxDay(maxDay, r.Day)
		}
		rc.Stats.Steps[name] += n
	}
	return maxDay, total, nil
}

// toMetricDays folds report rows into per-(day,country) rows plus a "*" total.
func toMetricDays(appID int64, rows []appstoreconnect.Row, from string) []models.MetricDay {
	type key struct{ day, country string }
	acc := map[key]*models.MetricDay{}
	get := func(day, country string) *models.MetricDay {
		k := key{day, country}
		if acc[k] == nil {
			acc[k] = &models.MetricDay{AppID: appID, Day: day, Country: country}
		}
		return acc[k]
	}
	for _, r := range rows {
		if r.Date < from {
			continue
		}
		c := appstoreconnect.Territory(r.Territory)
		for _, tgt := range []*models.MetricDay{get(r.Date, c), get(r.Date, "*")} {
			tgt.Downloads += r.Downloads
			tgt.Redownloads += r.Redownloads
			tgt.Updates += r.Updates
			tgt.Uninstalls += r.Deletions
			tgt.Crashes += r.Crashes
		}
	}
	out := make([]models.MetricDay, 0, len(acc))
	for _, v := range acc {
		out = append(out, *v)
	}
	return out
}

func (in *Ingester) upsert(ctx context.Context, rc *osync.RunContext, rows []models.MetricDay, cols store.MetricCols) (int64, error) {
	var n int64
	err := rc.DB.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = rc.DB.UpsertMetricDays(ctx, tx, rows, cols)
		return err
	})
	return n, err
}

// reviews --------------------------------------------------------------------

func (in *Ingester) syncReviews(ctx context.Context, rc *osync.RunContext, app models.App) error {
	var since time.Time
	if rc.Mode == models.SyncDelta {
		if cp, _ := rc.DB.GetCheckpoint(ctx, models.StoreAppStore, srcReviews, app.ID); cp != "" {
			if t, err := time.Parse(time.RFC3339, cp); err == nil {
				since = t.AddDate(0, 0, -rc.OverlapDays)
			}
		}
	}
	var newest time.Time
	var total int64
	err := in.c.Reviews(ctx, app.StoreAppID, since, func(batch []appstoreconnect.Review) bool {
		rows := make([]models.Review, 0, len(batch))
		for _, r := range batch {
			title, body, author, terr := r.Title, r.Body, r.Reviewer, r.Territory
			rv := models.Review{AppID: app.ID, StoreReviewID: r.ID, Rating: r.Rating, Title: nz(&title), Body: nz(&body), Author: nz(&author), Country: territoryToISO(terr), CreatedAt: r.CreatedAt}
			if r.Reply != nil {
				b := r.Reply.Body
				at := r.Reply.ModifiedAt
				rv.DeveloperReply, rv.DeveloperRepliedAt = &b, &at
			}
			rows = append(rows, rv)
			if r.CreatedAt.After(newest) {
				newest = r.CreatedAt
			}
		}
		err := rc.DB.Tx(ctx, func(tx *sql.Tx) error {
			n, err := rc.DB.UpsertReviews(ctx, tx, rows)
			total += n
			rc.AddRows(0, n)
			return err
		})
		return err == nil && ctx.Err() == nil
	})
	if err != nil {
		return err
	}
	rc.Log("info", &app.ID, "reviews: %d rows", total)
	if !newest.IsZero() {
		return rc.DB.SetCheckpoint(ctx, nil, models.StoreAppStore, srcReviews, app.ID, newest.UTC().Format(time.RFC3339))
	}
	return nil
}

func territoryToISO(t string) *string {
	if t == "" {
		return nil
	}
	// ASC returns alpha-3 (USA, GBR…); map the common ones, else keep first two letters
	if c, ok := alpha3[t]; ok {
		return &c
	}
	if len(t) == 3 {
		s := t[:2]
		return &s
	}
	return &t
}

var alpha3 = map[string]string{"USA": "US", "GBR": "GB", "DEU": "DE", "FRA": "FR", "POL": "PL", "ITA": "IT", "ESP": "ES", "NLD": "NL", "CAN": "CA", "AUS": "AU", "JPN": "JP", "CHN": "CN",
	"IND": "IN", "BRA": "BR", "MEX": "MX", "RUS": "RU", "SWE": "SE", "NOR": "NO", "DNK": "DK", "FIN": "FI", "CHE": "CH", "AUT": "AT", "BEL": "BE", "IRL": "IE", "PRT": "PT", "CZE": "CZ",
	"UKR": "UA", "TUR": "TR", "KOR": "KR", "TWN": "TW", "HKG": "HK", "SGP": "SG", "NZL": "NZ", "ZAF": "ZA", "ISR": "IL", "ARE": "AE", "SAU": "SA", "ARG": "AR", "CHL": "CL", "COL": "CO",
	"IDN": "ID", "MYS": "MY", "PHL": "PH", "THA": "TH", "VNM": "VN", "HUN": "HU", "ROU": "RO", "GRC": "GR", "BGR": "BG", "HRV": "HR", "SVK": "SK", "SVN": "SI", "LTU": "LT", "LVA": "LV", "EST": "EE"}

func nz(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// rating snapshot ------------------------------------------------------------

func (in *Ingester) syncRating(ctx context.Context, rc *osync.RunContext, app models.App) error {
	lk, err := in.c.Lookup(ctx, app.StoreAppID, "us")
	if err != nil {
		return err
	}
	avg, cnt := lk.RatingAvg, lk.RatingCount
	return rc.DB.UpdateAppRating(ctx, app.ID, &avg, &cnt)
}
