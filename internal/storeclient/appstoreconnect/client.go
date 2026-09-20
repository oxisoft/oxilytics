// Package appstoreconnect is a thin client for the App Store Connect API v1
// and the public iTunes lookup endpoint.
package appstoreconnect

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/oxisoft/oxilytics/internal/storeclient"
)

const (
	DefaultBaseURL = "https://api.appstoreconnect.apple.com"
	tokenLifetime  = 15 * time.Minute
)

type Client struct {
	BaseURL   string
	LookupURL string // iTunes lookup, default https://itunes.apple.com
	keyID     string
	issuerID  string
	key       *ecdsa.PrivateKey
	http      *storeclient.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func New(keyID, issuerID string, p8 []byte) (*Client, error) {
	block, _ := pem.Decode(p8)
	if block == nil {
		return nil, errors.New("key file is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not ECDSA")
	}
	hc := storeclient.NewClient()
	hc.Throttle = 350 * time.Millisecond // ~3 req/s, well under Apple's limits
	return &Client{BaseURL: DefaultBaseURL, LookupURL: "https://itunes.apple.com", keyID: keyID, issuerID: issuerID, key: ec, http: hc}, nil
}

// JWT ------------------------------------------------------------------------

func (c *Client) jwt() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExp) > time.Minute {
		return c.token, nil
	}
	now := time.Now()
	hdr, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": c.keyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": c.issuerID, "iat": now.Unix(), "exp": now.Add(tokenLifetime).Unix(), "aud": "appstoreconnect-v1"})
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(hdr) + "." + enc.EncodeToString(claims)
	h := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, c.key, h[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	c.token = signing + "." + enc.EncodeToString(sig)
	c.tokenExp = now.Add(tokenLifetime)
	return c.token, nil
}

var _ crypto.Hash = crypto.SHA256
var _ = big.NewInt

// transport ------------------------------------------------------------------

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any) ([]byte, error) {
	tok, err := c.jwt()
	if err != nil {
		return nil, err
	}
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.BaseURL + path
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	out, _, err := c.http.Do(ctx, req)
	return out, err
}

// generic JSON:API envelope
type page[T any] struct {
	Data  []T `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

func getAll[T any](ctx context.Context, c *Client, path string, q url.Values, max int) ([]T, error) {
	var out []T
	next := path
	for next != "" {
		var b []byte
		var err error
		if strings.HasPrefix(next, "http") {
			b, err = c.do(ctx, "GET", next, nil, nil)
		} else {
			b, err = c.do(ctx, "GET", next, q, nil)
		}
		if err != nil {
			return out, err
		}
		var p page[T]
		if err := json.Unmarshal(b, &p); err != nil {
			return out, fmt.Errorf("decode: %w", err)
		}
		out = append(out, p.Data...)
		if max > 0 && len(out) >= max {
			break
		}
		next = p.Links.Next
		q = nil
	}
	return out, nil
}

// apps -----------------------------------------------------------------------

type App struct {
	ID         string
	Name       string
	BundleID   string
	SKU        string
	PrimaryLoc string
}

type appResource struct {
	ID         string `json:"id"`
	Attributes struct {
		Name          string `json:"name"`
		BundleID      string `json:"bundleId"`
		SKU           string `json:"sku"`
		PrimaryLocale string `json:"primaryLocale"`
	} `json:"attributes"`
}

func (c *Client) Apps(ctx context.Context) ([]App, error) {
	q := url.Values{"fields[apps]": {"name,bundleId,sku,primaryLocale"}, "limit": {"200"}}
	rs, err := getAll[appResource](ctx, c, "/v1/apps", q, 0)
	if err != nil {
		return nil, err
	}
	out := make([]App, 0, len(rs))
	for _, r := range rs {
		out = append(out, App{ID: r.ID, Name: r.Attributes.Name, BundleID: r.Attributes.BundleID, SKU: r.Attributes.SKU, PrimaryLoc: r.Attributes.PrimaryLocale})
	}
	return out, nil
}

// Ping is the cheapest authenticated call.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "GET", "/v1/apps", url.Values{"limit": {"1"}}, nil)
	return err
}

// reviews --------------------------------------------------------------------

type Review struct {
	ID        string
	Rating    int
	Title     string
	Body      string
	Reviewer  string
	Territory string
	CreatedAt time.Time
	Reply     *ReviewReply
}

type ReviewReply struct {
	Body       string
	ModifiedAt time.Time
}

type reviewResource struct {
	ID         string `json:"id"`
	Attributes struct {
		Rating            int       `json:"rating"`
		Title             string    `json:"title"`
		Body              string    `json:"body"`
		ReviewerNickname  string    `json:"reviewerNickname"`
		CreatedDate       time.Time `json:"createdDate"`
		Territory         string    `json:"territory"`
	} `json:"attributes"`
	Relationships struct {
		Response struct {
			Data *struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"response"`
	} `json:"relationships"`
}

