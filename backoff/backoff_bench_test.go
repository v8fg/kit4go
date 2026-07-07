package backoff

import (
	"context"
	"testing"
)

// jitterName maps a Jitter mode to a stable benchmark subtest name.
func jitterName(j Jitter) string {
	switch j {
	case JitterNone:
		return "None"
	case JitterFull:
		return "Full"
	case JitterEqual:
		return "Equal"
	case JitterDecorrelated:
		return "Decorrelated"
	default:
		return "Unknown"
	}
}

// BenchmarkNext measures the next-delay computation under each jitter mode. The
// hot path holds the mutex and may draw from math/rand.
func BenchmarkNext(b *testing.B) {
	modes := []Jitter{JitterNone, JitterFull, JitterEqual, JitterDecorrelated}
	for _, j := range modes {
		b.Run(jitterName(j), func(b *testing.B) {
			bf := New(WithJitter(j), WithMaxAttempts(0))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, ok := bf.Next()
				if !ok {
					bf.Reset()
				}
			}
		})
	}
}

// BenchmarkNextParallel measures Next under contention (shared mutex + RNG).
func BenchmarkNextParallel(b *testing.B) {
	bf := New(WithJitter(JitterFull), WithMaxAttempts(0))
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, ok := bf.Next()
			if !ok {
				bf.Reset()
			}
		}
	})
}

// BenchmarkReset measures the sequence reset.
func BenchmarkReset(b *testing.B) {
	bf := New(WithMaxAttempts(0))
	bf.Next()
	b.ReportAllocs()

	for b.Loop() {
		bf.Reset()
	}
}

// BenchmarkWaitZero measures Wait with a zero delay (no actual sleep) — the
// timer-allocation path. Wait always builds a time.Timer even at d=0.
func BenchmarkWaitZero(b *testing.B) {
	bf := New(WithBase(0), WithMax(0), WithJitter(JitterNone), WithMaxAttempts(0))
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		_ = bf.Wait(ctx)
	}
}
