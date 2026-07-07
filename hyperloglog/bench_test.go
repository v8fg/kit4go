package hyperloglog

import (
	"bytes"
	"testing"
)

// baseline benchmark for the Add hot path. Tracks allocs/op so a hashing change
// that re-introduces per-call allocation is caught.

func BenchmarkAdd(b *testing.B) {
	h, _ := New(14)
	data := []byte("user-id-0123456789abcdef")
	b.ReportAllocs()

	for b.Loop() {
		h.Add(data)
	}
}

func BenchmarkAdd_Parallel(b *testing.B) {
	// Add is NOT synchronized; each goroutine uses its own sketch, then would
	// Merge. This measures the per-goroutine Add cost under contention.
	b.RunParallel(func(pb *testing.PB) {
		h, _ := New(14)
		data := []byte("user-id-0123456789abcdef")
		for pb.Next() {
			h.Add(data)
		}
	})
}

func BenchmarkAddString(b *testing.B) {
	h, _ := New(14)
	s := "user-id-0123456789abcdef"
	b.ReportAllocs()

	for b.Loop() {
		h.AddString(s)
	}
}

func BenchmarkEstimate(b *testing.B) {
	h, _ := New(14)
	for i := range 100_000 {
		var buf bytes.Buffer
		buf.WriteString("id-")
		// avoid fmt in the setup loop cost by direct byte variation
		buf.Write([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		h.Add(buf.Bytes())
	}
	b.ReportAllocs()

	for b.Loop() {
		_ = h.Estimate()
	}
}
