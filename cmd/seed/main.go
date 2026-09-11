// Command seed fills a dev database with realistic demo data so every screen
// can be exercised without store credentials. Dev only.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
)

func main() {
	path := flag.String("db", "./dev.db", "sqlite file")
	days := flag.Int("days", 400, "days of history")
	flag.Parse()
	ctx := context.Background()
	db, err := store.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rng := rand.New(rand.NewSource(42))

	type spec struct {
		name  string
		ios   string
		and   string
		base  float64
		trend float64
	}
	specs := []spec{
		{"My Notes", "1234567890", "io.example.notes", 120, 1.4},
		{"Habit Flow", "1234567891", "io.example.habit", 60, 0.8},
		{"Pocket Budget", "1234567892", "io.example.budget", 35, 1.1},
	}
	countries := []string{"US", "DE", "GB", "PL", "FR", "BR", "IN", "JP", "CA", "AU"}
	weights := []float64{0.3, 0.12, 0.1, 0.08, 0.07, 0.08, 0.1, 0.05, 0.05, 0.05}

	today := time.Now().UTC()
	for _, s := range specs {
		p := &models.Product{Name: s.name}
		if err := db.CreateProduct(ctx, p); err != nil {
			fmt.Fprintln(os.Stderr, "product:", err)
			os.Exit(1)
		}
		iconI := fmt.Sprintf("https://placehold.co/128x128/6366f1/white?text=%s", s.name[:1])
		iconA := fmt.Sprintf("https://placehold.co/128x128/10b981/white?text=%s", s.name[:1])
		bi, ba := "io.example."+p.Slug, s.and
		ios, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: s.ios, Name: s.name, BundleID: &bi, Platform: models.PlatformIOS, IconURL: &iconI})
		and, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: s.and, Name: s.name, BundleID: &ba, Platform: models.PlatformAndroid, IconURL: &iconA})
		_ = db.LinkApp(ctx, ios.ID, p.ID)
		_ = db.LinkApp(ctx, and.ID, p.ID)
		ra, rc := 4.2+rng.Float64()*0.6, int64(200+rng.Intn(5000))
		_ = db.UpdateAppRating(ctx, ios.ID, &ra, &rc)
		rb := 4.0 + rng.Float64()*0.8
		_ = db.UpdateAppRating(ctx, and.ID, &rb, nil)
		_ = db.TouchAppSynced(ctx, ios.ID)
		_ = db.TouchAppSynced(ctx, and.ID)

		for _, app := range []struct {
			a    *models.App
			mult float64
		}{{ios, 1}, {and, 1.8}} {
			var rows []models.MetricDay
			active := int64(5000 * app.mult)
			for i := *days; i >= 1; i-- {
				day := today.AddDate(0, 0, -i)
				dow := 1.0
				if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
					dow = 0.75
				}
				growth := math.Pow(s.trend, float64(*days-i)/365)
				dl := int64(s.base * app.mult * dow * growth * (0.8 + rng.Float64()*0.4))
				if rng.Intn(60) == 0 {
					dl *= 3 // feature spike
				}
				up := int64(float64(dl) * (1.5 + rng.Float64()))
				un := int64(float64(dl) * (0.2 + rng.Float64()*0.2))
				cr := int64(float64(dl) * 0.01 * (0.5 + rng.Float64()))
				active += dl - un
				d := day.Format("2006-01-02")
				total := models.MetricDay{AppID: app.a.ID, Day: d, Country: "*", Downloads: dl, Updates: up, Uninstalls: un, Crashes: cr}
				if app.a.Platform == models.PlatformAndroid {
					total.ANRs = cr / 4
					ad := active
					total.ActiveDevices = &ad
				} else {
					total.Redownloads = dl / 5
				}
				rows = append(rows, total)
				rem := dl
				for ci, c := range countries {
					n := int64(float64(dl) * weights[ci] * (0.7 + rng.Float64()*0.6))
					if ci == len(countries)-1 {
						n = rem
					}
					if n < 0 {
						n = 0
					}
					rem -= n
					rows = append(rows, models.MetricDay{AppID: app.a.ID, Day: d, Country: c, Downloads: n})
				}
			}
			if err := db.Tx(ctx, func(tx *sql.Tx) error {
				_, err := db.UpsertMetricDays(ctx, tx, rows, store.MetricCols{Downloads: true, Uninstalls: true, Crashes: true, ActiveDevices: app.a.Platform == models.PlatformAndroid})
				return err
			}); err != nil {
				fmt.Fprintln(os.Stderr, "metrics:", err)
				os.Exit(1)
			}

			// reviews
			titles := []string{"Great app", "Love it", "Needs work", "Crashes on launch", "Best in class", "Simple and fast", "Sync issue", "Five stars", "Meh", "Perfect for me"}
			bodies := []string{"Exactly what I needed, works flawlessly.", "The latest update broke the widget, please fix.", "Clean design, no ads, does one thing well.", "It crashes every time I open the settings page on my device.", "Would love dark mode scheduling.", "Fast support, they fixed my issue in a day.", "Sync between my phone and tablet stopped working.", "Been using it daily for a year.", "Too expensive for what it does.", "Solid app. Small bug with the calendar view."}
			var revs []models.Review
			for i := 0; i < 40+rng.Intn(60); i++ {
				k := rng.Intn(len(titles))
				rating := 5
				if k == 2 || k == 6 || k == 8 {
					rating = 2 + rng.Intn(2)
				} else if k == 3 {
					rating = 1
				} else if rng.Intn(4) == 0 {
					rating = 4
				}
				created := today.AddDate(0, 0, -rng.Intn(*days)).Add(time.Duration(rng.Intn(86400)) * time.Second)
				t, b := titles[k], bodies[k]
				c := countries[rng.Intn(len(countries))]
				ver := fmt.Sprintf("%d.%d.%d", 1+rng.Intn(3), rng.Intn(9), rng.Intn(9))
				au := fmt.Sprintf("user%d", rng.Intn(9000))
				rv := models.Review{AppID: app.a.ID, StoreReviewID: fmt.Sprintf("%s-%d", app.a.Store, i), Rating: rating, Title: &t, Body: &b, Author: &au, Country: &c, AppVersion: &ver, CreatedAt: created}
				if app.a.Platform == models.PlatformAndroid {
					rv.Title = nil
					dev := "Pixel 7"
					rv.Device = &dev
				}
				if rating <= 3 && rng.Intn(2) == 0 {
					reply := "Thanks for the report — a fix is on the way in the next release."
					at := created.Add(36 * time.Hour)
					rv.DeveloperReply, rv.DeveloperRepliedAt = &reply, &at
				}
				revs = append(revs, rv)
			}
			_ = db.Tx(ctx, func(tx *sql.Tx) error {
				_, err := db.UpsertReviews(ctx, tx, revs)
				return err
			})
		}
	}
	// an unassigned and an ignored listing
	_, _, _ = db.UpsertApp(ctx, &models.App{Store: models.StoreGooglePlay, StoreAppID: "io.example.notes.beta", Name: "My Notes (beta)", Platform: models.PlatformAndroid})
	old, _, _ := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "999000111", Name: "Legacy Reader", Platform: models.PlatformIOS})
	reason := "discontinued 2023"
	_ = db.IgnoreApp(ctx, old.ID, 0, &reason)

	// a couple of sync runs
	for i, st := range []models.Store{models.StoreAppStore, models.StoreGooglePlay} {
		r := &models.SyncRun{Store: st, Mode: models.SyncDelta, Trigger: models.TriggerSchedule}
		_ = db.CreateSyncRun(ctx, r)
		_ = db.StartSyncRun(ctx, r.ID, today.AddDate(0, 0, -4).Format("2006-01-02"), today.AddDate(0, 0, -1).Format("2006-01-02"), 3)
		_ = db.UpdateSyncProgress(ctx, r.ID, 3, int64(1200+i*300), int64(12+i))
		_ = db.AddSyncLog(ctx, r.ID, "info", nil, "run started")
		_ = db.AddSyncLog(ctx, r.ID, "info", nil, "3 active apps")
		_ = db.AddSyncLog(ctx, r.ID, "info", nil, "run finished")
		stats := `{"api_calls":42,"bytes":812345,"apps":{"1":{"name":"My Notes","metrics":400,"reviews":4,"ms":3200}},"step_ms":{"App Store Downloads":400}}`
		_ = db.FinishSyncRun(ctx, r.ID, models.SyncSucceeded, nil, &stats)
	}
	fmt.Println("seeded", *path)
}
