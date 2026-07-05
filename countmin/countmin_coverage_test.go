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
