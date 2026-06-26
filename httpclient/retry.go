package httpclient

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"time"
)

// retryableStatusCodes is the set of HTTP response status codes that trigger a
// retry: the 5xx server errors. 4xx client errors are the caller's fault and
// must not be retried; 2xx and 3xx are successes/redirects.
var retryableStatusCodes = map[int]struct{}{
	500: {},
	502: {},
	503: {},
	504: {},
}

// rng is the shared source of jitter for retryDelay. math/rand is good enough
// here — we want spread, not cryptographic unpredictability — and gosec's G404
// rule is disabled for this package in .golangci.yml.
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// shouldRetry reports whether the given response/error pair warrants a retry.
//
// Retryable:
//   - A response with a 5xx status (500, 502, 503, 504).
//   - A network-layer error: a timeout, a connection refusal / reset, an
//     unexpected EOF while reading the body, or a generic "connection closed"
//     error. These are detected via errors.Is against the net package's
//     sentinels and a net.OpError check.
//
// Not retryable:
//   - 4xx client errors (the request is malformed or unauthorised — retrying
//     wastes resources and will fail the same way).
//   - 2xx and 3xx responses (success and redirects).
//   - nil errors with a non-5xx response.
//
// Either argument may be nil: callers pass (resp, nil) when the round-trip
// succeeded but the status is being inspected, and (nil, err) when the
// round-trip itself failed.
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		// Context errors are never retried. context.Canceled means the caller
		// tore down the request; context.DeadlineExceeded means the overall
		// deadline (caller's or the per-request RequestTimeout) is exhausted,
		// and retrying would just blow past it again. The "timeout" retry case
		// in the spec refers to transport-layer net.Error timeouts (dial/read
		// timeouts), handled below — those are per-attempt and worth retrying
		// under a fresh attempt.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		// Transient read errors are retryable.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return true
		}
		// Transport-layer timeouts (dial/read) are retryable on a fresh
		// attempt — distinct from context deadlines above.
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// Guard against the rare case where a net.Error wraps a context
			// deadline (some wrappers do). errors.Is already caught the bare
			// context errors above, so this branch is genuine transport
			// timeouts only.
			if errors.Is(netErr, context.Canceled) || errors.Is(netErr, context.DeadlineExceeded) {
				return false
			}
			return true
		}
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			if errors.Is(opErr, context.Canceled) || errors.Is(opErr, context.DeadlineExceeded) {
				return false
			}
			return true
		}
		// Fallback: treat any remaining non-nil error as retryable. This
		// catches syscall-level connection refused / broken pipe errors that
		// do not wrap net.OpError on every platform.
		return true
	}
	if resp != nil {
		if _, ok := retryableStatusCodes[resp.StatusCode]; ok {
			return true
		}
	}
	return false
}

// retryDelay computes the delay before the (attempt+1)-th retry using
// exponential backoff with full jitter:
//
//	base   = min(maxWait, minWait * 2^attempt)
//	delay  = base * (0.5 + random(0, 0.5))   // jitter band [0.5*base, base)
//
// attempt is 0-indexed: the delay before the first retry (after the initial
// call fails) is computed with attempt=0. The jitter decorrelates retries
// across instances so a thundering herd does not form against a recovering
// downstream.
//
// If minWait <= 0 or maxWait <= 0 the result is 0, so a caller who disables
// backoff by zeroing both waits gets immediate retries.
func retryDelay(attempt int, minWait, maxWait time.Duration) time.Duration {
	if minWait <= 0 || maxWait <= 0 {
		return 0
	}
	// Cap the exponential at maxWait to avoid unbounded growth on a large
	// RetryMax. minWait * 2^attempt, clamped.
	backoff := int64(minWait)
	// Guard against overflow on very large attempt counts: stop doubling once
	// we exceed maxWait.
	for i := 0; i < attempt && backoff < int64(maxWait); i++ {
		next := backoff << 1
		// If shifting overflowed (went negative or wrapped), clamp to maxWait.
		if next <= backoff {
			backoff = int64(maxWait)
			break
		}
		backoff = next
	}
	if backoff > int64(maxWait) {
		backoff = int64(maxWait)
	}
	// Jitter: multiply by a factor in [0.5, 1.0).
	// rng.Float64()*0.5 gives [0.0, 0.5); add 0.5 for [0.5, 1.0).
	factor := 0.5 + rng.Float64()*0.5
	return time.Duration(float64(backoff) * factor)
}
