package trie_test

import (
	"fmt"
	"testing"

	"github.com/v8fg/kit4go/trie"
)

// trieKeys builds a deterministic set of n keys (paths under a few prefixes) so
// benchmarks are comparable across runs.
func trieKeys(n int) []string {
	keys := make([]string, 0, n)
	for i := range n {
		keys = append(keys, fmt.Sprintf("/api/v%d/resource/%d/item", i%8, i))
	}
	return keys
}

// BenchmarkInsertGet builds a trie from n keys then reads each back — the
// routing/lookup workload.
func BenchmarkInsertGet(b *testing.B) {
	keys := trieKeys(1000)
	b.ReportAllocs()
	for b.Loop() {
		t := trie.New[int]()
		for i, k := range keys {
			t.Insert(k, i)
		}
		for range keys {
			_, _ = t.Get(keys[0])
		}
	}
}

// BenchmarkGetHit measures lookup throughput on a pre-built trie (the steady-
// state hot path: many reads, few writes).
func BenchmarkGetHit(b *testing.B) {
	keys := trieKeys(1000)
	t := trie.New[int]()
	for i, k := range keys {
		t.Insert(k, i)
	}
	probe := keys[len(keys)/2]
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = t.Get(probe)
	}
}

// BenchmarkLongestPrefix measures prefix matching (the router's longest-match).
func BenchmarkLongestPrefix(b *testing.B) {
	t := trie.New[int]()
	t.Insert("/api/v1", 1)
	t.Insert("/api/v1/users", 2)
	t.Insert("/api/v1/users/123", 3)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = t.LongestPrefix("/api/v1/users/123/posts/456")
	}
}
