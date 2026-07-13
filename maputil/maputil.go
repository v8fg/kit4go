// Package maputil provides generic map helpers missing from Go 1.21's [maps]
// package: Merge, Invert, FromSlice, ToSlice, Equal.
//
// Pure standard library.
package maputil

import "maps"

// Merge combines multiple maps into a new one. Later maps take precedence on
// key conflicts.
func Merge[K comparable, V any](ms ...map[K]V) map[K]V {
	total := 0
	for _, m := range ms {
		total += len(m)
	}
	out := make(map[K]V, total)
	for _, m := range ms {
		maps.Copy(out, m)
	}
	return out
}

// Invert swaps keys and values. If two keys share the same value, the last one
// wins. Returns a new map.
func Invert[K comparable, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

// FromSlice builds a map from a slice using a key+value function.
func FromSlice[T any, K comparable, V any](s []T, fn func(T) (K, V)) map[K]V {
	m := make(map[K]V, len(s))
	for _, v := range s {
		k, val := fn(v)
		m[k] = val
	}
	return m
}

// ToSlice converts a map to a slice of key-value pairs.
func ToSlice[K comparable, V any](m map[K]V) []KV[K, V] {
	out := make([]KV[K, V], 0, len(m))
	for k, v := range m {
		out = append(out, KV[K, V]{Key: k, Value: v})
	}
	return out
}

// KV is a key-value pair.
type KV[K comparable, V any] struct {
	Key   K
	Value V
}

// Equal reports whether two maps have the same key-value pairs. Requires V to be
// comparable.
func Equal[K comparable, V comparable](a, b map[K]V) bool {
	return maps.Equal(a, b)
}

// Copy is an alias for maps.Copy (included for API discoverability).
func Copy[K comparable, V any](dst, src map[K]V) {
	maps.Copy(dst, src)
}
