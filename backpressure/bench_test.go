package backpressure

import (
	"sync/atomic"
	"testing"
)

func BenchmarkTryAcquire_Release(b *testing.B) {
	g := New(1 << 30) // effectively unlimited
	b.ReportAllocs()
	for b.Loop() {
		g.TryAcquire()
		g.Release()
	}
}

func BenchmarkTryAcquire_Contended(b *testing.B) {
	g := New(1000)
	var rejected atomic.Uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !g.TryAcquire() {
				rejected.Add(1)
				continue
			}
			g.Release()
		}
	})
}
