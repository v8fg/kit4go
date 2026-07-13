# sliceutil

Generic slice helpers missing from Go 1.21's [`slices`](https://pkg.go.dev/slices)
package. Pure standard library.

The stdlib `slices` package covers sort / contains / reverse / clone, but leaves
out the everyday transformations every Go project reimplements: chunking,
flattening, deduplication, partitioning, grouping, windowing, and slice→map
association.

## Quick start

```go
import "github.com/v8fg/kit4go/sliceutil"

// Batch processing.
for i, page := range sliceutil.Chunk(ids, 100) {
	_ = i
	_ = page
}

// Deduplicate preserving order.
uniq := sliceutil.Deduplicate([]int{3, 1, 3, 2, 1, 4}) // [3 1 2 4]

// Bucket by key.
byMod := sliceutil.GroupBy([]int{1, 2, 3, 4, 5, 6}, func(v int) int {
	return v % 3
}) // map[0:[3 6] 1:[1 4] 2:[2 5]]

// Split into passing / failing.
even, odd := sliceutil.Partition([]int{1, 2, 3, 4}, func(v int) bool { return v%2 == 0 })

// Build a lookup map.
m := sliceutil.Associate(users, func(u User) (int, string) { return u.ID, u.Name })
```

## API

| Function | Description |
|----------|-------------|
| `Chunk(s, n)` | Split into sub-slices of at most `n` — O(n) |
| `Flatten(slices)` | Concatenate `[][]T` → `[]T` |
| `Deduplicate(s)` | New slice, first-occurrence order, duplicates removed |
| `Partition(s, pred)` | `(pass, fail)` split |
| `GroupBy(s, keyFn)` | `map[K][]T` keyed by `keyFn` |
| `Window(s, n)` | All contiguous sub-slices of length `n` (rolling) |
| `Fill(s, v)` | In-place fill every element with `v` |
| `Repeat(v, n)` | New `[]T` with `v` repeated `n` times |
| `Reverse(s)` | New reversed copy (does not mutate input) |
| `Associate(s, fn)` | `map[K]V` from a key+value function |
| `Index(s, v)` | First index of `v`, or -1 (delegates to `slices.Index`) |
| `Contains(s, v)` | Membership test (delegates to `slices.Contains`) |

## Notes

- All functions (except `Fill`) return **new** allocations; the input slice is
  never mutated. `Reverse` clones before reversing.
- `Chunk` produces subslices with `cap == len`, so appending to one chunk cannot
  corrupt its neighbor.
- `Deduplicate`, `GroupBy`, and `Associate` require `comparable` element or key
  types (backed by maps).
- Not safe for concurrent mutation of the *same* underlying slice — Go slices
  offer no such guarantee.
