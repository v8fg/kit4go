package bitset_test

import (
	"fmt"

	"github.com/v8fg/kit4go/bitset"
)

func ExampleNew() {
	bs := bitset.New(128)
	bs.Set(5)
	bs.Set(10)
	bs.Set(100)
	fmt.Println("has 5:", bs.Test(5))
	fmt.Println("has 6:", bs.Test(6))
	fmt.Println("count:", bs.Len())
	fmt.Println("bits:", bs.ToSlice())
	// Output:
	// has 5: true
	// has 6: false
	// count: 3
	// bits: [5 10 100]
}
