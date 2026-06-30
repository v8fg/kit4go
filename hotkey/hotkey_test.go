package hotkey

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(0, 5) })
	require.Panics(t, func() { New(time.Second, 0) })
}

func TestTopReturnsHeaviestFirst(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	d := New(time.Second, 3, WithClock(clk.now))
	// "hot" gets 10 hits, "warm" gets 3, "cold" gets 1.
	for i := 0; i < 10; i++ {
		d.Touch("hot")
	}
	for i := 0; i < 3; i++ {
		d.Touch("warm")
	}
	d.Touch("cold")

	top := d.Top()
	require.Len(t, top, 3)
	require.Equal(t, "hot", top[0].Key)
	require.Equal(t, 10, top[0].Count)
	require.Equal(t, "warm", top[1].Key)
	require.Equal(t, 3, top[1].Count)
	require.Equal(t, "cold", top[2].Key)
	require.Equal(t, 1, top[2].Count)
}

func TestTopLimitedToK(t *testing.T) {
	d := New(time.Second, 2)
	for i := 0; i < 10; i++ {
		d.Touch("a")
		d.Touch("b")
		d.Touch("c") // c will be excluded (only top 2)
	}
	top := d.Top()
	require.Len(t, top, 2)
}

func TestExpiredKeysDropOff(t *testing.T) {
	clk := &fakeClock{t: time.Unix(2000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("old")
	clk.t = clk.t.Add(5 * time.Second) // past window
	d.Touch("new")

	top := d.Top()
	require.Len(t, top, 1)
	require.Equal(t, "new", top[0].Key)
	require.Equal(t, 0, d.Count("old"))
}

func TestCountInWindow(t *testing.T) {
	clk := &fakeClock{t: time.Unix(3000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("k")
	d.Touch("k")
	d.Touch("k")
	require.Equal(t, 3, d.Count("k"))
	clk.t = clk.t.Add(2 * time.Second)
	require.Equal(t, 0, d.Count("k"))
}

func TestReset(t *testing.T) {
	d := New(time.Second, 5)
	d.Touch("a")
	d.Touch("b")
	require.Equal(t, 2, d.Len())
	d.Reset()
	require.Equal(t, 0, d.Len())
	require.Empty(t, d.Top())
}

func TestIdleKeyPruned(t *testing.T) {
	clk := &fakeClock{t: time.Unix(4000, 0)}
	d := New(time.Second, 5, WithClock(clk.now))
	d.Touch("a")
	d.Touch("b")
	require.Equal(t, 2, d.Len())
	clk.t = clk.t.Add(10 * time.Second)
	d.Touch("c") // triggers idle pruning
	require.Equal(t, 1, d.Len())
}

func TestMaxKeysEviction(t *testing.T) {
	clk := &fakeClock{t: time.Unix(5000, 0)}
	d := New(time.Hour, 10, WithMaxKeys(2), WithClock(clk.now))
	d.Touch("a")
	clk.t = clk.t.Add(time.Second)
	d.Touch("b")
	clk.t = clk.t.Add(time.Second)
	d.Touch("c") // over cap -> evict "a" (fewest hits)
	require.Equal(t, 2, d.Len())
	top := d.Top()
	for _, hk := range top {
		require.NotEqual(t, "a", hk.Key)
	}
}

func TestTopExcludesZeroCountKeys(t *testing.T) {
	d := New(time.Second, 10)
	top := d.Top()
	require.Empty(t, top) // no keys -> empty
}

func TestConcurrency(t *testing.T) {
	d := New(time.Second, 5)
	var wg sync.WaitGroup
	const g = 16
	wg.Add(g)
	for i := 0; i < g; i++ {
		key := "hot"
		if i%3 == 0 {
			key = "warm"
		}
		go func(k string) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				d.Touch(k)
			}
		}(key)
	}
	wg.Wait()
	top := d.Top()
	require.NotEmpty(t, top)
	require.Equal(t, "hot", top[0].Key) // hot had more touches (12 goroutines vs 4)
}
