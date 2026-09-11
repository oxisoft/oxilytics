package httpapi

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/auth"
	"github.com/oxisoft/oxilytics/internal/config"
	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/setup"
	"github.com/oxisoft/oxilytics/internal/store"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

type noopIngester struct{ st models.Store }

func (n noopIngester) Store() models.Store { return n.st }
func (n noopIngester) DiscoverApps(ctx context.Context, rc *osync.RunContext) ([]models.App, error) {
	return rc.DB.ListApps(ctx, store.AppFilter{Store: n.st})
}
func (n noopIngester) SyncApp(ctx context.Context, rc *osync.RunContext, app models.App) error {
	rc.AddRows(1, 0)
	return nil
}

func newDataServer(t *testing.T) (*client, *client, *store.DB, *osync.Engine) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	st := settings.New(db)
	engine := osync.NewEngine(db, st, time.UTC, noopIngester{models.StoreAppStore})
	sched := osync.NewScheduler(engine, db, st, time.UTC, "", 0)
	_ = sched.Start(context.Background())
	t.Cleanup(sched.Stop)
	h := NewRouter(Deps{
		Cfg: &config.Config{SessionKey: key}, DB: db, Auth: auth.New(db, key, false, "t"), Settings: st,
		Setup: setup.Status{Stores: map[models.Store]setup.StoreStatus{models.StoreAppStore: {Configured: true}}},
		Sync:  SyncDeps{Engine: engine, Scheduler: sched},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	seedAdmin(t, db)
	hv, _ := auth.HashPassword("viewerpass1")
	_ = db.CreateUser(context.Background(), &models.User{Email: "v@x.io", Name: "V", Role: models.RoleViewer, PasswordHash: hv})
	admin := &client{t: t, srv: srv}
	admin.do("POST", "/api/auth/login", map[string]string{"email": "admin@x.io", "password": "password123"})
	viewer := &client{t: t, srv: srv}
	viewer.do("POST", "/api/auth/login", map[string]string{"email": "v@x.io", "password": "viewerpass1"})
	return admin, viewer, db, engine
}

func TestProductsAppsIgnoreFlow(t *testing.T) {
	admin, viewer, db, _ := newDataServer(t)
	ctx := context.Background()
	ios, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "111", Name: "My Notes", Platform: models.PlatformIOS})
	and, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: "io.x", Name: "My Notes", Platform: models.PlatformAndroid})
	old, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: "io.old", Name: "Old Test", Platform: models.PlatformAndroid})
	_ = db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := db.UpsertMetricDays(ctx, tx, []models.MetricDay{{AppID: old.ID, Day: time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02"), Country: "*", Downloads: 99}}, store.MetricCols{Downloads: true})
		return err
	})

	// create product, link both platforms
	code, out := admin.do("POST", "/api/products", map[string]any{"name": "My Notes"})
	if code != 201 {
		t.Fatalf("create product: %d %v", code, out)
	}
	pid := int64(out["id"].(float64))
	if code, _ := admin.do("POST", "/api/products/"+itoa(pid)+"/apps", map[string]any{"app_id": ios.ID}); code != 204 {
		t.Fatalf("link ios: %d", code)
	}
	if code, _ := admin.do("POST", "/api/products/"+itoa(pid)+"/apps", map[string]any{"app_id": and.ID}); code != 204 {
		t.Fatalf("link android: %d", code)
	}
	// viewer cannot link
	if code, _ := viewer.do("POST", "/api/products/"+itoa(pid)+"/apps", map[string]any{"app_id": old.ID}); code != 403 {
		t.Fatalf("viewer link: %d", code)
	}
	// platform taken
	if code, out := admin.do("POST", "/api/products/"+itoa(pid)+"/apps", map[string]any{"app_id": old.ID}); code != 409 || out["error"].(map[string]any)["code"] != "platform_taken" {
		t.Fatalf("platform taken: %d %v", code, out)
	}
	// product by slug with apps
	code, out = admin.do("GET", "/api/products/my-notes", nil)
	if code != 200 || len(out["apps"].([]any)) != 2 {
		t.Fatalf("get product: %d %v", code, out)
	}

	// ignore the old app: visible to nobody afterwards
	if code, _ := admin.do("POST", "/api/apps/"+itoa(old.ID)+"/ignore", map[string]any{"reason": "test build"}); code != 204 {
		t.Fatalf("ignore: %d", code)
	}
	if code, _ := viewer.do("GET", "/api/apps/"+itoa(old.ID), nil); code != 404 {
		t.Fatalf("viewer sees ignored app: %d", code)
	}
	if code, _ := viewer.do("GET", "/api/apps-ignored", nil); code != 403 {
		t.Fatalf("viewer ignored list: %d", code)
	}
	code, out = viewer.do("GET", "/api/metrics/summary", nil)
	if code != 200 || out["totals"].(map[string]any)["downloads"].(float64) != 0 {
		t.Fatalf("ignored data leaked into summary: %d %v", code, out)
	}
	var ignored []any
	if code := admin.doRaw("GET", "/api/apps-ignored", &ignored); code != 200 || len(ignored) != 1 {
		t.Fatalf("admin ignored list: %d %v", code, ignored)
	}
	if code, _ := admin.do("POST", "/api/apps/"+itoa(old.ID)+"/restore", nil); code != 204 {
		t.Fatalf("restore: %d", code)
	}
	var apps []any
	if code := admin.doRaw("GET", "/api/apps?unassigned=1", &apps); code != 200 || len(apps) != 1 {
		t.Fatalf("unassigned after restore: %d %d", code, len(apps))
	}
}