type reviewsPage struct {
	Data     []reviewResource `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			ResponseBody     string    `json:"responseBody"`
			LastModifiedDate time.Time `json:"lastModifiedDate"`
		} `json:"attributes"`
	} `json:"included"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// Reviews walks customerReviews newest-first and stops once createdDate is
// before `since` (zero = all). Each page is passed to fn; return false to stop.
func (c *Client) Reviews(ctx context.Context, appID string, since time.Time, fn func([]Review) bool) error {
	q := url.Values{"sort": {"-createdDate"}, "limit": {"200"}, "include": {"response"}}
	next := "/v1/apps/" + appID + "/customerReviews"
	for next != "" {
		var b []byte
		var err error
		if strings.HasPrefix(next, "http") {
			b, err = c.do(ctx, "GET", next, nil, nil)
		} else {
			b, err = c.do(ctx, "GET", next, q, nil)
		}
		if err != nil {
			return err
		}
		var p reviewsPage
		if err := json.Unmarshal(b, &p); err != nil {
			return fmt.Errorf("decode reviews: %w", err)
		}
		replies := map[string]ReviewReply{}
		for _, inc := range p.Included {
			if inc.Type == "customerReviewResponses" {
				replies[inc.ID] = ReviewReply{Body: inc.Attributes.ResponseBody, ModifiedAt: inc.Attributes.LastModifiedDate}
			}
		}
		out := make([]Review, 0, len(p.Data))
		stop := false
		for _, r := range p.Data {
			if !since.IsZero() && r.Attributes.CreatedDate.Before(since) {
				stop = true
				break
			}
			rv := Review{ID: r.ID, Rating: r.Attributes.Rating, Title: r.Attributes.Title, Body: r.Attributes.Body, Reviewer: r.Attributes.ReviewerNickname, Territory: r.Attributes.Territory, CreatedAt: r.Attributes.CreatedDate}
			if r.Relationships.Response.Data != nil {
				if rp, ok := replies[r.Relationships.Response.Data.ID]; ok {
					rv.Reply = &rp
				}
			}
			out = append(out, rv)
		}
		if len(out) > 0 && !fn(out) {
			return nil
		}
		if stop {
			return nil
		}
		next = p.Links.Next
	}
	return nil
}

// iTunes lookup (public) ------------------------------------------------------

type Lookup struct {
	ArtworkURL  string
	RatingAvg   float64
	RatingCount int64
	Kind        string // "software" / "mac-software"
}

func (c *Client) Lookup(ctx context.Context, appID, country string) (*Lookup, error) {
	u := fmt.Sprintf("%s/lookup?id=%s&country=%s", c.LookupURL, url.QueryEscape(appID), url.QueryEscape(country))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	b, _, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []struct {
			ArtworkURL512   string  `json:"artworkUrl512"`
			ArtworkURL100   string  `json:"artworkUrl100"`
			AverageRating   float64 `json:"averageUserRating"`
			UserRatingCount int64   `json:"userRatingCount"`
			Kind            string  `json:"kind"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	if len(res.Results) == 0 {
		return nil, storeclient.ErrNotFound
	}
	r := res.Results[0]
	art := r.ArtworkURL512
	if art == "" {
		art = r.ArtworkURL100
	}
	return &Lookup{ArtworkURL: art, RatingAvg: r.AverageRating, RatingCount: r.UserRatingCount, Kind: r.Kind}, nil
}

// analytics reports ----------------------------------------------------------

type AccessType string

const (
	AccessOneTime AccessType = "ONE_TIME_SNAPSHOT"
	AccessOngoing AccessType = "ONGOING"
)

type ReportRequest struct {
	ID         string
	AccessType AccessType
	Stopped    bool
}

type reportRequestResource struct {
	ID         string `json:"id"`
	Attributes struct {
		AccessType AccessType `json:"accessType"`
		// Apple returns a JSON boolean here, not a string.
		StoppedDueTo bool `json:"stoppedDueToInactivity"`
	} `json:"attributes"`
}

// ReportRequests lists existing requests for an app.
func (c *Client) ReportRequests(ctx context.Context, appID string) ([]ReportRequest, error) {
	rs, err := getAll[reportRequestResource](ctx, c, "/v1/apps/"+appID+"/analyticsReportRequests", url.Values{"limit": {"200"}}, 0)
	if err != nil {
		return nil, err
	}
	out := make([]ReportRequest, 0, len(rs))
	for _, r := range rs {
		out = append(out, ReportRequest{ID: r.ID, AccessType: r.Attributes.AccessType, Stopped: r.Attributes.StoppedDueTo})
	}
	return out, nil
}

// CreateReportRequest asks Apple to start producing reports for the app.
func (c *Client) CreateReportRequest(ctx context.Context, appID string, access AccessType) (*ReportRequest, error) {
	body := map[string]any{"data": map[string]any{
		"type":          "analyticsReportRequests",
		"attributes":    map[string]any{"accessType": access},
		"relationships": map[string]any{"app": map[string]any{"data": map[string]any{"type": "apps", "id": appID}}},
	}}
	b, err := c.do(ctx, "POST", "/v1/analyticsReportRequests", nil, body)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data reportRequestResource `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	return &ReportRequest{ID: res.Data.ID, AccessType: res.Data.Attributes.AccessType}, nil
}

type Report struct {
	ID       string
	Name     string
	Category string
}

type reportResource struct {
	ID         string `json:"id"`
	Attributes struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"attributes"`
}

// Reports lists the reports available under a request, optionally filtered by name.
func (c *Client) Reports(ctx context.Context, requestID, name string) ([]Report, error) {
	q := url.Values{"limit": {"200"}}
	if name != "" {
		q.Set("filter[name]", name)
	}
	rs, err := getAll[reportResource](ctx, c, "/v1/analyticsReportRequests/"+requestID+"/reports", q, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(rs))
	for _, r := range rs {
		out = append(out, Report{ID: r.ID, Name: r.Attributes.Name, Category: r.Attributes.Category})
	}
	return out, nil
}

type Instance struct {
	ID             string
	Granularity    string
	ProcessingDate string // YYYY-MM-DD
}

type instanceResource struct {
	ID         string `json:"id"`
	Attributes struct {
		Granularity    string `json:"granularity"`
		ProcessingDate string `json:"processingDate"`
	} `json:"attributes"`
}

// Instances lists DAILY instances for a report.
func (c *Client) Instances(ctx context.Context, reportID string) ([]Instance, error) {
	q := url.Values{"filter[granularity]": {"DAILY"}, "limit": {"200"}}
	rs, err := getAll[instanceResource](ctx, c, "/v1/analyticsReports/"+reportID+"/instances", q, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(rs))
	for _, r := range rs {
		out = append(out, Instance{ID: r.ID, Granularity: r.Attributes.Granularity, ProcessingDate: r.Attributes.ProcessingDate})
	}
	return out, nil
}

type segmentResource struct {
	ID         string `json:"id"`
	Attributes struct {
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
	} `json:"attributes"`
}

// SegmentURLs returns the signed download URLs for an instance.
func (c *Client) SegmentURLs(ctx context.Context, instanceID string) ([]string, error) {
	rs, err := getAll[segmentResource](ctx, c, "/v1/analyticsReportInstances/"+instanceID+"/segments", url.Values{"limit": {"200"}}, 0)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Attributes.URL)
	}
	return out, nil
}

// DownloadSegment fetches a gzipped CSV segment and returns the decompressed bytes.
func (c *Client) DownloadSegment(ctx context.Context, signedURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", signedURL, nil)
	if err != nil {
		return nil, err
	}
	b, _, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return io.ReadAll(io.LimitReader(gz, 256<<20))
	}
	return b, nil
}
