// Package latency provides a fixed-bucket latency histogram with a trailing
// sliding window, for collecting request/RPC latency percentiles (p50/p99/
// p999). Zero external dependencies.
//
// It is built for low-latency, high-throughput workloads (RTB/DSP/ADX
// bidding) where tail latency — not the mean — is the signal that matters: a
// 50ms bidding budget is blown by the slowest 1%, not by the average.
//
// # Quick start
//
//	h := latency.NewHistogram(latency.Options{}) // 60s window, RTB-tuned buckets
//	// ... per request ...
//	h.Observe(d)
//	s := h.Snapshot()
//	// s.P50, s.P99, s.P999, s.Min, s.Max, s.Mean, s.Count
//
// Wire it into an httpclient/tcpclient/grpcclient via the LatencyObserver
// option (one line, zero overhead when nil):
//
//	c := httpclient.NewClient(httpclient.ClientOptions{
//	    Latency: latency.NewHistogram(latency.Options{}),
//	})
//	// ... later, from a Prometheus scrape ...
//	fmt.Println(c.Latency.(*latency.Histogram).Snapshot().P99)
//
// # Buckets
//
// [DefaultBoundaries] span 100µs to 10s with extra resolution across the RTB
// 1–50ms range (9 buckets). Percentiles are linearly interpolated within a
// bucket — a conservative estimate that slightly over-states the tail (the
// same assumption HdrHistogram uses). Pass custom [Options.Boundaries] for a
// different distribution; invalid boundaries (non-monotonic, <= 0) cause
// [NewHistogram] to return nil.
//
// # Performance
//
//	BenchmarkHistogram_Observe                ~62 ns    0 allocs
//	BenchmarkHistogram_Observe_Parallel       ~185 ns   0 allocs (RunParallel)
//	BenchmarkHistogram_Quantile               ~920 ns   0 allocs
//	BenchmarkHistogram_Snapshot               ~935 ns   0 allocs
//	BenchmarkShardHistogram_Observe_Parallel  ~84 ns    0 allocs (32 shards)
//
// Observe is a binary bucket lookup plus a short mutex critical section
// (advance + increment), allocation-free — the same shape as limiter's
// sliding window. For single-instance million-QPS write volume use
// [NewShardHistogram]: round-robin sharding divides lock contention by N.
// Reads (Quantile/Snapshot) fold the window under the lock; on a sharded
// histogram they merge all shards, so they are slower — but reads are rare
// (a Prometheus scrape every ~15s).
//
// # Monitoring
//
// Every field of [Stats] is window-scoped (default 60s): a Snapshot reflects
// only samples from the trailing window, so it tracks current latency rather
// than lifetime. Use [Options.Window] to widen or narrow that view.
package latency
