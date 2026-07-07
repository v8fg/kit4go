package loadbalance

import (
	"fmt"
	"testing"
)

// benchEntries is a realistic upstream set: one heavy node (5x), two mid (1x),
// mirroring the SWRR canonical example. Sized so the per-Next cost is
// non-trivial but not dominated by a huge entry slice.
func benchEntries() []Entry[string] {
	return []Entry[string]{
		{Value: "a:8080", Weight: 5},
		{Value: "b:8080", Weight: 1},
		{Value: "c:8080", Weight: 1},
	}
}

// BenchmarkNew measures construction cost (id closure build + entry alloc +
// totalWeight accumulation) for the default SWRR strategy.
func BenchmarkNew(b *testing.B) {
	entries := benchEntries()
	b.ReportAllocs()

	for b.Loop() {
		New(strID, entries)
	}
}

// BenchmarkNextSWRR is the headline hot path: the default nginx smooth weighted
// round-robin pick. This is what every ad-tech request hits.
func BenchmarkNextSWRR(b *testing.B) {
	bal := New(strID, benchEntries())
	b.ReportAllocs()

	for b.Loop() {
		_, _ = bal.Next()
	}
}

func BenchmarkNextRoundRobin(b *testing.B) {
	bal := New(strID, benchEntries(), WithStrategy[string](StrategyRoundRobin))
	b.ReportAllocs()

	for b.Loop() {
		_, _ = bal.Next()
	}
}

func BenchmarkNextRandom(b *testing.B) {
	bal := New(strID, benchEntries(), WithStrategy[string](StrategyRandom))
	b.ReportAllocs()

	for b.Loop() {
		_, _ = bal.Next()
	}
}

func BenchmarkNextWeightedRandom(b *testing.B) {
	bal := New(strID, benchEntries(), WithStrategy[string](StrategyWeightedRandom))
	b.ReportAllocs()

	for b.Loop() {
		_, _ = bal.Next()
	}
}

// BenchmarkAdd covers the append path (no replacement), the common case for
// growing an upstream set. We measure a single Add per freshly-built balancer;
// the build cost is excluded via a pre-built pool consumed one-per-op (the
// standard idiom for destructive ops) rather than StopTimer/StartTimer churn.
func BenchmarkAdd(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		bal := New[string](strID, nil)
		bal.Add(Entry[string]{Value: "h:8080", Weight: 1})
	}
}

// BenchmarkRemove covers Remove on a present entry (the steady-state remove
// path, not the no-op miss). Each op removes from a freshly built balancer; the
// reported cost is dominated by New (the realistic per-remove lifecycle cost in
// a mutable set). For the isolated Remove-without-rebuild cost see
// BenchmarkRemoveHot below.
func BenchmarkRemove(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		bal := New(strID, benchEntries())
		bal.Remove("b:8080")
	}
}

// BenchmarkRemoveHot isolates the Remove hot path on a long-lived balancer by
// rebuilding a fresh one every 64 removals, amortizing the build cost.
func BenchmarkRemoveHot(b *testing.B) {
	const rebuildEvery = 64
	b.ReportAllocs()
	bal := New(strID, make([]Entry[string], 0, rebuildEvery+8))
	for i := range rebuildEvery {
		bal.Add(Entry[string]{Value: fmt.Sprintf("h-%d", i), Weight: 1})
	}

	for b.Loop() {
		if bal.Len() == 0 {
			b.StopTimer()
			bal = New(strID, make([]Entry[string], 0, rebuildEvery+8))
			for j := range rebuildEvery {
				bal.Add(Entry[string]{Value: fmt.Sprintf("h-%d", j), Weight: 1})
			}
			b.StartTimer()
		}
		bal.Remove("hot")
	}
}
