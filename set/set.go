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

// New builds a Set from the given values. Duplicates are collapsed. The backing
// map is pre-sized to len(vals) to avoid rehashing during construction.
func New[T comparable](vals ...T) *Set[T] {
	s := &Set[T]{m: make(map[T]struct{}, len(vals))}
	s.Add(vals...)
	return s
}

// WithCapacity builds an empty Set with a pre-sized backing map for the given
// number of elements — avoids rehashing when the final size is known.
func WithCapacity[T comparable](cap int) *Set[T] {
	return &Set[T]{m: make(map[T]struct{}, cap)}
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

// Union returns a new Set containing all elements from the given sets. The
// backing map is pre-sized to the total element count to avoid rehashing.
func Union[T comparable](sets ...*Set[T]) *Set[T] {
	total := 0
	for _, s := range sets {
		if s != nil {
			total += len(s.m)
		}
	}
	out := WithCapacity[T](total)
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

// Intersect returns a new Set of elements present in BOTH a and b. The backing
// map is pre-sized to the smaller set (upper bound on the result).
func Intersect[T comparable](a, b *Set[T]) *Set[T] {
	if a == nil || b == nil {
		return WithCapacity[T](0)
	}
	small, big := a, b
	if len(b.m) < len(a.m) {
		small, big = b, a
	}
	out := WithCapacity[T](len(small.m))
	for v := range small.m {
		if _, ok := big.m[v]; ok {
			out.m[v] = struct{}{}
		}
	}
	return out
}

// Difference returns a new Set of elements in a but NOT in b (a − b). The
// backing map is pre-sized to len(a) (upper bound).
func Difference[T comparable](a, b *Set[T]) *Set[T] {
	if a == nil {
		return WithCapacity[T](0)
	}
	out := WithCapacity[T](len(a.m))
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
// Pre-sized to len(a)+len(b) (upper bound).
func SymmetricDifference[T comparable](a, b *Set[T]) *Set[T] {
	cap := 0
	if a != nil {
		cap += len(a.m)
	}
	if b != nil {
		cap += len(b.m)
	}
	out := WithCapacity[T](cap)
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

// --- In-place mutation (zero-allocation hot paths) ---

// AddAll adds all elements from other into this set (in-place union).
func (s *Set[T]) AddAll(other *Set[T]) {
	if other == nil {
		return
	}
	for v := range other.m {
		s.m[v] = struct{}{}
	}
}

// RetainAll removes elements NOT in other (in-place intersection). Returns the
// number of elements removed.
func (s *Set[T]) RetainAll(other *Set[T]) int {
	removed := 0
	for v := range s.m {
		if other == nil || !other.Contains(v) {
			delete(s.m, v)
			removed++
		}
	}
	return removed
}

// RemoveAll removes elements that ARE in other (in-place difference). Returns
// the number of elements removed.
func (s *Set[T]) RemoveAll(other *Set[T]) int {
	removed := 0
	if other == nil {
		return 0
	}
	for v := range other.m {
		if _, ok := s.m[v]; ok {
			delete(s.m, v)
			removed++
		}
	}
	return removed
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
