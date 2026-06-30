package hyperloglog

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrecisionBounds(t *testing.T) {
	for _, p := range []uint8{3, 17, 0} {
		_, err := New(p)
		require.ErrorIs(t, err, ErrPrecision)
	}
	for _, p := range []uint8{4, 14, 16} {
		h, err := New(p)
		require.NoError(t, err)
		require.Equal(t, p, h.Precision())
		require.Len(t, h.reg, 1<<p)
	}
}

func TestEmptyEstimateZero(t *testing.T) {
	h, _ := New(14)
	require.Equal(t, float64(0), h.Estimate())
}

func TestDistinctCountAccuracy(t *testing.T) {
	const n = 50000
	for _, p := range []uint8{12, 14} {
		h, _ := New(p)
		for i := 0; i < n; i++ {
			h.AddString(fmt.Sprintf("user-%d", i))
		}
		est := h.Estimate()
		err := (est - float64(n)) / float64(n)
		// p=12 ~1.5% error, p=14 ~0.8%; allow 6% to avoid flakes.
		require.Less(t, abs(err), 0.06, "p=%d estimate=%.0f (err=%.3f)", p, est, err)
	}
}

// Adding the SAME element many times must not move the estimate.
func TestDuplicatesDoNotInflate(t *testing.T) {
	h, _ := New(14)
	for i := 0; i < 100000; i++ {
		h.AddString("same-user")
	}
	require.Less(t, h.Estimate(), 10.0) // ~1 distinct
}

func TestReset(t *testing.T) {
	h, _ := New(14)
	for i := 0; i < 1000; i++ {
		h.AddString(fmt.Sprintf("k%d", i))
	}
	require.Greater(t, h.Estimate(), 0.0)
	h.Reset()
	require.Equal(t, float64(0), h.Estimate())
}

func TestMerge(t *testing.T) {
	const half = 20000
	a, _ := New(14)
	b, _ := New(14)
	for i := 0; i < half; i++ {
		a.AddString(fmt.Sprintf("u-%d", i))
		b.AddString(fmt.Sprintf("u-%d", half+i)) // disjoint
	}
	require.NoError(t, a.Merge(b))
	est := a.Estimate()
	err := (est - float64(half*2)) / float64(half*2)
	require.Less(t, abs(err), 0.06)

	// Merge error on precision mismatch.
	small, _ := New(12)
	require.ErrorIs(t, a.Merge(small), ErrIncompatible)
}

func TestDeterministic(t *testing.T) {
	h1, _ := New(14)
	h2, _ := New(14)
	for i := 0; i < 5000; i++ {
		s := fmt.Sprintf("x-%d", i)
		h1.AddString(s)
		h2.AddString(s)
	}
	require.Equal(t, h1.Estimate(), h2.Estimate())
}

func TestAddHashedMatchesAdd(t *testing.T) {
	h1, _ := New(14)
	h2, _ := New(14)
	data := []byte("abc")
	h1.Add(data)
	h2.AddHashed(DefaultHash(data))
	require.Equal(t, h1.reg, h2.reg)
}

// Low precision exercises the alpha() small-m branches (16/32/64).
func TestLowPrecisionEstimates(t *testing.T) {
	for _, p := range []uint8{4, 5, 6} {
		h, _ := New(p)
		const n = 200
		for i := 0; i < n; i++ {
			h.AddString(fmt.Sprintf("k-%d", i))
		}
		// Low precision = high error; just assert it is positive and in a loose
		// band (alpha small-m branches are covered, not the accuracy target).
		est := h.Estimate()
		require.Greater(t, est, 0.0)
		require.Less(t, est, float64(n)*4)
	}
}

// A hash whose trailing (64-p) bits are all zero exercises the rho upper-bound
// clamp in AddHashed (rest all-zero -> LeadingZeros64 == 64 -> rho clamped).
func TestAddHashedRhoClamp(t *testing.T) {
	h, _ := New(14)
	// index in top p bits, bottom (64-p) bits zero.
	idx := uint64(7)
	x := idx << (64 - 14)
	require.NotPanics(t, func() { h.AddHashed(x) })
	// Register at index 7 set; estimate still computable.
	require.NotPanics(t, func() { _ = h.Estimate() })
}

// Concurrent producers each own a per-shard sketch; the shards are Merged after
// (the algorithm's intended concurrency model — Add itself is not locked).
func TestConcurrency_ShardedMerge(t *testing.T) {
	const g = 16
	const per = 3000
	shards := make([]*HyperLogLog, g)
	for i := range shards {
		shards[i], _ = New(14)
	}
	var wg sync.WaitGroup
	wg.Add(g)
	for i := 0; i < g; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < per; j++ {
				shards[i].AddString(fmt.Sprintf("u-%d", i*per+j))
			}
		}()
	}
	wg.Wait()
	merged, _ := New(14)
	for _, s := range shards {
		require.NoError(t, merged.Merge(s))
	}
	est := merged.Estimate()
	err := (est - float64(g*per)) / float64(g*per)
	require.Less(t, abs(err), 0.06)
}

// Random distinct keys across a wider space, just to exercise dispersion.
func TestRandomDistinct(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	h, _ := New(14)
	const n = 20000
	seen := make(map[string]struct{}, n)
	for len(seen) < n {
		k := fmt.Sprintf("k-%d", r.IntN(1<<24))
		h.AddString(k)
		seen[k] = struct{}{}
	}
	est := h.Estimate()
	err := (est - float64(n)) / float64(n)
	require.Less(t, abs(err), 0.06)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
