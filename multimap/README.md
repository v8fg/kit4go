# multimap

Generic one-to-many map: each key maps to a slice of values. Backed by
`map[K][]V`. Pure standard library.

Go's built-in map is one-to-one. When a key needs several values (multi-valued
query params, indexes, adjacency lists, grouped lookups), the `map[K][]V`
pattern is reimplemented everywhere — this packages it.

## Quick start

```go
import "github.com/v8fg/kit4go/multimap"

// Parse ?tag=go&tag=lib&env=prod
params := multimap.New[string, string]()
params.Add("tag", "go")
params.Add("tag", "lib")
params.Add("env", "prod")

params.Get("tag")         // [go lib]
params.Count("tag")       // 2
params.Len()              // 2 (distinct keys)

multimap.DeleteValue(params, "tag", "go") // remove one value
params.Delete("env")      // remove a key entirely
```

## API

| Method / Function | Description |
|-------------------|-------------|
| `New[K,V]()` | Empty MultiMap |
| `Add(k, v)` | Append `v` to `k`'s bucket |
| `AddAll(k, vs)` | Append a slice of values |
| `Set(k, vs)` | Replace bucket (copies; empty removes key) |
| `Get(k)` | Value slice (aliases storage; nil if absent) |
| `Has(k)` | At least one value under `k`? |
| `Count(k)` | Number of values under `k` |
| `Delete(k)` | Remove key + all its values |
| `DeleteValue(mm, k, v)` | Remove first `v` from `k` (requires `V comparable`) |
| `Keys()` | Keys with ≥1 value (non-deterministic order) |
| `Len()` | Count of keys with ≥1 value |
| `Each(fn)` | Iterate `(k, v)` pairs; stop when `fn` returns false |
| `Clear()` | Remove everything |

## Notes

- **`Get` aliases internal storage** — it returns the backing slice directly for
  zero-copy reads. Do not mutate it; copy first (`slices.Clone`) if you need a
  stable snapshot. In-place writes are visible through `Get`.
- **`DeleteValue`** is a free function (not a method) because it requires `V` to
  be `comparable`; Go does not allow a single type to have methods with
  differing constraints on `V`. A `MultiMap[K, V]` whose `V` is non-comparable
  (e.g. slices) simply cannot use it — use `Set` instead.
- A key whose bucket becomes empty (via `Set nil` or `DeleteValue` removing the
  last value) is deleted from the map, so `Has` / `Len` / `Keys` never report
  empty buckets.
- Not safe for concurrent use — protect with a `sync.RWMutex` or shard by key.
  For a one-to-one bidirectional map see `bimap`.
