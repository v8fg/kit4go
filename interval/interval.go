// Package interval provides a generic half-open interval [Start, End) with
// overlap, containment, intersection, and merge operations.
//
// The half-open convention [Start, End) means Start is inclusive and End is
// exclusive — the standard for range queries (adjacent intervals don't overlap,
// and End == Start of the next range). For a closed interval [Start, End], use
// ContainsInclusive.
//
// For time.Time intervals (the most common ad-tech/finance use: campaign date
// ranges, trading hours), convert to int64 via UnixNano since time.Time does not
// satisfy cmp.Ordered.
//
// Pure standard library.
package interval

import (
	"cmp"
	"errors"
	"slices"
)

// ErrInverted is returned by New when start >= end.
var ErrInverted = errors.New("interval: start must be less than end")

// Interval is a half-open range [Start, End). Start must be < End (see New).
type Interval[T cmp.Ordered] struct {
	Start T
	End   T
}

// New builds an Interval, returning an error if start >= end (empty or inverted).
func New[T cmp.Ordered](start, end T) (Interval[T], error) {
	if cmp.Compare(start, end) >= 0 {
		return Interval[T]{}, ErrInverted
	}
	return Interval[T]{Start: start, End: end}, nil
}

// MustNew is like New but panics on an inverted range.
func MustNew[T cmp.Ordered](start, end T) Interval[T] {
	i, err := New(start, end)
	if err != nil {
		panic(err)
	}
	return i
}

// Contains reports whether v is in [Start, End).
func (i Interval[T]) Contains(v T) bool {
	return cmp.Compare(v, i.Start) >= 0 && cmp.Compare(v, i.End) < 0
}

// ContainsInclusive reports whether v is in [Start, End].
func (i Interval[T]) ContainsInclusive(v T) bool {
	return cmp.Compare(v, i.Start) >= 0 && cmp.Compare(v, i.End) <= 0
}

// Overlaps reports whether i and other share any point in [Start, End) range.
// Adjacent intervals (i.End == other.Start) do NOT overlap (half-open).
func (i Interval[T]) Overlaps(other Interval[T]) bool {
	return cmp.Compare(i.Start, other.End) < 0 && cmp.Compare(other.Start, i.End) < 0
}

// IsBefore reports whether i is entirely before other (i.End <= other.Start).
func (i Interval[T]) IsBefore(other Interval[T]) bool {
	return cmp.Compare(i.End, other.Start) <= 0
}

// IsAfter reports whether i is entirely after other (i.Start >= other.End).
func (i Interval[T]) IsAfter(other Interval[T]) bool {
	return cmp.Compare(i.Start, other.End) >= 0
}

// Union returns the smallest interval containing both i and other. ok is false
// if they don't overlap or touch (there would be a gap).
func (i Interval[T]) Union(other Interval[T]) (Interval[T], bool) {
	if !i.Overlaps(other) && !i.touches(other) {
		return Interval[T]{}, false
	}
	return Interval[T]{
		Start: min(i.Start, other.Start),
		End:   max(i.End, other.End),
	}, true
}

// Intersect returns the intersection [max(Start), min(End)). ok is false if the
// result is empty (no overlap).
func (i Interval[T]) Intersect(other Interval[T]) (Interval[T], bool) {
	s := max(i.Start, other.Start)
	e := min(i.End, other.End)
	if cmp.Compare(s, e) >= 0 {
		return Interval[T]{}, false
	}
	return Interval[T]{Start: s, End: e}, true
}

// touches reports whether i and other are adjacent (i.End == other.Start or
// other.End == i.Start) — needed by Union to merge touching ranges.
func (i Interval[T]) touches(other Interval[T]) bool {
	return cmp.Compare(i.End, other.Start) == 0 || cmp.Compare(other.End, i.Start) == 0
}

// Merge sorts and merges overlapping or touching intervals into the minimum set
// of disjoint intervals.
func Merge[T cmp.Ordered](intervals []Interval[T]) []Interval[T] {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]Interval[T], len(intervals))
	copy(sorted, intervals)
	slices.SortFunc(sorted, func(a, b Interval[T]) int {
		return cmp.Compare(a.Start, b.Start)
	})
	merged := []Interval[T]{sorted[0]}
	for _, cur := range sorted[1:] {
		last := &merged[len(merged)-1]
		if cmp.Compare(cur.Start, last.End) <= 0 {
			if cmp.Compare(cur.End, last.End) > 0 {
				last.End = cur.End
			}
		} else {
			merged = append(merged, cur)
		}
	}
	return merged
}
