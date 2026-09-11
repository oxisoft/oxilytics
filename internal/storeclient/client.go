// Package storeclient holds shared HTTP plumbing for store API clients.
package storeclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

var (
	ErrAuth        = errors.New("authentication failed")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not found")
	ErrRateLimited = errors.New("rate limited")
	ErrNotReady    = errors.New("not ready")
)

// HTTPError carries the status and a body excerpt for diagnostics.
type HTTPError struct {
	Status int
	Body   string
	Kind   error
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

func (e *HTTPError) Unwrap() error { return e.Kind }

func classify(status int, body string) error {
	e := &HTTPError{Status: status, Body: truncate(body, 300)}
	switch {
	case status == 401:
		e.Kind = ErrAuth
	case status == 403:
		e.Kind = ErrForbidden
	case status == 404:
		e.Kind = ErrNotFound
	case status == 429:
		e.Kind = ErrRateLimited
	}
	return e
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Client wraps http.Client with retries on 429/5xx and a global throttle.
type Client struct {
	HTTP      *http.Client
	MaxRetry  int
	Throttle  time.Duration // min gap between requests (0 = none)
	UserAgent string
	last      time.Time
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 60 * time.Second}, MaxRetry: 4, UserAgent: "oxilytics/1.0"}
}

// Do executes the request with retry/backoff. The body is read fully and
// returned; callers get a nil error only for 2xx.
func (c *Client) Do(ctx context.Context, req *http.Request) ([]byte, http.Header, error) {
	if c.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetry; attempt++ {
		if c.Throttle > 0 {
			if wait := c.Throttle - time.Since(c.last); wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				}
			}
			c.last = time.Now()
		}
		resp, err := c.HTTP.Do(req.Clone(ctx))
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			backoff(ctx, attempt, 0)
			continue
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			backoff(ctx, attempt, 0)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, resp.Header, nil
		}
		lastErr = classify(resp.StatusCode, string(body))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			var ra time.Duration
			if s := resp.Header.Get("Retry-After"); s != "" {
				if n, err := strconv.Atoi(s); err == nil {
					ra = time.Duration(n) * time.Second
				}
			}
			if !backoff(ctx, attempt, ra) {
				return nil, nil, ctx.Err()
			}
			continue
		}
		return nil, resp.Header, lastErr
	}
	return nil, nil, lastErr
}

func backoff(ctx context.Context, attempt int, retryAfter time.Duration) bool {
	d := retryAfter
	if d == 0 {
		d = time.Duration(1<<uint(attempt))*time.Second + time.Duration(rand.Intn(500))*time.Millisecond
	}
	if d > 2*time.Minute {
		d = 2 * time.Minute
	}
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
