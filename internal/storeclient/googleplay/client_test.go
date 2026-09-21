package googleplay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf16"
)

func testSA(t *testing.T, tokenURL string) []byte {
	t.Helper()
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	b, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "sa@p.iam.gserviceaccount.com", "private_key": pemStr, "token_uri": tokenURL})
	return b
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := []byte{0xFF, 0xFE}
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return out
}

func TestDecodeUTF16(t *testing.T) {
	in := utf16le("Date,Package Name\n2026-09-01,io.x\n")
	got := string(DecodeUTF16(in))
	if got != "Date,Package Name\n2026-09-01,io.x\n" {
		t.Errorf("got %q", got)
	}
	if string(DecodeUTF16([]byte("\xEF\xBB\xBFabc"))) != "abc" {
		t.Error("utf8 bom not stripped")
	}
}

func TestClassifyObject(t *testing.T) {
	tests := []struct {
		name string
		kind ObjectKind
		pkg  string
		mon  string
	}{
		{"stats/installs/installs_io.x.notes_202609_overview.csv", KindInstallsOverview, "io.x.notes", "202609"},
		{"stats/installs/installs_io.x.notes_202609_country.csv", KindInstallsCountry, "io.x.notes", "202609"},
		{"stats/ratings/ratings_io.x.notes_202608_overview.csv", KindRatingsOverview, "io.x.notes", "202608"},
		{"stats/crashes/crashes_io.x.notes_202608_overview.csv", KindCrashesOverview, "io.x.notes", "202608"},
		{"reviews/reviews_io.x.notes_202607.csv", KindReviews, "io.x.notes", "202607"},
	}
	for _, tt := range tests {
		k, p, m, ok := ClassifyObject(tt.name)
		if !ok || k != tt.kind || p != tt.pkg || m != tt.mon {
			t.Errorf("%s → %v %s %s %v", tt.name, k, p, m, ok)
		}
	}
	if _, _, _, ok := ClassifyObject("stats/installs/installs_io.x_202609_os_version.csv"); ok {
		t.Error("os_version should not classify")
	}
}

func TestParseStats(t *testing.T) {
	data := utf16le("Date,Package Name,Daily Device Installs,Daily Device Uninstalls,Daily Device Upgrades,Daily User Installs,Daily User Uninstalls,Active Device Installs\n2026-09-01,io.x,10,2,5,9,1,1200\n2026-09-02,io.x,12,3,0,11,2,1210\n")
	rows, err := ParseStats(data)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %+v", err, rows)
	}
	if rows[0].DailyUserInstalls != 9 || *rows[1].ActiveDeviceInstalls != 1210 || rows[1].DailyDeviceUpgrades != 0 {
		t.Errorf("%+v", rows)
	}
	country := "Date,Package Name,Country,Daily Device Installs,Daily User Installs\n2026-09-01,io.x,PL,4,4\n"
	rows, _ = ParseStats([]byte(country))
	if len(rows) != 1 || rows[0].Country != "PL" {
		t.Errorf("country: %+v", rows)
	}
	ratings := "Date,Package Name,Daily Average Rating,Total Average Rating\n2026-09-01,io.x,4.5,4.31\n2026-09-02,io.x,,4.32\n"
	rows, _ = ParseStats([]byte(ratings))
	if len(rows) != 2 || *rows[0].DailyAvgRating != 4.5 || rows[1].DailyAvgRating != nil || *rows[1].TotalAvgRating != 4.32 {
		t.Errorf("ratings: %+v", rows)
	}
	crashes := "Date,Package Name,Daily Crashes,Daily ANRs\n2026-09-01,io.x,3,1\n"
	rows, _ = ParseStats([]byte(crashes))
	if len(rows) != 1 || rows[0].DailyCrashes != 3 || rows[0].DailyANRs != 1 {
		t.Errorf("crashes: %+v", rows)
	}
}

func TestParseReviews(t *testing.T) {
	data := "Package Name,App Version Code,App Version Name,Reviewer Language,Device,Review Submit Date and Time,Review Submit Millis Since Epoch,Review Last Update Date and Time,Review Last Update Millis Since Epoch,Star Rating,Review Title,Review Text,Developer Reply Date and Time,Developer Reply Millis Since Epoch,Developer Reply Text,Review Link\n" +
		"io.x,42,1.2.0,en,pixel7,2026-09-01T10:11:12Z,1,2026-09-01T10:11:12Z,1,5,Great,\"Love it, works\",2026-09-02T00:00:00Z,2,Thanks!,https://play.google.com/console/developers/1/app/2/user-feedback/review-details?reviewId=gp%3AAOq&corpus=PUBLIC_REVIEWS\n"
	rows, err := ParseReviews([]byte(data))
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %+v", err, rows)
	}
	r := rows[0]
	if r.ID != "gp%3AAOq" || r.Rating != 5 || r.Text != "Love it, works" || r.ReplyText != "Thanks!" || r.AppVersionName != "1.2.0" {
		t.Errorf("%+v", r)
	}
}

