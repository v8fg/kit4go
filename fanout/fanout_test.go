package fanout

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscribeAndPublish(t *testing.T) {
	f := New[int]()
	defer f.Close()
	s1 := f.Subscribe()
	s2 := f.Subscribe()
	require.Equal(t, 2, f.Subscribers())

	delivered := f.Publish(42)
	require.Equal(t, 2, delivered)

	v1 := <-s1.Ch
	v2 := <-s2.Ch
	require.Equal(t, 42, v1)
	require.Equal(t, 42, v2)
}

func TestUnsubscribe(t *testing.T) {
	f := New[string]()
	s1 := f.Subscribe()
	s2 := f.Subscribe()
	f.Unsubscribe(s1)
	require.Equal(t, 1, f.Subscribers())
	f.Publish("hello")
	v := <-s2.Ch
	require.Equal(t, "hello", v)
}

func TestCancelViaSubscription(t *testing.T) {
	f := New[int]()
	s := f.Subscribe()
	s.Cancel()
	require.Equal(t, 0, f.Subscribers())
}

func TestUnsubscribeNil(t *testing.T) {
	f := New[int]()
	require.NotPanics(t, func() { f.Unsubscribe(nil) })
}

func TestDropOnFullChannel(t *testing.T) {
	f := New[int](WithBufferSize[int](1))
	defer f.Close()
	s := f.Subscribe()
	f.Publish(1) // fills buffer
	f.Publish(2) // dropped (buffer full, no reader)
	require.Equal(t, uint64(1), f.Dropped())
	v := <-s.Ch
	require.Equal(t, 1, v)
}

func TestPublishBlocking(t *testing.T) {
	f := New[int]()
	defer f.Close()
	s := f.Subscribe()
	delivered, ok := f.PublishBlocking(context.Background(), 99)
	require.True(t, ok)
	require.Equal(t, 1, delivered)
	v := <-s.Ch
	require.Equal(t, 99, v)
}

func TestPublishBlockingCtxCancel(t *testing.T) {
	f := New[int]()
	defer f.Close()
	// Fill the subscriber's buffer (default 16) so the next PublishBlocking blocks.
	s := f.Subscribe()
	for i := 0; i < 16; i++ {
		f.Publish(i)
	}
	// Now the buffer is full; PublishBlocking will block (no reader).
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, ok := f.PublishBlocking(ctx, 999)
	require.False(t, ok)
	_ = s
}

func TestPublishAfterClose(t *testing.T) {
	f := New[int]()
	f.Close()
	require.Equal(t, 0, f.Publish(1))
}

func TestCloseIdempotent(t *testing.T) {
	f := New[int]()
	s := f.Subscribe()
	f.Close()
	f.Close()
	// Channel should be closed.
	_, ok := <-s.Ch
	require.False(t, ok)
}

func TestPublishedCounter(t *testing.T) {
	f := New[int]()
	defer f.Close()
	s := f.Subscribe()
	f.Publish(1)
	f.Publish(2)
	f.Publish(3)
	require.Equal(t, uint64(3), f.Published())
	for i := 0; i < 3; i++ {
		<-s.Ch
	}
}

func TestCustomBufferSize(t *testing.T) {
	f := New[int](WithBufferSize[int](5))
	defer f.Close()
	s := f.Subscribe()
	// Fill without dropping.
	for i := 0; i < 5; i++ {
		f.Publish(i)
	}
	require.Equal(t, uint64(0), f.Dropped())
	for i := 0; i < 5; i++ {
		v := <-s.Ch
		require.Equal(t, i, v)
	}
}

func TestManySubscribers(t *testing.T) {
	f := New[int]()
	defer f.Close()
	const n = 50
	subs := make([]*Subscription[int], n)
	for i := range subs {
		subs[i] = f.Subscribe()
	}
	require.Equal(t, n, f.Subscribers())
	delivered := f.Publish(7)
	require.Equal(t, n, delivered)
	for _, s := range subs {
		v := <-s.Ch
		require.Equal(t, 7, v)
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	f := New[int](WithBufferSize[int](100))
	defer f.Close()
	var received atomic.Int64
	const subs = 10
	var wg sync.WaitGroup
	for i := 0; i < subs; i++ {
		s := f.Subscribe()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range s.Ch {
				received.Add(1)
			}
		}()
	}
	// Publishers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				f.Publish(j)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	f.Close()
	wg.Wait()
	// Each of 500 messages delivered to up to 10 subscribers = up to 5000.
	// Some may be dropped (buffer full), so just assert we got a lot.
	require.Greater(t, received.Load(), int64(100))
}

// TestPublishBlockingAfterClose covers PublishBlocking's closed-guard branch
// (the `if f.closed.Load() { return 0, false }` early return at the top of
// PublishBlocking). Distinct from TestPublishAfterClose which only exercises
// the non-blocking Publish path, and from the deadlock regression test which
// hits the in-loop `<-f.done` abort rather than the top-of-function guard.
func TestPublishBlockingAfterClose(t *testing.T) {
	f := New[int]()
	f.Close()
	delivered, ok := f.PublishBlocking(context.Background(), 1)
	require.False(t, ok)
	require.Equal(t, 0, delivered)
	// published counter must not advance once closed (guard returns before Add).
	require.Equal(t, uint64(0), f.Published())
}
