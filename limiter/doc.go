// Package limiter provides rate limiting via token-bucket and sliding-window
// algorithms. Zero external dependencies.
//
// # Quick start
//
//	// Token bucket: 1000 QPS, burst 100
//	l := limiter.NewLimiter(limiter.LimiterOptions{
//	    Algorithm: "token_bucket",
//	    Rate:      1000,
//	    Burst:     100,
//	})
//	if l.Allow() {
//	    // handle request
//	}
//	// Or block until a token is available:
//	err := l.Wait(ctx) // returns ctx.Err() on timeout
//
//	// Sliding window: precise 500 QPS in 1s window
//	l2 := limiter.NewLimiter(limiter.LimiterOptions{
//	    Algorithm: "sliding_window",
//	    Rate:      500,
//	    Window:    time.Second,
//	})
//
// # Performance
//
//	BenchmarkTokenBucket_Allow          69 ns    0 allocs
//	BenchmarkTokenBucket_Allow_Parallel 206 ns   0 allocs (RunParallel)
//	BenchmarkSlidingWindow_Allow        66 ns    0 allocs
//	BenchmarkSlidingWindow_Allow_Parallel 210 ns  0 allocs
//	BenchmarkTokenBucket_Wait           63 ns    0 allocs
//
// Allow() is allocation-free on both algorithms. Token bucket uses atomic
// CAS on float64 bits (no mutex on the hot path).
//
// # Monitoring
//
//	m := l.Metrics() // Allowed, Denied, Acquired
package limiter
