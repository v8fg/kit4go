package limiter_test

import (
	"context"
	"fmt"
	"time"

	"github.com/v8fg/kit4go/limiter"
)

// ExampleNewLimiter shows the factory selecting each algorithm.
func ExampleNewLimiter() {
	// Token bucket: smooth 100 QPS with a 10-token burst.
	tb := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmTokenBucket,
		Rate:      100,
		Burst:     10,
	})
	defer tb.Close()

	// Sliding window: hard ceiling of 1000 requests per second.
	sw := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmSlidingWindow,
		Rate:      1000,
		Window:    time.Second,
	})
	defer sw.Close()

	fmt.Println(tb != nil, sw != nil)
	// Output:
	// true true
}

// ExampleLimiter_Allow shows the non-blocking probe.
func ExampleLimiter_Allow() {
	// Rate=2/s, Burst=2: two calls succeed, the third is denied until refill.
	lm := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmTokenBucket,
		Rate:      2,
		Burst:     2,
	})
	defer lm.Close()

	fmt.Println(lm.Allow()) // true (token 1)
	fmt.Println(lm.Allow()) // true (token 2)
	fmt.Println(lm.Allow()) // false (burst drained)
	// Output:
	// true
	// true
	// false
}

// ExampleLimiter_Wait shows the blocking acquire with a context deadline.
func ExampleLimiter_Wait() {
	// Rate=1000/s, Burst=1: after the first token, Wait blocks ~1ms then
	// succeeds.
	lm := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmTokenBucket,
		Rate:      1000,
		Burst:     1,
	})
	defer lm.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	lm.Allow() // drain
	if err := lm.Wait(ctx); err != nil {
		fmt.Println("wait failed:", err)
		return
	}
	fmt.Println("acquired after wait")
	// Output:
	// acquired after wait
}

// ExampleLimiter_TryAcquire shows the batch acquire.
func ExampleLimiter_TryAcquire() {
	lm := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmTokenBucket,
		Rate:      1,
		Burst:     5,
	})
	defer lm.Close()

	fmt.Println(lm.TryAcquire(3)) // true (3 <= burst 5)
	fmt.Println(lm.TryAcquire(5)) // false (only 2 left)
	fmt.Println(lm.TryAcquire(0)) // true (no-op)
	// Output:
	// true
	// false
	// true
}

// ExampleLimiter_Metrics shows the observability snapshot.
func ExampleLimiter_Metrics() {
	lm := limiter.NewLimiter(limiter.LimiterOptions{
		Algorithm: limiter.AlgorithmTokenBucket,
		Rate:      1,
		Burst:     2,
	})
	defer lm.Close()

	lm.Allow()       // 1 allowed, 1 acquired
	lm.Allow()       // 2 allowed, 2 acquired
	lm.Allow()       // denied (burst drained)
	lm.TryAcquire(0) // no-op

	m := lm.Metrics()
	fmt.Printf("allowed=%d denied=%d acquired=%d\n", m.Allowed, m.Denied, m.Acquired)
	// Output:
	// allowed=2 denied=1 acquired=2
}
