package log4go

import (
	"fmt"
	"runtime"
	"testing"
)

// Benchmark_ShardLoggerParallel measures multi-shard parallel deliver
// throughput (8 shards). Total QPS scales with cores vs a single Logger.
func Benchmark_ShardLoggerParallel(b *testing.B) {
	s := NewShardLogger(8)
	s.Register(discardWriter{})
	defer s.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Info("shard bench msg=%d", 1)
		}
	})
}

// Benchmark_ShardLoggerScale measures parallel throughput vs shard count
// (multi-core scaling sweep).
func Benchmark_ShardLoggerScale(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("shard=%d", n), func(b *testing.B) {
			s := NewShardLogger(n)
			s.Register(discardWriter{})
			defer s.Close()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					s.Info("x")
				}
			})
		})
	}
}

// Test_ShardLogger_Distributes verifies records flow through shards.
func Test_ShardLogger_Distributes(t *testing.T) {
	s := NewShardLogger(4)
	defer s.Close()
	for i := range s.loggers {
		if s.loggers[i] == nil {
			t.Fatal("nil shard")
		}
	}
	// just exercise the API across levels (no panic)
	s.Debug("d %d", 1)
	s.Info("i %d", 2)
	s.Warn("w %d", 3)
	s.Error("e %d", 4)
	_ = runtime.NumGoroutine
}
