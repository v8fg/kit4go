package cache_test

import (
	"context"
	"fmt"

	"github.com/v8fg/kit4go/cache"
)

func ExampleNewMemory() {
	store := cache.NewMemory[string]()
	ctx := context.Background()

	_ = store.Set(ctx, "session:1", "alice", 0)
	val, _ := store.Get(ctx, "session:1")
	fmt.Println(val)

	_, err := store.Get(ctx, "missing")
	fmt.Println(err)
	// Output:
	// alice
	// cache: key not found
}
