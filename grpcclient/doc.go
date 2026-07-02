// Package grpcclient provides gRPC interceptor middleware (not stubs) that
// adds timeout, retry, circuit-breaker, and metrics to any gRPC client.
//
// # Quick start
//
//	mw := grpcclient.NewMiddleware(grpcclient.ClientOptions{
//	    Target:         "bid-engine:50051",
//	    RequestTimeout: 20 * time.Millisecond, // RTB hard budget
//	    RetryMax:       2,
//	    RetryCodes:     []codes.Code{codes.Unavailable, codes.DeadlineExceeded},
//	    Breaker:        myBreaker, // optional
//	})
//	conn, err := grpc.Dial(opts.Target,
//	    grpc.WithTransportCredentials(insecure.NewCredentials()),
//	    grpc.WithUnaryInterceptor(mw.UnaryClientInterceptor()),
//	    grpc.WithStreamInterceptor(mw.StreamClientInterceptor()),
//	)
//	// Use conn with any generated stub...
//	client := pb.NewBidServiceClient(conn)
//
// # Performance
//
//	BenchmarkUnary               33 us    164 allocs (bufconn, no real network)
//	BenchmarkUnary_Parallel      12 us    149 allocs (RunParallel, amortized)
//	BenchmarkMiddleware_Metrics    2.4 ns     0 allocs
//
// The middleware adds negligible overhead — allocs are dominated by gRPC
// serialization and the protobuf wire format.
//
// # Retry (unary only)
//
// Retries gRPC codes in RetryCodes (default: Unavailable) with exponential
// backoff + jitter. DeadlineExceeded is excluded by default (a server may have
// committed before its deadline — retrying risks a duplicate side effect); opt
// in only for idempotent RPCs. Never retries on context cancellation or
// non-retryable codes (NotFound, PermissionDenied, etc.).
// Streams are NOT retried (semantically unsafe — caller must reconnect).
//
// # Monitoring
//
//	m := mw.Metrics()
//	// m.Total, m.Success, m.Failed, m.Retried
//	// m.Active — in-flight RPCs (real-time atomic)
//	mw.SetOnEvent(func(evt grpcclient.ClientEvent) {
//	    // evt.Name: "request"|"retry"|"success"|"failed"
//	    // evt.Method, evt.Code, evt.Attempt
//	})
package grpcclient
