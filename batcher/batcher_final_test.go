package batcher

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFlush_FirstSelectDoneBranch deterministically hits Flush's first-select
// `case <-b.done` branch (batcher.go:108-109). After Close the collector has
// exited and will never drain flushCh again; pre-filling flushCh to its
// capacity of 1 THEN makes Flush's `b.flushCh <- ack` send unwinnable, so the
// ready `<-b.done` case is selected and Flush returns via the done branch.
//
// Ordering matters: the pre-fill must happen AFTER Close so the collector
// (still alive before Close) cannot consume the placeholder and empty the
// slot, which would let the real Flush's send succeed and take the wrong
// branch.
func TestFlush_FirstSelectDoneBranch(t *testing.T) {
	b := New[int](10, 0, func([]int) {}, WithBufferSize[int](1))
	require.NoError(t, b.Close())
	// Now the collector is gone. Pre-fill flushCh (cap 1) so Flush's send
	// cannot succeed.
	b.flushCh <- make(chan struct{})

	done := make(chan struct{})
	go func() {
		b.Flush() // send blocked (flushCh full), done closed -> done branch
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush blocked: first-select done branch did not fire")
	}
}

// TestClose_FinalDrainMaxSizeFlushBranch hits Close's final-drain
// `len(buf) >= b.maxSize` flush branch (batcher.go:134-137).
//
// This branch is only reachable through an inherent race: an Add must land an
// item in b.in AFTER the collector's own <-done drain has emptied the channel
// and exited, but BEFORE Close acquires the write lock (the item then reaches
// Close's sweep). The window is real — Add holds the read lock during its
// buffered send, and Close's write-lock acquisition must wait for it — but
// scheduler-dependent, so there is no deterministic construction without
// modifying production code.
//
// Configuration tuned to make the race near-certain: maxSize=1 (any single
// straggler item trips the threshold) and buffer cap=1 (the producer's send
// blocks on nearly every iteration, so it is almost always holding the read
// lock mid-send when Close runs). The producer pumps for the ENTIRE duration
// of Close — it stops only once Add returns false (after Close closes done) —
// so an in-flight send is always racing Close's drain. Across many iterations
// the branch is hit hundreds of times per run, including under -race. The test
// PASSES regardless of whether the branch fires (it asserts only no deadlock
// / no error); coverage instrumentation records the hit.
func TestClose_FinalDrainMaxSizeFlushBranch(t *testing.T) {
	const iterations = 3000
	for range iterations {
		var flushed atomic.Int64
		// maxSize=1 so any single item Close drains trips the threshold.
		// buffer cap=1 so the producer's send blocks almost every iteration,
		// maximizing the time it spends holding the read lock mid-send (which
		// is what lets its item land in b.in during Close's straggler window).
		b := New[int](1, 0, func(batch []int) {
			flushed.Add(int64(len(batch)))
		}, WithBufferSize[int](1))

		var pumpWG sync.WaitGroup
		pumpWG.Go(func() {
			// Pump for the whole duration of Close: stop only when Add returns
			// false (done closed by Close), so an in-flight send is always
			// racing Close's final drain.
			for b.Add(0) {
			}
		})

		// Let the producer start and block on its first send (buffer cap=1),
		// holding the read lock; then Close races it.
		time.Sleep(5 * time.Microsecond)
		require.NoError(t, b.Close())
		pumpWG.Wait()

		// Sanity only: Close must complete cleanly. The producer stops when
		// Add returns false post-Close, so the accepted count is unknowable;
		// we do not assert a specific flush count.
		_ = flushed.Load()
	}
}
