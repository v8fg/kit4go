package idempotency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Verifies singleflight holds even across TTL expiry: N concurrent Do calls for
// a key whose cached result has just expired must coalesce into ONE fn execution
// (the first becomes the leader, the rest follow), not re-run fn N times. This
// guards the (mutex-protected) expired-reopen path.
func TestCache_SingleflightHoldsAcrossExpiry(t *testing.T) {
	var calls atomic.Int64
	c := New[int](WithTTL[int](20 * time.Millisecond))

	// Prime: first call succeeds and is cached (expires 20ms after completion).
	_, _ = c.Do(context.Background(), "k", func(context.Context) (int, error) {
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		return 1, nil
	})
	time.Sleep(40 * time.Millisecond) // now expired

	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range n {
		wg.Go(func() {
			<-start
			_, _ = c.Do(context.Background(), "k", func(context.Context) (int, error) {
				calls.Add(1)
				time.Sleep(30 * time.Millisecond) // slow leader so followers stack up
				return 1, nil
			})
		})
	}
	close(start)
	wg.Wait()

	if got := calls.Load(); got != 2 {
		t.Fatalf("fn ran %d times; want 2 (1 prime + 1 singleflighted leader after expiry)", got)
	}
}
