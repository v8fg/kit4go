# latency: latency histogram

A fixed-bucket latency histogram with a trailing time window, lock-free
sharding, and quantile estimation. Built for hot-path observation (request
latency in ad serving) where `Observe` must stay allocation-free under
contention.

## Features

- Fixed exponential bucket boundaries (configurable).
- Trailing window: old seconds roll off; clock-regression is clamped.
- Sharded counters (shard count auto-scales to GOMAXPROCS) for low contention.
- `Observe` is the hot path; `Snapshot` / quantile queries are cold.
- Defensive against empty windows and all-zero samples.

## Usage

- `NewHistogram(opts Options) *Histogram`.
- `(*Histogram).Observe(d time.Duration)`.
- `(*Histogram).Quantile(q float64) time.Duration` estimated percentile.
- `(*Histogram).Snapshot() Stats` point-in-time counts per bucket.
- `Stats` carries bucket counts and totals for export.

`Options` sets boundaries, window length, and shard count.

## Example

```go
import (
    "time"
    "github.com/v8fg/kit4go/latency"
)

h := latency.NewHistogram(latency.Options{})
h.Observe(time.Since(start))
p99 := h.Quantile(0.99)
```
