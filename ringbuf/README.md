# ringbuf

Generic circular buffer (ring buffer) with a fixed capacity that **overwrites
the oldest element** when full. Distinct from `ringbuffer/` which blocks on full
(producer/consumer backpressure); `ringbuf` overwrites — use it for last-N
values, not bounded queues.

Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/ringbuf"

c := ringbuf.New[int](3)
c.Push(1)
c.Push(2)
c.Push(3)
c.Push(4) // overwrites 1 (oldest)

c.ToSlice()  // [2, 3, 4]
c.Latest()   // (4, true)
c.At(0)      // (2, true) — oldest valid
```

## API

| Method | Description |
|--------|-------------|
| `New[T](cap)` | Create (panic if cap ≤ 0) |
| `Push(v)` | Add, overwrite oldest if full — O(1) |
| `Pop() (T, error)` | Remove + return oldest — O(1) |
| `At(i) (T, error)` | i-th from oldest (0) to newest |
| `Latest() (T, error)` | Most recently pushed |
| `Len() / Cap()` | Size / capacity |
| `IsFull() / IsEmpty()` | State |
| `Clear()` | Remove all |
| `ToSlice()` | Copy, oldest → newest |

Not safe for concurrent use (documented).
