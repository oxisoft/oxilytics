package appstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

// An app removed in App Store Connect is still in our database, but Apple
// answers its analytics requests with 403, which used to abort the whole run.
// Discovery must skip it so the remaining apps sync.
func TestDiscoverSkipsAppsRemovedFromAppStoreConnect(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	_ = db.Migrate(ctx)

	bundle := "io.x.pomodoro"
	if _, _, err := db.UpsertApp(ctx, &models.App{Store: models.StoreAppStore, StoreAppID: "999", Name: "Gone Pomodoro", BundleID: &bundle, Platform: models.PlatformIOS}); err != nil {
		t.Fatal(err)
	}

	var logs []string
	rc := &osync.RunContext{DB: db, Mode: models.SyncDelta,
		Log:     func(l string, _ *int64, f string, a ...any) { logs = append(logs, l+" "+fmt.Sprintf(f, a...)) },
		AddRows: func(int64, int64) {}, Stats: &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}}}

	apps, err := New(&mock{}).DiscoverApps(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].StoreAppID != "111" {
		t.Fatalf("want only the listed app 111, got %+v", apps)
	}
	found := false
	for _, l := range logs {
		if strings.HasPrefix(l, "info ") && strings.Contains(l, "999") && strings.Contains(l, "no longer in App Store Connect") {
			found = true
		}
	}
	if !found {
		t.Errorf("removed app not logged as skipped: %v", logs)
	}
	// History stays: the row is still in the database.
	all, _ := db.ListApps(ctx, store.AppFilter{Store: models.StoreAppStore})
	if len(all) != 2 {
		t.Errorf("removed app must stay in the DB, have %d apps", len(all))
	}
}
