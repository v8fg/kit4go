package reservoir_test

import (
	"fmt"

	"github.com/v8fg/kit4go/reservoir"
)

func ExampleSample() {
	s := reservoir.New[int](10)
	for i := 1; i <= 5; i++ {
		s.Offer(i) // offered 5 < cap 10 → all retained (fill phase)
	}
	fmt.Println(s.Count(), s.Cap())
	// Output: 5 10
}
