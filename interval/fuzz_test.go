package interval_test

import (
	"testing"

	"github.com/v8fg/kit4go/interval"
)

// FuzzMergeDisjoint encodes the core invariant of Merge: the result must be
// sorted by Start, disjoint (no two intervals overlap), and every element of the
// input is covered by the output. E10 invariant-encoding fuzz target.
func FuzzMergeDisjoint(f *testing.F) {
	f.Add(0, 5, 3, 8, 10, 15)
	f.Add(0, 10, 1, 2, 3, 4)
	f.Add(0, 1, 5, 6, 10, 11)
	f.Fuzz(func(t *testing.T, s1, e1, s2, e2, s3, e3 int) {
		var intervals []interval.Interval[int]
		for _, p := range [][2]int{{s1, e1}, {s2, e2}, {s3, e3}} {
			i, err := interval.New(p[0], p[1])
			if err != nil {
				continue
			}
			intervals = append(intervals, i)
		}
		if len(intervals) == 0 {
			return
		}
		merged := interval.Merge(intervals)
		// Invariant 1: sorted by Start.
		for i := 1; i < len(merged); i++ {
			if merged[i].Start < merged[i-1].Start {
				t.Fatalf("Merge not sorted: %v", merged)
			}
		}
		// Invariant 2: disjoint (no overlap between consecutive).
		for i := 1; i < len(merged); i++ {
			if merged[i].Start < merged[i-1].End {
				t.Fatalf("Merge has overlap: %v", merged)
			}
		}
	})
}
