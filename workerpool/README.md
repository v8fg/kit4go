# workerpool

A bounded worker pool: N goroutines process jobs from a queue, with backpressure
(Submit blocks when full), graceful shutdown (drain), and optional result
collection. Pure standard library.

## Why

When you need "N workers, M-deep queue" — bounded concurrent processing with
backpressure, graceful drain, and optional result collection — without
hand-rolling goroutine lifecycle, WaitGroups, and channel closing. Submit blocks
when the queue is full (natural backpressure); Close drains and waits.

## API

```go
pool := workerpool.New[int](4, workerpool.WithQueueSize[int](100))

// Fire-and-forget:
pool.Submit(ctx, func(ctx context.Context) (int, error) {
    return processBid(ctx)
})

// With result collection:
pool := workerpool.New[string](4, workerpool.WithResults[string](100))
pool.Submit(ctx, func(ctx context.Context) (string, error) { return callSSP(ctx) })
for r := range pool.Results() { /* r.Value, r.Err */ }

pool.Close() // drains queue, waits for workers
```

| Symbol | Behavior |
|---|---|
| `New[T](workers, opts...)` | Build (panics if ≤ 0) |
| `WithQueueSize(n)` | Queue cap (default = workers) |
| `WithResults(buffer)` | Enable result collection channel |
| `Submit(ctx, fn)` | Enqueue (blocks if full; ctx-aware) |
| `TrySubmit(ctx, fn) bool` | Non-blocking enqueue |
| `Results() <-chan Result[T]` | Result stream (nil if not enabled) |
| `Close()` | Stop accepting, drain, wait. Idempotent |
| `Workers() int` | Worker count |

## Ad-tech uses

- **Bounded bid evaluation** — cap concurrent bid-decision goroutines per SSP.
- **Bulk creative loading** — parallel fetch with bounded fan-out.
- **Batch event ingestion** — workers drain a queue of events to process/store.
- **Parallel HTTP fan-out** — send to K SSPs with at most N concurrent.

## Testing

98% coverage, `-race` clean. Covers fire-and-forget, results channel, TrySubmit
(full + accepted), Submit-after-Close (ErrClosed), Close drains queue (all 5
processed), Close idempotent, concurrency limit enforcement (50 jobs, 3 workers,
max-concurrent ≤ 3), Submit ctx-cancel, and Workers() accessor.

```bash
go test -race -cover ./workerpool/...
```
