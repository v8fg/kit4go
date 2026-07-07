package idempotency_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/v8fg/kit4go/idempotency"
)

// ExampleNew shows generic instantiation of the cache. The type parameter is
// the value type; idempotency.New[string]() builds a Cache[string] with the
// default 1m TTL and 4096-entry cap.
func ExampleNew() {
	c := idempotency.New[string]()
	_ = c
	fmt.Println("cache ready")
	// Output:
	// cache ready
}

// ExampleCache_Do demonstrates singleflight coalescing: N concurrent callers
// for the same key run fn exactly once, and every caller observes the leader's
// result. Here an atomic counter tracks fn invocations across 10 callers.
func ExampleCache_Do() {
	c := idempotency.New[string]()
	ctx := context.Background()

	var calls atomic.Int64
	fn := func(context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond) // slow leader so followers stack up
		return "order-42", nil
	}

	const n = 10
	results := make([]string, n)
	done := make(chan struct{})
	for i := range n {
		i := i
		go func() {
			defer func() { done <- struct{}{} }()
			v, _ := c.Do(ctx, "charge:abc", fn)
			results[i] = v
		}()
	}
	for range n {
		<-done
	}

	allSame := true
	for _, r := range results {
		if r != results[0] {
			allSame = false
		}
	}
	fmt.Printf("calls=%d allSame=%v result=%s\n", calls.Load(), allSame, results[0])
	// Output:
	// calls=1 allSame=true result=order-42
}

// ExampleCache_Do_cachedHit shows that a second Do for a key whose result is
// still within the TTL returns the cached value WITHOUT re-running fn.
func ExampleCache_Do_cachedHit() {
	c := idempotency.New[string](idempotency.WithTTL[string](time.Minute))
	ctx := context.Background()

	var calls atomic.Int64
	fn := func(context.Context) (string, error) {
		calls.Add(1)
		return "v1", nil
	}

	v1, _ := c.Do(ctx, "k", fn)
	v2, _ := c.Do(ctx, "k", fn) // served from cache, fn not re-run
	fmt.Printf("v1=%s v2=%s calls=%d\n", v1, v2, calls.Load())
	// Output:
	// v1=v1 v2=v1 calls=1
}

// ExampleWithTTL configures a short cache window. After the TTL elapses the
// next Do re-runs fn (the leader refreshes the cached value).
func ExampleWithTTL() {
	c := idempotency.New[string](idempotency.WithTTL[string](20 * time.Millisecond))
	ctx := context.Background()

	var calls atomic.Int64
	fn := func(context.Context) (string, error) {
		n := calls.Add(1)
		return fmt.Sprintf("run-%d", n), nil
	}

	first, _ := c.Do(ctx, "k", fn)
	time.Sleep(30 * time.Millisecond) // now expired
	second, _ := c.Do(ctx, "k", fn)   // fn re-runs as the new leader
	fmt.Printf("first=%s second=%s calls=%d\n", first, second, calls.Load())
	// Output:
	// first=run-1 second=run-2 calls=2
}
