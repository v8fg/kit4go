package backpressure_test

import (
	"fmt"

	"github.com/v8fg/kit4go/backpressure"
)

func ExampleGate() {
	gate := backpressure.New(2)
	fmt.Println(gate.TryAcquire())
	fmt.Println(gate.TryAcquire())
	fmt.Println(gate.TryAcquire())
	gate.Release()
	fmt.Println(gate.TryAcquire())
	// Output:
	// true
	// true
	// false
	// true
}
