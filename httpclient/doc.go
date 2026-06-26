// Package httpclient is a production-grade HTTP client built on top of
// net/http. It wraps the standard library's round-tripper with the cross-cutting
// concerns a real service-to-service call needs, so callers do not have to
// re-implement them on every site that dials out:
//
//   - Retry with exponential backoff and jitter. Transient failures (5xx
//     responses, connection resets, EOF, dial and read timeouts) are retried up
//     to RetryMax times with a delay bounded by [RetryWaitMin, RetryWaitMax].
//     4xx client errors and 2xx/3xx responses are never retried.
//   - Per-request and per-dial timeouts. ConnectTimeout bounds the TCP dial;
//     RequestTimeout bounds the whole request (applied via context.WithTimeout
//     on every call). IdleConnTimeout governs the shared connection pool.
//   - Connection pooling. A single [http.Transport] is shared across requests,
//     with MaxIdleConns / MaxIdleConnsPerHost sized via [ClientOptions], so
//     keep-alive reuse works out of the box.
//   - Circuit-breaker integration. A [CircuitBreaker] may be attached to a
//     client; when present each call is funneled through it so a failing
//     downstream can trip the breaker and shed load. The integration is via an
//     interface, so this package does NOT import the breaker package — callers
//     pass a *breaker.Breaker[T] which satisfies [CircuitBreaker], or any other
//     implementation.
//   - Metrics. Atomic counters track total, success (2xx), failed (non-2xx or
//     error) and retried call counts, readable via [Client.Metrics] without
//     blocking the hot path.
//
// The package depends only on the Go standard library; there are zero external
// dependencies, which keeps it cheap to pull into anything.
//
// # Usage
//
// Create a client with [NewClient] (zero-value options are filled with sensible
// defaults) and call any of the method helpers, or [Client.Do] for the full
// surface:
//
//	cli := httpclient.NewClient(httpclient.ClientOptions{
//	    RequestTimeout: 5 * time.Second,
//	    RetryMax:       2,
//	})
//	resp, err := cli.Get(ctx, "https://example.com/api", nil)
//	if err != nil {
//	    return err
//	}
//	log.Printf("status=%d body=%q", resp.StatusCode, resp.Body)
//
// All public methods on [Client] are safe for concurrent use by multiple
// goroutines.
package httpclient
