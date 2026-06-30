# ringbuffer

A generic, fixed-capacity ring buffer (circular buffer) with blocking and
non-blocking push/pop. Pure standard library.

## API

```go
rb := ringbuffer.New[*Event](1000)
rb.Push(event)        // blocks if full (backpressure)
ok := rb.TryPush(ev)  // false if full
v, _ := rb.Pop()      // blocks if empty
v, ok := rb.TryPop()  // false if empty
all := rb.Drain()     // non-blocking: returns all buffered
rb.Close()            // wakes blocked callers; Pop drains remaining then ErrClosed
```

| Symbol | Behavior |
|---|---|
| `New[T](cap)` | Build (min 1) |
| `Push(item)` / `TryPush(item)` | Blocking / non-blocking write |
| `Pop() (T, error)` / `TryPop() (T, bool)` | Blocking / non-blocking read |
| `Drain() []T` | Remove + return all (non-blocking) |
| `Close()` | Shutdown; wake blocked Push/Pop; remaining items still poppable |
| `Len()` / `Cap()` / `IsFull()` / `IsEmpty()` | Introspection |

`ErrClosed` is returned by Push after Close, and by Pop when closed + empty.

## Ad-tech uses

- **Event windows** — bounded impression/click rings for real-time aggregation.
- **Metric sample buffers** — cap memory for latency/throughput samples.
- **Producer-consumer decoupling** — bounded queue between pipeline stages.

## Testing

92% coverage, `-race` clean. Covers push/pop round-trip, FIFO order, wrap-around,
TryPush full / TryPop empty, Drain, Close (Push-after-close, Pop remaining then
error), blocking Pop waits, blocking Push waits, Close wakes blocked Push, and a
concurrent producer-consumer (4 producers + 1 consumer, 100 items, Close drains).

```bash
go test -race -cover ./ringbuffer/...
```
