package lru

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzLRUPutGet fuzzes the Set/Get round trip: after Set(key, value), Get(key)
// must return the same value and report present. It exercises updates to the
// same key (value must be the latest) and the "missing key" path (a Get for a
// never-set key must miss). The seed corpus uses compact []byte tokens so the
// generated corpus stays portable across the supported Go toolchains; the
// harness turns them into deterministic string keys and values.
func FuzzLRUPutGet(f *testing.F) {
	// Seed corpus: {key, value, staleKey}. staleKey drives the negative case.
	f.Add([]byte("a"), []byte("1"), []byte("zzz"))
	f.Add([]byte("k"), []byte("v"), []byte("k"))
	f.Add([]byte(""), []byte(""), []byte("nope"))
	f.Add([]byte("dup"), []byte("first"), []byte("missing"))
	f.Add([]byte("dup"), []byte("second"), []byte("missing"))

	f.Fuzz(func(t *testing.T, keyBytes, valBytes, staleKeyBytes []byte) {
		key := string(keyBytes)
		val := string(valBytes)
		staleKey := string(staleKeyBytes)

		// Large enough that nothing is evicted — isolates Put/Get correctness
		// from eviction behavior.
		c := New[string, string](WithMaxSize[string, string](1024))
		c.Set(key, val)

		// A never-set key (distinct from the live one) must be a miss.
		if staleKey != key {
			_, ok := c.Get(staleKey)
			require.False(t, ok, "Get(%q) reported present but was never Set", staleKey)
		}

		got, ok := c.Get(key)
		require.True(t, ok, "Get(%q) after Set(%q,%q) missed", key, key, val)
		require.Equal(t, val, got, "Get(%q) returned %q, want %q", key, got, val)

		// Overwrite path: re-Set must update the value and keep the entry live.
		updated := val + "'"
		c.Set(key, updated)
		got2, ok2 := c.Get(key)
		require.True(t, ok2, "Get(%q) after update missed", key)
		require.Equal(t, updated, got2, "Get(%q) returned %q after update, want %q", key, got2, updated)
	})
}

// FuzzLRUEviction fuzzes behavior at and beyond capacity: inserting more
// distinct keys than maxSize must evict the least-recently-used entry, and the
// cache length must never exceed maxSize. It rebuilds the cache deterministically
// from the fuzzed byte slice each iteration so eviction order is a pure function
// of the input.
func FuzzLRUEviction(f *testing.F) {
	// Seed corpus: each seed is a []byte stream that yields a sequence of
	// distinct integer keys. Small capacities (2-4) keep the eviction logic
	// observable rather than statistical.
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{1, 1, 1})    // duplicate key -> no eviction
	f.Add([]byte{9, 8, 7})    // all distinct, capacity 2 -> one eviction
	f.Add([]byte{5, 6, 5, 7}) // touch 5 to promote it, then evict 6
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, in []byte) {
		const cap = 4
		c := New[int, int](WithMaxSize[int, int](cap))

		// Deduplicate while preserving insertion order, since eviction depends on
		// distinct keys, not repeats. Repeats are still a valid input — they hit
		// the "update existing entry" path — but they must not grow the cache.
		seen := make(map[int]bool, len(in))
		keys := make([]int, 0, len(in))
		for _, b := range in {
			k := int(b)
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			} else {
				// Touch the existing key so it becomes most-recently-used; this
				// changes which entry should be evicted next.
				c.Get(k)
			}
		}

		for _, k := range keys {
			c.Set(k, k)
		}

		// Invariant 1: never grow past capacity.
		require.LessOrEqualf(t, c.Len(), cap,
			"Len=%d exceeded capacity %d after %d distinct Set calls (input=%v)",
			c.Len(), cap, len(keys), in)

		// Invariant 2: if we inserted more distinct keys than capacity, the
		// oldest among them must have been evicted. The first distinct key is the
		// insertion-oldest; unless a later Get promoted it, it must be gone.
		if len(keys) > cap {
			oldest := keys[0]
			_, ok := c.Get(oldest)
			require.Falsef(t, ok,
				"oldest distinct key %d should have been evicted (distinct=%d, cap=%d, input=%v)",
				oldest, len(keys), cap, in)

			// The newest distinct keys (the last `cap` inserted) must all be live.
			for i := len(keys) - cap; i < len(keys); i++ {
				k := keys[i]
				got, ok := c.Get(k)
				require.Truef(t, ok, "recent key %d was evicted despite being in the last %d (input=%v)", k, cap, in)
				require.Equal(t, k, got, "value mismatch for key %d (input=%v)", k, in)
			}
		}
	})
}
