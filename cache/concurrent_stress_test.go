package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/cache"
)

// TestConcurrentStress exercises the cache wrapper under real contention —
// N goroutines doing mixed Get/Set/Delete against a small bounded cache with
// TTL. Runs under -race to surface any memStore/TTL interaction race.
func TestConcurrentStress(t *testing.T) {
	s := cache.NewMemory[int](cache.WithMaxSize[int](100), cache.WithDefaultTTL[int](50*time.Millisecond))
	ctx := context.Background()
	const goroutines = 50
	const opsPerG = 500
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerG {
				key := fmt.Sprintf("k%d", (id*7+i)%200)
				switch i % 3 {
				case 0:
					_ = s.Set(ctx, key, i, 0)
				case 1:
					_, _ = s.Get(ctx, key)
				case 2:
					_ = s.Delete(ctx, key)
				}
			}
		}(g)
	}
	wg.Wait()
}
