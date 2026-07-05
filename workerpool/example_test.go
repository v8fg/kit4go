package workerpool_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/workerpool"
)

// ExampleNew shows a bounded pool running jobs fire-and-forget. This example
// has no // Output: comment (worker scheduling is async), so go test compiles
// but does not execute it.
func ExampleNew() {
	pool := workerpool.New[int](2, workerpool.WithQueueSize[int](8))
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		i := i
		_ = pool.Submit(ctx, func(_ context.Context) (int, error) {
			fmt.Println("processed", i)
			return i, nil
		})
	}

	pool.Close() // graceful shutdown: drains queued jobs and waits for workers
}
