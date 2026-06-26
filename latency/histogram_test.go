package latency_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/latency"
)

// --- Construction & options -------------------------------------------------

func TestNewHistogram_Defaults(t *testing.T) {
	h := latency.NewHistogram(latency.Options{})
	if h == nil {
		t.Fatal("NewHistogram(zero opts) = nil")
	}
	// A default histogram accepts observations and reports them.
	h.Observe(time.Millisecond)
	if got := h.Snapshot().Count; got != 1 {
		t.Fatalf("default Count=%d want 1", got)
	}
}

func TestNewHistogram_CustomWindow(t *testing.T) {
	// Sub-second windows are raised to 1s; the histogram still works.
	h := latency.NewHistogram(latency.Options{Window: 100 * time.Millisecond})
	if h == nil {
		t.Fatal("nil")
	}
	h.Observe(time.Millisecond)
}

func TestNewHistogram_CustomBoundaries(t *testing.T) {
	h := latency.NewHistogram(latency.Options{
		Boundaries: []time.Duration{time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond},
	})
	if h == nil {
		t.Fatal("nil")
	}
	h.Observe(5 * time.Millisecond)
	if got := h.Snapshot().Max; got != 5*time.Millisecond {
		t.Fatalf("Max=%v want 5ms", got)
	}
}

func TestNewHistogram_InvalidBoundaries(t *testing.T) {
	for _, bad := range [][]time.Duration{
		{2 * time.Millisecond, time.Millisecond},        // decreasing
		{0, time.Millisecond},                           // <= 0
		{time.Millisecond, time.Millisecond},            // not strictly increasing
		{-time.Millisecond, time.Millisecond},           // negative
	} {
		if latency.NewHistogram(latency.Options{Boundaries: bad}) != nil {
			t.Errorf("NewHistogram(%v) = non-nil, want nil", bad)
		}
	}
}

// --- Empty / edge observations ----------------------------------------------

func TestHistogram_Empty(t *testing.T) {
	h := latency.NewHistogram(latency.Options{})
	s := h.Snapshot()
	if s.Count != 0 || s.P99 != 0 || s.Min != 0 || s.Max != 0 {
		t.Fatalf("empty snapshot = %+v, want all zero", s)
	}
	if q := h.Quantile(0.99); q != 0 {
		t.Fatalf("empty Quantile(0.99) = %v, want 0", q)
	}
}

func TestHistogram_ObserveZeroAndNegative(t *testing.T) {
	h := latency.NewHistogram(latency.Options{})
	h.Observe(0)
	h.Observe(-5 * time.Millisecond)
	s := h.Snapshot()
	if s.Count != 2 {
		t.Fatalf("Count=%d want 2", s.Count)
	}
	if s.Min != 0 {
		t.Errorf("Min=%v want 0 (0 is the genuine minimum)", s.Min)
	}
	if s.Max != 0 {
		t.Errorf("Max=%v want 0", s.Max)
	}
}

func TestHistogram_ObserveHuge(t *testing.T) {
	// A pathological outlier lands in the overflow bucket. Quantile must stay
	// finite (capped at the last boundary), while Max records the exact value.
	h := latency.NewHistogram(latency.Options{})
	h.Observe(time.Hour)
	q := h.Quantile(1.0)
	if q <= 0 {
		t.Fatalf("Quantile(1.0)=%v, want > 0", q)
	}
	if q > 10*time.Second {
		t.Errorf("Quantile(1.0)=%v, want <= last boundary (10s)", q)
	}
	if got := h.Snapshot().Max; got != time.Hour {
		t.Errorf("Max=%v want 1h (exact, not interpolated)", got)
	}
}

// --- Quantile accuracy ------------------------------------------------------

func TestHistogram_Quantile_Tail(t *testing.T) {
	// 99 samples at 1ms, 1 at 50ms. The 99th percentile is the 99th of 100
	// sorted values = 1ms (only the 100th is the slow one). The 99.9th lands
	// on the slow sample's bucket.
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 99; i++ {
		h.Observe(time.Millisecond)
	}
	h.Observe(50 * time.Millisecond)
	s := h.Snapshot()
	if s.Count != 100 {
		t.Fatalf("Count=%d want 100", s.Count)
	}
	if s.P99 > 2*time.Millisecond {
		t.Errorf("P99=%v, want ~1ms (99 of 100 samples are 1ms)", s.P99)
	}
	if s.P999 < 30*time.Millisecond {
		t.Errorf("P999=%v, want >= 30ms (slow sample is in the 30-50ms bucket)", s.P999)
	}
	if s.P999 > 50*time.Millisecond {
		t.Errorf("P999=%v, want <= 50ms (bucket upper bound)", s.P999)
	}
}

func TestHistogram_Quantile_UniformP50(t *testing.T) {
	// Uniform spread 0..~50ms; the median of a uniform distribution is the
	// midpoint ~25ms. Allow bucket-resolution tolerance.
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 5000; i++ {
		h.Observe(time.Duration(i) * 10 * time.Microsecond) // 0 .. 49.99ms
	}
	p50 := h.Quantile(0.50)
	if p50 < 15*time.Millisecond || p50 > 35*time.Millisecond {
		t.Errorf("P50=%v, want within [15ms, 35ms]", p50)
	}
}

