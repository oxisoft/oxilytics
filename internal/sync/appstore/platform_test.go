package appstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

// An unreleased macOS-only App Store Connect record has no public listing, so
// the iTunes lookup cannot say "mac-software". Its platform must come from the
// App Store versions, or it lands as a second "iOS" listing that cannot be
// linked to the product that already holds the real iOS app (OxiTick,
// 2026-11-12). An app stored with the wrong platform is corrected on the next run.
func TestDiscoverPlatformFromAppStoreVersions(t *testing.T) {
	cases := []struct {
		name      string
		platforms []string
		err       error
		want      models.Platform
	}{
		{"macOS-only record, unreleased", []string{"MAC_OS"}, nil, models.PlatformMacOS},
		{"iOS record", []string{"IOS"}, nil, models.PlatformIOS},
		{"universal purchase stays one iOS listing", []string{"IOS", "MAC_OS"}, nil, models.PlatformIOS},
		{"versions unreadable: fall back to the public lookup", nil, errors.New("boom"), models.PlatformIOS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ctx := context.Background()
			if err := db.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			// Stored earlier with the old, lookup-only detection.
			bundle := "io.x.notes"
			if _, _, err := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "111", Name: "My Notes", BundleID: &bundle, Platform: models.PlatformIOS}); err != nil {
				t.Fatal(err)
			}
			rc := &osync.RunContext{DB: db, Mode: models.SyncDelta,
				Log:     func(string, *int64, string, ...any) {},
				AddRows: func(int64, int64) {}, Stats: &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}}}
			m := &mock{platforms: map[string][]string{"111": tc.platforms}, platformErr: tc.err}
			if _, err := New(m).DiscoverApps(ctx, rc); err != nil {
				t.Fatal(err)
			}
			apps, err := db.ListApps(ctx, store.AppFilter{Store: models.StoreAppStore})
			if err != nil {
				t.Fatal(err)
			}
			if len(apps) != 1 || apps[0].Platform != tc.want {
				t.Fatalf("want one app with platform %s, got %+v", tc.want, apps)
			}
		})
	}
}
