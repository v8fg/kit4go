package slidingwindow_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/v8fg/kit4go/slidingwindow"
)

func TestPushSumAvg(t *testing.T) {
	w := slidingwindow.New(3)
	w.Push(10)
	w.Push(20)
	w.Push(30)
	require.Equal(t, 60.0, w.Sum())
	require.Equal(t, 20.0, w.Avg())
	require.Equal(t, 3, w.Count())
}

func TestEviction(t *testing.T) {
	w := slidingwindow.New(3)
	w.Push(1)
	w.Push(2)
	w.Push(3)
	w.Push(4)                      // evicts 1
	require.Equal(t, 9.0, w.Sum()) // 2+3+4
	require.Equal(t, 3.0, w.Avg())
}

func TestMinMax(t *testing.T) {
	w := slidingwindow.New(3)
	w.Push(5)
	w.Push(3)
	w.Push(8)
	require.Equal(t, 3.0, w.Min())
	require.Equal(t, 8.0, w.Max())
	// Push evicts 5 (not an extreme).
	w.Push(6)
	require.Equal(t, 3.0, w.Min())
	require.Equal(t, 8.0, w.Max())
	// Push evicts 3 (the min) — cache invalidated, recompute.
	w.Push(7)
	require.Equal(t, 6.0, w.Min())
	require.Equal(t, 8.0, w.Max())
}

func TestEmptyAvg(t *testing.T) {
	w := slidingwindow.New(3)
	require.True(t, math.IsNaN(w.Avg()))
	require.True(t, math.IsNaN(w.Min()))
	require.True(t, math.IsNaN(w.Max()))
}

func TestClear(t *testing.T) {
	w := slidingwindow.New(3)
	w.Push(1)
	w.Clear()
	require.Equal(t, 0, w.Count())
	require.Equal(t, 0.0, w.Sum())
}

func TestCap(t *testing.T) {
	w := slidingwindow.New(5)
	require.Equal(t, 5, w.Cap())
}

// TimeWindow tests

func TestTimeWindowPush(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(10*time.Second, 100)
	base := time.Unix(1000, 0)
	tw.Push(10, base)
	tw.Push(20, base.Add(1*time.Second))
	require.Equal(t, 30.0, tw.Sum())
	require.Equal(t, 2, tw.Count())
}

func TestTimeWindowEviction(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(5*time.Second, 100)
	base := time.Unix(1000, 0)
	tw.Push(10, base)                    // t=0
	tw.Push(20, base.Add(3*time.Second)) // t=3
	// Push at +7s: cutoff = 7-5 = 2s. Entry at t=0 is before cutoff → evicted.
	// Entry at t=3 is after cutoff → retained.
	tw.Push(30, base.Add(7*time.Second))
	require.Equal(t, 50.0, tw.Sum()) // 20+30
	require.Equal(t, 2, tw.Count())
}

func TestTimeWindowAvg(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(60*time.Second, 100)
	base := time.Unix(1000, 0)
	tw.Push(10, base)
	tw.Push(20, base.Add(1*time.Second))
	require.Equal(t, 15.0, tw.Avg())
}

func TestTimeWindowEmpty(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(60*time.Second, 100)
	require.True(t, math.IsNaN(tw.Avg()))
	require.Equal(t, 0, tw.Count())
}

func TestTimeWindowAllExpired(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(1*time.Second, 100)
	base := time.Unix(1000, 0)
	tw.Push(10, base)
	tw.Push(20, base.Add(1*time.Second))
	// Push at +10s: everything expired.
	tw.Push(30, base.Add(10*time.Second))
	require.Equal(t, 30.0, tw.Sum())
	require.Equal(t, 1, tw.Count())
}

func TestTimeWindowCapFull(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(60*time.Second, 3)
	base := time.Unix(1000, 0)
	tw.Push(1, base)
	tw.Push(2, base.Add(1*time.Second))
	tw.Push(3, base.Add(2*time.Second))
	tw.Push(4, base.Add(3*time.Second)) // cap full, evicts oldest
	require.Equal(t, 3, tw.Count())
	require.Equal(t, 9.0, tw.Sum()) // 2+3+4
}

func TestNewPanic(t *testing.T) {
	require.Panics(t, func() { slidingwindow.New(0) })
	require.Panics(t, func() { slidingwindow.New(-1) })
}

func TestPushUpdatesMinMax(t *testing.T) {
	w := slidingwindow.New(5)
	w.Push(10)
	// After first Push, hasMinMax is false → Min/Max triggers ensureMinMax.
	// After that, subsequent pushes with hasMinMax=true update minVal/maxVal.
	require.Equal(t, 10.0, w.Min())
	require.Equal(t, 10.0, w.Max())
	w.Push(5) // v < minVal → update
	require.Equal(t, 5.0, w.Min())
	w.Push(20) // v > maxVal → update
	require.Equal(t, 20.0, w.Max())
}

func TestLen(t *testing.T) {
	w := slidingwindow.New(3)
	require.Equal(t, 0, w.Len())
	w.Push(1)
	require.Equal(t, 1, w.Len())
}

func TestNewTimeWindowPanic(t *testing.T) {
	require.Panics(t, func() { slidingwindow.NewTimeWindow(0, 100) })
}

func TestNewTimeWindowDefaultCap(t *testing.T) {
	tw := slidingwindow.NewTimeWindow(60*time.Second, 0) // defaults to 1024
	require.NotNil(t, tw)
	tw.Push(1.0, time.Unix(1000, 0))
	require.Equal(t, 1, tw.Count())
}
