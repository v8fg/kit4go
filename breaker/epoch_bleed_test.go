package breaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHalfOpenEpochNoBleed verifies the generation counter: a probe admitted in
// a previous half-open epoch (that outlasted a trip+cooldown) must not credit
// its success to the new epoch. recordSuccess with a stale gen is a no-op on the
// halfOpenSuccess counter; only fresh-epoch successes credit toward recovery.
func TestHalfOpenEpochNoBleed(t *testing.T) {
	b, clk := newFakeBreaker(BreakerOptions{
		MaxRequests:  3,
		OpenDuration: 10 * time.Second,
	})

	// Force Open, then advance past cooldown → Execute triggers HalfOpen (epoch 1).
	b.state.Store(int32(StateOpen))
	b.expiry.Store(clk.t.UnixNano())
	clk.add(11 * time.Second)

	_, err := b.Execute(context.Background(), func(context.Context) (int, error) {
		return 1, nil // success
	})
	require.NoError(t, err)
	gen1 := b.halfOpenGen.Load()
	require.Equal(t, int32(1), gen1)
	require.Equal(t, int32(1), b.halfOpenSuccess.Load())

	// Trip: a failing probe in epoch 1.
	_, _ = b.Execute(context.Background(), func(context.Context) (int, error) {
		return 0, errors.New("boom")
	})
	require.Equal(t, StateOpen, BreakerState(b.state.Load()))

	// Advance → HalfOpen (epoch 2, gen=2).
	clk.add(11 * time.Second)
	_, err = b.Execute(context.Background(), func(context.Context) (int, error) {
		return 1, nil // success
	})
	require.NoError(t, err)
	gen2 := b.halfOpenGen.Load()
	require.Equal(t, int32(2), gen2)
	require.Equal(t, int32(1), b.halfOpenSuccess.Load(), "epoch 2 fresh: 1 success")

	// Stale probe from epoch 1 completes successfully. Before the gen fix this
	// would credit epoch 2 (halfOpenSuccess → 2). Now it's a no-op.
	b.recordSuccess(gen1)
	require.Equal(t, int32(1), b.halfOpenSuccess.Load(),
		"stale epoch-1 success must not credit epoch 2 (gen gate)")

	// A fresh epoch-2 success DOES credit.
	b.recordSuccess(gen2)
	require.Equal(t, int32(2), b.halfOpenSuccess.Load())
}
