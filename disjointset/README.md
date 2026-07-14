# disjointset

Generic disjoint-set (union-find) with path compression and union by rank.
Near O(1) amortized per operation (inverse-Ackermann). Pure standard library.

Go's standard library has no union-find. This fills the gap for connectivity and
equivalence problems: connected components, Kruskal's minimum spanning tree,
cycle detection in undirected graphs, clustering, network reachability.

## Quick start

```go
import "github.com/v8fg/kit4go/disjointset"

uf := disjointset.New[int]()
uf.Union(1, 2)
uf.Union(2, 3)

uf.Connected(1, 3) // true — same component
uf.Connected(1, 4) // false
uf.Count()         // 1 (one set so far)
uf.Size(1)         // 3

// Count connected components from an undirected edge list:
edges := [][2]int{{0, 1}, {1, 2}, {3, 4}}
uf := disjointset.New[int]()
for _, e := range edges {
	uf.Union(e[0], e[1])
}
uf.Count() // 2
```

## API

| Method | Description |
|--------|-------------|
| `New[T]()` | Empty union-find |
| `Add(x)` | Register `x` as a singleton (no-op if known) |
| `Find(x)` | Representative (root) of `x`'s set; path-compresses |
| `Union(x, y)` | Merge the sets of `x` and `y` (union by rank) |
| `Connected(x, y)` | Same set? |
| `Count()` | Number of disjoint sets |
| `Size(x)` | Cardinality of `x`'s set |
| `Reset()` | Empty the structure |

## Notes

- **Auto-registration**: `Find`, `Union`, and `Connected` register an unknown
  element as a singleton, so callers need not `Add` first. `Add` exists for
  explicit pre-registration (e.g. recording an isolated node so it counts as its
  own component).
- **Near O(1) amortized**: path compression (flatten the tree on `Find`) plus
  union by rank (attach the shorter tree under the taller) give inverse-Ackermann
  complexity — effectively constant time per operation.
- `Size` is O(1) (set sizes are tracked at roots during `Union`).
- Not safe for concurrent use.
