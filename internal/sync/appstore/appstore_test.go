package appstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/storeclient/appstoreconnect"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

type mock struct {
	created  []appstoreconnect.AccessType
	requests []appstoreconnect.ReportRequest
	segments map[string]string // report name → TSV
}

func (m *mock) Apps(ctx context.Context) ([]appstoreconnect.App, error) {
	return []appstoreconnect.App{{ID: "111", Name: "My Notes", BundleID: "io.x.notes"}}, nil
}
func (m *mock) Lookup(ctx context.Context, appID, country string) (*appstoreconnect.Lookup, error) {
	return &appstoreconnect.Lookup{ArtworkURL: "https://a/512.png", RatingAvg: 4.5, RatingCount: 321, Kind: "software"}, nil
}
func (m *mock) Reviews(ctx context.Context, appID string, since time.Time, fn func([]appstoreconnect.Review) bool) error {
	all := []appstoreconnect.Review{
		{ID: "r1", Rating: 5, Title: "Great", Body: "love", Reviewer: "a", Territory: "USA", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "r2", Rating: 2, Title: "Meh", Body: "crashes", Reviewer: "b", Territory: "DEU", CreatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	var out []appstoreconnect.Review
	for _, r := range all {
		if since.IsZero() || !r.CreatedAt.Before(since) {
			out = append(out, r)
		}
	}
	fn(out)
	return nil
}
func (m *mock) ReportRequests(ctx context.Context, appID string) ([]appstoreconnect.ReportRequest, error) {
	return m.requests, nil
}
func (m *mock) CreateReportRequest(ctx context.Context, appID string, access appstoreconnect.AccessType) (*appstoreconnect.ReportRequest, error) {
	m.created = append(m.created, access)
	r := appstoreconnect.ReportRequest{ID: "req-" + string(access), AccessType: access}
	m.requests = append(m.requests, r)
	return &r, nil
}
func (m *mock) Reports(ctx context.Context, requestID, name string) ([]appstoreconnect.Report, error) {
	if _, ok := m.segments[name]; !ok {
		return nil, nil
	}
	return []appstoreconnect.Report{{ID: "rep|" + name, Name: name}}, nil
}
func (m *mock) Instances(ctx context.Context, reportID string) ([]appstoreconnect.Instance, error) {
	return []appstoreconnect.Instance{{ID: "inst|" + reportID, Granularity: "DAILY", ProcessingDate: "2026-09-03"}}, nil
}
func (m *mock) SegmentURLs(ctx context.Context, instanceID string) ([]string, error) {
	return []string{"seg|" + instanceID}, nil
}
func (m *mock) DownloadSegment(ctx context.Context, url string) ([]byte, error) {
	name := strings.TrimPrefix(url, "seg|inst|rep|")
	if _, ok := m.segments[name]; !ok {
		return nil, fmt.Errorf("mock: no segment for %q", url)
	}
	// the real client gunzips; the ingester receives plain TSV
	return []byte(m.segments[name]), nil
}

func TestFullSync(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	_ = db.Migrate(ctx)

	m := &mock{segments: map[string]string{
		appstoreconnect.ReportDownloads: "Date\tApp Apple Identifier\tDownload Type\tTerritory\tCounts\n2026-09-01\t111\tFirst-time download\tUnited States\t12\n2026-09-01\t111\tFirst-time download\tGermany\t3\n2026-09-01\t111\tUpdate\tGermany\t40\n2026-09-02\t111\tFirst-time download\tPoland\t5\n",
		appstoreconnect.ReportInstalls:  "Date\tApp Apple Identifier\tEvent\tTerritory\tCounts\n2026-09-01\t111\tDelete\tUnited States\t2\n",
		appstoreconnect.ReportCrashes:   "Date\tApp Apple Identifier\tTerritory\tCrashes\n2026-09-01\t111\tUnited States\t7\n",
	}}
	ing := New(m)
	ing.SnapshotWait = time.Second
	ing.PollEvery = 10 * time.Millisecond
	rc := &osync.RunContext{DB: db, Mode: models.SyncFull, From: "2008-07-10", To: "2026-09-10", OverlapDays: 3,
		Log: func(l string, _ *int64, f string, a ...any) { t.Logf("[%s] "+f, append([]any{l}, a...)...) }, AddRows: func(int64, int64) {}, Stats: &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}}}

	apps, err := ing.DiscoverApps(ctx, rc)
	if err != nil || len(apps) != 1 || apps[0].IconURL == nil || apps[0].Platform != models.PlatformIOS {
		t.Fatalf("discover: %v %+v", err, apps)
	}
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(m.created) != 2 || m.created[0] != appstoreconnect.AccessOneTime || m.created[1] != appstoreconnect.AccessOngoing {
		t.Errorf("requests created: %v", m.created)
	}
	tot, _ := db.SumTotals(ctx, store.Scope{}, "2026-09-01", "2026-09-02")
	if tot.Downloads != 20 || tot.Updates != 40 || tot.Uninstalls != 2 || tot.Crashes != 7 {
		t.Errorf("totals: %+v", tot)
	}
	countries, _, _ := db.TopCountries(ctx, store.Scope{}, "2026-09-01", "2026-09-02", "downloads", 5)
	if len(countries) != 3 || countries[0].Country != "US" || countries[0].Value != 12 {
		t.Errorf("countries: %+v", countries)
	}
	_, n, _ := db.ListReviews(ctx, store.ReviewFilter{})
	if n != 2 {
		t.Errorf("reviews = %d", n)
	}
	revs, _, _ := db.ListReviews(ctx, store.ReviewFilter{Country: "US"})
	if len(revs) != 1 {
		t.Errorf("territory mapping: %+v", revs)
	}
	app, _ := db.GetApp(ctx, apps[0].ID)
	if app.RatingAvg == nil || *app.RatingAvg != 4.5 || *app.RatingCount != 321 {
		t.Errorf("rating: %+v", app)
	}
	for _, src := range []string{"metrics", "crashes", "reviews", "asc_report_request"} {
		if cp, _ := db.GetCheckpoint(ctx, models.StoreAppStore, src, app.ID); cp == "" {
			t.Errorf("checkpoint %s missing", src)
		}
	}

	// delta run: uses ONGOING, no new requests, reviews since checkpoint
	rc.Mode = models.SyncDelta
	rc.From = "2026-09-06"
	before := len(m.created)
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("delta: %v", err)
	}
	if len(m.created) != before {
		t.Errorf("delta created requests: %v", m.created)
	}
	tot, _ = db.SumTotals(ctx, store.Scope{}, "2026-09-01", "2026-09-02")
	if tot.Downloads != 20 {
		t.Errorf("delta changed totals: %+v", tot)
	}
}
