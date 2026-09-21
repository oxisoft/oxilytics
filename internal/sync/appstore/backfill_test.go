package appstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxisoft/oxilytics/internal/models"
	"github.com/oxisoft/oxilytics/internal/store"
	"github.com/oxisoft/oxilytics/internal/storeclient/appstoreconnect"
	osync "github.com/oxisoft/oxilytics/internal/sync"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

// mockBase supplies the interface methods these tests do not exercise.
type mockBase = mock

// twoRequestMock models what Apple actually does, measured on 2026-09-21:
//
//   - ONE_TIME_SNAPSHOT: one file holding the app's whole history
//     (NeoMeter: 163 distinct dates, 878 first-time downloads in a single file)
//   - ONGOING: a fresh file daily, but only the last day or two
//     (the same app's ongoing file held 2 dates)
//
// Reading only ONGOING — the old delta behaviour — can never backfill history,
// however often it runs. This mock makes that failure reproducible.
type twoRequestMock struct {
	mock
	perRequest map[string]map[string]string // requestID → report name → TSV
	granPer    map[string]string
}

func (m *twoRequestMock) ReportRequests(ctx context.Context, appID string) ([]appstoreconnect.ReportRequest, error) {
	return []appstoreconnect.ReportRequest{
		{ID: "snap", AccessType: appstoreconnect.AccessOneTime},
		{ID: "ong", AccessType: appstoreconnect.AccessOngoing},
	}, nil
}

func (m *twoRequestMock) Reports(ctx context.Context, requestID, name string) ([]appstoreconnect.Report, error) {
	if _, ok := m.perRequest[requestID][name]; !ok {
		return nil, nil
	}
	return []appstoreconnect.Report{{ID: requestID + "|" + name, Name: name}}, nil
}

func (m *twoRequestMock) Instances(ctx context.Context, reportID, granularity string) ([]appstoreconnect.Instance, error) {
	want := m.granPer[reportID]
	if want == "" {
		want = appstoreconnect.GranularityDaily
	}
	if granularity != want {
		return nil, nil
	}
	return []appstoreconnect.Instance{{ID: "i|" + reportID, Granularity: want, ProcessingDate: "2026-09-21"}}, nil
}

func (m *twoRequestMock) SegmentURLs(ctx context.Context, instanceID string) ([]string, error) {
	return []string{"s|" + instanceID}, nil
}

func (m *twoRequestMock) DownloadSegment(ctx context.Context, url string) ([]byte, error) {
	// url = "s|i|<requestID>|<report name>"
	key := url[len("s|i|"):]
	for reqID, reports := range m.perRequest {
		if len(key) > len(reqID) && key[:len(reqID)+1] == reqID+"|" {
			if tsv, ok := reports[key[len(reqID)+1:]]; ok {
				return []byte(tsv), nil
			}
		}
	}
	return nil, nil
}

func TestSnapshotBackfillsHistoryAndOngoingAddsToday(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	const hdr = "Date\tApp Apple Identifier\tDownload Type\tTerritory\tCounts\n"
	m := &twoRequestMock{
		perRequest: map[string]map[string]string{
			// History: three older days, only in the snapshot.
			"snap": {
				appstoreconnect.ReportDownloads: hdr +
					"2026-07-01\t111\tFirst-time download\tUnited States\t10\n" +
					"2026-07-02\t111\tFirst-time download\tUnited States\t20\n" +
					"2026-09-20\t111\tFirst-time download\tUnited States\t5\n",
			},
			// Recent: overlaps 09-20 and adds 09-21.
			"ong": {
				appstoreconnect.ReportDownloads: hdr +
					"2026-09-20\t111\tFirst-time download\tUnited States\t5\n" +
					"2026-09-21\t111\tFirst-time download\tUnited States\t7\n",
			},
		},
		granPer: map[string]string{},
	}

	ing := New(m)
	ing.SnapshotGrace = time.Millisecond
	ing.PollEvery = time.Millisecond
	rc := &osync.RunContext{
		DB: db, Mode: models.SyncDelta, From: "2026-01-01", To: "2026-09-21", OverlapDays: 0,
		Log:     func(string, *int64, string, ...any) {},
		AddRows: func(int64, int64) {},
		Stats:   &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}},
	}

	apps, err := ing.DiscoverApps(ctx, rc)
	if err != nil || len(apps) != 1 {
		t.Fatalf("discover: %v %+v", err, apps)
	}
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// A delta run must still pick up July: that is the whole point of reading
	// the snapshot. The old code returned only the ONGOING request here.
	tot, _ := db.SumTotals(ctx, store.Scope{}, "2026-07-01", "2026-07-31")
	if tot.Downloads != 30 {
		t.Errorf("history not backfilled: July downloads = %d, want 30", tot.Downloads)
	}

	// The day present in BOTH files must be counted once, not twice: upserts
	// are keyed on (app, day, country).
	overlap, _ := db.SumTotals(ctx, store.Scope{}, "2026-09-20", "2026-09-20")
	if overlap.Downloads != 5 {
		t.Errorf("overlapping day double-counted: got %d, want 5", overlap.Downloads)
	}

	// And the ongoing-only day must be present.
	today, _ := db.SumTotals(ctx, store.Scope{}, "2026-09-21", "2026-09-21")
	if today.Downloads != 7 {
		t.Errorf("ongoing day missing: got %d, want 7", today.Downloads)
	}

	all, _ := db.SumTotals(ctx, store.Scope{}, "2026-01-01", "2026-12-31")
	if all.Downloads != 42 {
		t.Errorf("total downloads = %d, want 42 (10+20+5+7)", all.Downloads)
	}
}

// A report Apple offers only WEEKLY must still be ingested. Hardcoding DAILY
// meant the install/deletion report was never fetched at all.
func TestWeeklyOnlyReportIsStillIngested(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	m := &twoRequestMock{
		perRequest: map[string]map[string]string{
			"ong": {
				appstoreconnect.ReportInstalls: "Date\tApp Apple Identifier\tEvent\tTerritory\tCounts\n" +
					"2026-09-07\t111\tDelete\tUnited States\t4\n",
			},
		},
		granPer: map[string]string{
			"ong|" + appstoreconnect.ReportInstalls: appstoreconnect.GranularityWeekly,
		},
	}

	ing := New(m)
	ing.SnapshotGrace = time.Millisecond
	ing.PollEvery = time.Millisecond
	rc := &osync.RunContext{
		DB: db, Mode: models.SyncDelta, From: "2026-01-01", To: "2026-09-21", OverlapDays: 0,
		Log:     func(string, *int64, string, ...any) {},
		AddRows: func(int64, int64) {},
		Stats:   &osync.Stats{Apps: map[string]*osync.AppStat{}, Steps: map[string]int64{}},
	}
	apps, err := ing.DiscoverApps(ctx, rc)
	if err != nil || len(apps) != 1 {
		t.Fatalf("discover: %v %+v", err, apps)
	}
	if err := ing.SyncApp(ctx, rc, apps[0]); err != nil {
		t.Fatalf("sync: %v", err)
	}
	tot, _ := db.SumTotals(ctx, store.Scope{}, "2026-09-01", "2026-09-30")
	if tot.Uninstalls != 4 {
		t.Errorf("weekly-only report not ingested: uninstalls = %d, want 4", tot.Uninstalls)
	}
}
