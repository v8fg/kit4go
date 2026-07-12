package httpclient_test

import (
	"context"
	"io"
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

// droppedConnServer is an httptest server that reads the request body (simulating
// "server processed it"), then hijacks and closes the raw connection WITHOUT a
// response. The client sent the request, the server received it, but the
// response is lost — surfacing as resp==nil with io.EOF / io.ErrUnexpectedEOF.
func droppedConnServer(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("httptest server not hijackable")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_ = conn.Close()
	}))
}

// Regression: a non-idempotent POST whose request was sent but whose response
// was lost must NOT be retried — resp==nil with io.EOF/ErrUnexpectedEOF means the
// server did receive the body and may have processed it (double-charge risk).
func TestClient_PostNotRetriedOnSentButNoResponse(t *testing.T) {
	var hits atomic.Int64
	srv := droppedConnServer(t, &hits)
	defer srv.Close()

	c := httpclient.NewClient(httpclient.ClientOptions{
		RetryMax: 3, RetryWaitMin: time.Millisecond, RetryWaitMax: 2 * time.Millisecond,
	})
	_, _ = c.Post(context.Background(), srv.URL, []byte("charge"), nil)

	if got := hits.Load(); got != 1 {
		t.Fatalf("POST retried on sent-but-no-response: server hit %d times, want 1 (double-charge risk)", got)
	}
}

// Counter-check: an idempotent PUT to the same response-lost server IS retried
// (the EOF class stays retryable for idempotent methods).
func TestClient_PutRetriedOnSentButNoResponse(t *testing.T) {
	var hits atomic.Int64
	srv := droppedConnServer(t, &hits)
	defer srv.Close()

	c := httpclient.NewClient(httpclient.ClientOptions{
		RetryMax: 3, RetryWaitMin: time.Millisecond, RetryWaitMax: 2 * time.Millisecond,
	})
	_, _ = c.Put(context.Background(), srv.URL, []byte("x"), nil)

	if got, want := hits.Load(), int64(4); got != want { // 1 + RetryMax(3)
		t.Fatalf("idempotent PUT not retried on sent-but-no-response: server hit %d, want %d", got, want)
	}
}
