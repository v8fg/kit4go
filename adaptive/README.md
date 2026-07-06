# adaptive

A CPU-aware worker pool whose primary goal is to NOT saturate the host CPU.
Unlike a fixed-size pool that maximizes throughput, `adaptive` scales the worker
count DOWN when host CPU usage exceeds a target (default 0.75) so
latency-critical paths keep their headroom. When there is spare CPU and a queued
backlog, it scales back UP, bounded by `MinWorkers` and `MaxWorkers`.

The do-no-harm contract: this pool treats CPU headroom as a shared resource and
yields it when contested. It is intentionally NOT a throughput-maximizing pool.
Use `workerpool` for fixed-concurrency throughput; use `adaptive` when the
workload is opportunistic and must back off the host under load.

## Autoscale model

Every `SampleInterval` the autoscaler samples host CPU and adjusts the worker
count toward the target ceiling:

- CPU > `TargetCPU`: shrink by one (down to `MinWorkers`) to free headroom.
  This is the do-no-harm behavior: yield CPU when contested.
- CPU < `TargetCPU` and queued backlog: grow by one (up to `MaxWorkers`) to
  drain it. No backlog means no grow, so workers are not added to idle on an
  empty queue.
- Otherwise: hold steady.

Shrink is non-preemptive: a signaled worker finishes its current job, then
exits. Remaining queued jobs stay for the surviving workers. No task is ever
dropped by the autoscaler.

## API

| Symbol | Behavior |
|---|---|
| `New[Job](work, opts...) (*Pool[Job], error)` | Build, start `MinWorkers`, launch autoscaler; errors on bad config |
| `WithMinWorkers(n)` / `WithMaxWorkers(n)` | Worker bounds (default 1 / `NumCPU()*2`) |
| `WithTargetCPU(f)` | CPU fraction ceiling (default 0.75); must be in (0,1) |
| `WithSampleInterval(d)` | Autoscaler tick / CPU sample interval (default 1s) |
| `WithQueueSize(n)` | Job queue cap (default `MaxWorkers`); full queue is backpressure |
| `WithLoadMonitor(m)` | Inject a `LoadMonitor` (tests pass a fake; default samples gopsutil) |
| `(*Pool).Submit(ctx, j) error` | Enqueue; `ErrClosed` after close, `ErrFull` on full queue / ctx done |
| `(*Pool).TrySubmit(j) (bool, error)` | Non-blocking enqueue |
| `(*Pool).Workers() int` | Current live worker count (atomic; lags the last decision by up to `SampleInterval`) |
| `(*Pool).Close() error` | Stop autoscaler, reject new submits, drain queued jobs with surviving workers, wait. Idempotent |

`Close` waits for in-flight jobs to finish, so a `Work` func that blocks
indefinitely (stuck network call, held lock, infinite loop) will hang `Close`.
`Work` MUST be non-blocking or honor a context/deadline internally: Go cannot
preempt it. `Close`'s wait bound is `(in-flight jobs) * max(Work duration)`.

Sentinel errors: `ErrClosed` (Submit after Close), `ErrFull` (queue full).
Both are testable with `errors.Is`.

## Example

```go
var processed atomic.Int64

// A fake monitor makes the autoscaler deterministic; omit WithLoadMonitor in
// production and the pool samples gopsutil directly.
pool, err := adaptive.New[int](
    func(j int) { processed.Add(1) },
    adaptive.WithMinWorkers[int](1),
    adaptive.WithMaxWorkers[int](4),
    adaptive.WithTargetCPU[int](0.75),
    adaptive.WithSampleInterval[int](10*time.Millisecond),
    adaptive.WithLoadMonitor[int](fakeLoadMonitor{frac: 0.4}),
)
if err != nil {
    log.Fatal(err)
}

for i := 0; i < 16; i++ {
    _ = pool.Submit(context.Background(), i)
}
pool.Close() // graceful: drains queued jobs and waits for workers

fmt.Println("processed:", processed.Load())
```

## Testing

No live server is required. The autoscaler's CPU decisions are driven by an
injected `fakeMonitor` so they are deterministic; the package-var `cpuPercent`
sampler is swapped to cover the gopsutil error and empty-slice branches. A
single smoke test reads real kernel CPU counters via the default monitor (a
~5ms sample, no workload) and runs under `-short`.

```bash
go test -race -cover ./adaptive/...
```
