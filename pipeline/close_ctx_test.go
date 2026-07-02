package pipeline

import (
	"context"
	"testing"
	"time"
)

// Regression: Close must not deadlock when the consumer does not drain Out().
// Before the done-select fix, workers blocked on `p.out <- out` (full output,
// no reader) and Close's wg.Wait never returned.
func TestPipeline_CloseWithoutDrainingOut_NoDeadlock(t *testing.T) {
	p := New[int, int](1, func(ctx context.Context, n int) (int, bool, error) {
		return n * 2, true, nil
	})
	// Fill the output buffer and wedge workers on the out-send (no consumer).
	for i := 0; i < 5; i++ {
		go func(i int) { _ = p.Send(context.Background(), i) }(i)
	}
	time.Sleep(50 * time.Millisecond) // let workers produce + block on full out

	done := make(chan struct{})
	go func() { p.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked: out channel full and consumer not draining")
	}
}

// Regression: the stage must receive the submitter's per-call context, not a
// construction-time context.Background(). Before the fix the submitter ctx was
// dropped and the stage always saw Background().
func TestPipeline_StageReceivesSubmitterContext(t *testing.T) {
	var sawCtx context.Context
	p := New[int, int](1, func(ctx context.Context, n int) (int, bool, error) {
		sawCtx = ctx
		return n, true, nil
	})
	type k int
	ctx := context.WithValue(context.Background(), k(1), "marker") // a distinctive submitter ctx
	if err := p.Send(ctx, 1); err != nil {
		t.Fatalf("Send: %v", err)
	}
	p.Close() // wg.Wait ensures the worker wrote sawCtx before we read it

	if sawCtx == nil {
		t.Fatal("stage never ran")
	}
	if sawCtx != ctx {
		t.Fatal("stage received a context that is not the submitter's (was Background?)")
	}
	if sawCtx.Value(k(1)) != "marker" {
		t.Fatal("stage ctx lost the submitter's value")
	}
}
