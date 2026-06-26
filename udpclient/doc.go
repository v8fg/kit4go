// Package udpclient is a production-grade UDP client built on top of net. UDP is
// connectionless at the protocol level, but Go's net.Dial("udp", addr) returns a
// "connected" UDP socket ([net.UDPConn]) that filters inbound datagrams to the
// dialed peer and exposes a far simpler Read/Write API than raw
// [net.ListenUDP]. This package wraps that connected socket with the
// cross-cutting concerns a real datagram exchange needs, so callers do not have
// to re-implement them on every site that dials out:
//
//   - Timeout. WriteDeadline bounds every send; ReadDeadline bounds every
//     [Client.SendReceive] read. Both default to sensible values and are
//     overridable via [ClientOptions].
//   - Retry with exponential backoff and jitter. Transient failures (write
//     errors, read timeouts) are retried up to RetryMax times with a delay
//     bounded by [RetryWaitMin, RetryWaitMax]. A read timeout — typically the
//     sign of a dropped or unanswered datagram — is treated as retryable, since
//     a fresh attempt under a fresh deadline may succeed.
//   - Circuit-breaker integration. A [CircuitBreaker] may be attached to a
//     client; when present each call is funneled through it so a failing
//     downstream can trip the breaker and shed load. The integration is via an
//     interface, so this package does NOT import the breaker package — callers
//     pass an implementation (e.g. an adapter over *breaker.Breaker[T]) or any
//     other implementation.
//   - Metrics. Atomic counters track total, success, failed and retried call
//     counts, readable via [Client.Metrics] without blocking the hot path.
//   - Optional local bind. LocalAddress binds the source port of the outbound
//     socket before connecting, useful when a NAT/firewall pins a source port.
//
// The package depends only on the Go standard library; there are zero external
// dependencies, which keeps it cheap to pull into anything.
//
// # Semantics
//
// Send writes a single datagram and does not wait for a reply — fire-and-forget
// telemetry/statsd-style traffic. SendReceive writes one datagram and then
// reads exactly one reply datagram (the connected socket already filters out
// replies from any peer other than the dialed one). BufferSize caps the reply
// read buffer; datagrams larger than that are silently truncated by the kernel,
// so size it to your protocol's MTU.
//
// # Usage
//
// Create a client with [NewClient] (zero-value options are filled with sensible
// defaults) and call [Client.Send] or [Client.SendReceive]:
//
//	cli, err := udpclient.NewClient(udpclient.ClientOptions{
//	    Address:      "127.0.0.1:8125",
//	    ReadTimeout:  2 * time.Second,
//	    WriteTimeout: 500 * time.Millisecond,
//	    RetryMax:     2,
//	})
//	if err != nil {
//	    return err
//	}
//	defer cli.Close()
//	if err := cli.Send(ctx, []byte("statsd.metric:1|c")); err != nil {
//	    return err
//	}
//
// All public methods on [Client] are safe for concurrent use by multiple
// goroutines. The underlying [net.UDPConn] is shared across goroutines, so the
// socket and its deadlines are NOT independent per call: under concurrency,
// [Client.SendReceive] in particular contends on the read path. For high-throughput
// request/reply fan-out prefer one client per goroutine (or a small pool).
package udpclient
