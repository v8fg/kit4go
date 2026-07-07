package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/httpclient"
	"github.com/v8fg/kit4go/latency"
)

// fakeObserver records every Observe call under a mutex. It stands in for any
// LatencyObserver implementation and lets the test assert the hook fired with a
// sensible duration without depending on the latency package.
type fakeObserver struct {
	mu     sync.Mutex
	count  int
	total  time.Duration
	minDur time.Duration
}

func (f *fakeObserver) Observe(d time.Duration) {
	f.mu.Lock()
	f.count++
	f.total += d
	if f.minDur == 0 || d < f.minDur {
		f.minDur = d
	}
	f.mu.Unlock()
}

// TestClient_LatencyObserver_Fires verifies the observer is invoked exactly
// once per call, with a strictly-positive duration, when ClientOptions.Latency
// is set. It also covers the nil path implicitly: every other httpclient test
// leaves Latency nil and passes, proving the disabled path is inert.
func TestClient_LatencyObserver_Fires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	obs := &fakeObserver{}
	c := httpclient.NewClient(httpclient.ClientOptions{
		Latency:        obs,
		RequestTimeout: 5 * time.Second,
	})
	for range 5 {
		resp, err := c.Get(context.Background(), srv.URL, nil)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		resp.Release()
	}
	if obs.count != 5 {
		t.Fatalf("observe count=%d, want 5 (one per call)", obs.count)
	}
	if obs.minDur <= 0 {
		t.Errorf("min observed duration=%v, want > 0", obs.minDur)
	}
}

// TestClient_LatencyObserver_RealHistogram wires a real latency.Histogram
// through httpclient and confirms the end-to-end flow: the histogram (which
// satisfies LatencyObserver by structural typing) accumulates every call and
// reports a sensible percentile.
func TestClient_LatencyObserver_RealHistogram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := latency.NewHistogram(latency.Options{})
	c := httpclient.NewClient(httpclient.ClientOptions{
		Latency:        h,
		RequestTimeout: 5 * time.Second,
	})
	const n = 10
	for range n {
		resp, err := c.Get(context.Background(), srv.URL, nil)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		resp.Release()
	}
	s := h.Snapshot()
	if s.Count != n {
		t.Fatalf("histogram Count=%d, want %d", s.Count, n)
	}
	if s.P99 <= 0 {
		t.Errorf("P99=%v, want > 0 (latency was recorded)", s.P99)
	}
	if s.Max <= 0 {
		t.Errorf("Max=%v, want > 0", s.Max)
	}
}
