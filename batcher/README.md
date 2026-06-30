# batcher

A generic batch coalescer: collect items, flush them in batches via a callback,
triggered by size (N items), time (interval), or an explicit Flush/Close. Pure
standard library.

## Why

High-fan-in writes — telemetry, pixels, sensor samples, DB inserts, notifications
— are far cheaper amortized across a batch than per-item. `batcher` accumulates
items and flushes them on a size threshold or a timer, with backpressure (a slow
flusher naturally slows the producer) and safe shutdown.

## API

```go
b := batcher.New[*Event](1000, 2*time.Second, func(batch []*Event) {
    bulkInsert(ctx, batch)   // flush callback
}, batcher.WithBufferSize[*Event](5000))
defer b.Close()

b.Add(event)       // blocks when the buffer is full (backpressure)
b.Flush()          // synchronous: returns after the flush ran
```

| Method | Behavior |
|---|---|
| `New(maxSize, interval, flush, opts...)` | Flush at N items, every interval, or on Flush/Close |
| `Add(item) bool` | Enqueue (blocks on full); `false` after Close |
| `Flush()` | Drain + flush the current buffer synchronously |
| `Close() error` | Stop, drain in-flight, flush remainder, wait for exit (idempotent) |

| Option | Default | Effect |
|---|---|---|
| `WithBufferSize(n)` | `maxSize` | Input channel capacity (decouples producer from flusher) |

`interval <= 0` disables the time trigger (size + manual only).

## Correctness

- **Backpressure**: `Add` blocks when the input buffer is full.
- **Synchronous Flush**: drains the input channel into the current batch then
  flushes, so everything added before the Flush call is captured. (`Add` returns
  as soon as the item is in the buffered channel, not once the collector read it,
  so Flush drains explicitly.)
- **Safe shutdown**: `Close` signals the collector, which drains the in-flight
  buffer and flushes the remainder. The input channel is never closed, so `Add`
  after `Close` returns `false` without panicking.
- **Exactly-once flush**: each Added item reaches the flush callback exactly once
  (verified by a 16-goroutine × 300-item test asserting 100% delivery).

## Ad-tech / IoT / live / push uses

- **Pixel / beacon batching** — coalesce ad pixels into bulk inserts.
- **Sensor / IoT aggregation** — batch samples before upload.
- **Bulk DB inserts** — amortize per-row overhead.
- **Notification batching** — coalesce per-user pushes.

## Testing

97% statement coverage, `-race` clean. Covers size trigger, time trigger,
synchronous manual Flush (with the input-channel drain), Close flushes the
remainder, Add-after-Close, idempotent Close, buffer-size option, panic guards,
and a 16-goroutine concurrent delivery test.

```bash
go test -race -cover ./batcher/...
```
