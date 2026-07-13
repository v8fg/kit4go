// Package tuple provides generic Pair and Triple types for bundling 2 or 3
// values of potentially different types into a single value — useful as a map
// key (when comparable), a function return, or a lightweight struct substitute.
//
// Pure standard library.
package tuple

// Pair holds two values of potentially different types.
type Pair[A, B any] struct {
	First  A
	Second B
}

// NewPair builds a Pair.
func NewPair[A, B any](a A, b B) Pair[A, B] { return Pair[A, B]{First: a, Second: b} }

// Values returns the two values.
func (p Pair[A, B]) Values() (A, B) { return p.First, p.Second }

// Triple holds three values of potentially different types.
type Triple[A, B, C any] struct {
	First  A
	Second B
	Third  C
}

// NewTriple builds a Triple.
func NewTriple[A, B, C any](a A, b B, c C) Triple[A, B, C] {
	return Triple[A, B, C]{First: a, Second: b, Third: c}
}

// Values returns the three values.
func (t Triple[A, B, C]) Values() (A, B, C) { return t.First, t.Second, t.Third }
