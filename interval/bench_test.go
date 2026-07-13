package interval_test

import (
	"testing"

	"github.com/v8fg/kit4go/interval"
)

func BenchmarkContains(b *testing.B) {
	i := interval.MustNew(0, 1_000_000)
	b.ResetTimer()
	for b.Loop() {
		i.Contains(500_000)
	}
}

func BenchmarkOverlaps(b *testing.B) {
	a := interval.MustNew(0, 1000)
	c := interval.MustNew(500, 1500)
	b.ResetTimer()
	for b.Loop() {
		a.Overlaps(c)
	}
}

func BenchmarkMerge(b *testing.B) {
	intervals := make([]interval.Interval[int], 100)
	for i := range 100 {
		intervals[i] = interval.MustNew(i*10, i*10+8)
	}
	b.ResetTimer()
	for b.Loop() {
		interval.Merge(intervals)
	}
}
