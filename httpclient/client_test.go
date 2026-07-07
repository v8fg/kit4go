package httpclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/httpclient"
)

// fastOpts returns ClientOptions with tiny timeouts/backoffs so the timeout
// and retry tests run in milliseconds rather than seconds.
func fastOpts() httpclient.ClientOptions {
	return httpclient.ClientOptions{
		ConnectTimeout:  50 * time.Millisecond,
		RequestTimeout:  100 * time.Millisecond,
		RetryMax:        3,
		RetryWaitMin:    1 * time.Millisecond,
		RetryWaitMax:    10 * time.Millisecond,
		IdleConnTimeout: 5 * time.Second,
	}
}

// countingServer returns a server whose handler atomically increments a
// counter on each request. Callers read the count via the returned *uint64 to
// verify how many attempts were made.
func countingServer(status int, body string) (*httptest.Server, *uint64) {
	var calls uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&calls, 1)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	return srv, &calls
}

// statusSequence returns a server that responds with the given status codes in
// order, one per request, repeating the last once the sequence is exhausted.
// It also tracks total calls.
func statusSequence(codes ...int) (*httptest.Server, *uint64) {
	var calls uint64
	idx := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&calls, 1)
		mu.Lock()
		code := codes[idx]
		if idx < len(codes)-1 {
			idx++
		}
		mu.Unlock()
		w.WriteHeader(code)
		_, _ = fmt.Fprintf(w, "call-%d-status-%d", atomic.LoadUint64(&calls), code)
	}))
	return srv, &calls
}

func TestClient_Get_Success(t *testing.T) {
	srv, calls := countingServer(http.StatusOK, "hello")
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: unexpected err %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if string(resp.Body) != "hello" {
		t.Fatalf("Get: body = %q, want %q", resp.Body, "hello")
	}
	if got := atomic.LoadUint64(calls); got != 1 {
		t.Fatalf("Get: server calls = %d, want 1", got)
	}
	if resp.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("Get: Content-Type = %q, want text/plain", resp.Header.Get("Content-Type"))
	}
}

