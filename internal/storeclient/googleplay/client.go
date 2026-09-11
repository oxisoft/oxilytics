// Package googleplay is a client for the Play Console reports bucket (GCS)
// and the Android Publisher API, authenticated with a service account.
package googleplay

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/oxisoft/oxilytics/internal/storeclient"
)

const (
	DefaultStorageURL   = "https://storage.googleapis.com"
	DefaultPublisherURL = "https://androidpublisher.googleapis.com"
	DefaultTokenURL     = "https://oauth2.googleapis.com/token"
	scopes              = "https://www.googleapis.com/auth/devstorage.read_only https://www.googleapis.com/auth/androidpublisher"
)

type Client struct {
	StorageURL   string
	PublisherURL string
	TokenURL     string
	Bucket       string
	Email        string

	key  *rsa.PrivateKey
	http *storeclient.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func New(saJSON []byte, bucket string) (*Client, error) {
	var sa serviceAccount
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return nil, fmt.Errorf("service account json: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("service account json missing client_email/private_key")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("private_key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private_key is not RSA")
		}
		key = rk
	} else if k, err2 := x509.ParsePKCS1PrivateKey(block.Bytes); err2 == nil {
		key = k
	} else {
		return nil, fmt.Errorf("parse private_key: %w", err)
	}
	tokenURL := DefaultTokenURL
	if sa.TokenURI != "" {
		tokenURL = sa.TokenURI
	}
	hc := storeclient.NewClient()
	hc.Throttle = 100 * time.Millisecond
	return &Client{StorageURL: DefaultStorageURL, PublisherURL: DefaultPublisherURL, TokenURL: tokenURL, Bucket: bucket, Email: sa.ClientEmail, key: key, http: hc}, nil
}

