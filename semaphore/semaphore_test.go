package semaphore

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPanicOnZeroCap(t *testing.T) {
	require.Panics(t, func() { New(0) })
	require.Panics(t, func() { New(-1) })
}

func TestAcquireRelease(t *testing.T) {
	s := New(3)
	require.NoError(t, s.Acquire(context.Background(), 1))
	require.NoError(t, s.Acquire(context.Background(), 1))
	require.Equal(t, int64(1), s.Available())
	s.Release(1)
	require.Equal(t, int64(2), s.Available())
}

func TestTryAcquire(t *testing.T) {
	s := New(2)
	require.True(t, s.TryAcquire(1))
	require.True(t, s.TryAcquire(1))
	require.False(t, s.TryAcquire(1)) // full
	s.Release(1)
	require.True(t, s.TryAcquire(1))
}

func TestWeightedAcquire(t *testing.T) {
	s := New(10)
	require.NoError(t, s.Acquire(context.Background(), 7))
	require.Equal(t, int64(3), s.Available())
	require.False(t, s.TryAcquire(5)) // 3 < 5
	require.True(t, s.TryAcquire(3))  // exactly fits
	require.False(t, s.TryAcquire(1)) // full
	s.Release(7)
	require.Equal(t, int64(7), s.Available())
}

func TestExceedsCapacity(t *testing.T) {
	s := New(5)
	err := s.Acquire(context.Background(), 6)
	require.Error(t, err)
}

func TestAcquireBlocksUntilReleased(t *testing.T) {
	s := New(1)
	require.NoError(t, s.Acquire(context.Background(), 1))

	acquired := make(chan struct{})
	go func() {
		_ = s.Acquire(context.Background(), 1)
		close(acquired)
	}()
	time.Sleep(30 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("should have blocked")
	default:
	}
	s.Release(1)
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("Acquire not unblocked after Release")
	}
}

func TestContextCancel(t *testing.T) {
	s := New(1)
	require.NoError(t, s.Acquire(context.Background(), 1)) // fill

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Acquire(ctx, 1)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Acquire not cancelled")
	}
}

func TestCloseWakesBlocked(t *testing.T) {
	s := New(1)
	require.NoError(t, s.Acquire(context.Background(), 1))

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Acquire(context.Background(), 1)
	}()
	time.Sleep(30 * time.Millisecond)
	s.Close()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("Acquire not woken by Close")
	}
}

func TestCloseIdempotent(t *testing.T) {
	s := New(2)
	s.Close()
	s.Close() // must not panic
}

func TestReleaseUnderflow(t *testing.T) {
	s := New(2)
	require.Panics(t, func() { s.Release(1) }) // nothing acquired
}

func TestCap(t *testing.T) {
	s := New(42)
	require.Equal(t, 42, s.Cap())
}

func TestConcurrencyLimit(t *testing.T) {
	s := New(4)
	var concurrent atomic.Int64
	var maxConcurrent atomic.Int64
	var wg sync.WaitGroup
	const workers = 20
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			require.NoError(t, s.Acquire(context.Background(), 1))
			c := concurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if c <= old || maxConcurrent.CompareAndSwap(old, c) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			concurrent.Add(-1)
			s.Release(1)
		}()
	}
	wg.Wait()
	require.LessOrEqual(t, maxConcurrent.Load(), int64(4), "concurrency exceeded capacity")
	require.Positive(t, maxConcurrent.Load())
}

func TestZeroNDefaultsToOne(t *testing.T) {
	s := New(2)
	require.NoError(t, s.Acquire(context.Background(), 0)) // n=0 → 1
	require.Equal(t, int64(1), s.Available())
	s.Release(0) // n=0 → 1
	require.Equal(t, int64(2), s.Available())
}

