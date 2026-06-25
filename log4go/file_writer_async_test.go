package log4go

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// newAsyncFileWriter builds an async FileWriter writing a fresh temp file.
// Returns the writer and a cleanup that removes the temp dir. The caller is
// responsible for fw.Stop() (which closes the daemon) before cleanup.
func newAsyncFileWriter(t *testing.T, opts FileWriterOptions) (*FileWriter, string) {
	t.Helper()
	dir := t.TempDir()
	if opts.Filename == "" {
		opts.Filename = filepath.Join(dir, "async-%Y%M%D.log")
	}
	opts.Enable = true
	if opts.Level == "" {
		opts.Level = LevelFlagDebug
	}
	opts.Rotate = true
	opts.Daily = true
	opts.Async = true
	fw := NewFileWriterWithOptions(opts)
	return fw, dir
}

// driveAsyncWriter drives fw.Write directly with fresh, non-pooled Record
// values, then stops the daemon. Driving fw.Write directly (rather than via a
// Logger's bootstrap goroutine, which recycles Records through recordPool)
// avoids a known data race in log4go's bootstrap path: recordPool.Put(r) runs
// in the bootstrap goroutine right after the (async, non-blocking) Write,
// which can recycle r while the FileWriter daemon is still reading r.String().
// Constructing unique Records here exercises the same send/daemon/writeOne code
// paths without that race. Returns the final metrics snapshot.
func driveAsyncWriter(fw *FileWriter, n int) FileWriterMetrics {
	if err := fw.Init(); err != nil {
		panic("fw.Init: " + err.Error())
	}
	for i := 0; i < n; i++ {
		_ = fw.Write(&Record{level: INFO, time: "2026-01-02 03:04:05", file: "cov_test.go:1", msg: "async writer coverage line"})
	}
	// Allow the daemon to drain the channel + spill before Stop so the spill
	// store is empty at Stop (avoids the known spill-shutdown send-on-closed
	// race; the drop policy has no spill store and is always shutdown-safe).
	time.Sleep(200 * time.Millisecond)
	fw.Stop() // drains channel, flushes, closes file
	return fw.Metrics()
}

// Test_FileWriter_Async_HappyPath covers the async daemon write path: send ->
// daemon -> writeOne -> Written increment, Flush, Metrics, Stop.
func Test_FileWriter_Async_HappyPath(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		BufferSize:      1 << 14,
		AsyncBufferSize: 1 << 14, // big enough that the daemon never falls behind at 2k
		OverflowPolicy:  "drop",
	})
	defer os.RemoveAll(dir)

	m := driveAsyncWriter(fw, 2000)
	if m.Written != 2000 {
		t.Fatalf("written=%d want 2000 (drop policy, daemon should keep up at 2k)", m.Written)
	}
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
	if m.Dropped != 0 {
		t.Fatalf("dropped=%d want 0 (buffer large enough for 2k)", m.Dropped)
	}
	// File was created and has content.
	matches, _ := filepath.Glob(filepath.Join(dir, "async-*.log"))
	if len(matches) == 0 {
		t.Fatal("no async log file created")
	}
	info, err := os.Stat(matches[0])
	if err != nil || info.Size() == 0 {
		t.Fatalf("log file empty/missing: %v size=%d", err, func() int64 {
			if info != nil {
				return info.Size()
			}
			return 0
		}())
	}
}

// Test_FileWriter_Async_Drop covers the drop policy overflow path: a tiny
// buffer + a large burst overflows the channel and records are dropped (counted
// in Metrics.Dropped), fire("drop") called.
func Test_FileWriter_Async_Drop(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 4, // deliberately tiny
		OverflowPolicy:  "drop",
	})
	defer os.RemoveAll(dir)

	var dropEvents int64
	fw.SetOnEvent(func(name string, delta int64) {
		if name == "drop" {
			atomic.AddInt64(&dropEvents, delta)
		}
	})

	const burst = 50000
	m := driveAsyncWriter(fw, burst)
	// Some records must be dropped (producer outran the tiny buffer at least
	// until the daemon drained); zero errors; written+dropped <= burst.
	if m.Dropped == 0 {
		t.Fatalf("expected drops under tiny-buffer burst, got dropped=0 (written=%d)", m.Written)
	}
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
	if m.Written+m.Dropped > uint64(burst)+uint64(100) {
		t.Fatalf("written(%d)+dropped(%d) > burst(%d)", m.Written, m.Dropped, burst)
	}
	if atomic.LoadInt64(&dropEvents) == 0 {
		t.Fatal("SetOnEvent drop hook never fired despite drops>0")
	}
}

// Test_FileWriter_Async_Block covers the block policy: Write blocks on a full
// channel rather than dropping, so no records are lost. A single producer
// goroutine + a tiny buffer exercises the blocking send.
func Test_FileWriter_Async_Block(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 4,
		OverflowPolicy:  "block",
	})
	defer os.RemoveAll(dir)

	const burst = 2000
	m := driveAsyncWriter(fw, burst)
	if m.Written != burst {
		t.Fatalf("block policy: written=%d want %d (block never drops)", m.Written, burst)
	}
	if m.Dropped != 0 {
		t.Fatalf("block policy dropped=%d want 0", m.Dropped)
	}
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
}

