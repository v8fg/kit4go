// Package consistenthash implements rendezvous hashing (HRW — Highest Random
// Weight), a consistent-hashing scheme with no virtual nodes, no ring, and
// minimal key movement on membership change.
//
// For a given key the selected node is argmax over nodes of hash(nodeID || key).
// That single rule gives the consistency guarantees (adding or removing one node
// moves only ~1/N of keys) and a uniform spread. Lookups are O(N) in the node
// count, which is the right tradeoff for the typical tens-to-low-hundreds of
// nodes in shard routing; for very large sets use Maglev or a ring variant.
//
// Ad-tech uses: assigning an auction / user hash to a bidder shard, routing a
// request to a sticky upstream, or partitioning a keyspace across workers with
// minimal redistribution when the fleet scales.
package consistenthash

import (
	"hash/fnv"
	"sync"
)

// Hash turns a byte slice into a 64-bit digest. The default (DefaultHash) is
// FNV-1a 64: fast, deterministic, and good enough for bucketing. Inject a
// cryptographic hash only if predictability is a concern.
type Hash func(data []byte) uint64

// DefaultHash is FNV-1a 64.
func DefaultHash(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

// Map is a set of nodes T mapped via rendezvous hashing. The zero value is an
// empty, unusable map; construct with New. All methods are safe for concurrent
// use.
type Map[T any] struct {
	mu      sync.RWMutex
	id      func(T) string
	hash    Hash
	nodes   []T
	scratch sync.Pool // *[]byte, reused as hash(id || key) scratch
}

// Option configures a Map.
type Option[T any] func(*Map[T])

// WithHash overrides the hash function (default DefaultHash).
func WithHash[T any](h Hash) Option[T] { return func(m *Map[T]) { m.hash = h } }

// WithNodes seeds the map with nodes. Convenience for the common "construct with
// a node list" case; equivalent to calling Add after New. Duplicate IDs (by
// id()) are ignored, mirroring [Add] so the "distinct nodes" contract for GetN
// holds regardless of how nodes enter the map.
func WithNodes[T any](nodes ...T) Option[T] {
	return func(m *Map[T]) { m.appendDedupLocked(nodes...) }
}

// New builds a rendezvous-hashing map. id must return a stable, unique string
// for each node (e.g. host:port or a node ID); it is the hash input and the
// identity used by Remove.
func New[T any](id func(T) string, opts ...Option[T]) *Map[T] {
	m := &Map[T]{id: id, hash: DefaultHash}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Add inserts nodes. Duplicate IDs (by id()) are ignored.
func (m *Map[T]) Add(nodes ...T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendDedupLocked(nodes...)
}

// Remove drops the node whose id() matches the given node. No-op if absent.
func (m *Map[T]) Remove(node T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	target := m.id(node)
	out := m.nodes[:0]
	for _, n := range m.nodes {
		if m.id(n) == target {
			continue
		}
		out = append(out, n)
	}
	m.nodes = out
}

// Len returns the node count.
func (m *Map[T]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

// Get returns the node responsible for key, or ok=false when the map is empty.
// Selection is argmax of hash(id(node) || key).
func (m *Map[T]) Get(key string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var zero T
	if len(m.nodes) == 0 {
		return zero, false
	}
	var best T
	bestScore := uint64(0)
	bestSet := false
	bufp := m.acquireScratch()
	defer m.scratch.Put(bufp)
	buf := *bufp
	for _, n := range m.nodes {
		score := m.scoreNode(&buf, n, key)
		if !bestSet || score > bestScore {
			bestScore = score
			best = n
			bestSet = true
		}
	}
	*bufp = buf
	return best, true
}

// GetN returns up to n distinct nodes responsible for key, ordered by HRW score
// (highest first). Useful for replication: the first is primary, the rest are
// fallbacks. Returns fewer than n when the map has fewer nodes.
func (m *Map[T]) GetN(key string, n int) []T {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.nodes) == 0 || n <= 0 {
		return nil
	}
	type scored struct {
		node  T
		score uint64
	}
	scores := make([]scored, len(m.nodes))
	bufp := m.acquireScratch()
	defer m.scratch.Put(bufp)
	buf := *bufp
	for i, nd := range m.nodes {
		scores[i] = scored{node: nd, score: m.scoreNode(&buf, nd, key)}
	}
	*bufp = buf
	// Partial selection of the top-n by score; sort only when n exceeds nodes.
	if n > len(scores) {
		n = len(scores)
	}
	// Simple selection: full sort is fine for modest N.
	for i := range n {
		max := i
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[max].score {
				max = j
			}
		}
		scores[i], scores[max] = scores[max], scores[i]
	}
	out := make([]T, n)
	for i := range n {
		out[i] = scores[i].node
	}
	return out
}

// scoreNode hashes id(node) || key into buf, returning the digest. buf is a
// per-Map scratch buffer (pool-backed) reused across node iterations within a
// single Get/GetN and across calls; only its first use allocates. The byte
// sequence fed to the hash is identical to a fresh make([]byte, 0, len(id)+len(key))
// each iteration, so hash output is byte-for-byte unchanged — only the backing
// storage is recycled.
func (m *Map[T]) scoreNode(buf *[]byte, node T, key string) uint64 {
	nodeID := m.id(node)
	n := len(nodeID) + len(key)
	if cap(*buf) < n {
		*buf = make([]byte, n, n*2)
	} else {
		*buf = (*buf)[:n]
	}
	copy(*buf, nodeID)
	copy((*buf)[len(nodeID):], key)
	return m.hash(*buf)
}

// acquireScratch fetches a reusable []byte scratch from the pool. The returned
// pointer is Put back by the caller (via defer). Pool reuse means the per-node
// make in the original Get/GetN is replaced by a single amortized buffer that
// grows once to the largest (id || key) and is then recycled.
func (m *Map[T]) acquireScratch() *[]byte {
	v := m.scratch.Get()
	if v == nil {
		b := make([]byte, 0, 64)
		return &b
	}
	return v.(*[]byte)
}

func (m *Map[T]) containsLocked(id string) bool {
	for _, n := range m.nodes {
		if m.id(n) == id {
			return true
		}
	}
	return false
}

// appendDedupLocked appends nodes whose id() is not already present, skipping
// duplicates both against the existing set and within the incoming batch. The
// caller must hold m.mu (or be inside WithNodes, where the map is still under
// construction and unshared). This is the single dedup path shared by Add and
// WithNodes, so duplicate IDs can never reach m.nodes regardless of entry point.
func (m *Map[T]) appendDedupLocked(nodes ...T) {
	for _, n := range nodes {
		nid := m.id(n)
		if m.containsLocked(nid) {
			continue
		}
		m.nodes = append(m.nodes, n)
	}
}
