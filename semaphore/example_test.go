package semaphore_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/semaphore"
)

func ExampleNew() {
	s := semaphore.New(2)
	ctx := context.Background()

	// Hold two permits (capacity), then release one back.
	_ = s.Acquire(ctx, 2)
	fmt.Println("available:", s.Available())
	s.Release(1)
	fmt.Println("available:", s.Available())
	// Output:
	// available: 0
	// available: 1
}