func TestClientFlow(t *testing.T) {
	mux := http.NewServeMux()
	var tokenCalls int
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.FormValue("assertion") == "" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	})
	mux.HandleFunc("/storage/v1/b/bkt/o", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("pageToken") == "p2" {
			w.Write([]byte(`{"items":[{"name":"stats/installs/installs_io.x_202609_country.csv","generation":"2","size":"10"}]}`))
			return
		}
		w.Write([]byte(`{"items":[{"name":"stats/installs/installs_io.x_202609_overview.csv","generation":"1","md5Hash":"m","size":"100","updated":"2026-09-02T03:00:00Z"}],"nextPageToken":"p2"}`))
	})
	mux.HandleFunc("/storage/v1/b/bkt/o/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(utf16le("Date,Package Name,Daily User Installs\n2026-09-01,io.x,7\n"))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"reviews":[{"reviewId":"gp:1","authorName":"Ann","comments":[{"userComment":{"text":"nice","lastModified":{"seconds":"1756720000"},"starRating":4,"reviewerLanguage":"en","device":"pixel","appVersionName":"1.0"}},{"developerComment":{"text":"ty","lastModified":{"seconds":"1756800000"}}}]}]}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"e1"}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) })
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1/details", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"defaultLanguage":"en-US"}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1/listings/en-US", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"My Notes"}`))
	})
	// The app icon is a listing-level image type: /listings/{lang}/icon.
	// Screenshots are the ones nested under a form factor (phone/, tenTablet/).
	// This mock previously served "phone/icon", mirroring the same wrong
	// assumption the client made, so the pair agreed with each other and the
	// real API 404'd in production.
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1/listings/en-US/icon", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"images":[{"url":"https://img/icon"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(testSA(t, srv.URL+"/token"), "bkt")
	if err != nil {
		t.Fatal(err)
	}
	c.StorageURL, c.PublisherURL = srv.URL, srv.URL
	c.http.Throttle = 0
	ctx := context.Background()

	objs, err := c.ListObjects(ctx, "stats/installs/")
	if err != nil || len(objs) != 2 {
		t.Fatalf("list: %v %+v", err, objs)
	}
	if tokenCalls != 1 {
		t.Errorf("token calls = %d", tokenCalls)
	}
	data, err := c.Download(ctx, objs[0].Name)
	if err != nil || !strings.HasPrefix(string(data), "Date,") {
		t.Fatalf("download: %v %q", err, data)
	}
	rows, _ := ParseStats(data)
	if len(rows) != 1 || rows[0].DailyUserInstalls != 7 {
		t.Errorf("rows: %+v", rows)
	}
	rvs, err := c.Reviews(ctx, "io.x")
	if err != nil || len(rvs) != 1 || rvs[0].Rating != 4 || rvs[0].ReplyText != "ty" || rvs[0].CreatedAt.IsZero() {
		t.Fatalf("reviews: %v %+v", err, rvs)
	}
	l, err := c.Listing(ctx, "io.x")
	if err != nil || l.Title != "My Notes" || l.IconURL != "https://img/icon" {
		t.Fatalf("listing: %v %+v", err, l)
	}
	if tokenCalls != 1 {
		t.Errorf("token not cached: %d", tokenCalls)
	}
	if err := c.Ping(ctx); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// A listing whose icon endpoint fails must report the failure rather than
// return an empty IconURL as if the app simply had no icon. The original bug
// requested a path that 404s for every app and swallowed the error, so the
// dashboard showed placeholders and no log line ever said why.
func TestListingIconErrorIsReported(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"e1"}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) })
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1/details", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"defaultLanguage":"en-US"}`))
	})
	mux.HandleFunc("/androidpublisher/v3/applications/io.x/edits/e1/listings/en-US", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"title":"My Notes"}`))
	})
	// No handler for .../icon: the mux answers 404, exactly like the real API
	// did for the wrong path.
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New(testSA(t, srv.URL+"/token"), "bkt")
	if err != nil {
		t.Fatal(err)
	}
	c.StorageURL, c.PublisherURL = srv.URL, srv.URL
	c.http.Throttle = 0

	l, err := c.Listing(context.Background(), "io.x")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if l.Title != "My Notes" {
		t.Errorf("title = %q, want the listing title even when the icon fails", l.Title)
	}
	if l.IconURL != "" {
		t.Errorf("IconURL = %q, want empty", l.IconURL)
	}
	if l.IconErr == nil {
		t.Fatal("IconErr is nil: a failed icon lookup was swallowed, which is the bug")
	}
}
