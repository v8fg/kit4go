# priorityqueue

A generic max-heap priority queue backed by `container/heap`. Items with the
highest `Priority` value dequeue first. Pure standard library.

The queue is NOT safe for concurrent use. Callers that share a `Queue` across
goroutines must wrap every method in an external `sync.Mutex` (or use a
dedicated owner goroutine). This mirrors Go's own `container/heap`, which is
also lock-free by design: keeping the heap a plain data structure lets the
caller pick the right synchronization strategy instead of paying for locking
it may not need.

## Why

When the next item to process is the highest-weighted rather than the oldest,
a FIFO channel is the wrong shape. `Update` changes a queued item's priority
in place and re-heapifies in O(log n), which is required for Dijkstra / A* /
scheduler use-cases where priorities change after enqueue.

## API

| Symbol | Behavior |
|---|---|
| `New[T]() *Queue[T]` | Empty queue (zero value is not ready to use) |
| `Item[T]{Value, Priority}` | A single entry; `index` is heap-managed, do not set it |
| `(*Queue).Push(value, priority) *Item[T]` | Enqueue; returns the item for later `Update` |
| `(*Queue).Pop() (T, int, bool)` | Remove and return highest priority; false if empty |
| `(*Queue).Peek() (T, int, bool)` | Read highest without removing; false if empty |
| `(*Queue).Update(item, priority)` | Change a queued item's priority, re-heapify in O(log n) |
| `(*Queue).Len() int` | Current size |

`Update` must be called on an item returned by `Push` that is still in the
queue. Passing an item from another queue or one already popped has undefined
behavior.

## Example

```go
q := priorityqueue.New[string]()
q.Push("low", 1)
q.Push("high", 10)
q.Push("mid", 5)

for q.Len() > 0 {
    v, _, _ := q.Pop()
    fmt.Println(v)
}
// output:
// high
// mid
// low

// Peek and in-place priority change.
q2 := priorityqueue.New[string]()
later := q2.Push("later", 1)
q2.Push("now", 7)
v, prio, _ := q2.Peek() // 42-style read without removing
q2.Update(later, 99)    // promote "later" above "now" without re-enqueuing
top, topPrio, _ := q2.Pop()
```

## Testing

Pure standard library; no external services. Ordering, `Pop`/`Peek` on empty
queues, `Update` re-heapification, and coverage-driven fuzzing of arbitrary
push/update/pop sequences.

```bash
go test -race -cover ./priorityqueue/...
```
