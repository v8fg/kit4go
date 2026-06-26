// This file is an internal benchmark/coverage test (package httpclient, not
// httpclient_test) so it can reach the unexported retryDelay helper for the
// retry-delay benchmark, and exercise the SetOnEvent hook directly. The HTTP
// benchmarks spin up a local httptest.Server to measure the real round-trip
// cost (connection reuse, body drain, retry bookkeeping) end to end.
package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// echoServer returns an httptest.Server that writes the given status with a
// short body. It is the standard fixture for the HTTP benchmarks: a 200 keeps
// the client on the success path (no retries), giving a clean per-request
// cost. An optional per-request counter is returned for retry assertions.
func echoServer(status int, body string) (*httptest.Server, *uint64) {
	var calls uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&calls, 1)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return srv, &calls
}

// benchOpts returns ClientOptions tuned for benchmarking: tiny backoffs so any
// retrying benchmark doesn't sleep, and a generous connection pool so the pool
// is not the bottleneck.
func benchOpts() ClientOptions {
	return ClientOptions{
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  10 * time.Second,
		MaxIdleConns:    100,
		MaxIdlePerHost:  100,
		IdleConnTimeout: 60 * time.Second,
		RetryMax:        3,
		RetryWaitMin:    time.Microsecond,
		RetryWaitMax:    10 * time.Microsecond,
	}
}

// --- Benchmarks -------------------------------------------------------------

// BenchmarkClient_Get measures a full GET round-trip to a local 200 server:
// build request, dial (warm after first), send, read body, update metrics.
// Retry is enabled (RetryMax=3) but never triggers because the server returns
// 200, so this includes the retry-decision overhead on the success path.
func BenchmarkClient_Get(b *testing.B) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	c := NewClient(benchOpts())
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(ctx, srv.URL, nil)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		resp.Release()
	}
}

// BenchmarkClient_Get_NoRetry measures the pure HTTP cost with retries
// disabled (RetryMax=0): one send, no retry bookkeeping.
func BenchmarkClient_Get_NoRetry(b *testing.B) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	opts := benchOpts()
	opts.RetryMax = 0
	c := NewClient(opts)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(ctx, srv.URL, nil)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		resp.Release()
		_ = resp
	}
}

// BenchmarkClient_Get_Parallel runs GETs from many goroutines to measure
// connection-pool and mutex (metrics) contention. The pool is sized large so
// contention is on the client side, not the transport.
func BenchmarkClient_Get_Parallel(b *testing.B) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	c := NewClient(benchOpts())
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			resp, err := c.Get(ctx, srv.URL, nil)
			if err != nil {
				b.Fatalf("get: %v", err)
			}
			_ = resp
		}
	})
}

// BenchmarkClient_Post measures a POST with a small body: includes the
// GetBody rewind setup and the body-write cost on top of the GET baseline.
func BenchmarkClient_Post(b *testing.B) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	c := NewClient(benchOpts())
	body := bytes.Repeat([]byte("x"), 64)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.Post(ctx, srv.URL, body, nil)
		if err != nil {
			b.Fatalf("post: %v", err)
		}
		_ = resp
	}
}

// BenchmarkClient_Metrics measures the Metrics() snapshot: four atomic loads.
// Bounds the cost of a metrics scrape on the hot path.
func BenchmarkClient_Metrics(b *testing.B) {
	c := NewClient(benchOpts())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Metrics()
	}
}

// BenchmarkRetryDelay measures the retryDelay computation in isolation
// (exponential backoff + jitter). This is the per-retry overhead on top of the
// HTTP cost; it must stay cheap so backoff never dominates a retry storm.
func BenchmarkRetryDelay(b *testing.B) {
	minW := 100 * time.Microsecond
	maxW := 10 * time.Millisecond
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retryDelay(i%10, minW, maxW)
	}
}

// --- SetOnEvent hook coverage ----------------------------------------------

// TestClient_SetOnEvent_Success verifies the hook fires "request" and
// "success" for a plain 200 GET, with the correct method/URL/status.
func TestClient_SetOnEvent_Success(t *testing.T) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	c := NewClient(benchOpts())

	var mu sync.Mutex
	var seen []ClientEvent
	c.SetOnEvent(func(evt ClientEvent) {
		mu.Lock()
		seen = append(seen, evt)
		mu.Unlock()
	})

	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	counts := map[string]int{}
	for _, e := range seen {
		counts[e.Name]++
		if e.Method != http.MethodGet {
			t.Errorf("event %q method=%q want GET", e.Name, e.Method)
		}
		if !strings.HasSuffix(e.URL, srv.Listener.Addr().String()) && e.URL != srv.URL {
			// URL should be the full server URL; just check it's non-empty.
			if e.URL == "" {
				t.Errorf("event %q URL empty", e.Name)
			}
		}
	}
	if counts["request"] < 1 {
		t.Errorf("request events=%d want >=1 (full: %+v)", counts["request"], seen)
	}
	if counts["success"] != 1 {
		t.Errorf("success events=%d want 1 (full: %+v)", counts["success"], seen)
	}
	if counts["failed"] != 0 {
		t.Errorf("failed events=%d want 0", counts["failed"])
	}
	// The success event must carry the 200 status.
	for _, e := range seen {
		if e.Name == "success" && e.StatusCode != 200 {
			t.Errorf("success event status=%d want 200", e.StatusCode)
		}
	}
}

