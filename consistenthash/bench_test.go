package consistenthash

import (
	"strconv"
	"strings"
	"testing"
)

// BenchmarkNew measures constructing a Map and seeding it with nodes via
// WithNodes (the common entry point). Covers id-capture + slice growth.
func BenchmarkNew(b *testing.B) {
	b.Run("10nodes", func(b *testing.B) {
		nodes := makeNodes(10)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = New[string](strID, WithNodes(nodes...))
		}
	})
	b.Run("100nodes", func(b *testing.B) {
		nodes := makeNodes(100)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = New[string](strID, WithNodes(nodes...))
		}
	})
}

// BenchmarkAdd measures incremental membership growth. Each iteration adds one
// node to a live map, exercising the duplicate check (O(N) scan) + append.
func BenchmarkAdd(b *testing.B) {
	m := New[string](strID, WithNodes(makeNodes(100)...))
	add := "added-node"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(add + strconv.Itoa(i)) // unique id each time to force the scan
	}
}

// BenchmarkGet is the hot path: a single rendezvous lookup. Parameterized by
// node count since Get is O(N). 10/50/100 nodes cover the documented shard
// routing range; 500 shows the linear cost. The scratch buffer is pool-recycled,
// so steady-state allocs/op should be 0 regardless of node count.
func BenchmarkGet(b *testing.B) {
	for _, n := range []int{10, 50, 100, 500} {
		b.Run(strconv.Itoa(n)+"nodes", func(b *testing.B) {
			m := New[string](strID, WithNodes(makeNodes(n)...))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = m.Get("auction-42")
			}
		})
	}
}

// BenchmarkGetLongIDs exercises the oversized-input path: node IDs and key well
// beyond the initial scratch capacity force the buffer to grow once, then it is
// reused. Confirms the recycled scratch still yields 0 steady-state allocs even
// for large hash inputs.
func BenchmarkGetLongIDs(b *testing.B) {
	const nNodes = 100
	nodes := make([]string, nNodes)
	pad := strings.Repeat("x", 256) // force scratch growth past initial cap
	for i := range nNodes {
		nodes[i] = "node-" + strconv.Itoa(i) + "-" + pad
	}
	longKey := "auction-" + strings.Repeat("k", 256)
	m := New[string](strID, WithNodes(nodes...))
	b.ReportAllocs()

	for b.Loop() {
		_, _ = m.Get(longKey)
	}
}

// BenchmarkGetN measures replication-site selection (top-n by HRW score).
// GetN scores every node then partial-sorts, so it's heavier than Get.
func BenchmarkGetN(b *testing.B) {
	for _, n := range []int{10, 100, 500} {
		b.Run(strconv.Itoa(n)+"nodes", func(b *testing.B) {
			m := New[string](strID, WithNodes(makeNodes(n)...))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.GetN("auction-42", 3)
			}
		})
	}
}

// BenchmarkRemove measures node removal (O(N) id scan + in-place compaction).
func BenchmarkRemove(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		m := New[string](strID, WithNodes(makeNodes(100)...))
		b.StartTimer()
		m.Remove("node-0050")
	}
}

// BenchmarkDefaultHash isolates the FNV-1a 64 cost per Get iteration, since the
// hash dominates Get at large N.
func BenchmarkDefaultHash(b *testing.B) {
	data := []byte("node-0042auction-42")
	b.ReportAllocs()

	for b.Loop() {
		_ = DefaultHash(data)
	}
}

func makeNodes(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = "node-" + strconv.Itoa(i)
	}
	return out
}
