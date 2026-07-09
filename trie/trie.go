// Package trie is a generic, concurrency-safe prefix tree (trie) for string-keyed
// lookups. It supports exact match, longest-prefix match, and prefix-scan.
//
// Pure standard library. Ad-tech uses: domain/URL routing (match the longest
// configured domain for a request URL), SSP endpoint classification, keyword
// blocklists, creative-URL categorisation, and IP-prefix (CIDR-like) lookups
// when keys are canonicalised to fixed-width strings.
package trie

import (
	"sort"
	"strings"
	"sync"
)

// Trie is a prefix tree mapping string keys to values of type V. Safe for
// concurrent use. The tree is unbounded — there is deliberately no key-count cap
// or eviction. If a bounded cache is needed, layer a separate cache on top; a
// half-implemented eviction here would be a worse contract than an honest
// unbounded trie.
type Trie[V any] struct {
	mu    sync.RWMutex
	root  *node[V]
	count int // current key count
}

type node[V any] struct {
	children map[string]*node[V]
	value    V
	hasValue bool
}

// Option configures a Trie.
type Option[V any] func(*Trie[V])

// New builds an empty Trie.
func New[V any](opts ...Option[V]) *Trie[V] {
	t := &Trie[V]{root: &node[V]{children: make(map[string]*node[V])}}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// segment splits a key into trie segments by "/". Override with a custom
// segmenter if a different delimiter is needed.
func segments(key string) []string {
	key = strings.Trim(key, "/")
	if key == "" {
		return nil
	}
	return strings.Split(key, "/")
}

// Insert sets key=value. Overwrites if key already exists.
func (t *Trie[V]) Insert(key string, val V) {
	segs := segments(key)
	t.mu.Lock()
	defer t.mu.Unlock()
	cur := t.root
	for _, s := range segs {
		child, ok := cur.children[s]
		if !ok {
			child = &node[V]{children: make(map[string]*node[V])}
			cur.children[s] = child
		}
		cur = child
	}
	if !cur.hasValue {
		t.count++
	}
	cur.value = val
	cur.hasValue = true
}

// Get returns the value for an exact key match.
func (t *Trie[V]) Get(key string) (V, bool) {
	segs := segments(key)
	t.mu.RLock()
	defer t.mu.RUnlock()
	cur := t.descend(segs)
	if cur == nil || !cur.hasValue {
		var zero V
		return zero, false
	}
	return cur.value, true
}

// Has reports whether key has an exact match.
func (t *Trie[V]) Has(key string) bool {
	_, ok := t.Get(key)
	return ok
}

// LongestPrefix returns the value of the longest configured key that is a prefix
// of the query. For example, if keys "a", "a/b", and "a/b/c" are inserted, a
// query for "a/b/c/d" matches "a/b/c". Returns false if no prefix matches.
func (t *Trie[V]) LongestPrefix(query string) (V, string, bool) {
	segs := segments(query)
	t.mu.RLock()
	defer t.mu.RUnlock()
	cur := t.root
	var bestVal V
	var bestKey string
	var found bool
	// Walk the query path; track the last node that has a value.
	path := make([]string, 0, len(segs))
	for _, s := range segs {
		child, ok := cur.children[s]
		if !ok {
			break
		}
		cur = child
		path = append(path, s)
		if cur.hasValue {
			bestVal = cur.value
			bestKey = strings.Join(path, "/")
			found = true
		}
	}
	// Also check the root (empty prefix) — it has a value only if Insert("", v) was called.
	if !found && t.root.hasValue {
		return t.root.value, "", true
	}
	return bestVal, bestKey, found
}

// Delete removes a key. Returns true if the key existed.
func (t *Trie[V]) Delete(key string) bool {
	segs := segments(key)
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(segs) == 0 {
		had := t.root.hasValue
		t.root.hasValue = false
		return had
	}
	// Walk to the parent of the target, recording the path for cleanup.
	path := make([]*node[V], 0, len(segs))
	cur := t.root
	for _, s := range segs {
		child, ok := cur.children[s]
		if !ok {
			return false
		}
		path = append(path, cur)
		cur = child
	}
	if !cur.hasValue {
		return false
	}
	cur.hasValue = false
	t.count--
	// Prune empty nodes from the leaf upward.
	for i := len(segs) - 1; i >= 0; i-- {
		if len(cur.children) > 0 || cur.hasValue {
			break
		}
		parent := path[i]
		delete(parent.children, segs[i])
		cur = parent
	}
	return true
}

// KeysWithPrefix returns all keys that start with the given prefix.
func (t *Trie[V]) KeysWithPrefix(prefix string) []string {
	segs := segments(prefix)
	t.mu.RLock()
	defer t.mu.RUnlock()
	cur := t.descend(segs)
	if cur == nil {
		return nil
	}
	var results []string
	var walk func(n *node[V], path []string)
	walk = func(n *node[V], path []string) {
		if n.hasValue {
			results = append(results, strings.Join(path, "/"))
		}
		// Sort children for deterministic output order.
		keys := make([]string, 0, len(n.children))
		for seg := range n.children {
			keys = append(keys, seg)
		}
		sort.Strings(keys)
		for _, seg := range keys {
			walk(n.children[seg], append(path, seg))
		}
	}
	walk(cur, segs)
	return results
}

// Len returns the number of keys in the trie.
func (t *Trie[V]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// descend walks segments from the root; returns nil if any segment is missing.
func (t *Trie[V]) descend(segs []string) *node[V] {
	cur := t.root
	for _, s := range segs {
		child, ok := cur.children[s]
		if !ok {
			return nil
		}
		cur = child
	}
	return cur
}
