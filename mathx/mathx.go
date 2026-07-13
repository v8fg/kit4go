// Package mathx provides focused numeric helpers missing from the standard
// library: Sum, Product (reduce over a numeric slice), Clamp, Map (range
// remap), and Lerp (linear interpolation).
//
// Pure standard library. Ad-tech / finance uses: sum revenue/cost/impressions
// across a batch, clamp a bid price to [floor, ceiling], remap a probability to
// a score range, interpolate pacing.
package mathx

import "cmp"

// Numeric is the constraint for types supporting addition and multiplication.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Float is the constraint for floating-point types.
type Float interface{ ~float32 | ~float64 }

// Sum returns the arithmetic sum of vals. An empty input returns the zero
// value of T.
func Sum[T Numeric](vals ...T) T {
	var s T
	for _, v := range vals {
		s += v
	}
	return s
}

// Product returns the product of vals. An empty input returns 1 (the
// multiplicative identity).
func Product[T Numeric](vals ...T) T {
	var p T = 1
	for _, v := range vals {
		p *= v
	}
	return p
}

// Clamp clamps v to the inclusive range [lo, hi]. Panics if lo > hi.
func Clamp[T cmp.Ordered](v, lo, hi T) T {
	if lo > hi {
		panic("mathx: clamp lo > hi")
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Map remaps v from the input range [inStart, inEnd] to the output range
// [outStart, outEnd]. If inStart == inEnd (degenerate), returns outStart.
func Map[T Float](v, inStart, inEnd, outStart, outEnd T) T {
	if inStart == inEnd {
		return outStart
	}
	return outStart + (v-inStart)*(outEnd-outStart)/(inEnd-inStart)
}

// Lerp linearly interpolates between a and b by the parameter t. t=0 → a,
// t=1 → b, t=0.5 → midpoint. Not clamped (t outside [0,1] extrapolates).
func Lerp[T Float](a, b, t T) T {
	return a + (b-a)*t
}
