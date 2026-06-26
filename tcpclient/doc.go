// Package tcpclient is a pure-Go TCP and Unix-socket client built directly on
// top of the net package. It wraps a raw byte stream connection with the
// cross-cutting concerns a real service-to-service call needs, so callers do
// not have to re-implement them at every site that dials out:
//
//   - Connection pooling. A bounded, channel-backed pool keeps up to PoolSize
//     live connections per address and reuses them across calls. Idle
//     connections older than IdleTimeout are discarded on checkout, so stale
//     sockets are never handed to a caller.
//   - Per-operation timeouts. ConnectTimeout bounds the dial; ReadTimeout and
//     WriteTimeout bound each read/write via net.Conn deadlines (not context,
//     so a slow peer cannot hold a pooled connection forever).
//   - Retry with exponential backoff and jitter. Transient network failures
//     (timeouts, connection refused/reset, EOF) are retried up to RetryMax
//     times with a delay bounded by [RetryWaitMin, RetryWaitMax]. Context
//     cancellations are never retried.
//   - Circuit-breaker integration. A [CircuitBreaker] may be attached to a
//     client; when present each call is funneled through it so a failing
//     downstream can trip the breaker and shed load. The integration is via an
//     interface, so this package does NOT import the breaker package — callers
//     pass a *breaker.Breaker[T] wrapped to satisfy [CircuitBreaker], or any
//     other implementation.
//   - Metrics. Atomic counters track total, success, failed and retried call
//     counts, readable via [Client.Metrics] without blocking the hot path.
//
// Both "tcp" (host:port) and "unix" (/path/to/socket) networks are supported;
// set Network on [ClientOptions]. The package depends only on the Go standard
// library; there are zero external dependencies, which keeps it cheap to pull
// into anything.
//
// # Usage
//
// Create a client with [NewClient] (zero-value options are filled with sensible
// defaults) and call any of the helpers:
//
//	cli := tcpclient.NewClient(tcpclient.ClientOptions{
//	    Network:  "tcp",
//	    Address:  "127.0.0.1:9000",
//	    PoolSize: 16,
//	    RetryMax: 2,
//	})
//	resp, err := cli.SendReceive(ctx, []byte("PING\n"))
//	if err != nil {
//	    return err
//	}
//	log.Printf("reply=%q", resp)
//
// All public methods on [Client] are safe for concurrent use by multiple
// goroutines.
package tcpclient
