package countmin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewForError_EpsilonZero covers the epsilon <= 0 branch (defaults to
// 0.001).
func TestNewForError_EpsilonZero(t *testing.T) {
	c := NewForError(0, 0.01)
	require.Greater(t, c.Width(), uint32(0))
	require.Greater(t, c.Depth(), uint32(0))
}

// TestNewForError_EpsilonNegative covers the epsilon <= 0 branch with a
// negative value.
func TestNewForError_EpsilonNegative(t *testing.T) {
	c := NewForError(-1, 0.01)
	require.Greater(t, c.Width(), uint32(0))
}

// TestNewForError_DeltaZero covers the delta <= 0 branch (defaults to 0.001).
func TestNewForError_DeltaZero(t *testing.T) {
	c := NewForError(0.01, 0)
	require.Greater(t, c.Depth(), uint32(0))
}

// TestNewForError_DeltaNegative covers the delta <= 0 branch with a negative
// value.
func TestNewForError_DeltaNegative(t *testing.T) {
	c := NewForError(0.01, -0.5)
	require.Greater(t, c.Depth(), uint32(0))
}

// TestNewForError_DeltaAtOne covers the delta >= 1 branch (defaults to 0.001).
func TestNewForError_DeltaAtOne(t *testing.T) {
	c := NewForError(0.01, 1)
	require.Greater(t, c.Depth(), uint32(0))
}

// TestNewForError_DeltaAboveOne covers the delta >= 1 branch with a value > 1.
func TestNewForError_DeltaAboveOne(t *testing.T) {
	c := NewForError(0.01, 5)
	require.Greater(t, c.Depth(), uint32(0))
}

// TestMerge_IncompatibleDepth covers the depth-mismatch branch of Merge.
func TestMerge_IncompatibleDepth(t *testing.T) {
	a := New(2048, 5)
	b := New(2048, 7) // same width, different depth
	require.ErrorIs(t, a.Merge(b), ErrIncompatible)
}

// TestEstimate_EmptySketch covers the min-search loop of Estimate on a fresh
// sketch (every counter is 0).
func TestEstimate_EmptySketch(t *testing.T) {
	c := New(2048, 5)
	require.Equal(t, uint64(0), c.Estimate([]byte("never-seen")))
	require.Equal(t, uint64(0), c.EstimateString("also-unseen"))
}

// TestDoubleHash_Deterministic covers doubleHash indirectly via a pair of
// equal-key Add/Estimate calls (determinism is its contract).
func TestDoubleHash_Deterministic(t *testing.T) {
	h1a, h2a := doubleHash([]byte("key-1"))
	h1b, h2b := doubleHash([]byte("key-1"))
	require.Equal(t, h1a, h1b)
	require.Equal(t, h2a, h2b)
	// h2 must never be zero (Kirsch-Mitzenmacher requires it).
	require.NotZero(t, h2a)
	// Different inputs should (with overwhelming probability) hash differently.
	h1c, _ := doubleHash([]byte("key-2"))
	require.NotEqual(t, h1a, h1c)
}

// TestAdd_LargeCount exercises Add with a count > 1 and verifies the min across
// rows is the stored count (single-key, no collisions).
func TestAdd_LargeCount(t *testing.T) {
	c := New(2048, 5)
	c.Add([]byte("big"), 1<<40)
	require.Equal(t, uint64(1<<40), c.Estimate([]byte("big")))
	require.Equal(t, uint64(1<<40), c.Total())
}

// TestDoubleHash_H2NeverZero is a property test asserting the contract that
// doubleHash enforces (Kirsch-Mitzenmacher requires a non-zero h2 so the d row
// indices g_i = h1 + i*h2 are distinct). It exercises the post-guard return
// path; see TestDoubleHash_ZeroGuard_Unreachable for why the guard itself is
// not directly covered.
func TestDoubleHash_H2NeverZero(t *testing.T) {
	// Wide input sweep: empty, single byte, short ASCII, long, high-byte.
	inputs := [][]byte{
		nil,
		{},
		{0x00},
		[]byte("a"),
		[]byte("creative-abc"),
		[]byte("the quick brown fox jumps over the lazy dog"),
		make([]byte, 256),
	}
	for i := range inputs {
		inputs[i] = append(inputs[i], byte(i)) // perturb so high-byte slot is non-zero
	}
	for _, in := range inputs {
		_, h2 := doubleHash(in)
		require.NotZero(t, h2, "h2 must never be zero for input %v", in)
	}
}

// TestDoubleHash_ZeroGuard_Unreachable documents that the `if h2 == 0 { h2 = 1 }`
// branch in doubleHash (countmin.go:153-155) is a defensive guard that cannot
// be reached through any real call path without modifying production code.
//
// Rationale:
//   - h2 = splitmix64(fnv.New64().Sum64()). splitmix64 is a bijection over
//     uint64, so exactly one 64-bit preimage maps to 0, but FNV-1 (non-a) of
//     an arbitrary byte slice has no closed-form way to target that preimage.
//   - doubleHash builds the FNV-1 hasher inline (fnv.New64()), so there is no
//     dependency-injection seam: a test cannot substitute a hasher that yields
//     the preimage without editing production code.
//   - A brute-force byte-slice search for the preimage is infeasible (2^64
//     space) and would be a flaky, non-deterministic "test".
//
// The guard exists for correctness insurance: if a future hash change ever
// produced h2 == 0, every row index (h1 + i*0) % w would collapse to h1,
// defeating the multi-row collision cancellation that gives the sketch its
// error bounds. The guard forces h2 = 1 so the sketch stays functional rather
// than silently degrading to a single-row sketch.
//
// Coverage impact: this branch is the sole remaining uncovered statement
// (98.2% -> would be 100% if reachable). It is intentionally skipped.
func TestDoubleHash_ZeroGuard_Unreachable(t *testing.T) {
	t.Skip("defensive guard: h2 = splitmix64(fnv1(data)); reaching the " +
		"zero branch requires a hash preimage with no injection seam in " +
		"doubleHash — see comment for full rationale")
}
