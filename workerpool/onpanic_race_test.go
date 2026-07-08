package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestPool_SetOnPanic_Race is the R11-F1 regression test. SetOnPanic writes the
// hook field from the caller goroutine while safeCall's recover path reads it on
// the worker goroutine. Under -race the bare-field version flags a data race;
// the atomic.Pointer storage must be clean.
//
// One goroutine hammers SetOnPanic(fn)/SetOnPanic(nil) toggling; another
// hammers Submit of a panicking job so the recover path reads the hook
// concurrently with the writer. Run under -race.
func TestPool_SetOnPanic_Race(t *testing.T) {
	p := New[int](4, WithQueueSize[int](64))

	hook := func(any) {}
	stop := atomic.Bool{}
	done := make(chan struct{})

	// Writer goroutine: toggle the hook on and off.
	go func() {
		defer close(done)
		for !stop.Load() {
			p.SetOnPanic(hook)
			p.SetOnPanic(nil)
		}
	}()

	// Producer goroutine: flood panicking jobs so the worker recover path reads
	// the hook concurrently with the writer above.
	go func() {
		for !stop.Load() {
			_ = p.TrySubmit(context.Background(), func(context.Context) (int, error) {
				panic("boom")
			})
		}
	}()

	// Let the race window be exercised.
	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	<-done

	// Close must drain; Recovered proves the recover path actually fired (and
	// thus read onPanic) during the race window.
	p.Close()
	if p.Recovered() == 0 {
		t.Fatal("no panics recovered during race test; recover path never ran")
	}
}
