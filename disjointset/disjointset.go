// Package disjointset provides a generic disjoint-set (union-find / merge-find)
// data structure with path compression and union by rank. Near O(1) amortized
// per operation (inverse Ackermann). Pure standard library.
//
// Use cases: connected-components, Kruskal's minimum spanning tree, cycle
// detection in undirected graphs, equivalence clustering, network connectivity.
//
// Not safe for concurrent use — protect with a sync.Mutex (Find's path
// compression mutates the parent map).
package disjointset

// UnionFind is a disjoint-set forest over comparable element keys.
type UnionFind[T comparable] struct {
	parent map[T]T   // element -> parent (root is its own parent)
	rank   map[T]int // root -> rank (union-by-rank bound)
	size   map[T]int // root -> set cardinality
	count  int       // number of disjoint sets
}

// New creates an empty UnionFind.
func New[T comparable]() *UnionFind[T] {
	return &UnionFind[T]{
		parent: make(map[T]T),
		rank:   make(map[T]int),
		size:   make(map[T]int),
	}
}

// ensure registers x as a singleton set if it is not already known. All public
// operations auto-register unknown elements, so callers need not pre-Add.
func (uf *UnionFind[T]) ensure(x T) {
	if _, ok := uf.parent[x]; ok {
		return
	}
	uf.parent[x] = x
	uf.rank[x] = 0
	uf.size[x] = 1
	uf.count++
}

// Add registers x as a singleton set. No-op if x is already known.
func (uf *UnionFind[T]) Add(x T) { uf.ensure(x) }

// Find returns the representative (root) of x's set, applying path compression.
// Auto-registers x as a singleton if unknown.
func (uf *UnionFind[T]) Find(x T) T {
	uf.ensure(x)
	return uf.find(x)
}

// find returns the root of x with path compression. x MUST already be registered.
func (uf *UnionFind[T]) find(x T) T {
	p := uf.parent[x]
	if p == x {
		return x
	}
	root := uf.find(p)
	uf.parent[x] = root // path compression: point straight at the root
	return root
}

// Union merges the sets containing x and y. If they are already in the same set,
// this is a no-op. Auto-registers unknown elements.
func (uf *UnionFind[T]) Union(x, y T) {
	rx := uf.Find(x)
	ry := uf.Find(y)
	if rx == ry {
		return
	}
	// Union by rank: attach the shorter tree under the taller.
	if uf.rank[rx] < uf.rank[ry] {
		rx, ry = ry, rx
	}
	uf.parent[ry] = rx
	uf.size[rx] += uf.size[ry]
	if uf.rank[rx] == uf.rank[ry] {
		uf.rank[rx]++
	}
	uf.count--
}

// Connected reports whether x and y are in the same set.
func (uf *UnionFind[T]) Connected(x, y T) bool {
	return uf.Find(x) == uf.Find(y)
}

// Count returns the number of disjoint sets.
func (uf *UnionFind[T]) Count() int { return uf.count }

// Size returns the cardinality of x's set. O(1) after Find.
func (uf *UnionFind[T]) Size(x T) int {
	return uf.size[uf.Find(x)]
}

// Reset empties the structure.
func (uf *UnionFind[T]) Reset() {
	clear(uf.parent)
	clear(uf.rank)
	clear(uf.size)
	uf.count = 0
}
