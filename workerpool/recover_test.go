package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A panicking job must be recovered — counted in Recovered(), surfaced via the
// onPanic hook, and turned into the job's error — WITHOUT killing the worker.
// The next job on the same worker must still run.
func TestPool_JobPanicRecovered(t *testing.T) {
	p := New[int](1, WithResults[int](4))

	var hookFired atomic.Bool
	p.SetOnPanic(func(any) { hookFired.Store(true) })

	_ = p.Submit(context.Background(), func(context.Context) (int, error) { panic("boom") })
	// The worker must survive and process this normal job.
	_ = p.Submit(context.Background(), func(context.Context) (int, error) { return 42, nil })
	// Let the worker process both jobs before Close fires done — otherwise the
	// result-send select can race with done and drop the results.
	time.Sleep(100 * time.Millisecond)

	p.Close() // blocks until workers finish, then closes Results()

	var success int
	var panicErrs int
	for r := range p.Results() { // drain the now-closed results channel
		if r.Err != nil {
			panicErrs++
		} else {
			success = r.Value
		}
	}
	if panicErrs != 1 {
		t.Fatalf("panic job error count = %d, want 1", panicErrs)
	}
	if success != 42 {
		t.Fatalf("worker died after panic; normal job lost (got %d, want 42)", success)
	}
	if p.Recovered() != 1 {
		t.Fatalf("Recovered() = %d, want 1", p.Recovered())
	}
	if !hookFired.Load() {
		t.Fatal("onPanic hook not fired")
	}
}
