// Package tcpclient provides a TCP and Unix Socket client with connection
// pooling, timeout, retry, and circuit-breaker integration.
//
// # Quick start
//
//	// TCP
//	c := tcpclient.NewClient(tcpclient.ClientOptions{
//	    Network: "tcp",
//	    Address: "internal-svc:9200",
//	    PoolSize: 50,
//	    ReadTimeout: 5 * time.Second,
//	    RetryMax: 2,
//	})
//	resp, err := c.SendReceive(ctx, []byte("PING\r\n"))
//
//	// Unix Socket (IPC)
//	c := tcpclient.NewClient(tcpclient.ClientOptions{
//	    Network: "unix",
//	    Address: "/var/run/docker.sock",
//	    PoolSize: 10,
//	})
//	resp, err := c.SendReceive(ctx, []byte("GET /info HTTP/1.0\r\n\r\n"))
//
// # Performance
//
//	BenchmarkSend                   8 us     5 allocs  (pooled write, no reply)
//	BenchmarkSendReceive          180 us    40 allocs  (fresh dial per call¹)
//	BenchmarkSendReceive_Parallel  65 us    40 allocs  (1000 goroutines, amortized)
//	BenchmarkPool_GetPut           144 ns     1 alloc   (pool checkout/return)
//	BenchmarkClient_Metrics         3.8 ns    0 allocs
//
// Connection pooling reuses idle connections across calls (PoolSize cap).
// Idle connections past IdleTimeout are evicted on checkout. Clean-EOF
// connections are closed (not returned to pool) to prevent pool poisoning.
//
// ¹ SendReceive reads until EOF (or ReadTimeout). The benchmark's
// echo-then-close server closes after one reply, so every call dials a fresh
// connection and the serial number is dominated by the TCP handshake, not
// client overhead. Against a persistent peer (no half-close) the connection is
// pooled and per-call cost drops toward Send.
//
// # Monitoring
//
//	m := c.Metrics()
//	// m.Total, m.Success, m.Failed, m.Retried
//	// m.ActiveConn — in-flight connections (real-time atomic)
//	// m.PoolSize — idle pool depth
//	c.SetOnEvent(func(evt tcpclient.ClientEvent) {
//	    // evt.Name: "connect"|"send"|"receive"|"retry"|"success"|"failed"
//	})
package tcpclient
