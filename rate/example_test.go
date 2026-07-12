package rate_test

import (
	"context"
	"fmt"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/v8fg/kit4go/rate"
)

// ExampleLimiter demonstrates a distributed GCRA rate limiter backed by Redis.
// Construct a Limiter over a redis client, then Allow a key under a per-second
// limit. The decision (allowed + remaining) is computed atomically by a Lua
// script, so the limiter is safe across many processes sharing one Redis —
// unlike the in-process limiter/gcra, this one survives restarts and is shared
// across instances.
func ExampleLimiter() {
	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	limiter := rate.New(client)

	// 5 requests/second, burst 5.
	limit := rate.PerSecond(5, 5)
	ctx := context.Background()

	r1, _ := limiter.Allow(ctx, "user:42", limit)
	r2, _ := limiter.Allow(ctx, "user:42", limit)
	fmt.Println("allowed:", r1.Allowed, r2.Allowed)
	fmt.Println("remaining:", r1.Remaining, r2.Remaining)

	// Output:
	// allowed: true true
	// remaining: 4 3
}
