package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestExponentialBackoff_AttemptAbove30 covers the attempt > 30 branch (returns
// max to avoid overflow).
func TestExponentialBackoff_AttemptAbove30(t *testing.T) {
	b := ExponentialBackoff(1*time.Nanosecond, 1*time.Second)
	require.Equal(t, 1*time.Second, b(31))
	require.Equal(t, 1*time.Second, b(100))
}

// TestExponentialBackoff_OverflowSaturation covers the d <= 0 branch (overflow
// of base << (attempt-1) saturates to max).
func TestExponentialBackoff_OverflowSaturation(t *testing.T) {
	// Large base shifted left enough wraps negative; d <= 0 returns max.
	b := ExponentialBackoff(1<<60*time.Nanosecond, 500*time.Millisecond)
	require.Equal(t, 500*time.Millisecond, b(5))
}

// TestExponentialBackoff_BaseExceedsMax covers the d > max branch (base itself
// exceeds the cap).
func TestExponentialBackoff_BaseExceedsMax(t *testing.T) {
	b := ExponentialBackoff(10*time.Second, 1*time.Second)
	require.Equal(t, 1*time.Second, b(1))
}

// TestDo_ContextAlreadyCancelled covers the ctx.Done() branch at the top of the
// loop (returns before the first attempt).
func TestDo_ContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Do
	var calls atomic.Int64
	r := Do[int](ctx, func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, nil
	})
	require.ErrorIs(t, r.Err, context.Canceled)
	require.Equal(t, 0, r.Tries)
	require.Equal(t, int64(0), calls.Load(), "fn must not be called")
}

// TestDo_ContextCancelledDuringSleep covers the ctx.Done() branch inside the
// backoff timer wait (a retry is interrupted by cancellation mid-sleep).
func TestDo_ContextCancelledDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int64
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	r := Do[int](ctx, func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	}, WithMaxAttempts(100), WithBackoff(ConstantBackoff(1*time.Second)))
	require.ErrorIs(t, r.Err, context.Canceled)
	require.Equal(t, int64(1), calls.Load())
	// Tries = 1 (one attempt ran, sleep was interrupted before attempt 2).
	require.Equal(t, 1, r.Tries)
}

// TestDo_PermanentErrorPropagates covers the IsRetryable-false branch where the
// classifier is non-nil and returns false — the error propagates without
// further retry.
func TestDo_PermanentErrorPropagates(t *testing.T) {
	customErr := errors.New("nope")
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, customErr
	}, WithMaxAttempts(5), WithRetryable(func(err error) bool {
		return false // nothing is retryable
	}))
	require.ErrorIs(t, r.Err, customErr)
	require.Equal(t, 1, r.Tries)
	require.Equal(t, int64(1), calls.Load())
}

// TestConstantBackoff covers ConstantBackoff returning the same delay for any
// attempt.
func TestConstantBackoff(t *testing.T) {
	b := ConstantBackoff(42 * time.Millisecond)
	require.Equal(t, 42*time.Millisecond, b(1))
	require.Equal(t, 42*time.Millisecond, b(99))
}

// TestResult_ZeroValue covers the zero value of Result (Err == nil, Tries == 0).
func TestResult_ZeroValue(t *testing.T) {
	var r Result[int]
	require.Equal(t, 0, r.Value)
	require.NoError(t, r.Err)
	require.Equal(t, 0, r.Tries)
}

// TestWithBackoff_Option covers the WithBackoff option being applied.
func TestWithBackoff_Option(t *testing.T) {
	var calls atomic.Int64
	start := time.Now()
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	}, WithMaxAttempts(3), WithBackoff(ConstantBackoff(25*time.Millisecond)))
	elapsed := time.Since(start)
	require.Error(t, r.Err)
	require.Equal(t, 3, r.Tries)
	require.GreaterOrEqual(t, elapsed, 40*time.Millisecond) // 2 backoffs ~50ms
}

// TestExponentialBackoff_Normal covers the standard doubling on small attempts.
func TestExponentialBackoff_Normal(t *testing.T) {
	b := ExponentialBackoff(1*time.Millisecond, 1*time.Hour)
	require.Equal(t, 1*time.Millisecond, b(1))
	require.Equal(t, 2*time.Millisecond, b(2))
	require.Equal(t, 4*time.Millisecond, b(3))
	require.Equal(t, 8*time.Millisecond, b(4))
}
