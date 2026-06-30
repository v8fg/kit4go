package freqcap

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(time.Second, 0) })
	require.Panics(t, func() { New(0, 3) })
}

func TestAllowUnderCap(t *testing.T) {
	c := New(time.Hour, 3)
	require.True(t, c.Allow("u1"))
	require.True(t, c.Allow("u1"))
	require.True(t, c.Allow("u1"))
	require.False(t, c.Allow("u1")) // 4th in the hour -> rejected
}

func TestKeysAreIndependent(t *testing.T) {
	c := New(time.Hour, 1)
	require.True(t, c.Allow("a"))
	require.True(t, c.Allow("b")) // different key, own cap
	require.False(t, c.Allow("a"))
	require.False(t, c.Allow("b"))
}

func TestWindowExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(10*time.Second, 2, WithClock(clk.now))

	require.True(t, c.Allow("u"))
	clk.t = clk.t.Add(5 * time.Second)
	require.True(t, c.Allow("u"))
	require.False(t, c.Allow("u")) // 2 in window
	require.Equal(t, 2, c.Count("u"))

	clk.t = clk.t.Add(6 * time.Second) // first event now 11s old -> expired
	require.True(t, c.Allow("u"))      // 1 still in window + 1 new = 2
	require.False(t, c.Allow("u"))
}

func TestCountAndReset(t *testing.T) {
	c := New(time.Minute, 5)
	c.Allow("u")
	c.Allow("u")
	require.Equal(t, 2, c.Count("u"))
	c.Reset("u")
	require.Equal(t, 0, c.Count("u"))
	require.True(t, c.Allow("u")) // fresh after reset
}

func TestIdleKeysPruned(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Second, 5, WithClock(clk.now))
	c.Allow("a")
	c.Allow("b")
	require.Equal(t, 2, c.Len())
	clk.t = clk.t.Add(10 * time.Second) // both idle
	c.Allow("c")                        // triggers idle pruning
	require.Equal(t, 1, c.Len())
}

func TestMaxKeysEviction(t *testing.T) {
	clk := &fakeClock{t: time.Unix(0, 0)}
	c := New(time.Hour, 5, WithMaxKeys(2), WithClock(clk.now))
	c.Allow("a")
	clk.t = clk.t.Add(time.Second)
	c.Allow("b")
	clk.t = clk.t.Add(time.Second)
	c.Allow("c") // over cap (3 > 2): oldest-start key "a" evicted
	require.Equal(t, 2, c.Len())
	require.True(t, c.Allow("b")) // b still tracked
	require.True(t, c.Allow("c"))
}

func TestConcurrency(t *testing.T) {
	c := New(time.Second, 100)
	const goroutines = 32
	const perG = 200
	var wg sync.WaitGroup
	var allowed atomic.Int64
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := "k"
			if i%2 == 0 {
				key = "k2"
			}
			for j := 0; j < perG; j++ {
				if c.Allow(key) {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	// Across both keys, allowed must never exceed 2 * cap = 200.
	require.LessOrEqual(t, allowed.Load(), int64(200))
	require.Greater(t, allowed.Load(), int64(0))
}

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }
