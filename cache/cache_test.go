package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSetGetRoundTrip(t *testing.T) {
	c := NewMemory[string]()
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k", "v", 0))
	got, err := c.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, "v", got)
}

func TestGetMiss(t *testing.T) {
	c := NewMemory[int]()
	_, err := c.Get(context.Background(), "missing")
	require.ErrorIs(t, err, ErrMiss)
}

func TestDelete(t *testing.T) {
	c := NewMemory[string]()
	ctx := context.Background()
	c.Set(ctx, "k", "v", 0)
	require.True(t, c.Has(ctx, "k"))
	require.NoError(t, c.Delete(ctx, "k"))
	require.False(t, c.Has(ctx, "k"))
	_, err := c.Get(ctx, "k")
	require.ErrorIs(t, err, ErrMiss)
}

func TestTTLExpiry(t *testing.T) {
	c := NewMemory[int]()
	ctx := context.Background()
	c.Set(ctx, "k", 1, 50*time.Millisecond)
	require.True(t, c.Has(ctx, "k"))
	time.Sleep(80 * time.Millisecond)
	require.False(t, c.Has(ctx, "k"))
	_, err := c.Get(ctx, "k")
	require.ErrorIs(t, err, ErrMiss)
}

func TestDefaultTTL(t *testing.T) {
	c := NewMemory[int](WithDefaultTTL[int](50 * time.Millisecond))
	ctx := context.Background()
	// Set with ttl=0 -> uses default TTL.
	c.Set(ctx, "k", 1, 0)
	require.True(t, c.Has(ctx, "k"))
	time.Sleep(80 * time.Millisecond)
	require.False(t, c.Has(ctx, "k"))
}

func TestExplicitTTLOverridesDefault(t *testing.T) {
	c := NewMemory[int](WithDefaultTTL[int](50 * time.Millisecond))
	ctx := context.Background()
	// Explicit ttl > default -> the explicit one wins (key survives past default).
	c.Set(ctx, "k", 1, 1*time.Second)
	time.Sleep(80 * time.Millisecond)
	require.True(t, c.Has(ctx, "k"), "explicit TTL should override default")
}

func TestMaxSizeEviction(t *testing.T) {
	c := NewMemory[int](WithMaxSize[int](2))
	ctx := context.Background()
	c.Set(ctx, "a", 1, 0)
	c.Set(ctx, "b", 2, 0)
	c.Set(ctx, "c", 3, 0) // evicts "a" (LRU)
	_, err := c.Get(ctx, "a")
	require.ErrorIs(t, err, ErrMiss)
	v, _ := c.Get(ctx, "b")
	require.Equal(t, 2, v)
	v, _ = c.Get(ctx, "c")
	require.Equal(t, 3, v)
}

func TestZeroValueStored(t *testing.T) {
	c := NewMemory[*string]()
	ctx := context.Background()
	c.Set(ctx, "k", nil, 0)
	v, err := c.Get(ctx, "k")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestStoreInterfaceSatisfied(t *testing.T) {
	var s Store[int] = NewMemory[int]()
	require.NotNil(t, s)
}

func TestTypedValues(t *testing.T) {
	type user struct {
		ID   int
		Name string
	}
	c := NewMemory[user]()
	ctx := context.Background()
	c.Set(ctx, "u1", user{ID: 42, Name: "alice"}, 0)
	v, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, 42, v.ID)
	require.Equal(t, "alice", v.Name)
}

func TestErrMissIsSentinel(t *testing.T) {
	require.True(t, errors.Is(ErrMiss, ErrMiss))
}
