// Package limiter provides dependency-free rate limiters for Go.
//
// A rate limiter caps how often some action may happen — protecting a downstream
// service from overload, shaping inbound API traffic, or throttling retries. This
// package exposes a single [Limiter] interface and a [NewLimiter] factory that
// selects one of two algorithms via [LimiterOptions].Algorithm:
//
//   - "token_bucket": a classic token bucket with lazy refill. Tokens accrue
//     continuously at Rate per second up to a Burst capacity, so short bursts are
//     absorbed while the long-term rate holds. The hot path ([Allow]) is lock-free
//     (atomic CAS on the float64 token count); [Wait] blocks until a token is
//     available or the context is cancelled.
//
//   - "sliding_window": a sliding-window counter with one bucket per second (the
//     same design used by log4go's RateAlerter). At most Rate requests may land
//     inside the rolling Window (default 1s). It gives a precise, even cap with no
//     burst headroom — useful where a hard ceiling matters more than smoothing.
//
// Both implementations are safe for concurrent use and track [LimiterMetrics]
// (allowed/denied/acquired counters).
//
// # Quick start
//
//	lm := limiter.NewLimiter(limiter.LimiterOptions{
//	    Algorithm: "token_bucket",
//	    Rate:      100, // 100 tokens/sec
//	    Burst:     10,  // absorb a 10-token burst
//	})
//	defer lm.Close()
//
//	if lm.Allow() {
//	    // handle request
//	}
//
// For a blocking acquire, use [Limiter.Wait]:
//
//	if err := lm.Wait(ctx); err != nil {
//	    // ctx cancelled before a token was available
//	    return err
//	}
//
// [NewLimiter] returns nil for an unrecognised algorithm or an invalid Rate
// (Rate <= 0), so callers can rely on a non-nil limiter when the options validate.
package limiter