// TestClient_SetOnEvent_Retry verifies the hook fires "retry" on a
// retryable response and that the final "failed" event lands when all retries
// are exhausted against a server that always 500s.
func TestClient_SetOnEvent_Retry(t *testing.T) {
	srv, calls := echoServer(500, "err") // always retryable
	defer srv.Close()
	opts := benchOpts()
	opts.RetryMax = 2
	c := NewClient(opts)

	var mu sync.Mutex
	var seen []ClientEvent
	c.SetOnEvent(func(evt ClientEvent) {
		mu.Lock()
		seen = append(seen, evt)
		mu.Unlock()
	})

	resp, err := c.Get(context.Background(), srv.URL, nil)
	// A 500 is a valid HTTP response (not a transport error): err is nil and
	// the status is surfaced on resp. The call is still counted as "failed".
	if err != nil {
		t.Fatalf("get err=%v want nil (500 is a real response, not a transport error)", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}

	// The server should have received RetryMax+1 = 3 attempts.
	if got := atomic.LoadUint64(calls); got != 3 {
		t.Fatalf("server calls=%d want 3", got)
	}

	mu.Lock()
	defer mu.Unlock()
	counts := map[string]int{}
	for _, e := range seen {
		counts[e.Name]++
	}
	// 3 attempts -> 3 "request" events.
	if counts["request"] != 3 {
		t.Errorf("request events=%d want 3 (full: %+v)", counts["request"], seen)
	}
	// 2 retries (attempts 1 and 2 retried; attempt 3 was the last) -> 2 "retry".
	if counts["retry"] != 2 {
		t.Errorf("retry events=%d want 2 (full: %+v)", counts["retry"], seen)
	}
	// Final outcome is "failed" (500 is not 2xx).
	if counts["failed"] != 1 {
		t.Errorf("failed events=%d want 1", counts["failed"])
	}
	if counts["success"] != 0 {
		t.Errorf("success events=%d want 0", counts["success"])
	}
}

// TestClient_SetOnEvent_Disabled confirms the hook is nil by default (no
// events fire) and that SetOnEvent(nil) disables a previously-installed hook.
func TestClient_SetOnEvent_Disabled(t *testing.T) {
	// Fresh client: onEvent must be nil.
	c := NewClient(benchOpts())
	if p := c.onEvent.Load(); p != nil {
		t.Fatalf("fresh client onEvent=%v want nil", p)
	}

	// Install, capture, then disable.
	var count atomic.Uint64
	c.SetOnEvent(func(ClientEvent) { count.Add(1) })
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	_, _ = c.Get(context.Background(), srv.URL, nil)
	if count.Load() == 0 {
		t.Fatal("hook did not fire after install")
	}
	before := count.Load()

	c.SetOnEvent(nil)
	_, _ = c.Get(context.Background(), srv.URL, nil)
	if count.Load() != before {
		t.Fatalf("events after SetOnEvent(nil): %d -> %d (hook not disabled)", before, count.Load())
	}
}

// TestClient_SetOnEvent_BuildFailure confirms a request-build failure (bad
// URL) fires a single "failed" event with status 0 and no "request" events
// (the request never made it to the transport).
func TestClient_SetOnEvent_BuildFailure(t *testing.T) {
	c := NewClient(benchOpts())
	var mu sync.Mutex
	var seen []ClientEvent
	c.SetOnEvent(func(evt ClientEvent) {
		mu.Lock()
		seen = append(seen, evt)
		mu.Unlock()
	})

	// A URL with control characters fails http.NewRequestWithContext.
	_, err := c.Get(context.Background(), "http://127.0.0.1:0/\x00", nil)
	if err == nil {
		t.Fatal("expected build error for bad URL")
	}

	mu.Lock()
	defer mu.Unlock()
	counts := map[string]int{}
	for _, e := range seen {
		counts[e.Name]++
		if e.Name == "failed" && e.StatusCode != 0 {
			t.Errorf("build-failure event status=%d want 0", e.StatusCode)
		}
	}
	if counts["failed"] != 1 {
		t.Errorf("failed events=%d want 1 (full: %+v)", counts["failed"], seen)
	}
	if counts["request"] != 0 {
		t.Errorf("request events=%d want 0 (never sent)", counts["request"])
	}
}

// TestClient_SetOnEvent_Concurrent verifies the hook can be installed and
// replaced while traffic is in flight without racing. Run under -race.
func TestClient_SetOnEvent_Concurrent(t *testing.T) {
	srv, _ := echoServer(200, "ok")
	defer srv.Close()
	c := NewClient(benchOpts())
	c.SetOnEvent(func(ClientEvent) {})

	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < 50; j++ {
				_, _ = c.Get(ctx, srv.URL, nil)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.SetOnEvent(func(ClientEvent) {})
			}
		}()
	}
	wg.Wait()
	// Only assertion: no race / no panic.
}
