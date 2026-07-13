# stack

Generic LIFO stack backed by a slice. Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/stack"

s := stack.New[int]()
s.Push(10)
s.Push(20)

v, _ := s.Pop() // 20 (LIFO)
```

## API

| Method | Description |
|--------|-------------|
| `New[T](vals...)` | Pre-seed (vals[0] = bottom) |
| `Push(v)` | Add to top — O(1) amortized |
| `Pop() (T, bool)` | Remove + return top — O(1) |
| `Peek() (T, bool)` | Top without removing |
| `Len() / IsEmpty()` | Size queries |
| `Clear()` | Remove all |
| `ToSlice()` | Copy, bottom → top |

Zero-value is usable. Not safe for concurrent use (documented).
