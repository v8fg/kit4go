package freqcap_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/v8fg/kit4go/freqcap"
)

// BenchmarkAllowSameKey measures the under-cap hot path: repeated Allow on one
// key (the common "still within window" check per request).
func BenchmarkAllowSameKey(b *testing.B) {
	c := freqcap.New(time.Hour, 1_000_000)
	b.ReportAllocs()
	for b.Loop() {
		_ = c.Allow("hot-key")
	}
}

// BenchmarkAllowManyKeys rotates across many distinct keys — exercises map
// insertion and the per-key window allocation/eviction path.
func BenchmarkAllowManyKeys(b *testing.B) {
	c := freqcap.New(time.Hour, 1_000_000)
	keys := make([]string, 10_000)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		_ = c.Allow(keys[i%len(keys)])
		i++
	}
}
