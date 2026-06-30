package ringbuffer

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPushPopRoundTrip(t *testing.T) {
	rb := New[int](3)
	require.NoError(t, rb.Push(1))
	require.NoError(t, rb.Push(2))
	require.NoError(t, rb.Push(3))
	require.Equal(t, 3, rb.Len())
	require.True(t, rb.IsFull())

	v, err := rb.Pop()
	require.NoError(t, err)
	require.Equal(t, 1, v) // FIFO
	v, err = rb.Pop()
	require.NoError(t, err)
	require.Equal(t, 2, v)
	require.Equal(t, 1, rb.Len())
}

func TestTryPushFull(t *testing.T) {
	rb := New[int](2)
	require.True(t, rb.TryPush(1))
	require.True(t, rb.TryPush(2))
	require.False(t, rb.TryPush(3)) // full
}

func TestTryPopEmpty(t *testing.T) {
	rb := New[int](2)
	_, ok := rb.TryPop()
	require.False(t, ok)
}

func TestFIFOOrder(t *testing.T) {
	rb := New[string](5)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		require.NoError(t, rb.Push(s))
	}
	for _, expected := range []string{"a", "b", "c", "d", "e"} {
		v, _ := rb.Pop()
		require.Equal(t, expected, v)
	}
}

func TestWrapAround(t *testing.T) {
	rb := New[int](2)
	rb.Push(1)
	rb.Pop()
	rb.Push(2)
	rb.Push(3) // wraps to position 0
	v, _ := rb.Pop()
	require.Equal(t, 2, v)
	v, _ = rb.Pop()
	require.Equal(t, 3, v)
}

func TestDrain(t *testing.T) {
	rb := New[int](5)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	out := rb.Drain()
	require.Equal(t, []int{1, 2, 3}, out)
	require.True(t, rb.IsEmpty())
	require.Equal(t, 0, rb.Len())
	// Buffer still usable after drain.
	rb.Push(4)
	v, _ := rb.Pop()
	require.Equal(t, 4, v)
}

func TestDrainEmpty(t *testing.T) {
	rb := New[int](3)
	out := rb.Drain()
	require.Empty(t, out)
}

func TestClosePush(t *testing.T) {
	rb := New[int](2)
	rb.Close()
	err := rb.Push(1)
	require.ErrorIs(t, err, ErrClosed)
	require.False(t, rb.TryPush(1))
}

func TestClosePopRemaining(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Close()
	// Remaining items are still poppable.
	v, err := rb.Pop()
	require.NoError(t, err)
	require.Equal(t, 1, v)
	v, err = rb.Pop()
	require.NoError(t, err)
	require.Equal(t, 2, v)
	// Now empty + closed -> error.
	_, err = rb.Pop()
	require.ErrorIs(t, err, ErrClosed)
}

func TestBlockingPopWaits(t *testing.T) {
	rb := New[int](2)
	done := make(chan struct{})
	go func() {
		v, _ := rb.Pop()
		require.Equal(t, 42, v)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond) // let the Pop block
	require.NoError(t, rb.Push(42))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Pop did not unblock")
	}
}

func TestBlockingPushWaits(t *testing.T) {
	rb := New[int](1)
	rb.Push(1) // fill
	done := make(chan struct{})
	go func() {
		require.NoError(t, rb.Push(2)) // blocks until Pop makes room
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	v, _ := rb.Pop()
	require.Equal(t, 1, v)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Push did not unblock")
	}
}

func TestCloseWakesBlocked(t *testing.T) {
	rb := New[int](1)
	rb.Push(1) // fill
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- rb.Push(2) // blocks (full)
	}()
	<-started
	time.Sleep(100 * time.Millisecond) // ensure Push is blocked on Wait()
	rb.Close()
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrClosed)
	case <-time.After(2 * time.Second):
		t.Fatal("Push not woken by Close")
	}
}

func TestCapAndLen(t *testing.T) {
	rb := New[int](4)
	require.Equal(t, 4, rb.Cap())
	require.Equal(t, 0, rb.Len())
	rb.Push(1)
	rb.Push(2)
	require.Equal(t, 2, rb.Len())
}

func TestMinCapacity(t *testing.T) {
	rb := New[int](0)
	require.Equal(t, 1, rb.Cap())
}

func TestConcurrency(t *testing.T) {
	// Producer-consumer: 4 producers push 100 items total into a bounded buffer;
	// 1 consumer drains concurrently. Close after producers finish.
	rb := New[int](50)
	const total = 100

	var pwg sync.WaitGroup
	pwg.Add(4)
	for i := 0; i < 4; i++ {
		go func() {
			defer pwg.Done()
			for j := 0; j < total/4; j++ {
				_ = rb.Push(j)
			}
		}()
	}

	// Consumer drains concurrently.
	var mu sync.Mutex
	count := 0
	var cwg sync.WaitGroup
	cwg.Add(1)
	go func() {
		defer cwg.Done()
		for {
			_, err := rb.Pop()
			if errors.Is(err, ErrClosed) {
				return
			}
			if err == nil {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}
	}()

	pwg.Wait()
	rb.Close()
	cwg.Wait()
	require.Equal(t, total, count)
}
