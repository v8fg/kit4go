package workerpool

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Regression: concurrent Submit racing Close must never panic with
// "send on closed channel". Before the done-channel fix, Close closed the queue
// with no lock, so a Submit mid-send crashed the process.
func TestPool_ConcurrentSubmitClose_NoPanic(t *testing.T) {
	p := New[int](4, WithQueueSize[int](4))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				_ = p.Submit(context.Background(), func(context.Context) (int, error) { return j, nil })
			}
		}()
	}
	p.Close() // racing the 16 submitters
	wg.Wait()
}

// Regression: Close with an undrained results channel must not deadlock. Before
// the fix, workers blocked on `results <-` and Close's wg.Wait never returned.
func TestPool_CloseWithUndrainedResults_NoDeadlock(t *testing.T) {
	p := New[int](2, WithResults[int](1)) // results buffer 1, deliberately not drained
	for i := 0; i < 50; i++ {
		_ = p.TrySubmit(context.Background(), func(context.Context) (int, error) { return i, nil })
	}
	done := make(chan struct{})
	go func() { p.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked: workers blocked on full, undrained results channel")
	}
}
