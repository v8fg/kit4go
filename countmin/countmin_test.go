package countmin

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateNeverUndercounts(t *testing.T) {
	c := New(2048, 5)
	counts := map[string]uint64{"a": 3, "b": 1, "c": 7, "d": 2, "heavy": 1000}
	for k, n := range counts {
		c.Add([]byte(k), n)
	}
	for k, want := range counts {
		got := c.EstimateString(k)
		require.GreaterOrEqual(t, got, want, "%s: estimate %d < true %d (must never undercount)", k, got, want)
	}
}

func TestAddSingleDefault(t *testing.T) {
	c := New(2048, 5)
	c.AddString("x")
	c.AddString("x")
	require.Equal(t, uint64(2), c.EstimateString("x"))
}

func TestHeavyHitterStandsOut(t *testing.T) {
	c := New(4096, 6)
	// A heavy key (10000) among 1000 rare keys (~3 each).
	for i := 0; i < 1000; i++ {
		k := fmt.Sprintf("rare-%d", i)
		c.AddString(k)
		c.AddString(k)
		c.AddString(k)
	}
	for i := 0; i < 10000; i++ {
		c.AddString("heavy")
	}
	est := c.EstimateString("heavy")
	// Must be >= 10000 and not inflated beyond ~5% by collisions.
	require.GreaterOrEqual(t, est, uint64(10000))
	require.Less(t, est, uint64(11000), "heavy over-count too large: %d", est)
}

func TestTotal(t *testing.T) {
	c := New(2048, 5)
	c.Add([]byte("a"), 5)
	c.Add([]byte("b"), 7)
	require.Equal(t, uint64(12), c.Total())
}

func TestReset(t *testing.T) {
	c := New(2048, 5)
	c.AddString("a")
	c.AddString("a")
	require.Equal(t, uint64(2), c.EstimateString("a"))
	c.Reset()
	require.Equal(t, uint64(0), c.EstimateString("a"))
	require.Equal(t, uint64(0), c.Total())
}

func TestMerge(t *testing.T) {
	a := New(2048, 5)
	b := New(2048, 5)
	a.AddString("k")
	a.AddString("k")
	b.AddString("k")
	require.NoError(t, a.Merge(b))
	require.Equal(t, uint64(3), a.EstimateString("k"))
	require.Equal(t, uint64(3), a.Total())

	// Incompatible shape.
	diff := New(1024, 5)
	require.ErrorIs(t, a.Merge(diff), ErrIncompatible)
}

func TestNewForError(t *testing.T) {
	c := NewForError(0.01, 0.01)
	require.Greater(t, c.Width(), uint32(0))
	require.Greater(t, c.Depth(), uint32(0))
}

func TestDefaults(t *testing.T) {
	c := New(0, 0) // -> width 2048, depth 5
	require.Equal(t, uint32(2048), c.Width())
	require.Equal(t, uint32(5), c.Depth())
}

func TestDeterministic(t *testing.T) {
	a, b := New(2048, 5), New(2048, 5)
	for i := 0; i < 500; i++ {
		s := fmt.Sprintf("k-%d", i)
		a.AddString(s)
		b.AddString(s)
	}
	require.Equal(t, a.EstimateString("k-7"), b.EstimateString("k-7"))
}

// Add is not internally locked; the concurrency model is per-shard + Merge.
func TestConcurrency_ShardedMerge(t *testing.T) {
	const g = 16
	const per = 500
	shards := make([]*CountMinSketch, g)
	for i := range shards {
		shards[i] = New(2048, 5)
	}
	var wg sync.WaitGroup
	wg.Add(g)
	for i := 0; i < g; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < per; j++ {
				shards[i].AddString(fmt.Sprintf("k-%d", i*per+j))
			}
		}()
	}
	wg.Wait()
	merged := New(2048, 5)
	for _, s := range shards {
		require.NoError(t, merged.Merge(s))
	}
	require.Equal(t, uint64(g*per), merged.Total())
}