func TestSyncEndpoints(t *testing.T) {
	admin, viewer, db, engine := newDataServer(t)
	ctx := context.Background()
	_, _, _ = db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "111", Name: "A", Platform: models.PlatformIOS})

	if code, _ := viewer.do("POST", "/api/sync/runs", map[string]string{"store": "appstore", "mode": "delta"}); code != 403 {
		t.Fatalf("viewer start: %d", code)
	}
	code, out := admin.do("POST", "/api/sync/runs", map[string]string{"store": "googleplay", "mode": "delta"})
	if code != 409 || out["error"].(map[string]any)["code"] != "store_not_configured" {
		t.Fatalf("unconfigured: %d %v", code, out)
	}
	code, out = admin.do("POST", "/api/sync/runs", map[string]string{"store": "appstore", "mode": "delta"})
	if code != 201 {
		t.Fatalf("start: %d %v", code, out)
	}
	runID := int64(out["id"].(float64))
	engine.Wait(models.StoreAppStore)
	code, out = viewer.do("GET", "/api/sync/runs/"+itoa(runID), nil)
	if code != 200 || out["status"] != "succeeded" || out["rows_metrics"].(float64) != 1 {
		t.Fatalf("run: %d %v", code, out)
	}
	code, out = viewer.do("GET", "/api/sync/status", nil)
	if code != 200 || out["stores"].(map[string]any)["appstore"].(map[string]any)["last"] == nil {
		t.Fatalf("status: %d %v", code, out)
	}
	var logs []any
	if code := viewer.doRaw("GET", "/api/sync/runs/"+itoa(runID)+"/logs", &logs); code != 200 || len(logs) == 0 {
		t.Fatalf("logs: %d %d", code, len(logs))
	}
	// reset needs confirmation
	if code, _ := admin.do("POST", "/api/sync/reset", map[string]string{"store": "appstore", "confirm": "nope"}); code != 400 {
		t.Fatalf("reset without confirm: %d", code)
	}
	if code, _ := admin.do("POST", "/api/sync/reset", map[string]string{"store": "appstore", "confirm": "appstore"}); code != 204 {
		t.Fatalf("reset: %d", code)
	}
}

func TestMetricsAndReviewsEndpoints(t *testing.T) {
	admin, _, db, _ := newDataServer(t)
	ctx := context.Background()
	app, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "111", Name: "A", Platform: models.PlatformIOS})
	d1 := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
	d2 := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	body := "works great on my phone"
	_ = db.Tx(ctx, func(tx *sql.Tx) error {
		_, _ = db.UpsertMetricDays(ctx, tx, []models.MetricDay{
			{AppID: app.ID, Day: d1, Country: "*", Downloads: 5}, {AppID: app.ID, Day: d1, Country: "PL", Downloads: 5},
			{AppID: app.ID, Day: d2, Country: "*", Downloads: 7, Crashes: 1}, {AppID: app.ID, Day: d2, Country: "DE", Downloads: 7},
		}, store.MetricCols{Downloads: true, Crashes: true})
		_, err := db.UpsertReviews(ctx, tx, []models.Review{{AppID: app.ID, StoreReviewID: "r1", Rating: 5, Body: &body, CreatedAt: time.Now().UTC().AddDate(0, 0, -1)}})
		return err
	})
	code, out := admin.do("GET", "/api/metrics/summary?platform=ios", nil)
	if code != 200 || out["totals"].(map[string]any)["downloads"].(float64) != 12 || out["by_platform"].(map[string]any)["ios"] == nil {
		t.Fatalf("summary: %d %v", code, out)
	}
	code, out = admin.do("GET", "/api/metrics/series?metric=downloads&group=platform&bucket=day", nil)
	if code != 200 || len(out["buckets"].([]any)) != 30 || len(out["series"].([]any)) != 1 {
		t.Fatalf("series: %d %v", code, out)
	}
	code, out = admin.do("GET", "/api/metrics/countries", nil)
	if code != 200 || out["total"].(float64) != 12 {
		t.Fatalf("countries: %d %v", code, out)
	}
	if code, _ := admin.do("GET", "/api/metrics/series?metric=bogus", nil); code != 400 {
		t.Fatalf("bad metric: %d", code)
	}
	code, out = admin.do("GET", "/api/reviews?q=great", nil)
	if code != 200 || out["total"].(float64) != 1 {
		t.Fatalf("reviews: %d %v", code, out)
	}
	code, out = admin.do("GET", "/api/reviews/stats", nil)
	if code != 200 || out["count"].(float64) != 1 {
		t.Fatalf("review stats: %d %v", code, out)
	}
}
