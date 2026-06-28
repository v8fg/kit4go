package log4go

import (
	"io"
	"log"
	"testing"
)

// silence the ShardLogger startup banner so it doesn't corrupt bench output.
func silenceLog() func() {
	orig := log.Writer()
	log.SetOutput(io.Discard)
	return func() { log.SetOutput(orig) }
}

// Multi-core: single Logger (A) vs ShardLogger (D), fast vs slow writer.
// RunParallel = GOMAXPROCS producers; lower ns/op = higher aggregate QPS.

func Benchmark_MultiCore_A_Discard(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lg.Info("x")
		}
	})
}

func Benchmark_MultiCore_D4_Discard(b *testing.B) {
	restore := silenceLog()
	s := NewShardLogger(4)
	defer restore()
	defer s.Close()
	s.SetLevel(DEBUG)
	s.Register(discardWriter{})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Info("x")
		}
	})
}

func Benchmark_MultiCore_A_Slow(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(slowWriter{work: 2000})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lg.Info("x")
		}
	})
}

func Benchmark_MultiCore_D4_Slow(b *testing.B) {
	restore := silenceLog()
	s := NewShardLogger(4)
	defer restore()
	defer s.Close()
	s.SetLevel(DEBUG)
	s.Register(slowWriter{work: 2000})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Info("x")
		}
	})
}
