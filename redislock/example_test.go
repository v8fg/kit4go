package redislock_test

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/v8fg/kit4go/redislock"
)

// ExampleNew shows the construction shape for a distributed lock and the
// acquire/release flow. The Locker is built over any redis.Cmdable; here a
// single-node go-redis client is wired in directly.
//
// This example has no // Output: comment, so go test does not execute it; it
// is a compile-checked illustration. Running it requires a Redis at
// 127.0.0.1:6379 (TryLock dials on the acquire attempt).
func ExampleNew() {
	rc := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:6379"})

	locker := redislock.New(rc,
		redislock.WithTTL(10*time.Second),
		redislock.WithAutoRenew(true),
	)

	ctx := context.Background()
	lock, err := locker.TryLock(ctx, "budget-update")
	if err != nil {
		return // handle not-acquired / connection error
	}
	defer lock.Release(context.Background())

	// ... critical section: lock is held while this runs ...
	_ = lock
}
