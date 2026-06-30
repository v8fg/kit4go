package backoff

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNoneIsExponential(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(10*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	var got []time.Duration
	for i := 0; i < 5; i++ {
		d, ok := b.Next()
		require.True(t, ok)
		got = append(got, d)
	}
	// 10ms, 20ms, 40ms, 80ms, 160ms (no jitter).
	require.Equal(t, []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond, 160 * time.Millisecond}, got)
}

func TestCapped(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(1*time.Second), WithFactor(10), WithMax(5*time.Second))
	for i := 0; i < 4; i++ {
		d, _ := b.Next()
		require.LessOrEqual(t, d, 5*time.Second)
	}
	last, _ := b.Next()
	require.Equal(t, 5*time.Second, last) // saturated at cap
}

func TestMaxAttempts(t *testing.T) {
	b := New(WithMaxAttempts(3), WithJitter(JitterNone))
	_, ok := b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.True(t, ok)
	_, ok = b.Next()
	require.False(t, ok) // 4th attempt blocked
	require.Equal(t, 3, b.Attempt())
}

func TestReset(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(10*time.Millisecond), WithFactor(2))
	b.Next()
	b.Next()
	require.Equal(t, 2, b.Attempt())
	b.Reset()
	require.Equal(t, 0, b.Attempt())
	d, _ := b.Next()
	require.Equal(t, 10*time.Millisecond, d)
}

func TestFullJitterBounds(t *testing.T) {
	b := New(WithJitter(JitterFull), WithBase(100*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	cur := 100 * time.Millisecond
	for i := 0; i < 6; i++ {
		d, ok := b.Next()
		require.True(t, ok)
		require.LessOrEqual(t, d, cur, "full jitter must be <= exp")
		require.GreaterOrEqual(t, d, time.Duration(0))
		cur *= 2
		if cur > 10*time.Second {
			cur = 10 * time.Second
		}
	}
}

func TestEqualJitterBounds(t *testing.T) {
	b := New(WithJitter(JitterEqual), WithBase(100*time.Millisecond), WithFactor(2), WithMax(10*time.Second))
	cur := 100 * time.Millisecond
	for i := 0; i < 5; i++ {
		d, ok := b.Next()
		require.True(t, ok)
		// equal jitter: exp/2 <= d <= exp.
		require.GreaterOrEqual(t, d, cur/2)
		require.LessOrEqual(t, d, cur)
		cur *= 2
		if cur > 10*time.Second {
			cur = 10 * time.Second
		}
	}
}

func TestDecorrelatedBounds(t *testing.T) {
	b := New(WithJitter(JitterDecorrelated), WithBase(50*time.Millisecond), WithMax(5*time.Second))
	prev := 50 * time.Millisecond
	for i := 0; i < 6; i++ {
		d, ok := b.Next()
		require.True(t, ok)
		// decorrelated: base <= d <= min(max, last*3).
		upper := prev * 3
		if upper > 5*time.Second {
			upper = 5 * time.Second
		}
		require.GreaterOrEqual(t, d, 50*time.Millisecond)
		require.LessOrEqual(t, d, upper)
		prev = d
	}
}

func TestWaitSleepsAndAdvances(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(5*time.Millisecond), WithFactor(2))
	start := time.Now()
	require.NoError(t, b.Wait(context.Background()))
	require.GreaterOrEqual(t, time.Since(start), 4*time.Millisecond) // slept ~5ms
	require.Equal(t, 1, b.Attempt())
}

func TestWaitContextCancel(t *testing.T) {
	b := New(WithJitter(JitterNone), WithBase(time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := b.Wait(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitMaxAttempts(t *testing.T) {
	b := New(WithMaxAttempts(1), WithJitter(JitterNone), WithBase(time.Millisecond))
	require.NoError(t, b.Wait(context.Background()))
	require.ErrorIs(t, b.Wait(context.Background()), ErrMaxAttempts)
}

func TestDefaults(t *testing.T) {
	b := New() // default Full jitter, base 100ms, factor 2, max 10s
	d, ok := b.Next()
	require.True(t, ok)
	require.LessOrEqual(t, d, 100*time.Millisecond) // first exp value is base=100ms, full jitter <= that
}

func TestErrSentinel(t *testing.T) {
	require.True(t, errors.Is(ErrMaxAttempts, ErrMaxAttempts))
}
