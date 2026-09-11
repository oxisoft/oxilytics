package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
)

func seedApp(t *testing.T, d *DB, st models.Store, id, name string, plat models.Platform) *models.App {
	t.Helper()
	a, _, err := d.UpsertApp(context.Background(), &models.App{Store: st, StoreAppID: id, Name: name, Platform: plat})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestProductsAndLinking(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ios := seedApp(t, d, models.StoreAppStore, "111", "My Notes", models.PlatformIOS)
	and := seedApp(t, d, models.StoreGooglePlay, "io.x.notes", "My Notes", models.PlatformAndroid)
	ios2 := seedApp(t, d, models.StoreAppStore, "222", "Other", models.PlatformIOS)

	p := &models.Product{Name: "My Notes"}
	if err := d.CreateProduct(ctx, p); err != nil || p.Slug != "my-notes" {
		t.Fatalf("create: %v %+v", err, p)
	}
	p2 := &models.Product{Name: "My Notes Pro"}
	if err := d.CreateProduct(ctx, p2); err != nil || p2.Slug != "my-notes-pro" {
		t.Errorf("second product: %v %+v", err, p2)
	}
	p3 := &models.Product{Name: "My-Notes"}
	if err := d.CreateProduct(ctx, p3); err != nil || p3.Slug != "my-notes-2" {
		t.Errorf("slug collision not handled: %v %+v", err, p3)
	}

	if err := d.LinkApp(ctx, ios.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.LinkApp(ctx, and.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	// second iOS app on same product → conflict (one platform per product)
	if err := d.LinkApp(ctx, ios2.ID, p.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("platform-taken not enforced: %v", err)
	}
	// delete product with apps → conflict
	if err := d.DeleteProduct(ctx, p.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("delete with apps: %v", err)
	}
	// suggestion: exact normalised match only
	found, err := d.FindProductByNormalizedName(ctx, "  my NOTES ")
	if err != nil || found.ID != p.ID {
		t.Errorf("normalized find: %v %+v", err, found)
	}
	if _, err := d.FindProductByNormalizedName(ctx, "my notes pro max"); !errors.Is(err, ErrNotFound) {
		t.Errorf("fuzzy should not match: %v", err)
	}

	// ignore unlinks
	reason := "stale"
	if err := d.IgnoreApp(ctx, and.ID, 0, &reason); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetApp(ctx, and.ID)
	if got.ProductID != nil || got.IgnoredAt == nil {
		t.Errorf("ignore did not unlink: %+v", got)
	}
	active, _ := d.ListApps(ctx, AppFilter{})
	if len(active) != 2 {
		t.Errorf("active apps = %d", len(active))
	}
	ign, _ := d.ListIgnoredApps(ctx)
	if len(ign) != 1 || *ign[0].IgnoredReason != "stale" {
		t.Errorf("ignored list: %+v", ign)
	}
	// upsert of an ignored app keeps it ignored
	_, created, _ := d.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: "io.x.notes", Name: "My Notes v2", Platform: models.PlatformAndroid})
	got, _ = d.GetApp(ctx, and.ID)
	if created || got.IgnoredAt == nil || got.Name != "My Notes v2" {
		t.Errorf("upsert ignored: created=%v %+v", created, got)
	}
	if err := d.RestoreApp(ctx, and.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetApp(ctx, and.ID)
	if got.IgnoredAt != nil || got.ProductID != nil {
		t.Errorf("restore: %+v", got)
	}
}

func TestMetricsAndQueries(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ios := seedApp(t, d, models.StoreAppStore, "111", "My Notes", models.PlatformIOS)
	and := seedApp(t, d, models.StoreGooglePlay, "io.x.notes", "My Notes", models.PlatformAndroid)
	p := &models.Product{Name: "My Notes"}
	_ = d.CreateProduct(ctx, p)
	_ = d.LinkApp(ctx, ios.ID, p.ID)
	_ = d.LinkApp(ctx, and.ID, p.ID)

	err := d.Tx(ctx, func(tx *sql.Tx) error {
		rows := []models.MetricDay{
			{AppID: ios.ID, Day: "2026-09-01", Country: "*", Downloads: 10, Updates: 5},
			{AppID: ios.ID, Day: "2026-09-01", Country: "US", Downloads: 7},
			{AppID: ios.ID, Day: "2026-09-01", Country: "DE", Downloads: 3},
			{AppID: ios.ID, Day: "2026-09-02", Country: "*", Downloads: 20},
			{AppID: and.ID, Day: "2026-09-01", Country: "*", Downloads: 30},
			{AppID: and.ID, Day: "2026-09-01", Country: "PL", Downloads: 30},
			{AppID: and.ID, Day: "2026-08-25", Country: "*", Downloads: 100},
		}
		if _, err := d.UpsertMetricDays(ctx, tx, rows, MetricCols{Downloads: true}); err != nil {
			return err
		}
		// crashes for same rows from a different source must not wipe downloads
		_, err := d.UpsertMetricDays(ctx, tx, []models.MetricDay{{AppID: ios.ID, Day: "2026-09-01", Country: "*", Crashes: 4}}, MetricCols{Crashes: true})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	tot, err := d.SumTotals(ctx, Scope{ProductID: &p.ID}, "2026-09-01", "2026-09-02")
	if err != nil || tot.Downloads != 60 || tot.Crashes != 4 || tot.Updates != 5 {
		t.Errorf("totals: %v %+v", err, tot)
	}
	byPlat, _ := d.SumTotalsByPlatform(ctx, Scope{ProductID: &p.ID}, "2026-09-01", "2026-09-02")
	if byPlat["ios"].Downloads != 30 || byPlat["android"].Downloads != 30 {
		t.Errorf("by platform: %+v", byPlat)
	}
	series, err := d.Series(ctx, Scope{}, "2026-09-01", "2026-09-02", "downloads", "platform", "day")
	if err != nil || len(series) != 3 {
		t.Errorf("series: %v %+v", err, series)
	}
	monthly, _ := d.Series(ctx, Scope{}, "2026-08-01", "2026-09-30", "downloads", "total", "month")
	if len(monthly) != 2 || monthly[0].Value != 100 || monthly[1].Value != 60 {
		t.Errorf("monthly: %+v", monthly)
	}
	countries, sum, _ := d.TopCountries(ctx, Scope{}, "2026-09-01", "2026-09-02", "downloads", 10)
	if len(countries) != 3 || countries[0].Country != "PL" || sum != 40 {
		t.Errorf("countries: %+v %d", countries, sum)
	}
	// platform filter
	tot, _ = d.SumTotals(ctx, Scope{Platforms: []models.Platform{models.PlatformAndroid}}, "2026-09-01", "2026-09-02")
	if tot.Downloads != 30 {
		t.Errorf("platform scope: %+v", tot)
	}
	// ignored app excluded
	_ = d.IgnoreApp(ctx, and.ID, 0, nil)
	tot, _ = d.SumTotals(ctx, Scope{}, "2026-09-01", "2026-09-02")
	if tot.Downloads != 30 {
		t.Errorf("ignored not excluded: %+v", tot)
	}
	prows, err := d.ProductRows(ctx, "2026-09-01", "2026-09-02", "2026-08-30", "2026-08-31", false)
	if err != nil || len(prows) != 1 || prows[0].Totals.Downloads != 30 || len(prows[0].Apps) != 1 {
		t.Errorf("product rows: %v %+v", err, prows)
	}
}

func TestReviews(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ios := seedApp(t, d, models.StoreAppStore, "111", "My Notes", models.PlatformIOS)
	title := "Great app"
	body := "Really helps me track everything"
	reply := "Thanks!"
	err := d.Tx(ctx, func(tx *sql.Tx) error {
		_, err := d.UpsertReviews(ctx, tx, []models.Review{
			{AppID: ios.ID, StoreReviewID: "r1", Rating: 5, Title: &title, Body: &body, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
			{AppID: ios.ID, StoreReviewID: "r2", Rating: 2, Body: strPtr("crashes on launch"), CreatedAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), DeveloperReply: &reply},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// update in place
	_ = d.Tx(ctx, func(tx *sql.Tx) error {
		_, err := d.UpsertReviews(ctx, tx, []models.Review{{AppID: ios.ID, StoreReviewID: "r1", Rating: 4, Title: &title, Body: &body, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}})
		return err
	})
	list, total, err := d.ListReviews(ctx, ReviewFilter{})
	if err != nil || total != 2 || list[0].StoreReviewID != "r1" || list[0].Rating != 4 {
		t.Fatalf("list: %v %d %+v", err, total, list)
	}
	_, total, _ = d.ListReviews(ctx, ReviewFilter{Query: "crash"})
	if total != 1 {
		t.Errorf("fts: %d", total)
	}
	_, total, _ = d.ListReviews(ctx, ReviewFilter{Ratings: []int{5, 4}})
	if total != 1 {
		t.Errorf("rating filter: %d", total)
	}
	yes := true
	_, total, _ = d.ListReviews(ctx, ReviewFilter{Replied: &yes})
	if total != 1 {
		t.Errorf("replied filter: %d", total)
	}
	_, total, _ = d.ListReviews(ctx, ReviewFilter{From: "2026-09-01", To: "2026-09-01"})
	if total != 1 {
		t.Errorf("date filter: %d", total)
	}
	st, _ := d.ReviewStats(ctx, ReviewFilter{})
	if st.Count != 2 || st.Histogram[4] != 1 || st.Histogram[2] != 1 || *st.Avg != 3 {
		t.Errorf("stats: %+v", st)
	}
	latest, _ := d.LatestReviewTime(ctx, ios.ID)
	if !latest.Valid || latest.String[:10] != "2026-09-01" {
		t.Errorf("latest: %+v", latest)
	}
}

func TestSyncRuns(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	r := &models.SyncRun{Store: models.StoreAppStore, Mode: models.SyncFull, Trigger: models.TriggerManual}
	if err := d.CreateSyncRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	_ = d.StartSyncRun(ctx, r.ID, "2026-01-01", "2026-09-10", 3)
	_ = d.UpdateSyncProgress(ctx, r.ID, 1, 100, 5)
	_ = d.AddSyncLog(ctx, r.ID, "info", nil, "hello")
	got, _ := d.GetSyncRun(ctx, r.ID)
	if got.Status != models.SyncRunning || got.AppsDone != 1 || *got.RangeTo != "2026-09-10" {
		t.Errorf("run: %+v", got)
	}
	logs, _ := d.ListSyncLogs(ctx, r.ID, 0, 100)
	if len(logs) != 1 || logs[0].Message != "hello" {
		t.Errorf("logs: %+v", logs)
	}
	n, _ := d.MarkInterruptedRuns(ctx)
	if n != 1 {
		t.Errorf("interrupted = %d", n)
	}
	last, err := d.LastSyncRun(ctx, models.StoreAppStore)
	if err != nil || last.Status != models.SyncInterrupted {
		t.Errorf("last: %v %+v", err, last)
	}
	app := seedApp(t, d, models.StoreAppStore, "1", "A", models.PlatformIOS)
	_ = d.SetCheckpoint(ctx, nil, models.StoreAppStore, "metrics", app.ID, "2026-09-09")
	c, _ := d.GetCheckpoint(ctx, models.StoreAppStore, "metrics", app.ID)
	if c != "2026-09-09" {
		t.Errorf("checkpoint: %q", c)
	}
	if c, _ := d.GetCheckpoint(ctx, models.StoreAppStore, "metrics", 99); c != "" {
		t.Errorf("missing checkpoint: %q", c)
	}
}

func strPtr(s string) *string { return &s }
