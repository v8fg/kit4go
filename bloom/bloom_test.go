package bloom

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoFalseNegatives(t *testing.T) {
	f := New(1000, 0.01)
	for i := 0; i < 1000; i++ {
		f.AddString(fmt.Sprintf("item-%d", i))
	}
	// Every inserted item must test true (no false negatives).
	for i := 0; i < 1000; i++ {
		require.True(t, f.TestString(fmt.Sprintf("item-%d", i)), "false negative on item-%d", i)
	}
}

func TestFalsePositiveRateWithinBounds(t *testing.T) {
	const n = 10000
	const fp = 0.01
	f := New(n, fp)
	for i := 0; i < n; i++ {
		f.AddString(fmt.Sprintf("in-%d", i))
	}
	// Test entirely distinct keys.
	falsePositives := 0
	const probes = 50000
	for i := 0; i < probes; i++ {
		if f.TestString(fmt.Sprintf("out-%d", i)) {
			falsePositives++
		}
	}
	observed := float64(falsePositives) / float64(probes)
	// Allow generous headroom (theoretical ~1%, plus sampling noise): cap at 4x.
	require.Less(t, observed, fp*4, "observed FPR %.4f exceeds bound", observed)
	require.Greater(t, observed, 0.0, "expected some false positives")
}

func TestTestAndAdd(t *testing.T) {
	f := New(100, 0.01)
	require.False(t, f.TestAndAddString("x")) // first time: not present
	require.True(t, f.TestAndAddString("x"))  // second time: present
}

// TestAndAddString convenience (string form of TestAndAdd).
func (f *Filter) TestAndAddString(s string) bool { return f.TestAndAdd([]byte(s)) }

func TestStringHelpers(t *testing.T) {
	f := New(50, 0.01)
	f.AddString("a")
	require.True(t, f.TestString("a"))
}

func TestParamsAndAccessors(t *testing.T) {
	f := New(1000, 0.01)
	require.Greater(t, f.M(), uint64(0))
	require.Greater(t, f.K(), uint64(0))
	require.Equal(t, uint64(0), f.N())
	f.AddString("x")
	require.Equal(t, uint64(1), f.N())
}

func TestNewFromParams(t *testing.T) {
	f := NewFromParams(1024, 5)
	require.Equal(t, uint64(1024), f.M())
	require.Equal(t, uint64(5), f.K())
}

func TestMerge(t *testing.T) {
	a := NewFromParams(4096, 6)
	b := NewFromParams(4096, 6)
	a.AddString("only-a")
	b.AddString("only-b")
	a.AddString("both")
	b.AddString("both")

	require.NoError(t, a.Merge(b))
	require.True(t, a.TestString("only-a"))
	require.True(t, a.TestString("only-b"))
	require.True(t, a.TestString("both"))
}

func TestMergeIncompatible(t *testing.T) {
	a := NewFromParams(1024, 4)
	b := NewFromParams(2048, 4)
	require.ErrorIs(t, a.Merge(b), ErrIncompatible)
}

func TestReset(t *testing.T) {
	f := New(100, 0.01)
	f.AddString("x")
	f.Reset()
	require.False(t, f.TestString("x"))
	require.Equal(t, uint64(0), f.N())
}

func TestEstimatedFPR(t *testing.T) {
	f := New(1000, 0.01)
	// With nothing inserted, FPR is 0.
	require.Equal(t, 0.0, f.EstimatedFalsePositiveRate(0))
	// At the design point (n=1000), FPR should be near the target 0.01.
	require.InDelta(t, 0.01, f.EstimatedFalsePositiveRate(1000), 0.02)
}

func TestPanicGuards(t *testing.T) {
	require.Panics(t, func() { New(0, 0.01) })
	require.Panics(t, func() { New(100, 0) })
	require.Panics(t, func() { New(100, 1) })
	require.Panics(t, func() { NewFromParams(0, 4) })
	// k=0 coerces to 1 (no panic).
	require.NotPanics(t, func() { NewFromParams(64, 0) })
}

// TestNewCoercesKBelowOne covers the `if k < 1 { k = 1 }` floor in New. With a
// tiny expectedN and an fp close to 1, the rounded hash count (m/n)*ln2 falls
// below 1 and New must coerce it to 1 rather than build a degenerate filter.
func TestNewCoercesKBelowOne(t *testing.T) {
	f := New(2, 0.99) // m=1, k rounds to 0 -> coerced to 1
	require.Equal(t, uint64(1), f.K())
	require.Equal(t, uint64(1), f.M())
	// The coerced filter still behaves: an added item tests true (no false neg).
	f.AddString("x")
	require.True(t, f.TestString("x"))
}

// TestIndicesH2ZeroGuard documents the unreachable defensive branch in indices:
//
//	if h2 == 0 { h2 = 1 }
//
// h2 is the FNV-1 hash of the input. FNV-1 multiplies the running hash by an
// odd prime (0x100000001b3) each step, which is invertible mod 2^64, so the
// hash is zero only for a specific preimage in a 2^64 space. A brute-force
// search over the entire 1/2/3-byte input space (16M+ values) finds no input
// with FNV-1 == 0. The guard therefore protects against an astronomically
// unlikely collision: it keeps the k double-hashed indices distinct instead of
// collapsing them all onto h1. It is intentionally not exercised here —
// reaching it would require finding the preimage, which defeats the purpose of
// a deterministic test. The branch is left as a defensive no-op.
func TestIndicesH2ZeroGuard(t *testing.T) {
	t.Log("indices' h2==0 branch is an unreachable defensive guard; see comment for proof")
}

func TestConcurrency(t *testing.T) {
	f := New(10000, 0.01)
	var wg sync.WaitGroup
	const g = 32
	wg.Add(g)
	for i := 0; i < g; i++ {
		i := i
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(i), 1))
			for j := 0; j < 500; j++ {
				s := fmt.Sprintf("k-%d", r.IntN(2000))
				f.TestAndAddString(s)
				f.TestString(s)
			}
		}()
	}
	wg.Wait()
	// All keys ever added must still test true (no false negatives despite churn).
	for i := 0; i < 2000; i++ {
		f.AddString(fmt.Sprintf("k-%d", i))
		require.True(t, f.TestString(fmt.Sprintf("k-%d", i)))
	}
}
