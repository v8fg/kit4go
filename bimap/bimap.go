// Package bimap provides a generic bidirectional map — a map that can be looked
// up by key OR by value in O(1). Insertions enforce that both keys and values
// are unique (a one-to-one mapping). Pure standard library.
//
// Ad-tech / finance uses: enum↔string (status code ↔ HTTP status text), ID↔name
// (campaign ID ↔ campaign name for bidirectional resolution), short-code ↔ URL
// (shortlink encode/decode).
package bimap

import "errors"

// ErrDuplicateKey is returned by Insert when the key already exists.
var ErrDuplicateKey = errors.New("bimap: key already exists")

// ErrDuplicateValue is returned by Insert when the value already exists.
var ErrDuplicateValue = errors.New("bimap: value already exists")

// BiMap is a bidirectional map enforcing a one-to-one relationship between
// keys (K) and values (V). Both K and V must be comparable.
//
// Concurrency: not safe for concurrent use — protect with a sync.RWMutex.
type BiMap[K, V comparable] struct {
	forward map[K]V
	reverse map[V]K
}

// New creates an empty BiMap.
func New[K, V comparable]() *BiMap[K, V] {
	return &BiMap[K, V]{
		forward: make(map[K]V),
		reverse: make(map[V]K),
	}
}

// FromMap builds a BiMap from a regular map. Returns an error if two keys map to
// the same value (violates the one-to-one invariant).
func FromMap[K, V comparable](m map[K]V) (*BiMap[K, V], error) {
	bm := New[K, V]()
	for k, v := range m {
		if err := bm.Insert(k, v); err != nil {
			return nil, err
		}
	}
	return bm, nil
}

// Insert adds a key-value pair. Returns ErrDuplicateKey or ErrDuplicateValue if
// either side already exists.
func (b *BiMap[K, V]) Insert(k K, v V) error {
	if _, exists := b.forward[k]; exists {
		return ErrDuplicateKey
	}
	if _, exists := b.reverse[v]; exists {
		return ErrDuplicateValue
	}
	b.forward[k] = v
	b.reverse[v] = k
	return nil
}

// MustInsert is like Insert but panics on duplicate.
func (b *BiMap[K, V]) MustInsert(k K, v V) {
	if err := b.Insert(k, v); err != nil {
		panic(err)
	}
}

// Get returns the value for a key. ok is false if the key is absent.
func (b *BiMap[K, V]) Get(k K) (V, bool) {
	v, ok := b.forward[k]
	return v, ok
}

// GetKey returns the key for a value. ok is false if the value is absent.
func (b *BiMap[K, V]) GetKey(v V) (K, bool) {
	k, ok := b.reverse[v]
	return k, ok
}

// HasKey reports whether k exists.
func (b *BiMap[K, V]) HasKey(k K) bool { _, ok := b.forward[k]; return ok }

// HasValue reports whether v exists.
func (b *BiMap[K, V]) HasValue(v V) bool { _, ok := b.reverse[v]; return ok }

// Delete removes the pair identified by key k. Returns true if the key existed.
func (b *BiMap[K, V]) Delete(k K) bool {
	v, ok := b.forward[k]
	if !ok {
		return false
	}
	delete(b.forward, k)
	delete(b.reverse, v)
	return true
}

// DeleteValue removes the pair identified by value v. Returns true if the value existed.
func (b *BiMap[K, V]) DeleteValue(v V) bool {
	k, ok := b.reverse[v]
	if !ok {
		return false
	}
	delete(b.reverse, v)
	delete(b.forward, k)
	return true
}

// Len returns the number of pairs.
func (b *BiMap[K, V]) Len() int { return len(b.forward) }

// Keys returns all keys (unordered).
func (b *BiMap[K, V]) Keys() []K {
	out := make([]K, 0, len(b.forward))
	for k := range b.forward {
		out = append(out, k)
	}
	return out
}

// Values returns all values (unordered).
func (b *BiMap[K, V]) Values() []V {
	out := make([]V, 0, len(b.reverse))
	for v := range b.reverse {
		out = append(out, v)
	}
	return out
}

// Clear removes all pairs.
func (b *BiMap[K, V]) Clear() {
	clear(b.forward)
	clear(b.reverse)
}
