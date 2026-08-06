package workerpool_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/v8fg/kit4go/workerpool"
)

// TestPanicSoak verifies that the library-owned recover contract holds under
// sustained load: submitting 50K jobs (10% panicking) must not leak goroutines.
// After Close, the goroutine count must return to within +2 of baseline.
func TestPanicSoak(t *testing.T) {
	base := runtime.NumGoroutine()
	p := workerpool.New[int](4)
	ctx := context.Background()

	const total = 50000
	done := make(chan struct{}, total)
	for i := range total {
		_ = p.Submit(ctx, func(ctx context.Context) (int, error) {
			if i%10 == 0 {
				panic("soak-panic")
			}
			done <- struct{}{}
			return 0, nil
		})
	}

	// Drain successful completions.
	for range total * 9 / 10 {
		<-done
	}
	p.Close()

	// Give replacement goroutines time to exit.
	for range 10 {
		if runtime.NumGoroutine() <= base+1 {
			break
		}
		runtime.Gosched()
	}
	now := runtime.NumGoroutine()
	if now > base+2 {
		t.Errorf("goroutine leak: baseline=%d now=%d (delta=%d)", base, now, now-base)
	}
	if p.Recovered() == 0 {
		t.Error("expected Recovered() > 0 after panicking jobs")
	}
}
