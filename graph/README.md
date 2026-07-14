# graph

Generic directed graph with traversal and analysis: BFS, DFS, topological sort,
and cycle detection. Backed by an adjacency list keyed by comparable node values.
Pure standard library.

Go's standard library has no graph type. This fills the gap for the common
case — dependency resolution, build-ordering, reachability — without pulling in
a heavy graph library.

## Quick start

```go
import "github.com/v8fg/kit4go/graph"

// Build a dependency graph: edge "from -> to" means from must come before to.
g := graph.New[string]()
g.AddEdge("base", "auth")
g.AddEdge("base", "db")
g.AddEdge("auth", "api")
g.AddEdge("db", "api")

// Topological order (build order).
order, err := g.TopoSort() // [base auth db api], nil

// A cycle makes ordering impossible.
g.AddEdge("api", "base")
_, err = g.TopoSort()       // err == graph.ErrCycle
g.HasCycle()                // true
```

## API

| Method | Description |
|--------|-------------|
| `New[T]()` | Empty directed graph |
| `AddNode(n)` | Add an isolated node |
| `AddEdge(from, to)` | Directed edge (auto-adds nodes; duplicates collapsed) |
| `RemoveEdge(from, to)` | Remove an edge (no-op if absent) |
| `HasEdge(from, to)` | Edge exists? |
| `HasNode(n)` / `Len()` | Membership / node count |
| `Nodes()` | All nodes, insertion order (copy) |
| `Neighbors(n)` | Direct successors of `n` (copy) |
| `BFS(start)` | Breadth-first visit order (nil if start absent) |
| `DFS(start)` | Depth-first pre-order (nil if start absent) |
| `TopoSort()` | Topological order of all nodes; `ErrCycle` if cyclic |
| `HasCycle()` | Contains a cycle (incl. self-loops)? |

## Notes

- **Directed only.** For an undirected graph, add both `a->b` and `b->a`.
- **Deterministic output.** Edges are stored in insertion order and nodes are
  tracked in insertion order, so BFS / DFS / TopoSort always yield the same
  result for the same graph — important for reproducible builds and tests.
- **TopoSort** uses Kahn's algorithm with an insertion-order tiebreak, so among
  the (often many) valid topological orders it returns a stable, predictable one.
- **Self-loops** (`AddEdge(n, n)`) are treated as cycles.
- `HasEdge` and `AddEdge` dedup are O(out-degree of `from`) — fine for typical
  graphs. For very dense graphs a set-backed adjacency would be faster but loses
  deterministic ordering.
- Not safe for concurrent mutation; concurrent reads are fine.
