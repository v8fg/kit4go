package fanout_test

import (
	"context"
	"testing"
	"time"

	"github.com/v8fg/kit4go/fanout"
)

// Regression: PublishBlocking on a full subscriber channel under a
// non-cancellable context, racing Close, must not deadlock. Before the
// done-signal fix, PublishBlocking held the RLock while blocked on the full
// channel, and Close's write Lock waited forever for it.
func TestFanout_PublishBlockingClose_NoDeadlock(t *testing.T) {
	f := fanout.New[int](fanout.WithBufferSize[int](1))
	_ = f.Subscribe()                                 // buffer 1, never drained
	_, _ = f.PublishBlocking(context.Background(), 1) // fills the 1-slot buffer

	publishDone := make(chan struct{})
	go func() {
		_, _ = f.PublishBlocking(context.Background(), 2) // blocks: buffer full, Background ctx
		close(publishDone)
	}()
	time.Sleep(50 * time.Millisecond) // let PublishBlocking park on the full channel

	closeDone := make(chan struct{})
	go func() { f.Close(); close(closeDone) }()
	select {
	case <-closeDone:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked against PublishBlocking on a full subscriber channel")
	}

	select {
	case <-publishDone:
		// PublishBlocking aborted via the done signal and returned
	case <-time.After(2 * time.Second):
		t.Fatal("PublishBlocking did not abort on Close")
	}
}
