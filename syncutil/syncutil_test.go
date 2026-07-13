package syncutil_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/syncutil"
)

func TestOrDone(t *testing.T) {
	src := make(chan int, 3)
	src <- 1
	src <- 2
	src <- 3
	close(src)

	ctx := context.Background()
	var got []int
	for v := range syncutil.OrDone(ctx, src) {
		got = append(got, v)
	}
	require.Equal(t, []int{1, 2, 3}, got)
}

func TestOrDone_CancelBeforeClose(t *testing.T) {
	src := make(chan int) // never closed
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	for range syncutil.OrDone(ctx, src) {
	}
	elapsed := time.Since(start)
	require.Less(t, elapsed, 500*time.Millisecond, "OrDone should return promptly on cancel")
}

func TestMerge(t *testing.T) {
	ch1 := make(chan int, 2)
	ch2 := make(chan int, 2)
	ch1 <- 10
	ch2 <- 20
	close(ch1)
	close(ch2)

	ctx := context.Background()
	var got []int
	for v := range syncutil.Merge(ctx, ch1, ch2) {
		got = append(got, v)
	}
	require.ElementsMatch(t, []int{10, 20}, got)
}

func TestMerge_Cancel(t *testing.T) {
	src := make(chan int) // never closed
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	for range syncutil.Merge(ctx, src) {
	}
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestPromise_SetGet(t *testing.T) {
	p := syncutil.NewPromise[int]()
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.Set(42)
	}()

	v, err := p.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, 42, v)
}

func TestPromise_SetErr(t *testing.T) {
	p := syncutil.NewPromise[string]()
	go p.SetErr(context.DeadlineExceeded)

	_, err := p.Get(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPromise_GetCancelledCtx(t *testing.T) {
	p := syncutil.NewPromise[int]() // never Set
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Get(ctx)
	require.Error(t, err) // ctx deadline
}

func TestPromise_MultipleGetters(t *testing.T) {
	p := syncutil.NewPromise[int]()
	var received atomic.Int32

	for range 5 {
		go func() {
			v, _ := p.Get(context.Background())
			if v == 42 {
				received.Add(1)
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	p.Set(42)
	time.Sleep(10 * time.Millisecond)

	require.Equal(t, int32(5), received.Load())
}

func TestPromise_SetTwicePanics(t *testing.T) {
	p := syncutil.NewPromise[int]()
	p.Set(1)
	require.Panics(t, func() { p.Set(2) })
}

func TestPromise_Done(t *testing.T) {
	p := syncutil.NewPromise[int]()
	select {
	case <-p.Done():
		t.Fatal("Done should not be closed before Set")
	default:
	}
	p.Set(99)
	select {
	case <-p.Done():
	default:
		t.Fatal("Done should be closed after Set")
	}
}
