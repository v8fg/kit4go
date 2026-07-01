# fanout

Broadcasts a message to N subscribers (pub/sub fan-out). Each subscriber gets
its own buffered channel; Publish is non-blocking (drops on full). Pure standard
library.

## Why

Decouple a producer from multiple consumers: one Publish point, N independent
subscribers with their own buffer depth and read speed. A slow subscriber drops
messages; a fast one gets them all. No goroutine-per-subscriber overhead — each
subscriber reads its channel at its own pace.

## API

```go
f := fanout.New[*Event](fanout.WithBufferSize[*Event](64))
defer f.Close()

logSub := f.Subscribe()   // logging pipeline
metricsSub := f.Subscribe() // metrics pipeline

f.Publish(event) // non-blocking; delivered to both

ev := <-logSub.Ch   // each reads independently
ev2 := <-metricsSub.Ch
```

| Symbol | Behavior |
|---|---|
| `New(opts...)` | Build (default buffer 16) |
| `WithBufferSize(n)` | Per-subscriber channel cap |
| `Subscribe() *Subscription` | Register (returns sub.Ch) |
| `Publish(msg) int` | Non-blocking broadcast; returns delivered count |
| `PublishBlocking(ctx, msg) (int, bool)` | Blocking broadcast (ctx-aware) |
| `Unsubscribe(sub)` / `sub.Cancel()` | Remove + close channel |
| `Subscribers() int` | Active count |
| `Published() / Dropped()` | Counters |
| `Close()` | Unsubscribe all, close channels. Idempotent |

**Concurrency safety**: Publish holds a read lock during delivery; Close blocks
until in-flight Publish calls finish. No "send on closed channel" race.

## Ad-tech uses

- **Event broadcast**: win/impression/click → logging + analytics + billing +
  attribution, from a single publish point.
- **Config change notification**: notify all services of a flag change.
- **Cache invalidation signal**: broadcast invalidation to multiple local caches.

## Testing

98% coverage, `-race` clean. Covers subscribe+publish, unsubscribe, cancel,
nil-sub guard, drop-on-full, PublishBlocking (success + ctx-cancel),
publish-after-close, close idempotent, published counter, custom buffer, 50
subscribers, and concurrent 5-publisher/10-subscriber stress.

```bash
go test -race -cover ./fanout/...
```
