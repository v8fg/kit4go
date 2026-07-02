package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/httpclient"
)

// Regression: a non-idempotent method (POST) that receives a 5xx response must
// NOT be retried — the server may have produced its side effect, so a retry
// would duplicate it (double charge / duplicate write).
func TestClient_PostNotRetriedOn5xx(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.NewClient(httpclient.ClientOptions{
		RetryMax: 3, RetryWaitMin: time.Millisecond, RetryWaitMax: 2 * time.Millisecond,
	})
	_, _ = c.Post(context.Background(), srv.URL, nil, nil)

	if got := hits.Load(); got != 1 {
		t.Fatalf("POST retried on 5xx: server hit %d times, want 1 (non-idempotent)", got)
	}
}

// Counter-check: an idempotent method (GET) IS retried on 5xx (RetryMax+1 sends).
func TestClient_GetRetriedOn5xx(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.NewClient(httpclient.ClientOptions{
		RetryMax: 3, RetryWaitMin: time.Millisecond, RetryWaitMax: 2 * time.Millisecond,
	})
	_, _ = c.Get(context.Background(), srv.URL, nil)

	if want := int64(4); hits.Load() != want { // RetryMax=3 => 1 + 3 retries
		t.Fatalf("GET not retried on 5xx: server hit %d times, want %d", hits.Load(), want)
	}
}
