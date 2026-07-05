package redis_test

import (
	"context"

	"github.com/v8fg/kit4go/redis"
)

// ExampleNew shows the construction shape for a single-node client and the
// basic Ping/Close surface. The wrapper routes the full go-redis command
// surface through Client.Cmdable().
//
// This example has no // Output: comment, so go test does not execute it; it
// is a compile-checked illustration. Running it requires a Redis at
// 127.0.0.1:6379.
func ExampleNew() {
	c, err := redis.New(
		redis.WithAddrs("127.0.0.1:6379"),
		redis.WithMode(redis.ModeSingle),
	)
	if err != nil {
		return // handle config error
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		return // handle connection error
	}

	_ = c.Cmdable().Set(ctx, "greeting", "hello", 0).Err()
}
