// Package appstore ingests App Store Connect data into the store.
package appstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	Instances(ctx context.Context, reportID, granularity string) ([]appstoreconnect.Instance, error)
	SegmentURLs(ctx context.Context, instanceID string) ([]string, error)
	DownloadSegment(ctx context.Context, url string) ([]byte, error)
}

type Ingester struct {
	c Client
	// SnapshotGrace is a short courtesy wait for a ONE_TIME_SNAPSHOT that Apple
	// might produce immediately. It is deliberately small: the request id is
	// persisted, so an unready snapshot is picked up by a later run instead of
	// blocking this one. Apple typically takes hours to days, which is far
	// longer than any sync should hold a slot open.
	SnapshotGrace time.Duration
	PollEvery     time.Duration
	// Concurrency bounds how many apps are synced at once.
	Concurrency int
}

func New(c Client) *Ingester {
	return &Ingester{c: c, SnapshotGrace: 2 * time.Minute, PollEvery: 20 * time.Second, Concurrency: 4}
}

func (in *Ingester) Store() models.Store { return models.StoreAppStore }

// AppConcurrency implements sync.ConcurrentIngester.
func (in *Ingester) AppConcurrency() int { return in.Concurrency }

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

// VerifyReportNames checks that Apple still offers every report we ask for,
// once per run, and fails the run loudly if one is unknown everywhere.
//
// This exists because of a real and expensive failure: the constants named
// "App Store Downloads" and "App Store Installation and Deletion", which Apple
// does not publish under those names. Every sync then completed "successfully"
// with zero rows for months, and the logs said "report not available" — which
// reads like a fact about the app rather than a typo in our code.
//
// A per-app absence is legitimate (unreleased apps, reports Apple does not
// produce for that title), so the check passes as soon as ANY app offers the
// name. Only a name that no app recognises is treated as a bug.
func (in *Ingester) VerifyReportNames(ctx context.Context, rc *osync.RunContext, apps []models.App) error {
	unknown := map[string]bool{}
	for _, n := range appstoreconnect.AllReportNames {
		unknown[n] = true
	}
	checked := 0
	for _, app := range apps {
		if len(unknown) == 0 || checked >= 5 || ctx.Err() != nil {
			break
		}
		reqs, err := in.c.ReportRequests(ctx, app.StoreAppID)
		if err != nil || len(reqs) == 0 {
			continue
		}
		var probed bool
		for _, r := range reqs {
			if r.Stopped {
				continue
			}
			for name := range unknown {
				reps, err := in.c.Reports(ctx, r.ID, name)
				if err == nil && len(reps) > 0 {
					delete(unknown, name)
					probed = true
				}
			}
		}
		if probed {
			checked++
		}
	}
	if len(unknown) == 0 || checked == 0 {
		// Either everything resolved, or no app had a usable request yet and
		// there is nothing to conclude.
		return nil
	}
	var names []string
	for n := range unknown {
		names = append(names, strconv.Quote(n))
	}
	sort.Strings(names)
	return &osync.FatalError{Err: fmt.Errorf(
		"Apple offers no report named %s for any app — the names in "+
			"internal/storeclient/appstoreconnect/parse.go are wrong or Apple renamed them; "+
			"syncing would silently store zero rows", strings.Join(names, ", "))}
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
	reqIDs, err := in.reportRequests(ctx, rc, app)
	if err != nil {
		return err
	}
	if len(reqIDs) == 0 {
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
		// Read every report request, not just one.
		//
		// The two request types carry different data and neither is a superset:
		// a ONE_TIME_SNAPSHOT instance holds the app's full history in a single
		// file (measured: 163 days in one), while an ONGOING instance holds
		// only the last day or two but keeps arriving daily. Using ONGOING
		// alone — the old behaviour for delta runs — meant history could never
		// be backfilled no matter how often we synced.
		//
		// Upserts are keyed on (app, day, country), so overlapping days from
		// both sources converge on the same rows rather than double-counting.
		var (
			maxDay   string
			total    int64
			anyFound bool
			lastErr  error
		)
		for _, reqID := range reqIDs {
			day, n, err := in.ingestReport(ctx, rc, app, reqID, s.name, s.from, s.cols)
			if err != nil {
				if errors.Is(err, storeclient.ErrNotFound) {
					lastErr = err
					continue
				}
				return fmt.Errorf("%s: %w", s.name, err)
			}
			anyFound = true
			total += n
			maxDay = osync.MaxDay(maxDay, day)
		}
		// A report may be genuinely absent for one app — Apple does not offer
		// crash reports for every title, and a brand-new app has none of them
		// yet. What must never pass silently is a name that NO app recognises,
		// which is what "App Store Downloads" was: a typo that cost months of
		// data while every sync reported success.
		//
		// So: absent for this app is a warning, absent everywhere is fatal,
		// and the distinction is drawn in VerifyReportNames at run start rather
		// than guessed at per app.
		if !anyFound && lastErr != nil {
			rc.Log("warn", &app.ID, "Apple offers no %q report for this app", s.name)
			continue
		}
		n := total
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

// reportRequests returns every request worth reading, newest data last.
//
// Both types are returned because they carry different things: the
// ONE_TIME_SNAPSHOT is a single file with the app's whole history, the ONGOING
// request produces a fresh file each day but only covers recent days. A sync
// that reads only one of them either never backfills or never updates.
//
// A snapshot is requested on any run that lacks one, not just full runs: an app
// added later would otherwise never get its history, and Apple produces the
// snapshot asynchronously anyway, so a later run picks it up for free.
func (in *Ingester) reportRequests(ctx context.Context, rc *osync.RunContext, app models.App) ([]string, error) {
	reqs, err := in.c.ReportRequests(ctx, app.StoreAppID)
	if err != nil {
		if errors.Is(err, storeclient.ErrForbidden) {
			return nil, &osync.FatalError{Err: fmt.Errorf("analytics reports forbidden — API key needs App Manager/Admin role: %w", err)}
		}
		return nil, err
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
	if snapshot == "" {
		r, err := in.c.CreateReportRequest(ctx, app.StoreAppID, appstoreconnect.AccessOneTime)
		if err != nil {
			// Not fatal: ongoing data still flows, we just have no history yet.
			rc.Log("warn", &app.ID, "create ONE_TIME_SNAPSHOT request: %v", err)
		} else {
			snapshot = r.ID
			rc.Log("info", &app.ID, "requested ONE_TIME_SNAPSHOT %s for historical backfill; Apple produces it asynchronously and a later run will pick it up", snapshot)
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
	// Snapshot first so its historical rows land before the ongoing file's
	// recent days; both upsert on the same key, so order only affects logs.
	var out []string
	for _, id := range []string{snapshot, ongoing} {
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

// ingestReport downloads every instance of a report newer than `from`.
//
// Apple publishes a given report at exactly one granularity — downloads are
// DAILY, installs and deletions only WEEKLY — and asking for the wrong one
// returns an empty list rather than an error. So each granularity is tried in
// turn and the first that has instances wins. Hardcoding DAILY meant the
// install report was never fetched at all.
func (in *Ingester) ingestReport(ctx context.Context, rc *osync.RunContext, app models.App, reqID, name, from string, cols store.MetricCols) (string, int64, error) {
	reps, err := in.c.Reports(ctx, reqID, name)
	if err != nil {
		return "", 0, err
	}
	if len(reps) == 0 {
		return "", 0, storeclient.ErrNotFound
	}
	var insts []appstoreconnect.Instance
	for _, g := range appstoreconnect.PreferredGranularities {
		got, err := in.c.Instances(ctx, reps[0].ID, g)
		if err != nil {
			return "", 0, err
		}
		if len(got) > 0 {
			insts = got
			break
		}
	}
	if len(insts) == 0 {
		// The report exists but Apple has produced no files for it yet.
		// Normal for a new app or a report type with no activity.
		return "", 0, nil
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
			// Apple's install events are a re-download onto a new device, not
			// a first-time download, so they are folded into redownloads
			// rather than inflating the downloads figure.
			tgt.Redownloads += r.Installs
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
		// The public storefront has no entry for an app that has not been
		// released yet. That is a fact about the app, not a failure of the
		// sync, and it must not mark the app — or the whole run — as failed.
		if errors.Is(err, storeclient.ErrNotFound) {
			rc.Log("info", &app.ID, "no public store listing yet (unreleased); skipping rating")
			return nil
		}
		return err
	}
	avg, cnt := lk.RatingAvg, lk.RatingCount
	return rc.DB.UpdateAppRating(ctx, app.ID, &avg, &cnt)
}
