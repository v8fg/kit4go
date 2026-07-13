// Package sliceutil provides generic slice helpers missing from Go 1.21's
// [slices] package: Chunk, Flatten, Deduplicate, Partition, GroupBy, Window,
// Fill, Repeat, Reverse (in-place), Associate.
//
// Pure standard library.
package sliceutil

import "slices"

// Chunk splits s into sub-slices of at most size elements. The last chunk may
// be shorter. Returns nil if size <= 0 or s is empty.
func Chunk[T any](s []T, size int) [][]T {
	if size <= 0 || len(s) == 0 {
		return nil
	}
	chunks := make([][]T, 0, (len(s)+size-1)/size)
	for i := 0; i < len(s); i += size {
		end := min(i+size, len(s))
		chunks = append(chunks, s[i:end:end])
	}
	return chunks
}

// Flatten concatenates multiple slices into one. Returns nil if the total
// length is zero (the package convention: empty input → nil).
func Flatten[T any](slices [][]T) []T {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	if total == 0 {
		return nil
	}
	out := make([]T, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// Deduplicate returns a new slice with duplicates removed, preserving the first
// occurrence order. Requires comparable T. Returns nil for empty input.
func Deduplicate[T comparable](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	if len(s) == 1 {
		return slices.Clone(s)
	}
	seen := make(map[T]struct{}, len(s))
	out := make([]T, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// Partition splits s into two slices: the first contains elements where pred
// returns true, the second where it returns false.
func Partition[T any](s []T, pred func(T) bool) ([]T, []T) {
	a := make([]T, 0, len(s))
	b := make([]T, 0, len(s))
	for _, v := range s {
		if pred(v) {
			a = append(a, v)
		} else {
			b = append(b, v)
		}
	}
	return a, b
}

// GroupBy groups elements by a key function, returning a map from key to the
// slice of elements with that key.
func GroupBy[T any, K comparable](s []T, keyFn func(T) K) map[K][]T {
	m := make(map[K][]T)
	for _, v := range s {
		k := keyFn(v)
		m[k] = append(m[k], v)
	}
	return m
}

// Window returns all contiguous sub-slices of length n. Returns nil if n <= 0
// or n > len(s).
func Window[T any](s []T, n int) [][]T {
	if n <= 0 || n > len(s) {
		return nil
	}
	windows := make([][]T, 0, len(s)-n+1)
	for i := 0; i <= len(s)-n; i++ {
		windows = append(windows, s[i:i+n:i+n])
	}
	return windows
}

// Fill sets every element of s to v (in-place).
func Fill[T any](s []T, v T) {
	for i := range s {
		s[i] = v
	}
}

// Repeat returns a new slice with v repeated n times.
func Repeat[T any](v T, n int) []T {
	if n <= 0 {
		return nil
	}
	s := make([]T, n)
	for i := range s {
		s[i] = v
	}
	return s
}

// Reverse returns a reversed copy of s.
func Reverse[T any](s []T) []T {
	out := slices.Clone(s)
	slices.Reverse(out)
	return out
}

// Associate transforms a slice into a map using a key+value function.
func Associate[T any, K comparable, V any](s []T, fn func(T) (K, V)) map[K]V {
	m := make(map[K]V, len(s))
	for _, v := range s {
		k, val := fn(v)
		m[k] = val
	}
	return m
}

// Index returns the index of the first occurrence of v, or -1 if not found.
func Index[T comparable](s []T, v T) int {
	return slices.Index(s, v)
}

// Contains reports whether v is in s.
func Contains[T comparable](s []T, v T) bool {
	return slices.Contains(s, v)
}
