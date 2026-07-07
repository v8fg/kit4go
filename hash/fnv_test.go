package hash

import (
	"hash/fnv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Empty-input FNV values are the offset bases (well-known constants).
func TestFNV_EmptyOffsetBasis(t *testing.T) {
	require.Equal(t, uint32(0x811c9dc5), FNV1a32(nil))
	require.Equal(t, uint64(0xcbf29ce484222325), FNV1a64(nil))
}

// Cross-check against the stdlib hash/fnv reference for assorted inputs.
func TestFNV_MatchesStdlib(t *testing.T) {
	inputs := []string{"", "a", "abc", "user_hash_42", "auction|imp|win", "shard-key-0xdeadbeef"}
	for _, s := range inputs {
		d := []byte(s)

		a32 := fnv.New32a()
		a32.Write(d)
		require.Equal(t, a32.Sum32(), FNV1a32(d), "FNV1a32(%q)", s)

		a64 := fnv.New64a()
		a64.Write(d)
		require.Equal(t, a64.Sum64(), FNV1a64(d), "FNV1a64(%q)", s)

		n32 := fnv.New32()
		n32.Write(d)
		require.Equal(t, n32.Sum32(), FNV132(d), "FNV132(%q)", s)

		n64 := fnv.New64()
		n64.Write(d)
		require.Equal(t, n64.Sum64(), FNV164(d), "FNV164(%q)", s)
	}
}

// String helpers must equal the byte form of the same string.
func TestFNV_StringHelpers(t *testing.T) {
	s := "bucket-key"
	require.Equal(t, FNV1a32([]byte(s)), FNV1aString32(s))
	require.Equal(t, FNV1a64([]byte(s)), FNV1aString64(s))
}

// FNV-1a and FNV-1 differ on non-empty input (different op order).
func TestFNV_1aVs1(t *testing.T) {
	d := []byte("non-empty")
	require.NotEqual(t, FNV1a32(d), FNV132(d))
	require.NotEqual(t, FNV1a64(d), FNV164(d))
}

// Deterministic across goroutines (FNV has no shared state, but verify the wrapper).
func TestFNV_Concurrent(t *testing.T) {
	const n = 64
	want := FNV1a64([]byte("k"))
	var wg sync.WaitGroup
	ok := make(chan bool, n)
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			for range 200 {
				if FNV1a64([]byte("k")) != want {
					ok <- false
					return
				}
			}
			ok <- true
		}()
	}
	wg.Wait()
	close(ok)
	for v := range ok {
		require.True(t, v)
	}
}
