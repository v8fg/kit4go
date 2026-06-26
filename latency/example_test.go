package latency_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/latency"
)

// ExampleNewHistogram shows the basic observe/snapshot cycle. Count is exact,
// so it has a deterministic Output; the percentile fields are
// bucket-interpolated and intentionally not asserted here.
func ExampleNewHistogram() {
	h := latency.NewHistogram(latency.Options{})
	for i := 0; i < 1000; i++ {
		h.Observe(time.Duration(i) * time.Microsecond)
	}
	fmt.Println("count:", h.Snapshot().Count)
	// Output: count: 1000
}

// ExampleShardHistogram shows the high-throughput path: a sharded histogram for
// single-instance million-QPS write volume.
func ExampleShardHistogram() {
	// 0 selects AutoShardCount = max(2, GOMAXPROCS/2).
	h := latency.NewShardHistogram(0, latency.Options{})
	h.Observe(5 * time.Millisecond)
	fmt.Println(h.Snapshot().Count)
	// Output: 1
}
