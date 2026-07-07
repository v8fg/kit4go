package reservoir

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPanicOnZeroK(t *testing.T) {
	require.Panics(t, func() { New[int](0) })
	require.Panics(t, func() { New[int](-1) })
}

func TestFillPhase(t *testing.T) {
	s := New[int](5)
	for i := 1; i <= 5; i++ {
		s.Offer(i)
	}
	require.Equal(t, 5, s.Count())
	require.ElementsMatch(t, []int{1, 2, 3, 4, 5}, s.Sample())
}

func TestCap(t *testing.T) {
	s := New[string](3)
	require.Equal(t, 3, s.Cap())
}

func TestNeverExceedsK(t *testing.T) {
	s := New[int](3)
	for i := range 10000 {
		s.Offer(i)
	}
	require.Equal(t, 10000, s.Count())
	require.Len(t, s.Sample(), 3)
}

func TestReset(t *testing.T) {
	s := New[int](3)
	s.Offer(1)
	s.Offer(2)
	require.Equal(t, 2, s.Count())
	s.Reset()
	require.Equal(t, 0, s.Count())
	require.Empty(t, s.Sample())
}

func TestUniformDistribution(t *testing.T) {
	// With a known seed, verify each of 10 items (stream 1..100, k=10) has
	// roughly equal representation over many independent runs. Each item should
	// appear in the sample ~10% of the time.
	const n = 100
	const k = 10
	const runs = 5000
	freq := make([]int, n+1) // freq[item]
	for run := range uint64(runs) {
		s := NewWithOpts[int](k, WithSeed[int](run, run+1))
		for i := 1; i <= n; i++ {
			s.Offer(i)
		}
		for _, v := range s.Sample() {
			freq[v]++
		}
	}
	// Expected: each item appears ~runs * k / n = 5000 * 10 / 100 = 500 times.
	// Assert within ±25% (generous, no false flakes).
	expect := runs * k / n
	for i := 1; i <= n; i++ {
		require.InDelta(t, expect, freq[i], float64(expect)*0.25,
			"item %d freq %d, expected ~%d", i, freq[i], expect)
	}
}

func TestDeterministicWithSeed(t *testing.T) {
	s1 := NewWithOpts[int](5, WithSeed[int](42, 43))
	s2 := NewWithOpts[int](5, WithSeed[int](42, 43))
	for i := range 100 {
		s1.Offer(i)
		s2.Offer(i)
	}
	require.Equal(t, s1.Sample(), s2.Sample())
}

func TestConcurrentOffer(t *testing.T) {
	s := New[int](50)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1000 {
			s.Offer(i)
		}
	}()
	<-done
	require.Equal(t, 1000, s.Count())
	require.Len(t, s.Sample(), 50)
}

func TestSmallStream(t *testing.T) {
	s := New[int](10)
	s.Offer(1)
	s.Offer(2)
	require.Equal(t, 2, s.Count())
	require.Len(t, s.Sample(), 2)
}

func TestSampleIsCopy(t *testing.T) {
	s := New[int](3)
	s.Offer(1)
	s.Offer(2)
	s.Offer(3)
	sample := s.Sample()
	sample[0] = 999
	require.Equal(t, 1, s.Sample()[0], "Sample should return a copy")
}
