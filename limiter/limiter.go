package limiter

import (
	"context"
	"errors"
)

// ErrRateLimited is returned by [Limiter.Wait] only when the caller has wired it
// up as a sentinel (Wait itself returns ctx.Err() on cancellation; this error is
// exported for callers that want to distinguish "would block" from "cancelled"
// when composing limiters).
var ErrRateLimited = errors.New("limiter: rate limited")

// Limiter is a rate limiter. All methods are safe for concurrent use.
//
// Implementations are deliberately minimal: a non-blocking probe ([Allow]), a
// blocking acquire ([Wait]), a batch acquire ([TryAcquire]), and lifecycle /
// observability hooks ([Close], [Metrics]).
type Limiter interface {
	// Allow attempts to acquire one token without blocking. It returns true if a
	// token was acquired (the caller may proceed), false if the rate limit was
	// hit. Allow never waits and never panics.
	Allow() bool

	// Wait blocks until one token is acquired or until ctx is cancelled. It
	// returns nil on success, or ctx.Err() if the context expires first.
	Wait(ctx context.Context) error

	// TryAcquire attempts to acquire n tokens at once. It returns true only if
	// all n were acquired atomically (no partial acquisition). n <= 0 returns
	// true without consuming tokens.
	TryAcquire(n int) bool

	// Close releases any resources. It is idempotent and safe to call from any
	// goroutine; subsequent calls to Allow/Wait/TryAcquire are no-ops.
	Close()

	// Metrics returns a point-in-time snapshot of the limiter's counters. The
	// snapshot is best-effort and may not reflect in-flight calls.
	Metrics() LimiterMetrics
}

// LimiterMetrics is the observability snapshot returned by [Limiter.Metrics].
// Counters are monotonically non-decreasing for the lifetime of a limiter.
type LimiterMetrics struct {
	// Allowed is the number of single-token Allow/TryAcquire(1)/Wait calls that
	// succeeded.
	Allowed uint64

	// Denied is the number of Allow/TryAcquire calls that failed (Wait does not
	// count as denied — it blocks instead).
	Denied uint64

	// Acquired is the total number of tokens consumed across all successful
	// calls (so TryAcquire(n) adds n). For single-token acquires it equals
	// Allowed.
	Acquired uint64
}

// NewLimiter builds a [Limiter] from opts.
//
// Contract:
//   - Rate <= 0 returns nil.
//   - An empty Algorithm defaults to token bucket (treated as "unset").
//   - A non-empty but unrecognised Algorithm returns nil — check for nil before
//     use. This deliberately rejects typos (e.g. "tokn_bucket") instead of
//     silently falling back to token-bucket semantics.
//
// opts is normalised via [LimiterOptions.withDefaults] before use, so partial
// config (e.g. omitting Burst) is tolerated. Note that only an empty Algorithm
// is defaulted; a misspelled non-empty value reaches the switch unmodified and
// is rejected by the default arm.
func NewLimiter(opts LimiterOptions) Limiter {
	if opts.Rate <= 0 {
		return nil
	}
	opts = opts.withDefaults()
	switch opts.Algorithm {
	case AlgorithmTokenBucket:
		return newTokenBucket(opts.Rate, opts.Burst)
	case AlgorithmSlidingWindow:
		return newSlidingWindow(opts.Rate, opts.Window)
	case AlgorithmFixedWindow:
		return newFixedWindow(int64(opts.Rate), opts.Window)
	case AlgorithmLeakyBucket:
		return newLeakyBucket(opts.Rate, opts.Burst)
	case AlgorithmGCRA:
		return newGCRA(opts.Rate, opts.Burst)
	default:
		return nil
	}
}
