package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPanicOnZeroWorkers(t *testing.T) {
	require.Panics(t, func() { New[int](0) })
	require.Panics(t, func() { New[int](-1) })
}

func TestSubmitFireAndForget(t *testing.T) {
	p := New[int](2)
	defer p.Close()

	var ran atomic.Int64
	require.NoError(t, p.Submit(context.Background(), func(ctx context.Context) (int, error) {
		ran.Add(1)
		return 42, nil
	}))
	// Give workers time to process.
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(1), ran.Load())
}

func TestSubmitWithResults(t *testing.T) {
	p := New[string](2, WithResults[string](10))
	defer p.Close()

	p.Submit(context.Background(), func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	p.Submit(context.Background(), func(ctx context.Context) (string, error) {
		return "", errors.New("boom")
	})

	results := []Result[string]{}
	for r := range p.Results() {
		results = append(results, r)
		if len(results) == 2 {
			break
		}
	}
	require.Len(t, results, 2)
}

func TestTrySubmitFull(t *testing.T) {
	p := New[int](1, WithQueueSize[int](1))
	defer p.Close()

	// Fill the queue + worker. Worker is processing 1, queue has 1.
	block := make(chan struct{})
	p.Submit(context.Background(), func(ctx context.Context) (int, error) {
		<-block // block the worker
		return 0, nil
	})
	time.Sleep(20 * time.Millisecond)
	require.True(t, p.TrySubmit(context.Background(), func(ctx context.Context) (int, error) { return 0, nil }))
	require.False(t, p.TrySubmit(context.Background(), func(ctx context.Context) (int, error) { return 0, nil }))
	close(block)
}

func TestSubmitAfterClose(t *testing.T) {
	p := New[int](2)
	p.Close()
	require.ErrorIs(t, p.Submit(context.Background(), func(ctx context.Context) (int, error) { return 0, nil }), ErrClosed)
	require.False(t, p.TrySubmit(context.Background(), func(ctx context.Context) (int, error) { return 0, nil }))
}

func TestCloseDrainsQueue(t *testing.T) {
	p := New[int](1, WithQueueSize[int](5))
	var processed atomic.Int64
	for i := 0; i < 5; i++ {
		require.NoError(t, p.Submit(context.Background(), func(ctx context.Context) (int, error) {
			processed.Add(1)
			return 0, nil
		}))
	}
	p.Close() // blocks until all 5 are processed
	require.Equal(t, int64(5), processed.Load())
}

func TestCloseIdempotent(t *testing.T) {
	p := New[int](2)
	p.Close()
	p.Close() // must not panic
}

func TestConcurrencyLimit(t *testing.T) {
	p := New[int](3, WithQueueSize[int](50))
	defer p.Close()
	var concurrent atomic.Int64
	var maxConcurrent atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Submit(context.Background(), func(ctx context.Context) (int, error) {
				c := concurrent.Add(1)
				for {
					old := maxConcurrent.Load()
					if c <= old || maxConcurrent.CompareAndSwap(old, c) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				concurrent.Add(-1)
				return 0, nil
			})
		}()
	}
	wg.Wait()
	p.Close()
	require.LessOrEqual(t, maxConcurrent.Load(), int64(3))
	require.Positive(t, maxConcurrent.Load())
}

func TestSubmitCtxCancel(t *testing.T) {
	p := New[int](1, WithQueueSize[int](1))
	defer p.Close()
	// Block the worker + fill the queue.
	block := make(chan struct{})
	p.Submit(context.Background(), func(ctx context.Context) (int, error) { <-block; return 0, nil })
	p.Submit(context.Background(), func(ctx context.Context) (int, error) { return 0, nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.Submit(ctx, func(ctx context.Context) (int, error) { return 0, nil })
	require.ErrorIs(t, err, context.Canceled)
	close(block)
}

func TestWorkers(t *testing.T) {
	p := New[int](7)
	defer p.Close()
	require.Equal(t, 7, p.Workers())
}
