// Package iterx provides functional combinators over Go 1.23+ range-over-func
// iterators ([iter.Seq] / [iter.Seq2]): Map, Filter, Take, Drop, Collect,
// Reduce, Chain, Zip, Range. Pure standard library.
//
// The combinators are lazy — Map/Filter/Take/Drop/Chain/Zip return an iter.Seq
// that pulls from the source only when iterated. Collect and Reduce are the
// terminal operations that consume the iterator.
//
// Early termination is propagated: if a consumer stops ranging (via break or
// return), the upstream iterator is informed so it can release resources.
package iterx

import (
	"iter"

	"github.com/v8fg/kit4go/tuple"
)

// Map applies fn to each element of seq, yielding the results lazily.
func Map[T, U any](seq iter.Seq[T], fn func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(fn(v)) {
				return
			}
		}
	}
}

// Filter yields only the elements of seq for which pred returns true.
func Filter[T any](seq iter.Seq[T], pred func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if pred(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Take yields at most the first n elements of seq. n <= 0 yields nothing.
func Take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		count := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			count++
			if count >= n {
				return
			}
		}
	}
}

// Drop skips the first n elements, then yields the rest. n <= 0 yields all.
func Drop[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		skipped := 0
		for v := range seq {
			if skipped < n {
				skipped++
				continue
			}
			if !yield(v) {
				return
			}
		}
	}
}

// Collect materializes seq into a slice. Returns nil for an empty iterator.
func Collect[T any](seq iter.Seq[T]) []T {
	var out []T
	for v := range seq {
		out = append(out, v)
	}
	return out
}

// Reduce folds seq left-to-right with fn, starting from initial.
func Reduce[T, U any](seq iter.Seq[T], initial U, fn func(U, T) U) U {
	acc := initial
	for v := range seq {
		acc = fn(acc, v)
	}
	return acc
}

// Chain concatenates multiple seqs in order, yielding a single seq.
func Chain[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, s := range seqs {
			for v := range s {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Zip pairs elements from a and b, stopping at the shorter. Uses iter.Pull so
// the two iterators advance together.
func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq[tuple.Pair[A, B]] {
	return func(yield func(tuple.Pair[A, B]) bool) {
		next, stop := iter.Pull(b)
		defer stop()
		for va := range a {
			vb, ok := next()
			if !ok {
				return
			}
			if !yield(tuple.NewPair(va, vb)) {
				return
			}
		}
	}
}

// Range yields integers from start (inclusive) to end (exclusive), stepping by
// step. A positive step counts up; a negative step counts down; step == 0
// yields nothing.
func Range(start, end, step int) iter.Seq[int] {
	return func(yield func(int) bool) {
		if step == 0 {
			return
		}
		if step > 0 {
			for i := start; i < end; i += step {
				if !yield(i) {
					return
				}
			}
			return
		}
		for i := start; i > end; i += step {
			if !yield(i) {
				return
			}
		}
	}
}

// Seq2Keys yields the keys of an iter.Seq2.
func Seq2Keys[K, V any](seq iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range seq {
			if !yield(k) {
				return
			}
		}
	}
}

// Seq2Values yields the values of an iter.Seq2.
func Seq2Values[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}
