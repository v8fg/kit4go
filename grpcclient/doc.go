// Package grpcclient is a production-grade gRPC client wrapper built on top of
// google.golang.org/grpc. It does NOT generate or own any stubs — instead it
// provides the cross-cutting middleware a real service-to-service call needs as
// [grpc.UnaryClientInterceptor] / [grpc.StreamClientInterceptor], so callers wire
// it onto any *grpc.ClientConn they construct themselves:
//
//   - Retry with exponential backoff and jitter. Unary RPCs whose gRPC status
//     code is in RetryCodes (default Unavailable and DeadlineExceeded) are
//     retried up to RetryMax times with a delay bounded by
//     [RetryWaitMin, RetryWaitMax]. Streams are never retried by this package:
//     retrying a stream is semantically unsafe, so the caller must reconnect.
//   - Per-RPC timeout. RequestTimeout is applied via context.WithTimeout on
//     every unary and stream call. A caller-supplied deadline that is tighter
//     than RequestTimeout always wins.
//   - Circuit-breaker integration. A [CircuitBreaker] may be attached; when
//     present each call is funneled through it so a failing downstream can trip
//     the breaker and shed load. The integration is via an interface, so this
//     package does NOT import the breaker package — callers pass a
//     *breaker.Breaker[T] which satisfies [CircuitBreaker], or any other
//     implementation.
//   - Metrics. Atomic counters track total, success, failed and retried call
//     counts, readable via [Middleware.Metrics] without blocking the hot path.
//   - Event hooks. An optional callback ([Middleware.SetOnEvent]) fires for
//     every notable outcome (request, retry, success, failed), mirroring the
//     hook pattern used by httpclient, breaker and log4go.
//
// The package depends only on google.golang.org/grpc.
//
// # Usage
//
// Build a [Middleware] from options (zero-value fields are filled with sensible
// defaults) and pass its interceptors to grpc.Dial:
//
//	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
//	    Target:         "localhost:50051",
//	    RequestTimeout: 5 * time.Second,
//	    RetryMax:       2,
//	})
//	conn, err := grpc.Dial(opts.Target,
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
//	    grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
//	)
//	// Use conn with generated stubs...
//
// [DialConn] is a convenience that performs the same wiring with insecure
// credentials and returns the *grpc.ClientConn ready for stub construction.
//
// All public methods on [Middleware] are safe for concurrent use by multiple
// goroutines.
package grpcclient
