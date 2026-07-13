# queue

Generic FIFO queue backed by a slice with a sliding head index (amortized O(1)
enqueue/dequeue, periodic compaction). Pure standard library.

Distinct from `ringbuffer/` (bounded, channel-based for producer/consumer
handoff); `queue` is unbounded and synchronous.

## Quick start

```go
import "github.com/v8fg/kit4go/queue"

q := queue.New[int]()
q.Enqueue(10)
q.Enqueue(20)

v, _ := q.Dequeue() // 10 (FIFO)
```

## API

| Method | Description |
|--------|-------------|
| `New[T](vals...)` | Pre-seed (vals[0] = front) |
| `Enqueue(v)` | Add to back — O(1) amortized |
| `Dequeue() (T, bool)` | Remove + return front — O(1) amortized |
| `Front() / Back() (T, bool)` | Peek without removing |
| `Len() / IsEmpty()` | Size queries |
| `Clear()` | Remove all |
| `ToSlice()` | Copy, front → back |

Zero-value is usable. Not safe for concurrent use (documented).
