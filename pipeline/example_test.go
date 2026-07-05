package pipeline_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/pipeline"
)

// ExampleNew builds a two-stage pipeline that doubles even numbers and emits
// them on the output channel. This example has no // Output: comment (worker
// scheduling is async), so go test compiles but does not execute it.
func ExampleNew() {
	// Stage: keep and double even inputs, drop odd ones.
	doubleEvens := pipeline.New[int, int](2, func(_ context.Context, n int) (int, bool, error) {
		if n%2 != 0 {
			return 0, false, nil // drop
		}
		return n * 2, true, nil
	})
	defer doubleEvens.Close()

	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		_ = doubleEvens.Send(ctx, i)
	}

	// Consume the results after sending: read until Out() is closed by Close.
	go func() {
		for out := range doubleEvens.Out() {
			fmt.Println(out)
		}
	}()
}
