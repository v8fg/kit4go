// Package omap provides an insertion-ordered map — a [map] that remembers the
// order keys were first added (LinkedHashMap semantics). Pure standard library.
//
// Go's built-in map iterates in random order, which breaks reproducible output
// (e.g. JSON marshaling, configuration diffs, golden-file tests). omap preserves
// insertion order: Set, Keys, Values, and Each visit keys in first-add order.
package omap

// Map is a map from K to V that preserves insertion order.
type Map[K comparable, V any] struct {
	m    map[K]V
	keys []K // insertion order, no duplicates
}

// New creates an empty insertion-ordered Map.
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{m: make(map[K]V)}
}

// Set stores v under k. A new key is appended to the end; updating an existing
// key keeps its original position.
func (m *Map[K, V]) Set(k K, v V) {
	if _, ok := m.m[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.m[k] = v
}

// Get returns the value for k and whether it was present.
func (m *Map[K, V]) Get(k K) (V, bool) {
	v, ok := m.m[k]
	return v, ok
}

// Has reports whether k is present.
func (m *Map[K, V]) Has(k K) bool {
	_, ok := m.m[k]
	return ok
}

// Delete removes k. Returns true if k was present. O(n) (shifts to preserve
// order of the remaining keys).
func (m *Map[K, V]) Delete(k K) bool {
	if _, ok := m.m[k]; !ok {
		return false
	}
	delete(m.m, k)
	for i, key := range m.keys {
		if key == k {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
	return true
}

// Len returns the number of entries.
func (m *Map[K, V]) Len() int { return len(m.keys) }

// Keys returns all keys in insertion order (a copy).
func (m *Map[K, V]) Keys() []K {
	out := make([]K, len(m.keys))
	copy(out, m.keys)
	return out
}

// Values returns all values in insertion order (a copy).
func (m *Map[K, V]) Values() []V {
	out := make([]V, len(m.keys))
	for i, k := range m.keys {
		out[i] = m.m[k]
	}
	return out
}

// Each calls fn for every (key, value) pair in insertion order. Iteration stops
// early if fn returns false.
func (m *Map[K, V]) Each(fn func(K, V) bool) {
	for _, k := range m.keys {
		if !fn(k, m.m[k]) {
			return
		}
	}
}

// Clear removes all entries.
func (m *Map[K, V]) Clear() {
	clear(m.m)
	m.keys = m.keys[:0]
}
