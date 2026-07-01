# semaphore

A weighted counting semaphore for concurrency limiting. Pure standard library.

## Why

When you need to cap the number of concurrent operations against a shared
resource — outbound calls to a specific SSP, concurrent DB connections,
goroutine fan-out — a semaphore is the primitive. Acquire blocks until a permit
is available; Release returns it. Weighted acquires let large operations consume
multiple permits.

## API

```go
sem := semaphore.New(100) // max 100 concurrent

err := sem.Acquire(ctx, 1) // blocks until a permit is free
defer sem.Release(1)
// ... do the rate-limited work ...

if sem.TryAcquire(1) { // non-blocking; false if full
    defer sem.Release(1)
    // ...
}
```

| Symbol | Behavior |
|---|---|
| `New(capacity)` | Build (panics if ≤ 0) |
| `Acquire(ctx, n)` | Block until n permits available (ctx-aware) |
| `TryAcquire(n) bool` | Non-blocking acquire |
| `Release(n)` | Return n permits (panics on underflow) |
| `Available() int64` | Free permits |
| `Cap() int` | Capacity |
| `Close()` | Wake all blocked Acquires (ErrClosed) |

## Ad-tech uses

- **Per-SSP concurrency cap** — limit concurrent bid requests to a specific SSP
  (each Acquire(1) before the outbound call, Release(1) after).
- **DB connection guard** — cap concurrent queries per pool.
- **Worker fan-out** — cap goroutines per batch.
- **Weighted** — a heavy operation Acquire(5) while a light one Acquire(1).

## Testing

98% coverage, `-race` clean. Covers acquire/release, TryAcquire (full/empty),
weighted acquire, exceeds-capacity error, blocking-until-released, context
cancellation, Close-wakes-blocked, idempotent Close, release-underflow panic,
concurrency-limit enforcement (20 workers, cap 4, max-concurrent ≤ 4), and
zero-n-defaults-to-one.

```bash
go test -race -cover ./semaphore/...
```
