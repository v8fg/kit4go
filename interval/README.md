# interval

Generic half-open interval `[Start, End)` with overlap, containment,
intersection, and merge operations. Pure standard library.

For `time.Time` intervals convert to `int64` via `UnixNano` (`time.Time` does not
satisfy `cmp.Ordered`).

## Quick start

```go
import "github.com/v8fg/kit4go/interval"

a := interval.MustNew(0, 10)
b := interval.MustNew(5, 15)

a.Contains(7)           // true
a.Overlaps(b)           // true
u, _ := a.Union(b)      // [0, 15)
in, _ := a.Intersect(b) // [5, 10)

merged := interval.Merge([]interval.Interval[int]{
    interval.MustNew(0, 5),
    interval.MustNew(3, 8),
    interval.MustNew(10, 15),
}) // → [0,8), [10,15)
```

## API

| Method | Description |
|--------|-------------|
| `New(start, end)` | Build (error on inverted) |
| `MustNew(start, end)` | Build (panic on inverted) |
| `Contains(v)` | v ∈ [Start, End) |
| `ContainsInclusive(v)` | v ∈ [Start, End] |
| `Overlaps(other)` | Share any point |
| `IsBefore(other)` / `IsAfter(other)` | Strictly before/after |
| `Union(other)` | Merge if overlapping/touching |
| `Intersect(other)` | Intersection |
| `Merge(intervals)` | Sort + collapse overlapping/touching |