// auth -----------------------------------------------------------------------

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExp) > 2*time.Minute {
		return c.token, nil
	}
	now := time.Now()
	enc := base64.RawURLEncoding
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": c.Email, "scope": scopes, "aud": c.TokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	signing := enc.EncodeToString(hdr) + "." + enc.EncodeToString(claims)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	assertion := signing + "." + enc.EncodeToString(sig)

	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, "POST", c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	b, _, err := c.http.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &res); err != nil || res.AccessToken == "" {
		return "", fmt.Errorf("token response: %s", string(b))
	}
	c.token = res.AccessToken
	c.tokenExp = now.Add(time.Duration(res.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	b, _, err := c.http.Do(ctx, req)
	return b, err
}

func (c *Client) doJSON(ctx context.Context, method, u string, body any) ([]byte, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var rdr *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = strings.NewReader(string(b))
	} else {
		rdr = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	b, _, err := c.http.Do(ctx, req)
	return b, err
}

// GCS bucket -----------------------------------------------------------------

type Object struct {
	Name       string
	Generation string
	MD5        string
	Size       int64
	Updated    time.Time
}

// ListObjects lists bucket objects under a prefix (all pages).
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	pageToken := ""
	for {
		q := url.Values{"prefix": {prefix}, "maxResults": {"1000"}, "fields": {"items(name,generation,md5Hash,size,updated),nextPageToken"}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		b, err := c.get(ctx, c.StorageURL+"/storage/v1/b/"+url.PathEscape(c.Bucket)+"/o?"+q.Encode())
		if err != nil {
			return out, err
		}
		var res struct {
			Items []struct {
				Name       string    `json:"name"`
				Generation string    `json:"generation"`
				MD5        string    `json:"md5Hash"`
				Size       string    `json:"size"`
				Updated    time.Time `json:"updated"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return out, fmt.Errorf("decode list: %w", err)
		}
		for _, it := range res.Items {
			var size int64
			fmt.Sscan(it.Size, &size)
			out = append(out, Object{Name: it.Name, Generation: it.Generation, MD5: it.MD5, Size: size, Updated: it.Updated})
		}
		if res.NextPageToken == "" {
			return out, nil
		}
		pageToken = res.NextPageToken
	}
}

// Download fetches an object's bytes, transcoding UTF-16 to UTF-8 when the
// file starts with a BOM (Play reports do).
func (c *Client) Download(ctx context.Context, name string) ([]byte, error) {
	b, err := c.get(ctx, c.StorageURL+"/storage/v1/b/"+url.PathEscape(c.Bucket)+"/o/"+url.PathEscape(name)+"?alt=media")
	if err != nil {
		return nil, err
	}
	return DecodeUTF16(b), nil
}

// Android Publisher ----------------------------------------------------------

type Review struct {
	ID           string
	Rating       int
	Text         string
	Author       string
	Language     string
	Device       string
	AppVersion   string
	CreatedAt    time.Time
	ModifiedAt   time.Time
	ReplyText    string
	ReplyAt      time.Time
}

// Reviews returns reviews from the last 7 days (API limitation).
func (c *Client) Reviews(ctx context.Context, pkg string) ([]Review, error) {
	var out []Review
	token := ""
	for {
		q := url.Values{"maxResults": {"100"}}
		if token != "" {
			q.Set("token", token)
		}
		b, err := c.get(ctx, c.PublisherURL+"/androidpublisher/v3/applications/"+url.PathEscape(pkg)+"/reviews?"+q.Encode())
		if err != nil {
			return out, err
		}
		var res struct {
			Reviews []struct {
				ReviewID   string `json:"reviewId"`
				AuthorName string `json:"authorName"`
				Comments   []struct {
					UserComment *struct {
						Text         string `json:"text"`
						LastModified struct {
							Seconds string `json:"seconds"`
						} `json:"lastModified"`
						StarRating   int    `json:"starRating"`
						ReviewerLang string `json:"reviewerLanguage"`
						Device       string `json:"device"`
						AppVersion   string `json:"appVersionName"`
					} `json:"userComment"`
					DeveloperComment *struct {
						Text         string `json:"text"`
						LastModified struct {
							Seconds string `json:"seconds"`
						} `json:"lastModified"`
					} `json:"developerComment"`
				} `json:"comments"`
			} `json:"reviews"`
			TokenPagination struct {
				NextPageToken string `json:"nextPageToken"`
			} `json:"tokenPagination"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return out, fmt.Errorf("decode reviews: %w", err)
		}
		for _, r := range res.Reviews {
			rv := Review{ID: r.ReviewID, Author: r.AuthorName}
			for _, cm := range r.Comments {
				if cm.UserComment != nil {
					uc := cm.UserComment
					rv.Text, rv.Rating, rv.Language, rv.Device, rv.AppVersion = uc.Text, uc.StarRating, uc.ReviewerLang, uc.Device, uc.AppVersion
					rv.ModifiedAt = secondsToTime(uc.LastModified.Seconds)
					if rv.CreatedAt.IsZero() {
						rv.CreatedAt = rv.ModifiedAt
					}
				}
				if cm.DeveloperComment != nil {
					rv.ReplyText = cm.DeveloperComment.Text
					rv.ReplyAt = secondsToTime(cm.DeveloperComment.LastModified.Seconds)
				}
			}
			out = append(out, rv)
		}
		if res.TokenPagination.NextPageToken == "" {
			return out, nil
		}
		token = res.TokenPagination.NextPageToken
	}
}

func secondsToTime(s string) time.Time {
	var n int64
	fmt.Sscan(s, &n)
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// Listing fetches the store title and icon through a throwaway edit.
type Listing struct {
	Title   string
	IconURL string
}

func (c *Client) Listing(ctx context.Context, pkg string) (*Listing, error) {
	base := c.PublisherURL + "/androidpublisher/v3/applications/" + url.PathEscape(pkg) + "/edits"
	b, err := c.doJSON(ctx, "POST", base, map[string]any{})
	if err != nil {
		return nil, err
	}
	var edit struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &edit); err != nil || edit.ID == "" {
		return nil, fmt.Errorf("edit insert: %s", string(b))
	}
	defer func() { _, _ = c.doJSON(ctx, "DELETE", base+"/"+edit.ID, nil) }()

	out := &Listing{}
	if b, err := c.get(ctx, base+"/"+edit.ID+"/details"); err == nil {
		var d struct {
			DefaultLanguage string `json:"defaultLanguage"`
		}
		_ = json.Unmarshal(b, &d)
		lang := d.DefaultLanguage
		if lang == "" {
			lang = "en-US"
		}
		if lb, err := c.get(ctx, base+"/"+edit.ID+"/listings/"+url.PathEscape(lang)); err == nil {
			var l struct {
				Title string `json:"title"`
			}
			_ = json.Unmarshal(lb, &l)
			out.Title = l.Title
		}
		if ib, err := c.get(ctx, base+"/"+edit.ID+"/listings/"+url.PathEscape(lang)+"/phone/icon"); err == nil {
			var im struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			}
			_ = json.Unmarshal(ib, &im)
			if len(im.Images) > 0 {
				out.IconURL = im.Images[0].URL
			}
		}
	}
	return out, nil
}

// Ping: obtain a token and list one object.
func (c *Client) Ping(ctx context.Context) error {
	q := url.Values{"maxResults": {"1"}, "prefix": {"stats/installs/"}}
	_, err := c.get(ctx, c.StorageURL+"/storage/v1/b/"+url.PathEscape(c.Bucket)+"/o?"+q.Encode())
	return err
}
