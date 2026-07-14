package memoize_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/memoize"
)

func TestMemoize(t *testing.T) {
	var calls atomic.Int64
	slow := memoize.Memoize(func(n int) int {
		calls.Add(1)
		return n * n
	})

	require.Equal(t, 9, slow(3))
	require.Equal(t, 9, slow(3)) // cached
	require.Equal(t, 16, slow(4))
	require.Equal(t, 9, slow(3)) // still cached

	require.Equal(t, int64(2), calls.Load(), "fn called once per unique key")
}

func TestMemoizeStringKey(t *testing.T) {
	expand := memoize.Memoize(func(s string) int { return len(s) })
	require.Equal(t, 5, expand("hello"))
	require.Equal(t, 5, expand("hello"))
	require.Equal(t, 0, expand(""))
}

func TestMemoizeDifferentResultsPerKey(t *testing.T) {
	m := memoize.Memoize(func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	})
	require.Equal(t, "yes", m(true))
	require.Equal(t, "no", m(false))
	require.Equal(t, "yes", m(true))
}

func TestMemoizeErr(t *testing.T) {
	var calls atomic.Int64
	fn := memoize.MemoizeErr(func(k string) (int, error) {
		calls.Add(1)
		if k == "bad" {
			return 0, errors.New("boom")
		}
		return len(k), nil
	})

	// Success path: cached.
	v, err := fn("hello")
	require.NoError(t, err)
	require.Equal(t, 5, v)
	v, err = fn("hello")
	require.NoError(t, err)
	require.Equal(t, 5, v)
	require.Equal(t, int64(1), calls.Load(), "success cached")

	// Error path: NOT cached — retries each call.
	_, err = fn("bad")
	require.Error(t, err)
	_, err = fn("bad")
	require.Error(t, err)
	require.Equal(t, int64(3), calls.Load(), "errors not cached → re-called")

	// After error, a success caches normally.
	v, err = fn("ok")
	require.NoError(t, err)
	require.Equal(t, 2, v)
}

// TestMemoizeConcurrent proves thread-safety: many goroutines hitting the same
// uncached key concurrently must all return the correct result without racing
// (no panic, consistent value). The pure fn makes duplicate computation safe.
func TestMemoizeConcurrent(t *testing.T) {
	var calls atomic.Int64
	m := memoize.Memoize(func(n int) int {
		calls.Add(1)
		return n * 2
	})

	const goroutines = 100
	const key = 42
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]int, goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i] = m(key)
		}(i)
	}
	wg.Wait()

	for _, r := range results {
		require.Equal(t, 84, r, "all goroutines see the correct memoized result")
	}
}

// TestMemoizeErrConcurrent proves MemoizeErr is race-free under contention
// (run with -race).
func TestMemoizeErrConcurrent(t *testing.T) {
	var calls atomic.Int64
	m := memoize.MemoizeErr(func(k int) (int, error) {
		calls.Add(1)
		return k + 1, nil
	})

	var wg sync.WaitGroup
	wg.Add(50)
	for range 50 {
		go func() {
			defer wg.Done()
			v, err := m(10)
			require.NoError(t, err)
			require.Equal(t, 11, v)
		}()
	}
	wg.Wait()
}
