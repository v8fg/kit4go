package mathx_test

import (
	"fmt"

	"github.com/v8fg/kit4go/mathx"
)

func ExampleSum() {
	impressions := []int{1200, 3500, 800, 4200}
	total := mathx.Sum(impressions...)
	fmt.Println("total:", total)
	// Output: total: 9700
}

func ExampleClamp() {
	bid := 75.50
	floor, ceiling := 10.0, 50.0
	fmt.Println("clamped:", mathx.Clamp(bid, floor, ceiling))
	// Output: clamped: 50
}
