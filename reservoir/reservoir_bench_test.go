package reservoir

import (
	"testing"
)

// BenchmarkOfferSteady measures Offer once the reservoir is full — the
// steady-state Algorithm R path (one rng draw + maybe one slot write).
func BenchmarkOfferSteady(b *testing.B) {
	s := NewWithOpts[int](128, WithSeed[int](1, 2))
	for i := 0; i < 128; i++ {
		s.Offer(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Offer(i)
	}
}

// BenchmarkOfferFill measures Offer during the initial fill (append path).
func BenchmarkOfferFill(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		s := NewWithOpts[int](128, WithSeed[int](1, 2))
		for i := 0; i < 128; i++ {
			s.Offer(i)
		}
	}
}

// BenchmarkSample measures the snapshot copy (allocates a new slice of k items).
func BenchmarkSample(b *testing.B) {
	s := NewWithOpts[int](128, WithSeed[int](1, 2))
	for i := 0; i < 128; i++ {
		s.Offer(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Sample()
	}
}

// BenchmarkCount measures the counter accessor (acquires the mutex).
func BenchmarkCount(b *testing.B) {
	s := NewWithOpts[int](128, WithSeed[int](1, 2))
	for i := 0; i < 128; i++ {
		s.Offer(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Count()
	}
}
