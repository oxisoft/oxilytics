package storeclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"errors"
)

func TestDoRetriesAndClassifies(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&n, 1) {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
		case 2:
			w.WriteHeader(503)
		default:
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	c := NewClient()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	body, _, err := c.Do(context.Background(), req)
	if err != nil || string(body) != "ok" {
		t.Fatalf("got %q %v after %d attempts", body, err, n)
	}
	if n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestDoNoRetryOn4xx(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(403)
		w.Write([]byte(`{"errors":[{"detail":"nope"}]}`))
	}))
	defer srv.Close()
	c := NewClient()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, _, err := c.Do(context.Background(), req)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	if n != 1 {
		t.Errorf("attempts = %d", n)
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 403 {
		t.Errorf("HTTPError not carried: %v", err)
	}
}