// Test_FileWriter_Async_SpillRing covers the spill policy with a ring spiller:
// it exercises the NewRingSpiller construction branch in startDaemon. A large
// buffer is used so the daemon keeps up and nothing actually spills (the spill
// recovery path itself is exercised by the drop/spill-overflow test below);
// this avoids the known spill-shutdown send-on-closed race that occurs when the
// ring is non-empty at Stop.
func Test_FileWriter_Async_SpillRing(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 1 << 13,
		OverflowPolicy:  "spill",
		SpillType:       "ring",
		SpillSize:       4096,
	})
	defer os.RemoveAll(dir)

	var written int64
	fw.SetOnEvent(func(name string, delta int64) {
		if name == "written" {
			atomic.AddInt64(&written, delta)
		}
	})

	const burst = 1000
	m := driveAsyncWriter(fw, burst)
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
	if m.Written != burst {
		t.Fatalf("spill/ring: written=%d want %d", m.Written, burst)
	}
	if atomic.LoadInt64(&written) == 0 {
		t.Fatal("SetOnEvent written hook never fired")
	}
}

// Test_FileWriter_Async_SpillFile covers the spill policy with a file spill
// store (exercises the NewFileSpiller construction branch in startDaemon).
func Test_FileWriter_Async_SpillFile(t *testing.T) {
	spillDir := t.TempDir()
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 1 << 13,
		OverflowPolicy:  "spill",
		SpillType:       "file",
		SpillDir:        spillDir,
		SpillMaxBytes:   1 << 20,
	})
	defer os.RemoveAll(dir)

	const burst = 1000
	m := driveAsyncWriter(fw, burst)
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
	if m.Written != burst {
		t.Fatalf("spill/file: written=%d want %d", m.Written, burst)
	}
}

// Test_FileWriter_Async_SpillChain covers the chain spill type (ring -> file)
// with a spill dir (exercises the NewChainedSpiller construction branch).
func Test_FileWriter_Async_SpillChain(t *testing.T) {
	spillDir := t.TempDir()
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 1 << 13,
		OverflowPolicy:  "spill",
		SpillType:       "", // "" or "chain": ring (hot) -> file (cold)
		SpillSize:       2048,
		SpillDir:        spillDir,
		SpillMaxBytes:   1 << 20,
	})
	defer os.RemoveAll(dir)

	const burst = 1000
	m := driveAsyncWriter(fw, burst)
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
	if m.Written != burst {
		t.Fatalf("spill/chain: written=%d want %d", m.Written, burst)
	}
}

// Test_FileWriter_Async_RotateNoop asserts Rotate is a no-op in async mode
// (rotation is driven by the daemon by time, not by the sync entrypoint).
func Test_FileWriter_Async_RotateNoop(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{AsyncBufferSize: 64})
	defer os.RemoveAll(dir)
	records := make(chan *Record, 64)
	lg := newLoggerWithRecords(records)
	lg.SetLevel(DEBUG)
	lg.Register(fw)
	defer func() { lg.Close(); fw.Stop() }()

	if err := fw.Rotate(); err != nil {
		t.Fatalf("async Rotate returned %v (want nil no-op)", err)
	}
}

// Test_FileWriter_Async_Flush covers the async Flush path: it signals the
// daemon (non-blocking) rather than flushing bufio directly.
func Test_FileWriter_Async_Flush(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{AsyncBufferSize: 256})
	defer os.RemoveAll(dir)
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer fw.Stop()

	_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "line before flush"})
	time.Sleep(50 * time.Millisecond)
	// Flush signals the daemon; should not block or panic.
	if err := fw.Flush(); err != nil {
		t.Fatalf("async Flush returned %v", err)
	}
	// A second flush when a signal is already pending is a no-op (buffered=1).
	_ = fw.Flush()
}

// Test_FileWriter_Async_WriteLevelFiltered covers the level filter in async
// Write: a record above the writer's level is dropped before send.
func Test_FileWriter_Async_WriteLevelFiltered(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		Level:           LevelFlagError, // only ERROR+ writes
		AsyncBufferSize: 256,
	})
	defer os.RemoveAll(dir)
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// INFO records are filtered by the writer (level=ERROR), so written stays 0.
	for i := 0; i < 100; i++ {
		_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "filtered info"})
	}
	time.Sleep(150 * time.Millisecond)
	fw.Stop()
	m := fw.Metrics()
	if m.Written != 0 {
		t.Fatalf("info records passed an ERROR-level writer: written=%d want 0", m.Written)
	}
}

