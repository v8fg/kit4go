package countmin

import "testing"

// baseline benchmark for the Add / Estimate hot paths. Tracks allocs/op so a
// hashing change that re-introduces per-call allocation is caught.

func BenchmarkAdd(b *testing.B) {
	c := New(2048, 5)
	data := []byte("creative-id-0123456789abcdef")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Add(data, 1)
	}
}

func BenchmarkEstimate(b *testing.B) {
	c := New(2048, 5)
	data := []byte("creative-id-0123456789abcdef")
	for i := 0; i < 1000; i++ {
		c.Add(data, 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Estimate(data)
	}
}
