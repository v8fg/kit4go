package interval_test

import (
	"fmt"

	"github.com/v8fg/kit4go/interval"
)

func ExampleMustNew() {
	a := interval.MustNew(0, 10)
	b := interval.MustNew(5, 15)

	fmt.Println("overlap:", a.Overlaps(b))
	u, _ := a.Union(b)
	fmt.Printf("union: [%d, %d)\n", u.Start, u.End)
	in, _ := a.Intersect(b)
	fmt.Printf("intersect: [%d, %d)\n", in.Start, in.End)

	// Output:
	// overlap: true
	// union: [0, 15)
	// intersect: [5, 10)
}

func ExampleMerge() {
	intervals := []interval.Interval[int]{
		interval.MustNew(0, 5),
		interval.MustNew(3, 8),
		interval.MustNew(10, 15),
		interval.MustNew(12, 20),
	}
	merged := interval.Merge(intervals)
	for _, m := range merged {
		fmt.Printf("[%d, %d) ", m.Start, m.End)
	}
	// Output: [0, 8) [10, 20)
}
