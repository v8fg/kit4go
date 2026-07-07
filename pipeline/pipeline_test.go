package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(0, func(ctx context.Context, i int) (int, bool, error) { return i, true, nil }) })
	require.Panics(t, func() { New(1, Stage[int, int](nil)) })
}

func TestTransformAndCollect(t *testing.T) {
	p := New(2, func(ctx context.Context, n int) (int, bool, error) {
		return n * 2, true, nil
	}, WithOutputBuffer[int, int](10))
	for i := 1; i <= 5; i++ {
		require.NoError(t, p.Send(context.Background(), i))
	}
	p.Close()
	var results []int
	for r := range p.Out() {
		results = append(results, r)
	}
	require.ElementsMatch(t, []int{2, 4, 6, 8, 10}, results)
}

func TestFilterDropsItems(t *testing.T) {
	p := New(2, func(ctx context.Context, n int) (int, bool, error) {
		if n%2 == 0 {
			return 0, false, nil
		}
		return n, true, nil
	}, WithOutputBuffer[int, int](10))
	for i := 1; i <= 6; i++ {
		p.Send(context.Background(), i)
	}
	p.Close()
	var results []int
	for r := range p.Out() {
		results = append(results, r)
	}
	require.ElementsMatch(t, []int{1, 3, 5}, results)
}

func TestErrorDropsItem(t *testing.T) {
	p := New(1, func(ctx context.Context, n int) (int, bool, error) {
		if n == 3 {
			return 0, false, errors.New("bad")
		}
		return n, true, nil
	}, WithOutputBuffer[int, int](10))
	for i := 1; i <= 5; i++ {
		p.Send(context.Background(), i)
	}
	p.Close()
	var results []int
	for r := range p.Out() {
		results = append(results, r)
	}
	require.ElementsMatch(t, []int{1, 2, 4, 5}, results)
}

func TestTypeTransform(t *testing.T) {
	p := New(1, func(ctx context.Context, n int) (string, bool, error) {
		return "v" + itoa(n), true, nil
	}, WithOutputBuffer[int, string](10))
	p.Send(context.Background(), 42)
	p.Close()
	r := <-p.Out()
	require.Equal(t, "v42", r)
}

func TestSendAfterClose(t *testing.T) {
	p := New(1, func(ctx context.Context, n int) (int, bool, error) { return n, true, nil })
	p.Close()
	// Send after Close may succeed (item goes to the never-closed input buffer)
	// or return ErrClosed (done is closed). Both are acceptable; the item is
	// never processed since workers have exited. Assert no panic.
	_ = p.Send(context.Background(), 1)
}

func TestCloseIdempotent(t *testing.T) {
	p := New(1, func(ctx context.Context, n int) (int, bool, error) { return n, true, nil })
	p.Close()
	p.Close()
}

func TestBackpressure(t *testing.T) {
	p := New(1, func(ctx context.Context, n int) (int, bool, error) {
		time.Sleep(50 * time.Millisecond)
		return n, true, nil
	}, WithInputBuffer[int, int](1), WithOutputBuffer[int, int](10))
	defer p.Close()
	require.NoError(t, p.Send(context.Background(), 1))
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, p.Send(context.Background(), 2))
	done := make(chan error, 1)
	go func() { done <- p.Send(context.Background(), 3) }()
	select {
	case <-done:
		t.Fatal("Send should have blocked")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var concurrent atomic.Int64
	var maxConcurrent atomic.Int64
	p := New(3, func(ctx context.Context, n int) (int, bool, error) {
		c := concurrent.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		concurrent.Add(-1)
		return n, true, nil
	}, WithInputBuffer[int, int](20), WithOutputBuffer[int, int](20))
	for i := range 20 {
		p.Send(context.Background(), i)
	}
	p.Close()
	for range p.Out() {
	}
	require.LessOrEqual(t, maxConcurrent.Load(), int64(3))
	require.Positive(t, maxConcurrent.Load())
}

func TestChainPipelines(t *testing.T) {
	p1 := New(2, func(ctx context.Context, n int) (int, bool, error) {
		return n + 1, true, nil
	}, WithOutputBuffer[int, int](10))
	p2 := New(2, func(ctx context.Context, n int) (string, bool, error) {
		return "x" + itoa(n), n > 2, nil
	}, WithInputBuffer[int, string](10), WithOutputBuffer[int, string](10))
	go func() {
		for r := range p1.Out() {
			p2.Send(context.Background(), r)
		}
		p2.Close()
	}()
	p1.Send(context.Background(), 1)
	p1.Send(context.Background(), 2)
	p1.Send(context.Background(), 3)
	p1.Close()
	var results []string
	for r := range p2.Out() {
		results = append(results, r)
	}
	require.ElementsMatch(t, []string{"x3", "x4"}, results)
}

func TestWorkers(t *testing.T) {
	p := New(5, func(ctx context.Context, n int) (int, bool, error) { return n, true, nil })
	defer p.Close()
	require.Equal(t, 5, p.Workers())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
