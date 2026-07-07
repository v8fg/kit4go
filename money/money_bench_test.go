package money

import (
	"testing"
)

// BenchmarkAdd measures the hot-path exact arithmetic op (int64 add). Add/Sub/Mul
// on int64 minor units must be 0 allocs/op.
func BenchmarkAdd(b *testing.B) {
	a := MustFromMinor(123456, "USD")
	c := MustFromMinor(654321, "USD")
	b.ReportAllocs()

	for b.Loop() {
		_, _ = a.Add(c)
	}
}

// BenchmarkSub measures the subtraction hot path.
func BenchmarkSub(b *testing.B) {
	a := MustFromMinor(1234567, "USD")
	c := MustFromMinor(123456, "USD")
	b.ReportAllocs()

	for b.Loop() {
		_, _ = a.Sub(c)
	}
}

// BenchmarkMul measures integer scalar multiplication (exact).
func BenchmarkMul(b *testing.B) {
	a := MustFromMinor(12345, "USD")
	b.ReportAllocs()

	for b.Loop() {
		_, _ = a.Mul(7)
	}
}

// BenchmarkScale measures ratio scaling with rounding (tax/discount path).
func BenchmarkScale(b *testing.B) {
	a := MustFromMinor(1234567, "USD")
	b.ReportAllocs()

	for b.Loop() {
		_, _ = a.Scale(1.07, RoundHalfUp)
	}
}

// BenchmarkString measures the major-unit string rendering.
func BenchmarkString(b *testing.B) {
	a := MustFromMinor(-1234567, "USD")
	b.ReportAllocs()

	for b.Loop() {
		_ = a.String()
	}
}

// BenchmarkParse measures amount-string parsing into minor units.
func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse("USD", "-12345.67")
	}
}