// TestWeightedAcquireBlocksUntilReleased exercises the n>1 blocking slow path:
// a weighted acquire must wait until enough permits are returned, atomically.
func TestWeightedAcquireBlocksUntilReleased(t *testing.T) {
	s := New(5)
	require.NoError(t, s.Acquire(context.Background(), 4)) // 1 left
	require.Equal(t, int64(1), s.Available())

	acquired := make(chan struct{})
	go func() {
		_ = s.Acquire(context.Background(), 3) // needs 3, only 1 free → blocks
		close(acquired)
	}()
	time.Sleep(30 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("weighted Acquire should have blocked")
	default:
	}
	require.NoError(t, s.Acquire(context.Background(), 1)) // consumes the last free permit
	s.Release(2)                                           // now 2 free, still < 3
	time.Sleep(30 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("weighted Acquire still short of permits")
	default:
	}
	s.Release(2) // now 4 free ≥ 3 → unblocks
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("weighted Acquire not unblocked once enough permits returned")
	}
	// After the weighted acquire succeeded, exactly 3 permits were consumed.
	require.Equal(t, int64(1), s.Available())
}

func TestTryAcquireExceedsCapacity(t *testing.T) {
	s := New(3)
	require.False(t, s.TryAcquire(4), "TryAcquire above capacity must be false")
}

// TestReleaseAfterCloseIsNoop documents the Close/Release semantic: a deferred
// Release that races with shutdown must not panic (the channel is closed).
func TestReleaseAfterCloseIsNoop(t *testing.T) {
	s := New(2)
	require.NoError(t, s.Acquire(context.Background(), 2))
	s.Close()
	require.NotPanics(t, func() { s.Release(2) }) // would otherwise exceed capacity
}

// TestAcquireUnitAfterClose exercises the n==1 fast-path select against a
// closed semaphore. With the channel empty and s.closed closed, the pre-check
// select races <-s.closed against default; both routes still return ErrClosed
// (the second select also has a closed case). Run many iterations to exercise
// both resolutions under the race detector.
func TestAcquireUnitAfterClose(t *testing.T) {
	for i := 0; i < 200; i++ {
		s := New(1)
		require.NoError(t, s.Acquire(context.Background(), 1)) // drain the only permit
		s.Close()
		err := s.Acquire(context.Background(), 1)
		require.ErrorIs(t, err, ErrClosed, "iteration %d", i)
	}
}

// TestWeightedAcquireClose exercises the weighted (n>1) slow-path Close case:
// a multi-token Acquire blocked waiting for enough permits must return ErrClosed
// when Close is called.
func TestWeightedAcquireClose(t *testing.T) {
	s := New(5)
	require.NoError(t, s.Acquire(context.Background(), 4)) // 1 free, weighted Acquire(3) blocks

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Acquire(context.Background(), 3)
	}()
	time.Sleep(30 * time.Millisecond) // let it park in the weighted select

	select {
	case <-errCh:
		t.Fatal("weighted Acquire should have blocked")
	default:
	}

	s.Close()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, ErrClosed)
	case <-time.After(time.Second):
		t.Fatal("weighted Acquire not woken by Close")
	}
}

// TestWeightedAcquireContextCancel exercises the weighted (n>1) slow-path ctx
// cancellation case: a multi-token Acquire blocked waiting for permits must
// return ctx.Err() when the context is cancelled.
func TestWeightedAcquireContextCancel(t *testing.T) {
	s := New(5)
	require.NoError(t, s.Acquire(context.Background(), 4)) // 1 free, weighted Acquire(3) blocks

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Acquire(ctx, 3)
	}()
	time.Sleep(30 * time.Millisecond) // let it park in the weighted select

	select {
	case <-errCh:
		t.Fatal("weighted Acquire should have blocked")
	default:
	}

	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("weighted Acquire not cancelled")
	}
}

// TestTryAcquireZeroN exercises the n<=0 clamp in TryAcquire: it must behave
// exactly like TryAcquire(1).
func TestTryAcquireZeroN(t *testing.T) {
	s := New(1)
	require.True(t, s.TryAcquire(0))   // n=0 → 1, one permit available
	require.False(t, s.TryAcquire(-1)) // n=-1 → 1, now full
	require.Equal(t, int64(0), s.Available())
}
