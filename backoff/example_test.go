package backoff_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/v8fg/kit4go/backoff"
)

func ExampleBackoff_Next() {
	b := backoff.New(
		backoff.WithBase(10*time.Millisecond),
		backoff.WithFactor(2.0),
		backoff.WithMax(time.Hour),
		backoff.WithJitter(backoff.JitterNone),
		backoff.WithMaxAttempts(4),
	)
	for {
		d, ok := b.Next()
		if !ok {
			break
		}
		fmt.Println(d)
	}
	// Output:
	// 10ms
	// 20ms
	// 40ms
	// 80ms
}

// ExampleBackoff_Wait shows the canonical retry loop: call a fallible operation,
// and on failure Wait for the next backoff delay before trying again. Wait
// returns nil to signal "retry now" and ErrMaxAttempts to signal "give up".
// Here every attempt fails, so the loop exhausts the cap and surfaces the
// sentinel error. With a zero base and JitterNone the delays are 0, making the
// example deterministic.
func ExampleBackoff_Wait() {
	b := backoff.New(
		backoff.WithBase(0),
		backoff.WithJitter(backoff.JitterNone),
		backoff.WithMaxAttempts(3),
	)
	ctx := context.Background()

	var lastErr error
	for {
		// Simulate an operation that always fails.
		err := errors.New("upstream unavailable")
		if err == nil {
			break // success
		}
		lastErr = err

		if waitErr := b.Wait(ctx); waitErr != nil {
			// Cap reached (or context cancelled): stop retrying.
			fmt.Println(waitErr)
			break
		}
	}
	fmt.Println("attempts:", b.Attempt())
	fmt.Println("last error:", lastErr)
	// Output:
	// backoff: max attempts reached
	// attempts: 3
	// last error: upstream unavailable
}
