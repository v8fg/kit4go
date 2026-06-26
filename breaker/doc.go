// Package breaker implements a generic, lock-light circuit breaker.
//
// A circuit breaker wraps an operation and short-circuits calls to it once the
// operation starts failing at a sustained rate, giving the downstream a chance
// to recover instead of being hammered by retries. It is the same idea as
// Michael Nygard's "Circuit Breaker" pattern (Release It!, 2007) and the
// gobreaker/sony/gobreaker design, re-expressed with Go generics so the
// wrapped operation can return a typed value with zero reflection.
//
// # States
//
// A breaker cycles through three states:
//
//   - Closed: traffic flows. Every call is recorded in a sliding window. When
//     the in-window failure rate reaches FailRate over at least MinRequests
//     calls, the breaker trips to Open.
//   - Open: traffic is blocked. Execute returns ErrCircuitOpen immediately
//     without invoking fn. After OpenDuration elapses the breaker moves to
//     HalfOpen on the next call.
//   - HalfOpen: a bounded number of probe calls (MaxRequests) are allowed. If
//     all probes succeed the breaker returns to Closed; any failure sends it
//     straight back to Open.
//
// # Counting
//
// Failures are tracked with a per-second sliding window (the same ring design
// used by log4go.RateAlerter): one int bucket per second of Interval, a base
// second cursor, and a running sum so recording a call is O(1) amortized and
// allocates nothing on the hot path.
//
// # Usage
//
// Create a breaker with [NewBreaker], then wrap each risky call with
// [Breaker.Execute]:
//
//	b := breaker.NewBreaker[string](breaker.BreakerOptions{
//	    Name:         "billing",
//	    Interval:     60 * time.Second,
//	    OpenDuration: 30 * time.Second,
//	    FailRate:     0.5,
//	    MinRequests:  10,
//	    MaxRequests:  5,
//	})
//
//	resp, err := b.Execute(ctx, func(ctx context.Context) (string, error) {
//	    return callBilling(ctx)
//	})
//	if errors.Is(err, breaker.ErrCircuitOpen) {
//	    // fall back, shed load, return a cached value, etc.
//	}
//
// All public methods are safe for concurrent use. State and metrics use atomics;
// the mutex is taken only for the brief state-transition critical sections and
// while recording into the sliding window.
package breaker
