// Package httpclient wraps net/http with retry, timeout, circuit-breaker
// integration, HTTP/2 support, connection pooling, and metrics.
//
// # Quick start
//
//	c := httpclient.NewClient(httpclient.ClientOptions{
//	    ConnectTimeout: 5 * time.Second,
//	    RequestTimeout: 30 * time.Second,
//	    RetryMax:       3,
//	    EnableHTTP2:    true,
//	    Breaker:        myBreaker, // optional
//	})
//	resp, err := c.Get(ctx, "https://api.example.com/users", nil)
//	if err == nil {
//	    fmt.Println(resp.StatusCode, string(resp.Body))
//	    resp.Release() // return to pool (optional, reduces GC on hot paths)
//	}
//
// # Performance
//
//	BenchmarkClient_Get           85 us   80 allocs (net/http transport dominated)
//	BenchmarkClient_Get_Parallel  35 us   80 allocs (RunParallel, amortized)
//	BenchmarkClient_Post         105 us   96 allocs
//	BenchmarkClient_Metrics        1.8 ns  0 allocs
//	BenchmarkRetryDelay            8 ns    0 allocs
//
// Client-side bookkeeping is negligible (~2 ns). The 80 allocs per request
// are from net/http's HTTP/1.1 transport (MIME header parsing, context
// cancelCtx, persistConn). HTTP/2 multiplexes requests over a single
// connection, reducing per-request latency for high-fanout calls.
//
// Response.Release() returns the Response struct to a sync.Pool — call it
// when done to reduce GC pressure. drainBody uses a pooled bytes.Buffer.
//
// # Retry
//
// Retries 5xx and network errors (timeout, connection refused) with
// exponential backoff + jitter. Never retries 4xx (client errors) or 2xx.
//
// # Monitoring
//
//	m := c.Metrics() // Total, Success, Failed, Retried
//	c.SetOnEvent(func(evt httpclient.ClientEvent) {
//	    // evt.Name: "request"|"retry"|"success"|"failed"
//	    // evt.Method, evt.URL, evt.StatusCode, evt.Attempt
//	})
package httpclient
