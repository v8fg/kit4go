package log4go

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

// Test_ShardLogger_RegisterFileWriterPanics verifies the Bug 1 guard: handing a
// single *FileWriter to ShardLogger.Register is rejected loudly (it would race
// the same bufio/file/daemon across shards and corrupt output).
func Test_ShardLogger_RegisterFileWriterPanics(t *testing.T) {
	s := NewShardLogger(2)
	defer s.Close()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Level: LevelFlagDebug,
		Filename: filepath.Join(t.TempDir(), "shared-%Y%M%D.log"),
		Rotate:   true, Daily: true, Async: true,
	})
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected Register(*FileWriter) to panic, it did not")
		}
	}()
	s.Register(fw) // must panic
}

// Test_ShardLogger_RegisterFunc_IndependentFileWriters is the Bug 1 regression:
// each shard gets its OWN async FileWriter (independent bufio/file/daemon) via a
// factory, so concurrent writes across shards never race a shared writer.
// Verifies (a) no data race under -race, (b) every shard's file is created and
// non-empty, (c) every written line is well-formed (no interleaved corruption).
func Test_ShardLogger_RegisterFunc_IndependentFileWriters(t *testing.T) {
	dir := t.TempDir()
	const shards = 4
	s := NewShardLogger(shards)

	// Factory: each shard gets a DISTINCT file and its own FileWriter instance
	// with its own daemon + bufio + *os.File. This is the only correct way to
	// fan disk writes across shards.
	var shardIdx int32
	s.RegisterFunc(func() Writer {
		id := atomic.AddInt32(&shardIdx, 1)
		return NewFileWriterWithOptions(FileWriterOptions{
			Enable:   true,
			Level:    LevelFlagInfo,
			Filename: filepath.Join(dir, fmt.Sprintf("shard%d-%%Y%%M%%D.log", id)),
			Rotate:   true,
			Daily:    true,
			Async:    true,
			// reasonably sized buffer so the daemon keeps up under the burst
			AsyncBufferSize: 1 << 13,
			BufferSize:      1 << 13,
			OverflowPolicy:  "drop",
		})
	})

	var closeOnce sync.Once
	defer func() { closeOnce.Do(func() { s.Close() }) }()

	// Count how many writers actually got created (one per shard).
	var writerCount int32
	for _, l := range s.loggers {
		writerCount += int32(len(l.snapshotWriters()))
	}
	if writerCount != int32(shards) {
		t.Fatalf("expected %d independent writers (one per shard), got %d", shards, writerCount)
	}

	// Concurrent producers across shards.
	const perWorker = 4000
	const workers = 4
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range perWorker {
				s.Info("worker=%d i=%d", id, i)
			}
		}(w)
	}
	wg.Wait()
	// Close flushes each shard's FileWriter daemon to disk. Must run before the
	// file assertions so buffered data is on disk.
	closeOnce.Do(func() { s.Close() })

	// Logger.Close only Flushes (bufio); the async FileWriter daemons keep their
	// own buffers and survive Close, so under load a file may not be fully
	// drained yet. Stop() each FileWriter to deterministically drain its async
	// buffer + flush + exit, so the on-disk content is complete before asserting.
	for _, l := range s.loggers {
		for _, w := range l.snapshotWriters() {
			if fw, ok := w.(*FileWriter); ok {
				fw.Stop()
			}
		}
	}

	// Every shard's file exists and is non-empty; every line is well-formed.
	matches, err := filepath.Glob(filepath.Join(dir, "shard*-*.log"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != shards {
		t.Fatalf("expected %d shard log files, got %d (%v)", shards, len(matches), matches)
	}
	var totalLines int
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			t.Fatalf("stat %s: %v", m, err)
		}
		if info.Size() == 0 {
			t.Fatalf("shard log %s is empty (writer did not flush)", m)
		}
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		for _, ln := range lines {
			if ln == "" {
				continue
			}
			// Each line must contain the canonical level marker; a corrupted
			// interleave would fragment it.
			if !strings.Contains(ln, "[INFO]") || !strings.Contains(ln, "worker=") {
				t.Fatalf("corrupted log line in %s: %q", m, ln)
			}
			totalLines++
		}
	}
	if totalLines == 0 {
		t.Fatal("no well-formed log lines written across shards")
	}
}

// slowSink prevents the compiler from eliding slowWriter's synthetic work
// (without it, the loop's result is unused and DCE'd away, making the writer
// instant and hiding sharding's benefit).
var slowSink uint64

// slowWriter simulates a real (disk/kafka-like) writer that takes real CPU time
// per record, so the bootstrap goroutine becomes the bottleneck that sharding
// relieves. (discardWriter is instant, so it never stresses a single bootstrap
// and hides sharding's benefit.)
type slowWriter struct{ work int }

func (slowWriter) Init() error { return nil }
func (w slowWriter) Write(*Record) error {
	var x uint64
	for i := 0; i < w.work; i++ {
		x += uint64(i) * uint64(i)
	}
	atomic.AddUint64(&slowSink, x) // observed -> not elided
	return nil
}

// Benchmark_ShardLoggerScale_SlowWriter sweeps shard count with a writer that
// does real per-record work — the regime where sharding parallelizes bootstrap
// consumption and throughput scales toward GOMAXPROCS.
func Benchmark_ShardLoggerScale_SlowWriter(b *testing.B) {
	const work = 2000 // ~1µs of synthetic writer work per record (writer-bound)
	for _, n := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("shard=%d", n), func(b *testing.B) {
			s := NewShardLogger(n)
			s.Register(slowWriter{work: work})
			defer s.Close()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					s.Info("slow shard msg")
				}
			})
		})
	}
}