func TestClient_Post_BodyReceived(t *testing.T) {
	var gotBody string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	resp, err := c.Post(context.Background(), srv.URL, []byte("payload"), nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Post: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("Post: method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotBody != "payload" {
		t.Fatalf("Post: server received body %q, want %q", gotBody, "payload")
	}
}

func TestClient_Put_And_Delete(t *testing.T) {
	var sawPut, sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			sawPut = true
		case http.MethodDelete:
			sawDelete = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	if _, err := c.Put(context.Background(), srv.URL, []byte("x"), nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := c.Delete(context.Background(), srv.URL, nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !sawPut {
		t.Fatal("Put: server never saw a PUT")
	}
	if !sawDelete {
		t.Fatal("Delete: server never saw a DELETE")
	}
}

func TestClient_Retry_On500Then200(t *testing.T) {
	srv, calls := statusSequence(http.StatusInternalServerError, http.StatusOK)
	defer srv.Close()

	opts := fastOpts()
	c := httpclient.NewClient(opts)
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get: final status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := atomic.LoadUint64(calls); got != 2 {
		t.Fatalf("Get: server calls = %d, want 2 (1 retry)", got)
	}
	m := c.Metrics()
	if m.Retried != 1 {
		t.Fatalf("Metrics.Retried = %d, want 1", m.Retried)
	}
	if m.Total != 1 {
		t.Fatalf("Metrics.Total = %d, want 1", m.Total)
	}
	if m.Success != 1 {
		t.Fatalf("Metrics.Success = %d, want 1", m.Success)
	}
}

func TestClient_Retry_AllAttemptsFail(t *testing.T) {
	// Always 500 → client retries RetryMax times then returns the last 500.
	srv, calls := countingServer(http.StatusInternalServerError, "nope")
	defer srv.Close()

	opts := fastOpts()
	c := httpclient.NewClient(opts)
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: unexpected err %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Get: status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	wantCalls := uint64(opts.RetryMax + 1)
	if got := atomic.LoadUint64(calls); got != wantCalls {
		t.Fatalf("Get: server calls = %d, want %d", got, wantCalls)
	}
	m := c.Metrics()
	if m.Retried != uint64(opts.RetryMax) {
		t.Fatalf("Metrics.Retried = %d, want %d", m.Retried, opts.RetryMax)
	}
	if m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

func TestClient_NoRetry_On400(t *testing.T) {
	srv, calls := countingServer(http.StatusBadRequest, "bad")
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: unexpected err %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Get: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if got := atomic.LoadUint64(calls); got != 1 {
		t.Fatalf("Get: server calls = %d, want 1 (no retry on 4xx)", got)
	}
	if m := c.Metrics(); m.Retried != 0 {
		t.Fatalf("Metrics.Retried = %d, want 0", m.Retried)
	}
}

func TestClient_NoRetry_On200(t *testing.T) {
	srv, calls := countingServer(http.StatusOK, "ok")
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	if _, err := c.Get(context.Background(), srv.URL, nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := atomic.LoadUint64(calls); got != 1 {
		t.Fatalf("server calls = %d, want 1", got)
	}
}

func TestClient_RequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than RequestTimeout so the per-request ctx deadline fires
		// while the handler is still sleeping.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	opts := fastOpts()
	opts.RequestTimeout = 30 * time.Millisecond
	opts.RetryMax = 0 // a single attempt is enough to observe the timeout
	c := httpclient.NewClient(opts)

	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Get: expected timeout error, got nil")
	}
	// Should have failed well under the server's 200ms sleep.
	if elapsed > 150*time.Millisecond {
		t.Fatalf("Get: elapsed = %v, expected to bail out early on timeout", elapsed)
	}
	m := c.Metrics()
	if m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

func TestClient_ConnectTimeout(t *testing.T) {
	// Connecting to a closed port on the loopback triggers a connection error
	// quickly. 127.0.0.1:1 is reserved and refuses connections.
	opts := fastOpts()
	opts.ConnectTimeout = 20 * time.Millisecond
	opts.RetryMax = 0 // no retry, we just want the connect failure
	c := httpclient.NewClient(opts)

	_, err := c.Get(context.Background(), "http://127.0.0.1:1/", nil)
	if err == nil {
		t.Fatal("Get: expected connect error, got nil")
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

// fakeBreaker is a test double for httpclient.CircuitBreaker. It delegates to
// fn (so the request actually happens) and counts Execute calls + failures.
type fakeBreaker struct {
	calls    uint64
	failures uint64
}

func (b *fakeBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	atomic.AddUint64(&b.calls, 1)
	err := fn(ctx)
	if err != nil {
		atomic.AddUint64(&b.failures, 1)
	}
	return err
}

func TestClient_BreakerIntegration_DelegatesAndCounts(t *testing.T) {
	srv, calls := countingServer(http.StatusOK, "ok")
	defer srv.Close()

	br := &fakeBreaker{}
	opts := fastOpts()
	opts.Breaker = br
	c := httpclient.NewClient(opts)

	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get: status = %d", resp.StatusCode)
	}
	if got := atomic.LoadUint64(&br.calls); got != 1 {
		t.Fatalf("breaker.Execute calls = %d, want 1", got)
	}
	if got := atomic.LoadUint64(calls); got != 1 {
		t.Fatalf("server calls = %d, want 1", got)
	}
	if atomic.LoadUint64(&br.failures) != 0 {
		t.Fatalf("breaker.failures = %d, want 0", atomic.LoadUint64(&br.failures))
	}
}

func TestClient_BreakerIntegration_OpenShortCircuits(t *testing.T) {
	// A breaker that always returns ErrCircuitOpen without calling fn: the
	// HTTP server should never be hit.
	srv, calls := countingServer(http.StatusOK, "ok")
	defer srv.Close()

	opts := fastOpts()
	opts.Breaker = &explicitBreaker{err: errors.New("circuit open")}
	c := httpclient.NewClient(opts)

	_, err := c.Get(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("Get: expected circuit-open error")
	}
	if got := atomic.LoadUint64(calls); got != 0 {
		t.Fatalf("server calls = %d, want 0 (breaker should short-circuit)", got)
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}

// explicitBreaker always returns the configured error without calling fn.
type explicitBreaker struct{ err error }

func (b *explicitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return b.err
}

func TestClient_Metrics_Accumulate(t *testing.T) {
	srv, _ := countingServer(http.StatusOK, "ok")
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	for i := range 5 {
		if _, err := c.Get(context.Background(), srv.URL, nil); err != nil {
			t.Fatalf("Get[%d]: %v", i, err)
		}
	}
	m := c.Metrics()
	if m.Total != 5 {
		t.Fatalf("Metrics.Total = %d, want 5", m.Total)
	}
	if m.Success != 5 {
		t.Fatalf("Metrics.Success = %d, want 5", m.Success)
	}
	if m.Failed != 0 {
		t.Fatalf("Metrics.Failed = %d, want 0", m.Failed)
	}
}

func TestClient_FollowRedirect_Default(t *testing.T) {
	target, _ := countingServer(http.StatusOK, "landed")
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	// Zero-value options should default to following redirects.
	c := httpclient.NewClient(httpclient.ClientOptions{})
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Get: status = %d, want %d (should follow redirect)", resp.StatusCode, http.StatusOK)
	}
	if string(resp.Body) != "landed" {
		t.Fatalf("Get: body = %q, want %q", resp.Body, "landed")
	}
}

func TestClient_NoFollowRedirect(t *testing.T) {
	target, _ := countingServer(http.StatusOK, "landed")
	defer target.Close()

	sawRedirect := uint64(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint64(&sawRedirect, 1)
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	opts := httpclient.ClientOptions{}.WithNoRedirect()
	c := httpclient.NewClient(opts)
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("Get: status = %d, want %d (redirect not followed)", resp.StatusCode, http.StatusFound)
	}
	if got := atomic.LoadUint64(&sawRedirect); got != 1 {
		t.Fatalf("redirect server calls = %d, want 1", got)
	}
}

func TestClient_Concurrent_RaceSafe(t *testing.T) {
	srv, _ := countingServer(http.StatusOK, "ok")
	defer srv.Close()

	opts := fastOpts()
	opts.RequestTimeout = 5 * time.Second // generous for 50 concurrent under -race
	c := httpclient.NewClient(opts)
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for range n {
		go func() {
			defer wg.Done()
			resp, err := c.Get(context.Background(), srv.URL, nil)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent Get failed: %v", err)
	}
	if m := c.Metrics(); m.Total != n {
		t.Fatalf("Metrics.Total = %d, want %d", m.Total, n)
	}
}

func TestClient_CustomHeaders(t *testing.T) {
	var sawUA, sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUA = r.Header.Get("X-Trace-Id")
		sawAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	_, err := c.Get(context.Background(), srv.URL, map[string]string{
		"X-Trace-Id":    "abc-123",
		"Authorization": "Bearer xyz",
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sawUA != "abc-123" {
		t.Fatalf("X-Trace-Id = %q, want %q", sawUA, "abc-123")
	}
	if sawAuth != "Bearer xyz" {
		t.Fatalf("Authorization = %q, want %q", sawAuth, "Bearer xyz")
	}
}

func TestClient_Body_FullyRead(t *testing.T) {
	payload := strings.Repeat("ABCDEFGHIJ", 1000) // 10k
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(resp.Body) != len(payload) {
		t.Fatalf("body length = %d, want %d", len(resp.Body), len(payload))
	}
	if string(resp.Body) != payload {
		t.Fatal("body content mismatch")
	}
}

func TestClient_EmptyBody_IsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := httpclient.NewClient(fastOpts())
	resp, err := c.Get(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Body != nil {
		t.Fatalf("body = %v, want nil for 204", resp.Body)
	}
}

func TestClient_ContextCanceled_NotRetried(t *testing.T) {
	// Server that blocks until the request ctx is cancelled.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	opts := fastOpts()
	opts.RequestTimeout = 0 // rely on caller ctx
	c := httpclient.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.Get(ctx, srv.URL, nil)
	if err == nil {
		t.Fatal("Get: expected ctx-deadline error")
	}
	// Should not retry on ctx cancellation: retried must be 0.
	if m := c.Metrics(); m.Retried != 0 {
		t.Fatalf("Metrics.Retried = %d, want 0 (no retry on ctx cancel)", m.Retried)
	}
}

func TestClient_MalformedURL_ReturnsError(t *testing.T) {
	c := httpclient.NewClient(fastOpts())
	_, err := c.Get(context.Background(), "http://[::1:bad", nil)
	if err == nil {
		t.Fatal("Get: expected error for malformed URL")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Fatalf("err = %v, want a build-request error", err)
	}
	if m := c.Metrics(); m.Failed != 1 {
		t.Fatalf("Metrics.Failed = %d, want 1", m.Failed)
	}
}
