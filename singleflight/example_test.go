package singleflight_test

import (
	"fmt"

	"github.com/v8fg/kit4go/singleflight"
)

// ExampleGroup_Do shows the API and the no-caching property. Each Do runs fn
// unless an identical-key call is already in flight — in which case the caller
// shares that call's result (Shared=true) instead of running fn. Results are not
// cached after completion, so a later Do runs fn again. The concurrent dedup is
// exercised by TestDoDeduplicates (50 racing goroutines, fn runs exactly once).
func ExampleGroup_Do() {
	g := singleflight.New[string, int]()

	r1 := g.Do("k", func() (int, error) { return 1, nil })
	r2 := g.Do("k", func() (int, error) { return 2, nil }) // not cached → runs again

	fmt.Println(r1.Value, r1.Shared)
	fmt.Println(r2.Value, r2.Shared)
	// Output:
	// 1 false
	// 2 false
}
