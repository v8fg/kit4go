package slidingwindow_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/slidingwindow"
)

func ExampleNew() {
	w := slidingwindow.New(3)
	w.Push(10)
	w.Push(20)
	w.Push(30)
	w.Push(40) // evicts 10
	fmt.Printf("sum=%.0f avg=%.1f\n", w.Sum(), w.Avg())
	// Output: sum=90 avg=30.0
}

func ExampleNewTimeWindow() {
	tw := slidingwindow.NewTimeWindow(5*time.Second, 100)
	base := time.Unix(1000, 0)
	tw.Push(100, base)
	tw.Push(200, base.Add(2*time.Second))
	tw.Push(300, base.Add(4*time.Second)) // all within 5s of the latest push
	fmt.Printf("count=%d sum=%.0f\n", tw.Count(), tw.Sum())
	// Output: count=3 sum=600
}
