package consistenthash

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func strID(s string) string { return s }

func TestEmpty(t *testing.T) {
	m := New[string](strID)
	_, ok := m.Get("k")
	require.False(t, ok)
	require.Equal(t, 0, m.Len())
	require.Empty(t, m.GetN("k", 3))
}

func TestGetDeterministic(t *testing.T) {
	m := New[string](strID, WithNodes("a", "b", "c"))
	a, ok := m.Get("auction-1")
	require.True(t, ok)
	// Same key + same membership => same node, always.
	for i := 0; i < 20; i++ {
		got, _ := m.Get("auction-1")
		require.Equal(t, a, got)
	}
}

func TestStableOnAdd(t *testing.T) {
	m := New[string](strID, WithNodes("n1", "n2", "n3", "n4"))
	owners := make(map[string]string)
	for i := 0; i < 5000; i++ {
		k := "k" + strconv.Itoa(i)
		owners[k], _ = m.Get(k)
	}

	// Add one node: only ~1/N keys should move (within tolerance).
	m.Add("n5")
	moved := 0
	for k, old := range owners {
		now, _ := m.Get(k)
		if now != old {
			moved++
		}
	}
	// Expected ~1000 of 5000 (1/5); allow generous bounds (no false flakes).
	require.Less(t, moved, 1700, "too many keys moved on add: %d", moved)
	require.Greater(t, moved, 400, "suspiciously few keys moved on add: %d", moved)
}

func TestStableOnRemove(t *testing.T) {
	m := New[string](strID, WithNodes("n1", "n2", "n3", "n4"))
	owners := make(map[string]string)
	for i := 0; i < 5000; i++ {
		k := "k" + strconv.Itoa(i)
		owners[k], _ = m.Get(k)
	}
	m.Remove("n2")
	moved := 0
	stayedOff := 0
	for k, old := range owners {
		now, _ := m.Get(k)
		if now != old {
			moved++
			if old != "n2" {
				stayedOff++ // a non-n2 key moved — should be ~0
			}
		}
	}
	// Only keys that were on n2 should move.
	require.Less(t, stayedOff, 50, "non-removed-node keys moved: %d", stayedOff)
	require.Greater(t, moved, 400, "expected keys to move off removed node")
}

func TestRemoveNoop(t *testing.T) {
	m := New[string](strID, WithNodes("a", "b"))
	m.Remove("missing")
	require.Equal(t, 2, m.Len())
}

func TestAddDuplicateIgnored(t *testing.T) {
	m := New[string](strID, WithNodes("a"))
	m.Add("a")
	require.Equal(t, 1, m.Len())
}

func TestBalance(t *testing.T) {
	m := New[string](strID, WithNodes("n1", "n2", "n3", "n4"))
	counts := make(map[string]int)
	const N = 40000
	for i := 0; i < N; i++ {
		got, _ := m.Get(strconv.Itoa(i))
		counts[got]++
	}
	// Each node should get ~N/4; assert within ±15%.
	expect := N / 4
	for _, n := range []string{"n1", "n2", "n3", "n4"} {
		require.InDelta(t, expect, counts[n], float64(expect)*0.15, "node %s imbalance", n)
	}
}

func TestGetNReplication(t *testing.T) {
	m := New[string](strID, WithNodes("a", "b", "c", "d", "e"))
	primary, _ := m.Get("k")
	top := m.GetN("k", 3)
	require.Len(t, top, 3)
	require.Equal(t, primary, top[0], "GetN[0] must equal Get (highest score)")
	// Distinct.
	seen := map[string]bool{}
	for _, n := range top {
		require.False(t, seen[n], "GetN returned a duplicate")
		seen[n] = true
	}
	// n larger than node count returns all.
	require.Len(t, m.GetN("k", 99), 5)
	require.Empty(t, m.GetN("k", 0))
}

func TestCustomHashAndTypedNodes(t *testing.T) {
	type node struct{ host string }
	m := New[node](func(n node) string { return n.host },
		WithHash[node](func(b []byte) uint64 {
			// a deliberately different hash (sum of bytes) — still consistent.
			var s uint64
			for _, c := range b {
				s = s*31 + uint64(c)
			}
			return s
		}),
		WithNodes(node{"h1"}, node{"h2"}, node{"h3"}))
	got, ok := m.Get("user:42")
	require.True(t, ok)
	require.Contains(t, []string{"h1", "h2", "h3"}, got.host)
}

func TestConcurrency(t *testing.T) {
	m := New[int](func(i int) string { return strconv.Itoa(i) },
		WithNodes(1, 2, 3, 4, 5, 6))
	var wg sync.WaitGroup
	var bad atomic.Int64
	const g = 16
	wg.Add(g * 2)
	for i := 0; i < g; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				m.Add(100 + i*10 + (j % 5)) // churn
				if _, ok := m.Get("k"); !ok && m.Len() > 0 {
					bad.Add(1)
				}
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				m.GetN("k", 3)
				m.Len()
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(0), bad.Load())
}

func ExampleNew() {
	// Assign each auction ID to a bidder shard; scaling out moves few keys.
	m := New(strID, WithNodes("shard-1", "shard-2", "shard-3"))
	node, ok := m.Get("auction-42")
	fmt.Println(ok, node != "")
	// Output: true true
}
