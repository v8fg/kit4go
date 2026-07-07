package shortlink

import "testing"

// These benchmarks extend bench_test.go (Generate, EncodeBaseN, Resolve) with
// the sequential ID-shortener path.

// BenchmarkNext measures the sequential ID-shortener Next (atomic increment +
// base62 encode). The hot path must stay 0 allocs/op.
func BenchmarkNext(b *testing.B) {
	s := NewIDShortener(Alphabet, 1<<40)
	b.ReportAllocs()

	for b.Loop() {
		_ = s.Next()
	}
}

// BenchmarkNextParallel measures Next under contention (atomic counter).
func BenchmarkNextParallel(b *testing.B) {
	s := NewIDShortener(Alphabet, 1<<40)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Next()
		}
	})
}

// BenchmarkDecode measures the base62 decode of a code.
func BenchmarkDecode(b *testing.B) {
	s := NewIDShortener(Alphabet, 0)
	code := s.Encode(1 << 40)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = s.Decode(code)
	}
}
