# slidingwindow

Sliding window with O(1) aggregate queries. Two variants:
- **Window**: count-based (last N values), O(1) Push/Sum/Avg/Count
- **TimeWindow**: time-based (last T duration), auto-evicts expired entries

Pure standard library. Uses: rolling p99 latency, moving average price,
last-N CTR, rolling volatility.

## Quick start

```go
import "github.com/v8fg/kit4go/slidingwindow"

// Count-based
w := slidingwindow.New(100) // last 100 values
w.Push(42.5)
w.Sum()  // O(1)
w.Avg()  // O(1)
w.Min()  // O(1) cached, O(n) on extreme eviction
w.Max()

// Time-based
tw := slidingwindow.NewTimeWindow(5*time.Second, 1000)
tw.Push(42.5, time.Now())
tw.Sum()
tw.Avg()
```
