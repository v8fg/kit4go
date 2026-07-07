package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FuzzCacheGetSet fuzzes arbitrary key/value pairs through the Set then Get
// round-trip. With ttl=0 the value must be immediately retrievable; with a
// positive ttl in the past, Get must return ErrMiss after expiry.
func FuzzCacheGetSet(f *testing.F) {
	// Seed corpus.
	f.Add("k", "v", int64(0))          // no expiry -> round-trips
	f.Add("user:1", "alice", int64(0)) // realistic key
	f.Add("", "", int64(0))            // empty key/value
	f.Add("k", "v", int64(-1))         // negative ttl -> treated as no expiry

	f.Fuzz(func(t *testing.T, key, val string, ttlNanos int64) {
		ctx := context.Background()
		c := NewMemory[string]()

		ttl := time.Duration(ttlNanos)
		require.NoError(t, c.Set(ctx, key, val, ttl))

		// Zero/negative ttl means "no expiry" for the in-memory backend, so
		// the value must round-trip immediately.
		if ttl <= 0 {
			got, err := c.Get(ctx, key)
			require.NoError(t, err)
			require.Equal(t, val, got)
			return
		}

		// Positive ttl: the value is present now...
		got, err := c.Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, val, got)

		// ...and, after the ttl elapses, must expire to ErrMiss. Cap the wait
		// so a pathologically large ttl doesn't stall the fuzzer.
		if ttl <= time.Second {
			time.Sleep(ttl + 10*time.Millisecond)
			_, err = c.Get(ctx, key)
			require.ErrorIs(t, err, ErrMiss)
		}
	})
}

// FuzzCacheDelete asserts that Delete never panics for any key and, after a
// Set/Delete sequence, the key is gone.
func FuzzCacheDelete(f *testing.F) {
	f.Add("k", "v")
	f.Add("missing", "")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, key, val string) {
		ctx := context.Background()
		c := NewMemory[string]()

		require.NotPanics(t, func() {
			require.NoError(t, c.Set(ctx, key, val, 0))
			require.NoError(t, c.Delete(ctx, key))
			// Deleting a key that was never set / already deleted is a no-op,
			// never a panic.
			require.NoError(t, c.Delete(ctx, key))
			require.NoError(t, c.Delete(ctx, key+"#suffix"))
		})

		_, err := c.Get(ctx, key)
		require.ErrorIs(t, err, ErrMiss)
		require.False(t, c.Has(ctx, key))
	})
}
