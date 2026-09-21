package appstoreconnect

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestJWTShape(t *testing.T) {
	c, err := New("KEYID12345", "issuer-uuid", testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := c.jwt()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts = %d", len(parts))
	}
	tok2, _ := c.jwt()
	if tok != tok2 {
		t.Error("token not cached")
	}
}

func TestAppsReviewsAndReports(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"data":[{"id":"111","attributes":{"name":"My Notes","bundleId":"io.x.notes","sku":"NOTES","primaryLocale":"en-US"}}],"links":{"next":""}}`))
	})
	mux.HandleFunc("/v1/apps/111/customerReviews", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(`{"data":[{"id":"r3","attributes":{"rating":2,"title":"old","body":"meh","reviewerNickname":"c","createdDate":"2025-01-01T00:00:00Z","territory":"DEU"}}],"links":{"next":""}}`))
			return
		}
		w.Write([]byte(`{"data":[
		 {"id":"r1","attributes":{"rating":5,"title":"Great","body":"love it","reviewerNickname":"a","createdDate":"2026-09-01T10:00:00Z","territory":"USA"},"relationships":{"response":{"data":{"id":"resp1"}}}},
		 {"id":"r2","attributes":{"rating":4,"title":"Good","body":"ok","reviewerNickname":"b","createdDate":"2026-08-01T10:00:00Z","territory":"GBR"}}],
		 "included":[{"type":"customerReviewResponses","id":"resp1","attributes":{"responseBody":"thanks!","lastModifiedDate":"2026-09-02T00:00:00Z"}}],
		 "links":{"next":"http://` + r.Host + `/v1/apps/111/customerReviews?page=2"}}`))
	})
	mux.HandleFunc("/v1/apps/111/analyticsReportRequests", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"req1","attributes":{"accessType":"ONGOING","stoppedDueToInactivity":false}}],"links":{}}`))
	})
	mux.HandleFunc("/v1/analyticsReportRequests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		w.Write([]byte(`{"data":{"id":"req2","attributes":{"accessType":"ONE_TIME_SNAPSHOT"}}}`))
	})
	mux.HandleFunc("/v1/analyticsReportRequests/req1/reports", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter[name]") != ReportDownloads {
			w.Write([]byte(`{"data":[]}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"rep1","attributes":{"name":"App Store Downloads","category":"APP_USAGE"}}]}`))
	})
	mux.HandleFunc("/v1/analyticsReports/rep1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"inst1","attributes":{"granularity":"DAILY","processingDate":"2026-09-02"}}]}`))
	})
	mux.HandleFunc("/v1/analyticsReportInstances/inst1/segments", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"seg1","attributes":{"url":"http://` + r.Host + `/seg1.gz","checksum":"x"}}]}`))
	})
	mux.HandleFunc("/seg1.gz", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte("Date\tApp Apple Identifier\tApp Name\tDownload Type\tTerritory\tCounts\n2026-09-01\t111\tMy Notes\tFirst-time download\tUnited States\t12\n2026-09-01\t111\tMy Notes\tRedownload\tUnited States\t3\n2026-09-01\t111\tMy Notes\tUpdate\tGermany\t40\n"))
		gz.Close()
		w.Write(buf.Bytes())
	})
	mux.HandleFunc("/lookup", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"resultCount":1,"results":[{"artworkUrl512":"https://img/512.png","averageUserRating":4.6,"userRatingCount":1234,"kind":"software"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, _ := New("K", "I", testKey(t))
	c.BaseURL = srv.URL
	c.LookupURL = srv.URL
	c.http.Throttle = 0
	ctx := context.Background()

	apps, err := c.Apps(ctx)
	if err != nil || len(apps) != 1 || apps[0].BundleID != "io.x.notes" {
		t.Fatalf("apps: %v %+v", err, apps)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("auth header = %q", gotAuth)
	}

	// reviews: since 2026-07-01 → r1, r2 then stops before paging to r3
	var got []Review
	err = c.Reviews(ctx, "111", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), func(rs []Review) bool { got = append(got, rs...); return true })
	if err != nil || len(got) != 2 {
		t.Fatalf("reviews: %v %d", err, len(got))
	}
	if got[0].Reply == nil || got[0].Reply.Body != "thanks!" {
		t.Errorf("reply not joined: %+v", got[0])
	}
	// all reviews → 3
	got = nil
	_ = c.Reviews(ctx, "111", time.Time{}, func(rs []Review) bool { got = append(got, rs...); return true })
	if len(got) != 3 {
		t.Errorf("all reviews = %d", len(got))
	}

	reqs, err := c.ReportRequests(ctx, "111")
	if err != nil || len(reqs) != 1 || reqs[0].AccessType != AccessOngoing {
		t.Fatalf("report requests: %v %+v", err, reqs)
	}
	nr, err := c.CreateReportRequest(ctx, "111", AccessOneTime)
	if err != nil || nr.ID != "req2" {
		t.Fatalf("create request: %v %+v", err, nr)
	}
	reps, err := c.Reports(ctx, "req1", ReportDownloads)
	if err != nil || len(reps) != 1 {
		t.Fatalf("reports: %v %+v", err, reps)
	}
	insts, err := c.Instances(ctx, "rep1", GranularityDaily)
	if err != nil || len(insts) != 1 || insts[0].ProcessingDate != "2026-09-02" {
		t.Fatalf("instances: %v %+v", err, insts)
	}
	urls, err := c.SegmentURLs(ctx, "inst1")
	if err != nil || len(urls) != 1 {
		t.Fatalf("segments: %v %v", err, urls)
	}
	data, err := c.DownloadSegment(ctx, urls[0])
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseReport(data)
	if err != nil || len(rows) != 3 {
		t.Fatalf("parse: %v %+v", err, rows)
	}
	if rows[0].Downloads != 12 || rows[1].Redownloads != 3 || rows[2].Updates != 40 || Territory(rows[2].Territory) != "DE" {
		t.Errorf("rows: %+v", rows)
	}

	lk, err := c.Lookup(ctx, "111", "us")
	if err != nil || lk.RatingAvg != 4.6 || lk.RatingCount != 1234 || lk.ArtworkURL == "" {
		t.Fatalf("lookup: %v %+v", err, lk)
	}
	if err := c.Ping(ctx); err != nil {
		t.Errorf("ping: %v", err)
	}
}

func TestParseReportShapes(t *testing.T) {
	// Installation and Deletion report (Event column)
	rows, err := ParseReport([]byte("Date\tApp Apple Identifier\tEvent\tTerritory\tCounts\n2026-09-01\t1\tInstall\tPL\t5\n2026-09-01\t1\tDelete\tPL\t2\n"))
	if err != nil || len(rows) != 2 || rows[0].Installs != 5 || rows[1].Deletions != 2 {
		t.Errorf("install/delete: %v %+v", err, rows)
	}
	// Crashes report (column per metric)
	rows, err = ParseReport([]byte("Date\tApp Apple Identifier\tTerritory\tCrashes\n2026-09-01\t1\tUS\t7\n"))
	if err != nil || len(rows) != 1 || rows[0].Crashes != 7 {
		t.Errorf("crashes: %v %+v", err, rows)
	}
	// empty
	rows, err = ParseReport(nil)
	if err != nil || rows != nil {
		t.Errorf("empty: %v %+v", err, rows)
	}
}
