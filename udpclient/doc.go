// Package udpclient provides a UDP datagram client with timeout, retry,
// and circuit-breaker integration. Uses the connected-socket model (filters
// replies to the dialed address).
//
// # Quick start
//
//	c, err := udpclient.NewClient(udpclient.ClientOptions{
//	    Address:      "statsd:8125",
//	    ReadTimeout:  2 * time.Second,
//	    RetryMax:     0, // fire-and-forget
//	})
//	// Fire-and-forget (statsd metrics, syslog)
//	c.Send(ctx, []byte("page_views:1|c"))
//
//	// Request-response (DNS-style query)
//	resp, err := c.SendReceive(ctx, query)
//
// # Performance
//
//	BenchmarkSend                  8.6 us    2 allocs  (datagram send)
//	BenchmarkSendReceive           32 us     6 allocs  (send + read reply)
//	BenchmarkSend_Parallel         8 us     2 allocs  (RunParallel)
//	BenchmarkClient_Metrics         2.2 ns    0 allocs
//
// UDP is connectionless — no pool, no active-connection tracking. The
// client holds a single persistent *net.UDPConn (connected mode), reused
// across all calls.
//
// # Monitoring
//
//	m := c.Metrics() // Total, Success, Failed, Retried
//	c.SetOnEvent(func(evt udpclient.ClientEvent) {
//	    // evt.Name: "send"|"receive"|"retry"|"success"|"failed"
//	})
package udpclient
