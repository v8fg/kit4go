# omap

Insertion-ordered map — a [map] that remembers the order keys were first added
(LinkedHashMap semantics). Pure standard library.

Go's built-in map iterates in random order. That breaks reproducible output —
JSON marshaling, config diffs, golden-file tests, audit logs all want a stable
order. omap preserves insertion order: `Set`, `Keys`, `Values`, and `Each` visit
keys in first-add order.

## Quick start

```go
import "github.com/v8fg/kit4go/omap"

m := omap.New[string, int]()
m.Set("first", 1)
m.Set("second", 2)
m.Set("third", 3)

m.Keys()   // [first second third] — deterministic, every time
m.Values() // [1 2 3]

m.Set("first", 99) // update keeps original position
m.Keys()           // still [first second third]

m.Delete("second")
m.Keys()           // [first third] — order preserved
```

## API

| Method | Description |
|--------|-------------|
| `New[K,V]()` | Empty ordered map |
| `Set(k, v)` | Insert or update (update keeps position) — O(1) |
| `Get(k)` | `(V, ok)` — O(1) |
| `Has(k)` | Present? — O(1) |
| `Delete(k)` | Remove; returns whether present — O(n) (shifts to preserve order) |
| `Len()` | Entry count |
| `Keys()` | All keys, insertion order (copy) |
| `Values()` | All values, insertion order (copy) |
| `Each(fn)` | Iterate in order; stop when `fn` returns false |
| `Clear()` | Remove all |

## Notes

- **Insertion order, not sorted order.** Keys appear in first-add order. For
  sorted iteration, copy `Keys()` and `slices.Sort` it.
- **Update preserves position**: `Set` on an existing key changes its value but
  not its place in the order.
- **Delete is O(n)**: it shifts the remaining keys left to preserve their order.
  For workloads with heavy deletion, consider rebuilding periodically. Get/Set/
  Has are O(1).
- `Keys` and `Values` return copies; mutating them does not affect the map.
- Not safe for concurrent use.
