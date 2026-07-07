package countmin

import "testing"

// baseline benchmark for the Add / Estimate hot paths. Tracks allocs/op so a
// hashing change that re-introduces per-call allocation is caught.

func BenchmarkAdd(b *testing.B) {
	c := New(2048, 5)
	data := []byte("creative-id-0123456789abcdef")
	b.ReportAllocs()

	for b.Loop() {
		c.Add(data, 1)
	}
}

func BenchmarkEstimate(b *testing.B) {
	c := New(2048, 5)
	data := []byte("creative-id-0123456789abcdef")
	for range 1000 {
		c.Add(data, 1)
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = c.Estimate(data)
	}
}
