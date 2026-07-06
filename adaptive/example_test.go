package adaptive_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/v8fg/kit4go/adaptive"
)

// fakeLoadMonitor is a deterministic LoadMonitor for the example. Real CPU is
// intentionally NOT used here so the example is reproducible.
type fakeLoadMonitor struct {
	frac float64
}

func (m fakeLoadMonitor) CPU() (float64, error) { return m.frac, nil }

// ExampleNew shows an adaptive pool that processes jobs while keeping host CPU
// under a target. A fake monitor is injected so the example is deterministic;
// in production, omit WithLoadMonitor and the pool samples gopsutil directly.
//
// This example has no // Output: comment because worker scheduling and the
// autoscaler tick are timing-dependent, so go test compiles but does not
// execute it.
func ExampleNew() {
	var processed atomic.Int64

	// CPU reported at 0.4 (under the 0.75 target): the pool keeps headroom for
	// latency-critical paths and only grows when there is queued backlog.
	pool, err := adaptive.New[int](
		func(j int) { processed.Add(1) },
		adaptive.WithMinWorkers[int](1),
		adaptive.WithMaxWorkers[int](4),
		adaptive.WithTargetCPU[int](0.75),
		adaptive.WithSampleInterval[int](10*time.Millisecond),
		adaptive.WithLoadMonitor[int](fakeLoadMonitor{frac: 0.4}),
	)
	if err != nil {
		fmt.Println("new pool:", err)
		return
	}

	for i := 0; i < 16; i++ {
		_ = pool.Submit(context.Background(), i)
	}
	pool.Close() // graceful: drains queued jobs and waits for workers

	fmt.Println("processed:", processed.Load())
}
