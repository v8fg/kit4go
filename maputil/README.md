# maputil

Generic map helpers missing from Go 1.21's [`maps`](https://pkg.go.dev/maps)
package. Pure standard library.

The stdlib `maps` package covers keys / values / clone / copy / delete / equal,
but leaves out merging, inverting, and slice↔map conversions that every Go
project reimplements.

## Quick start

```go
import "github.com/v8fg/kit4go/maputil"

// Merge config layers (last-wins on conflict).
cfg := maputil.Merge(defaults, envOverrides, cliFlags)

// Invert a lookup table.
numToWord := maputil.Invert(map[string]int{"one": 1, "two": 2}) // map[int]string{1:"one",2:"two"}

// Build a map from a slice.
m := maputil.FromSlice(rows, func(r Row) (int, string) { return r.ID, r.Name })

// Materialize to key-value pairs (map iteration is unordered).
for _, kv := range maputil.ToSlice(m) {
	fmt.Println(kv.Key, kv.Value)
}
```

## API

| Function | Description |
|----------|-------------|
| `Merge(ms...)` | New map, later maps override earlier on key conflict |
| `Invert(m)` | New map with keys↔values swapped (last-wins on dup values) |
| `FromSlice(s, fn)` | `map[K]V` from a key+value function |
| `ToSlice(m)` | `[]KV[K,V]` (map iteration order is random) |
| `Equal(a, b)` | Same key-value pairs (delegates to `maps.Equal`) |
| `Copy(dst, src)` | Alias for `maps.Copy` |

`KV[K, V]` is an exported `{ Key K; Value V }` struct usable as a map key when
both `K` and `V` are comparable.

## Notes

- All functions return **new** maps; inputs are never mutated.
- `Invert` requires values to be `comparable` (they become keys). On duplicate
  values, the last key wins.
- `ToSlice` returns a non-nil empty slice for an empty map.
- Map iteration order is non-deterministic; sort the result if order matters.
