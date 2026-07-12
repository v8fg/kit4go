package trie

import (
	"strings"
	"testing"
)

// FuzzInsertGetRoundTrip encodes the core trie invariant: every key that has
// been Inserted is Get-able with the last value written for its NORMALIZED
// segment path. The trie segments keys by "/" with leading/trailing "/"
// trimmed, so keys that normalize to the same path ("/0", "0", "//0//") are ONE
// entry — the invariant compares on the normalized form. Catches any
// structure-corruption bug where a normalized key is lost or mis-valued. E10
// invariant-encoding fuzz target.
func FuzzInsertGetRoundTrip(f *testing.F) {
	f.Add("hello", "world", "help", "")
	f.Add("a", "ab", "abc", "b")
	f.Add("0", "/0", "//0", "0/") // normalization-equivalent keys
	f.Fuzz(func(t *testing.T, a, b, c, d string) {
		tr := New[int]()
		norm := func(k string) string { return strings.Join(segments(k), "/") }
		pairs := []struct {
			k string
			v int
		}{{a, 1}, {b, 2}, {c, 3}, {d, 4}}
		expected := map[string]int{}
		for _, p := range pairs {
			tr.Insert(p.k, p.v)
			expected[norm(p.k)] = p.v // last write per normalized key wins
		}
		for k, want := range expected {
			if got, ok := tr.Get(k); !ok || got != want {
				t.Errorf("Get(%q) = (%d, %v), want (%d, true)", k, got, ok, want)
			}
		}
	})
}
