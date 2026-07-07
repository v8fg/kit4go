package cache

import (
	"context"
	"testing"
	"time"
)

// BenchmarkGet measures a cache hit on the in-memory backend (delegates to
// lru.Get, which promotes the entry).
func BenchmarkGet(b *testing.B) {
	s := NewMemory[string](WithMaxSize[string](1024))
	ctx := context.Background()
	_ = s.Set(ctx, "k", "v", 0)
	b.ReportAllocs()

	for b.Loop() {
		_, _ = s.Get(ctx, "k")
	}
}

// BenchmarkGetMiss measures a cache miss (no entry / ErrMiss path).
func BenchmarkGetMiss(b *testing.B) {
	s := NewMemory[string](WithMaxSize[string](1024))
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = s.Get(ctx, "absent")
	}
}

// BenchmarkSet measures an insert/refresh (lru.Set under the write lock).
func BenchmarkSet(b *testing.B) {
	s := NewMemory[string](WithMaxSize[string](1024))
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		_ = s.Set(ctx, "k", "v", 0)
	}
}

// BenchmarkSetWithTTL measures an insert with a per-entry TTL.
func BenchmarkSetWithTTL(b *testing.B) {
	s := NewMemory[string](WithMaxSize[string](1024))
	ctx := context.Background()
	b.ReportAllocs()

	for b.Loop() {
		_ = s.Set(ctx, "k", "v", time.Minute)
	}
}

// BenchmarkHas measures the Contains check (Peek, no promotion).
func BenchmarkHas(b *testing.B) {
	s := NewMemory[string](WithMaxSize[string](1024))
	ctx := context.Background()
	_ = s.Set(ctx, "k", "v", 0)
	b.ReportAllocs()

	for b.Loop() {
		_ = s.Has(ctx, "k")
	}
}
