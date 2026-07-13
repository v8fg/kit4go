// Package set provides a generic, type-safe set for comparable types, backed by
// a map. It supports the standard set algebra (union, intersection, difference,
// symmetric difference) and predicates (subset, superset, disjoint, equal).
//
// A zero-value Set is NOT usable — build one with [New] or [From].
//
// Concurrency: like a Go map, a Set is NOT safe for concurrent use. For
// concurrent access, protect it with a sync.RWMutex (RLock for read-only
// methods, Lock for mutators) or use a per-key shard.
//
// Pure standard library. Ad-tech / finance uses: tag sets (does this creative
// have the "holiday" tag?), capability sets (does this user have "read"?),
// allow/deny list membership, de-duplication of batch IDs, and set-algebra over
// A/B-test cohorts or audience segments.
package set

// Set is a collection of unique values of type T.
type Set[T comparable] struct {
	m map[T]struct{}
}

// New builds a Set from the given values. Duplicates are collapsed.
func New[T comparable](vals ...T) *Set[T] {
	s := &Set[T]{m: make(map[T]struct{}, len(vals))}
	s.Add(vals...)
	return s
}

// From builds a Set from a slice (convenience for New(s...)).
func From[T comparable](slice []T) *Set[T] { return New(slice...) }

// Add inserts values; already-present values are no-ops.
func (s *Set[T]) Add(vals ...T) {
	for _, v := range vals {
		s.m[v] = struct{}{}
	}
}

// Remove deletes values; absent values are no-ops.
func (s *Set[T]) Remove(vals ...T) {
	for _, v := range vals {
		delete(s.m, v)
	}
}

// Contains reports whether v is in the set.
func (s *Set[T]) Contains(v T) bool {
	_, ok := s.m[v]
	return ok
}

// ContainsAll reports whether every value is in the set.
func (s *Set[T]) ContainsAll(vals ...T) bool {
	for _, v := range vals {
		if _, ok := s.m[v]; !ok {
			return false
		}
	}
	return true
}

// ContainsAny reports whether at least one value is in the set.
func (s *Set[T]) ContainsAny(vals ...T) bool {
	for _, v := range vals {
		if _, ok := s.m[v]; ok {
			return true
		}
	}
	return false
}

// Len returns the number of elements.
func (s *Set[T]) Len() int { return len(s.m) }

// IsEmpty reports whether the set has no elements.
func (s *Set[T]) IsEmpty() bool { return len(s.m) == 0 }

// Clear removes all elements.
func (s *Set[T]) Clear() { clear(s.m) }

// Pop removes and returns an arbitrary element. ok is false if the set is empty.
func (s *Set[T]) Pop() (val T, ok bool) {
	for v := range s.m {
		delete(s.m, v)
		return v, true
	}
	return val, false
}

// Each calls fn for every element. Iteration order is unspecified (map order).
func (s *Set[T]) Each(fn func(T)) {
	for v := range s.m {
		fn(v)
	}
}

// Filter returns a new Set containing only elements for which fn returns true.
func (s *Set[T]) Filter(fn func(T) bool) *Set[T] {
	out := New[T]()
	for v := range s.m {
		if fn(v) {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// ToSlice returns the elements as a slice in unspecified order.
func (s *Set[T]) ToSlice() []T {
	out := make([]T, 0, len(s.m))
	for v := range s.m {
		out = append(out, v)
	}
	return out
}

// Clone returns a shallow copy of the set.
func (s *Set[T]) Clone() *Set[T] {
	out := New[T]()
	for v := range s.m {
		out.m[v] = struct{}{}
	}
	return out
}

// --- Set algebra (functional, return new sets) ---

// Union returns a new Set containing all elements from the given sets.
func Union[T comparable](sets ...*Set[T]) *Set[T] {
	out := New[T]()
	for _, s := range sets {
		if s == nil {
			continue
		}
		for v := range s.m {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// Intersect returns a new Set of elements present in BOTH a and b.
func Intersect[T comparable](a, b *Set[T]) *Set[T] {
	out := New[T]()
	if a == nil || b == nil {
		return out
	}
	// Iterate the smaller set for fewer lookups.
	small, big := a, b
	if len(b.m) < len(a.m) {
		small, big = b, a
	}
	for v := range small.m {
		if _, ok := big.m[v]; ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// Difference returns a new Set of elements in a but NOT in b (a − b).
func Difference[T comparable](a, b *Set[T]) *Set[T] {
	out := New[T]()
	if a == nil {
		return out
	}
	for v := range a.m {
		if b == nil {
			out.m[v] = struct{}{}
			continue
		}
		if _, ok := b.m[v]; !ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// SymmetricDifference returns a new Set of elements in a or b but not both (a Δ b).
func SymmetricDifference[T comparable](a, b *Set[T]) *Set[T] {
	out := New[T]()
	if a != nil {
		for v := range a.m {
			if b == nil || !b.Contains(v) {
				out.m[v] = struct{}{}
			}
		}
	}
	if b != nil {
		for v := range b.m {
			if a == nil || !a.Contains(v) {
				out.m[v] = struct{}{}
			}
		}
	}
	return out
}

// --- Predicates ---

// IsSubset reports whether every element of sub is in sup.
func IsSubset[T comparable](sub, sup *Set[T]) bool {
	if sub == nil {
		return true
	}
	if sup == nil {
		return sub.IsEmpty()
	}
	for v := range sub.m {
		if _, ok := sup.m[v]; !ok {
			return false
		}
	}
	return true
}

// IsSuperset reports whether every element of sub is in sup (IsSuperset(sup, sub)).
func IsSuperset[T comparable](sup, sub *Set[T]) bool {
	return IsSubset(sub, sup)
}

// IsDisjoint reports whether a and b have no elements in common.
func IsDisjoint[T comparable](a, b *Set[T]) bool {
	if a == nil || b == nil {
		return true
	}
	// Iterate the smaller set.
	small, big := a, b
	if len(b.m) < len(a.m) {
		small, big = b, a
	}
	for v := range small.m {
		if _, ok := big.m[v]; ok {
			return false
		}
	}
	return true
}

// Equal reports whether a and b contain the same elements.
func Equal[T comparable](a, b *Set[T]) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.m) != len(b.m) {
		return false
	}
	for v := range a.m {
		if _, ok := b.m[v]; !ok {
			return false
		}
	}
	return true
}