func TestHistogram_Quantile_IdenticalSamples(t *testing.T) {
	// 101 identical 5ms samples. They all fall in the (3ms, 5ms] bucket, so the
	// interpolated P50 reports the bucket midpoint (~4ms), NOT exactly 5ms —
	// this documents the within-bucket interpolation bias.
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 101; i++ {
		h.Observe(5 * time.Millisecond)
	}
	p50 := h.Quantile(0.50)
	if p50 < 3*time.Millisecond || p50 > 5*time.Millisecond {
		t.Errorf("P50=%v, want within [3ms, 5ms] (the bucket range)", p50)
	}
}

func TestHistogram_Quantile_Clamp(t *testing.T) {
	h := latency.NewHistogram(latency.Options{})
	h.Observe(time.Millisecond)
	h.Observe(10 * time.Millisecond)
	// q <= 0 -> minimum sample; q >= 1 -> maximum.
	if q := h.Quantile(-1); q != h.Quantile(0) {
		t.Errorf("Quantile(-1)=%v != Quantile(0)=%v", q, h.Quantile(0))
	}
	if q := h.Quantile(2); q != h.Quantile(1) {
		t.Errorf("Quantile(2)=%v != Quantile(1)=%v", q, h.Quantile(1))
	}
}

// --- Snapshot fields --------------------------------------------------------

func TestHistogram_Snapshot_MeanExact(t *testing.T) {
	// Mean is computed from the exact per-bucket sum, so it is exact (unlike
	// the interpolated percentiles).
	h := latency.NewHistogram(latency.Options{})
	h.Observe(0)
	h.Observe(10 * time.Millisecond)
	h.Observe(20 * time.Millisecond)
	s := h.Snapshot()
	if s.Count != 3 {
		t.Fatalf("Count=%d want 3", s.Count)
	}
	if s.Mean != 10*time.Millisecond {
		t.Errorf("Mean=%v want 10ms", s.Mean)
	}
	if s.Min != 0 || s.Max != 20*time.Millisecond {
		t.Errorf("Min=%v Max=%v want 0 / 20ms", s.Min, s.Max)
	}
}

// --- Sliding window ---------------------------------------------------------

func TestHistogram_WindowExpiry(t *testing.T) {
	// A 1s window: samples from a previous second must drop out of the count.
	h := latency.NewHistogram(latency.Options{Window: time.Second})
	for i := 0; i < 100; i++ {
		h.Observe(time.Millisecond)
	}
	if got := h.Snapshot().Count; got != 100 {
		t.Fatalf("pre-expiry Count=%d want 100", got)
	}
	time.Sleep(1100 * time.Millisecond)
	h.Observe(time.Millisecond) // 1 fresh sample in the new window
	if got := h.Snapshot().Count; got != 1 {
		t.Fatalf("post-expiry Count=%d want 1", got)
	}
}

// --- Concurrency ------------------------------------------------------------

func TestHistogram_Concurrent(t *testing.T) {
	h := latency.NewHistogram(latency.Options{})

	const goroutines = 32
	const perG = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// A reader goroutine hammers Snapshot/Quantile concurrently with the
	// writers to exercise the mergeBuf-under-lock path under -race.
	var stop atomic.Bool
	go func() {
		for !stop.Load() {
			_ = h.Snapshot()
			_ = h.Quantile(0.99)
		}
	}()

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				h.Observe(time.Duration(i) * time.Microsecond)
			}
		}()
	}
	wg.Wait()
	stop.Store(true)

	s := h.Snapshot()
	if want := uint64(goroutines * perG); s.Count != want {
		t.Fatalf("Count=%d want %d", s.Count, want)
	}
}

// --- Sharded histogram ------------------------------------------------------

func TestShardHistogram_Distribution(t *testing.T) {
	h := latency.NewShardHistogram(8, latency.Options{})
	if h == nil {
		t.Fatal("nil")
	}
	const n = 10000
	for i := 0; i < n; i++ {
		h.Observe(time.Duration(i%50) * time.Millisecond)
	}
	if got := h.Snapshot().Count; got != n {
		t.Fatalf("Count=%d want %d", got, n)
	}
}

func TestShardHistogram_MatchesSingle(t *testing.T) {
	// The same data fed through a single histogram and a sharded one must
	// produce percentiles within one bucket width of each other.
	opts := latency.Options{}
	single := latency.NewHistogram(opts)
	shard := latency.NewShardHistogram(16, opts)

	const n = 100000
	for i := 0; i < n; i++ {
		d := time.Duration(i%100) * time.Microsecond
		single.Observe(d)
		shard.Observe(d)
	}
	for _, q := range []float64{0.50, 0.90, 0.99} {
		a := single.Quantile(q)
		b := shard.Quantile(q)
		// Allow a generous tolerance: both are bucket-interpolated and the
		// round-robin split can shift a boundary sample by one bucket.
		diff := a - b
		if diff < 0 {
			diff = -diff
		}
		if diff > 5*time.Millisecond {
			t.Errorf("Quantile(%v): single=%v shard=%v diff=%v", q, a, b, diff)
		}
	}
}

func TestAutoShardCount(t *testing.T) {
	n := latency.AutoShardCount()
	if n < 2 {
		t.Fatalf("AutoShardCount=%d, want >= 2", n)
	}
	// NewShardHistogram(0) selects AutoShardCount and must be non-nil.
	if latency.NewShardHistogram(0, latency.Options{}) == nil {
		t.Fatal("NewShardHistogram(0) = nil")
	}
}

func TestNewShardHistogram_InvalidOptions(t *testing.T) {
	if latency.NewShardHistogram(4, latency.Options{
		Boundaries: []time.Duration{2 * time.Millisecond, time.Millisecond},
	}) != nil {
		t.Fatal("NewShardHistogram with invalid boundaries should return nil")
	}
}
