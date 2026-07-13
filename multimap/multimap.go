// Package multimap provides a generic one-to-many map: each key maps to a slice
// of values. Backed by map[K][]V. Pure standard library.
//
// Use cases: indexes, grouped lookups, adjacency lists, multi-valued query
// params (e.g. ?tag=a&tag=b). For a one-to-one map see [bimap].
//
// Not safe for concurrent use — protect with a sync.RWMutex or shard by key.
package multimap

// MultiMap maps each key to a slice of values.
type MultiMap[K comparable, V any] struct {
	m map[K][]V
}

// New creates an empty MultiMap.
func New[K comparable, V any]() *MultiMap[K, V] {
	return &MultiMap[K, V]{m: make(map[K][]V)}
}

// Add appends v to the bucket for k. The key is created if absent.
func (mm *MultiMap[K, V]) Add(k K, v V) {
	mm.m[k] = append(mm.m[k], v)
}

// AddAll appends all of vs to the bucket for k.
func (mm *MultiMap[K, V]) AddAll(k K, vs []V) {
	mm.m[k] = append(mm.m[k], vs...)
}

// Set replaces the bucket for k with vs (a copy). Removes the key if vs is empty.
func (mm *MultiMap[K, V]) Set(k K, vs []V) {
	if len(vs) == 0 {
		delete(mm.m, k)
		return
	}
	bucket := make([]V, len(vs))
	copy(bucket, vs)
	mm.m[k] = bucket
}

// Get returns the value slice for k. The returned slice aliases the internal
// storage — do not mutate it; copy first if you need to. Returns nil if k is
// absent.
func (mm *MultiMap[K, V]) Get(k K) []V {
	return mm.m[k]
}

// Has reports whether k has at least one value.
func (mm *MultiMap[K, V]) Has(k K) bool {
	bucket, ok := mm.m[k]
	return ok && len(bucket) > 0
}

// Count returns the number of values under k (0 if absent).
func (mm *MultiMap[K, V]) Count(k K) int {
	return len(mm.m[k])
}

// Delete removes k and all its values.
func (mm *MultiMap[K, V]) Delete(k K) {
	delete(mm.m, k)
}

// DeleteValue removes the first occurrence of v from k's bucket. Returns true
// if v was found and removed. Requires V to be comparable.
func DeleteValue[K comparable, V comparable](mm *MultiMap[K, V], k K, v V) bool {
	bucket, ok := mm.m[k]
	if !ok {
		return false
	}
	for i, x := range bucket {
		if x == v {
			bucket = append(bucket[:i], bucket[i+1:]...)
			if len(bucket) == 0 {
				delete(mm.m, k)
			} else {
				mm.m[k] = bucket
			}
			return true
		}
	}
	return false
}

// Keys returns all keys with at least one value. Order is non-deterministic.
func (mm *MultiMap[K, V]) Keys() []K {
	keys := make([]K, 0, len(mm.m))
	for k, bucket := range mm.m {
		if len(bucket) > 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

// Len returns the number of keys with at least one value.
func (mm *MultiMap[K, V]) Len() int {
	n := 0
	for _, bucket := range mm.m {
		if len(bucket) > 0 {
			n++
		}
	}
	return n
}

// Each calls fn for every (key, value) pair. Iteration stops if fn returns false.
// Order is non-deterministic.
func (mm *MultiMap[K, V]) Each(fn func(K, V) bool) {
	for k, bucket := range mm.m {
		for _, v := range bucket {
			if !fn(k, v) {
				return
			}
		}
	}
}

// Clear removes all keys and values.
func (mm *MultiMap[K, V]) Clear() {
	clear(mm.m)
}