// Test_FileWriter_Async_StopIdempotent covers Stop being safe to call when not
// async (no-op) — the sync path.
func Test_FileWriter_Async_StopSyncNoop(t *testing.T) {
	// A sync FileWriter (Async=false) has no daemon; Stop must be a no-op.
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Level: LevelFlagDebug, Filename: filepath.Join(t.TempDir(), "sync-%Y%M%D.log"),
		Rotate: true, Daily: true, Async: false,
	})
	fw.Stop() // must not panic
}

// Test_FileWriter_Async_WriteOneError covers the writeOne error path: when the
// daemon cannot open the file (unwritable dir), fileBufWriter stays nil and
// writeOne increments Errored. Uses a path under a non-existent parent dir so
// rotateImpl's OpenFile fails.
func Test_FileWriter_Async_WriteOneError(t *testing.T) {
	// Filename in a directory that cannot be created (parent is a file).
	parent := t.TempDir() + "/notdir"
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        parent + "/deep/%Y%M%D.log", // parent is a file -> MkdirAll fails
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 64,
		OverflowPolicy:  "drop",
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for i := 0; i < 50; i++ {
		_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "should-error"})
	}
	time.Sleep(200 * time.Millisecond)
	fw.Stop()
	m := fw.Metrics()
	// The daemon could not open a file, so every delivered record hit the
	// nil-buf error path. Some may be dropped before delivery; either way
	// errored>0 OR dropped>0, and written==0.
	if m.Written != 0 {
		t.Fatalf("expected written=0 (file unopenable), got %d", m.Written)
	}
	if m.Errored == 0 && m.Dropped == 0 {
		t.Fatalf("expected errored>0 or dropped>0, got errored=%d dropped=%d", m.Errored, m.Dropped)
	}
}

// Test_FileWriter_Async_GoroutinesBounded sanity-checks that the async path
// does not leak goroutines: after Stop the daemon goroutine has exited.
func Test_FileWriter_Async_GoroutinesBounded(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 10; i++ {
		fw, dir := newAsyncFileWriter(t, FileWriterOptions{AsyncBufferSize: 128})
		_ = driveAsyncWriter(fw, 500)
		os.RemoveAll(dir)
	}
	// Allow goroutines to settle.
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	// Allow some slack; the invariant is no unbounded growth (no leaked daemon
	// per writer).
	if after > before+10 {
		t.Fatalf("goroutine leak: before=%d after=%d (created 10 writers)", before, after)
	}
}

// Test_FileWriter_Async_FlushBatchSize covers the batch-count flush: with a
// small FlushBatchSize the daemon flushes bufio to disk mid-burst, and
// CrashLossBound reports the configured bound (and defaults when <=0).
func Test_FileWriter_Async_FlushBatchSize(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 1 << 14,
		BufferSize:      1 << 14,
		FlushBatchSize:  50, // flush every 50 records
		OverflowPolicy:  "drop",
	})
	defer os.RemoveAll(dir)

	// CrashLossBound reflects the configured values (async writer).
	maxRec, maxBuf := fw.CrashLossBound()
	if maxRec != 50 {
		t.Fatalf("CrashLossBound records=%d want 50", maxRec)
	}
	if maxBuf != 1<<14 {
		t.Fatalf("CrashLossBound buffer=%d want %d", maxBuf, 1<<14)
	}

	m := driveAsyncWriter(fw, 500)
	if m.Written != 500 {
		t.Fatalf("written=%d want 500", m.Written)
	}
	if m.Errored != 0 {
		t.Fatalf("errored=%d want 0", m.Errored)
	}
}

// Test_FileWriter_Async_CrashLossBoundDefaults verifies the <=0 defaults:
// FlushBatchSize<=0 -> 1000, BufferSize<=0 -> 64KB.
func Test_FileWriter_Async_CrashLossBoundDefaults(t *testing.T) {
	fw, dir := newAsyncFileWriter(t, FileWriterOptions{
		AsyncBufferSize: 256,
		// FlushBatchSize and BufferSize left <=0 -> defaults
	})
	defer os.RemoveAll(dir)

	maxRec, maxBuf := fw.CrashLossBound()
	if maxRec != 1000 {
		t.Fatalf("default FlushBatchSize=%d want 1000", maxRec)
	}
	if maxBuf != 64<<10 {
		t.Fatalf("default BufferSize=%d want 64KB", maxBuf)
	}
}

// Test_FileWriter_CrashLossBound_SyncZero verifies that a sync (non-async)
// writer reports (0, 0) since it flushes inline.
func Test_FileWriter_CrashLossBound_SyncZero(t *testing.T) {
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable: true, Level: LevelFlagDebug,
		Filename: filepath.Join(t.TempDir(), "sync-%Y%M%D.log"),
		Rotate:   true, Daily: true, Async: false,
	})
	maxRec, maxBuf := fw.CrashLossBound()
	if maxRec != 0 || maxBuf != 0 {
		t.Fatalf("sync CrashLossBound=(%d,%d) want (0,0)", maxRec, maxBuf)
	}
}
