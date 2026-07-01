package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var errTransient = errors.New("transient")
var errPermanent = errors.New("permanent")

func TestSuccessOnFirstTry(t *testing.T) {
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		return 42, nil
	})
	require.NoError(t, r.Err)
	require.Equal(t, 42, r.Value)
	require.Equal(t, 1, r.Tries)
}

func TestRetriesUntilSuccess(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		n := calls.Add(1)
		if n < 3 {
			return 0, errTransient
		}
		return 99, nil
	}, WithMaxAttempts(5))
	require.NoError(t, r.Err)
	require.Equal(t, 99, r.Value)
	require.Equal(t, 3, r.Tries)
}

func TestMaxAttemptsExhausted(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	}, WithMaxAttempts(3))
	require.ErrorIs(t, r.Err, errTransient)
	require.Equal(t, 3, r.Tries)
	require.Equal(t, int64(3), calls.Load())
}

func TestPermanentErrorAbortsImmediately(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errPermanent
	}, WithMaxAttempts(10), WithRetryable(func(err error) bool {
		return errors.Is(err, errTransient)
	}))
	require.ErrorIs(t, r.Err, errPermanent)
	require.Equal(t, 1, r.Tries)
	require.Equal(t, int64(1), calls.Load())
}

func TestRetryableClassification(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		n := calls.Add(1)
		if n == 1 {
			return 0, errPermanent // non-retryable → abort
		}
		return 1, nil
	}, WithMaxAttempts(5), WithRetryable(func(err error) bool {
		return !errors.Is(err, errPermanent)
	}))
	require.ErrorIs(t, r.Err, errPermanent)
	require.Equal(t, 1, r.Tries)
}

func TestBackoffDelays(t *testing.T) {
	var calls atomic.Int64
	start := time.Now()
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	}, WithMaxAttempts(3), WithBackoff(ConstantBackoff(50*time.Millisecond)))
	elapsed := time.Since(start)
	require.Error(t, r.Err)
	require.Equal(t, 3, r.Tries)
	// 2 delays of 50ms each = ~100ms minimum.
	require.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
}

func TestExponentialBackoff(t *testing.T) {
	b := ExponentialBackoff(10*time.Millisecond, 1*time.Second)
	require.Equal(t, 10*time.Millisecond, b(1))
	require.Equal(t, 20*time.Millisecond, b(2))
	require.Equal(t, 40*time.Millisecond, b(3))
	require.Equal(t, 1*time.Second, b(100)) // capped
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int64
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	r := Do[int](ctx, func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	}, WithMaxAttempts(100), WithBackoff(ConstantBackoff(100*time.Millisecond)))
	require.ErrorIs(t, r.Err, context.Canceled)
}

func TestUnlimitedAttempts(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		n := calls.Add(1)
		if n < 100 {
			return 0, errTransient
		}
		return 7, nil
	}, WithMaxAttempts(0), WithBackoff(NoBackoff()))
	require.NoError(t, r.Err)
	require.Equal(t, 7, r.Value)
	require.Equal(t, 100, r.Tries)
}

func TestDefaultMaxAttempts(t *testing.T) {
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		return 0, errTransient
	})
	require.Error(t, r.Err)
	require.Equal(t, 3, r.Tries) // default = 3
}

func TestNoBackoff(t *testing.T) {
	b := NoBackoff()
	require.Equal(t, time.Duration(0), b(1))
	require.Equal(t, time.Duration(0), b(100))
}

func TestNilRetryableRetriesAll(t *testing.T) {
	// Without WithRetryable, all errors are retried.
	var calls atomic.Int64
	r := Do[int](context.Background(), func(ctx context.Context) (int, error) {
		n := calls.Add(1)
		if n == 1 {
			return 0, errors.New("any error")
		}
		return 5, nil
	}, WithMaxAttempts(3))
	require.NoError(t, r.Err)
	require.Equal(t, 5, r.Value)
	require.Equal(t, 2, r.Tries)
}
