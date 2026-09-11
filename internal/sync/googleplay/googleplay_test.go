package googleplay

import (
	"context"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/settings"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/storeclient/googleplay"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

type mock struct {
	objects   map[string][]googleplay.Object
	files     map[string]string
	downloads int
}

func (m *mock) ListObjects(ctx context.Context, prefix string) ([]googleplay.Object, error) {
	return m.objects[prefix], nil
}
func (m *mock) Download(ctx context.Context, name string) ([]byte, error) {
	m.downloads++
	return utf16le(m.files[name]), nil
}
func (m *mock) Reviews(ctx context.Context, pkg string) ([]googleplay.Review, error) {
	return []googleplay.Review{{ID: "gp:live1", Rating: 3, Text: "fresh", Author: "Bo", CreatedAt: time.Now().UTC()}}, nil
}
func (m *mock) Listing(ctx context.Context, pkg string) (*googleplay.Listing, error) {
	return &googleplay.Listing{Title: "My Notes", IconURL: "https://i/icon.png"}, nil
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := []byte{0xFF, 0xFE}
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func TestFullThenDelta(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	_ = db.Migrate(ctx)

	cur := time.Now().UTC().Format("200601")
	old := "202401"
	m := &mock{
		objects: map[string][]googleplay.Object{
			"stats/installs/": {
				{Name: "stats/installs/installs_io.x_" + old + "_overview.csv", Generation: "1"},
				{Name: "stats/installs/installs_io.x_" + old + "_country.csv", Generation: "1"},
				{Name: "stats/installs/installs_io.x_" + cur + "_overview.csv", Generation: "5"},
			},
			"stats/ratings/": {{Name: "stats/ratings/ratings_io.x_" + cur + "_overview.csv", Generation: "1"}},
			"stats/crashes/": {{Name: "stats/crashes/crashes_io.x_" + old + "_overview.csv", Generation: "1"}},
			"reviews/":       {{Name: "reviews/reviews_io.x_" + old + ".csv", Generation: "1"}},
		},
		files: map[string]string{
			"stats/installs/installs_io.x_" + old + "_overview.csv": "Date,Package Name,Daily Device Installs,Daily Device Uninstalls,Daily Device Upgrades,Daily User Installs,Daily User Uninstalls,Active Device Installs\n2024-01-05,io.x,10,1,2,9,1,500\n2024-01-06,io.x,12,0,0,11,0,510\n",
			"stats/installs/installs_io.x_" + old + "_country.csv":  "Date,Package Name,Country,Daily Device Installs,Daily Device Uninstalls,Daily Device Upgrades,Daily User Installs,Daily User Uninstalls\n2024-01-05,io.x,PL,6,0,0,6,0\n2024-01-05,io.x,DE,3,0,0,3,0\n",
			"stats/installs/installs_io.x_" + cur + "_overview.csv":  "Date,Package Name,Daily Device Installs,Daily Device Uninstalls,Daily Device Upgrades,Daily User Installs,Daily User Uninstalls,Active Device Installs\n" + time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02") + ",io.x,4,0,0,4,0,900\n",
			"stats/ratings/ratings_io.x_" + cur + "_overview.csv":    "Date,Package Name,Daily Average Rating,Total Average Rating\n" + time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02") + ",io.x,4.0,4.27\n",
			"stats/crashes/crashes_io.x_" + old + "_overview.csv":    "Date,Package Name,Daily Crashes,Daily ANRs\n2024-01-05,io.x,2,1\n",
			"reviews/reviews_io.x_" + old + ".csv":                    "Package Name,App Version Code,App Version Name,Reviewer Language,Device,Review Submit Date and Time,Review Submit Millis Since Epoch,Review Last Update Date and Time,Review Last Update Millis Since Epoch,Star Rating,Review Title,Review Text,Developer Reply Date and Time,Developer Reply Millis Since Epoch,Developer Reply Text,Review Link\nio.x,1,1.0,en,pixel,2024-01-05T10:00:00Z,1,2024-01-05T10:00:00Z,1,5,,Great,,,,https://x/?reviewId=gp%3A1\n",
		},
	}
	ing := New(m)
	logs := 0
	rc := &osync.RunContext{DB: db, Mode: models.SyncFull, From: "2008-07-10", To: time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"), OverlapDays: 3, Suggest: true,
		Log: func(string, *int64, string, ...any) { logs++ }, AddRows: func(int64, int64) {}, Stats: &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}}}
	_ = settings.New(db)

	apps, err := ing.DiscoverApps(ctx, rc)
	if err != nil || len(apps) != 1 || apps[0].Name != "My Notes" || apps[0].IconURL == nil {
		t.Fatalf("discover: %v %+v", err, apps)
	}
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("sync: %v", err)
	}
	tot, _ := db.SumTotals(ctx, store.Scope{}, "2024-01-01", "2024-01-31")
	if tot.Downloads != 20 || tot.Uninstalls != 1 || tot.Updates != 2 || tot.Crashes != 2 || tot.ANRs != 1 {
		t.Errorf("totals: %+v", tot)
	}
	countries, _, _ := db.TopCountries(ctx, store.Scope{}, "2024-01-01", "2024-01-31", "downloads", 5)
	if len(countries) != 2 || countries[0].Country != "PL" || countries[0].Value != 6 {
		t.Errorf("countries: %+v", countries)
	}
	_, nrev, _ := db.ListReviews(ctx, store.ReviewFilter{})
	if nrev != 2 { // csv + live api
		t.Errorf("reviews = %d", nrev)
	}
	app, _ := db.GetApp(ctx, apps[0].ID)
	if app.RatingAvg == nil || *app.RatingAvg != 4.27 {
		t.Errorf("rating snapshot: %+v", app.RatingAvg)
	}
	cp, _ := db.GetCheckpoint(ctx, models.StoreGooglePlay, "metrics", app.ID)
	if cp == "" {
		t.Error("checkpoint not set")
	}
	full := m.downloads

	// delta: old months unchanged → skipped; current month re-read
	rc.Mode = models.SyncDelta
	rc.From = time.Now().UTC().AddDate(0, 0, -4).Format("2006-01-02")
	apps, _ = ing.DiscoverApps(ctx, rc)
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("delta: %v", err)
	}
	if m.downloads-full > 2 { // installs current + ratings current
		t.Errorf("delta downloaded %d files, expected ≤2", m.downloads-full)
	}
	// data still consistent (idempotent)
	tot, _ = db.SumTotals(ctx, store.Scope{}, "2024-01-01", "2024-01-31")
	if tot.Downloads != 20 {
		t.Errorf("after delta: %+v", tot)
	}
}
