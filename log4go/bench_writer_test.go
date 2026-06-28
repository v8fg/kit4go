package log4go

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

func benchRecord() *Record {
	return &Record{level: INFO, time: "2026-06-25 00:00:00", file: "bench_test.go:1", msg: "benchmark writer message payload"}
}

// benchRecordWithFields is a record carrying 3 structured fields (the realistic
// structured-logging payload shape used by the JSON-codec benchmarks).
func benchRecordWithFields() *Record {
	return &Record{
		level:  INFO,
		time:   "2026-06-25T15:04:05.000+0800",
		file:   "svc.go:42",
		msg:    "benchmark writer message payload",
		fields: []field{fld("trace_id", "abc"), fld("user", 42), fld("route", "/api/v1")},
	}
}

// redirectStdoutToPipe points os.Stdout at a pipe drained to io.Discard (no
// terminal blocking) and returns a restore func. Callers MUST defer it.
func redirectStdoutToPipe(t testing.TB) func() {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, r); close(done) }()
	return func() {
		os.Stdout = orig
		_ = w.Close()
		<-done
	}
}

// silenceStdLogger redirects the standard log output to io.Discard for the
// duration of an async-writer benchmark. Async writers (File/Kafka/Net) emit
// throttled overflow alerts via log.Printf when the bounded channel fills under
// high benchmark load; those log calls contend on the log mutex and skew ns/op.
// Silencing them yields the true Write-path cost. Restored on cleanup.
func silenceStdLogger(t testing.TB) func() {
	t.Helper()
	orig := log.Writer()
	log.SetOutput(io.Discard)
	return func() { log.SetOutput(orig) }
}

// ---------------------------------------------------------------------------
// ConsoleWriter
// ---------------------------------------------------------------------------

// Benchmark_ConsoleWriter_Write measures console write throughput with stdout
// redirected to a pipe drained to io.Discard (no terminal blocking).
func Benchmark_ConsoleWriter_Write(b *testing.B) {
	restore := redirectStdoutToPipe(b)
	defer restore()

	cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo})
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cw.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark_ConsoleWriter_WriteColor measures console write with color enabled.
func Benchmark_ConsoleWriter_WriteColor(b *testing.B) {
	restore := redirectStdoutToPipe(b)
	defer restore()

	cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo, Color: true})
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cw.Write(rec)
	}
}

// Benchmark_ConsoleWriter_WriteBuffered measures the buffered (bufio) path:
// writes go to a 4KB bufio.Writer flushed by the bootstrap timer, cutting
// per-record stdout syscalls. This is the container-stdout-collection mode.
func Benchmark_ConsoleWriter_WriteBuffered(b *testing.B) {
	restore := redirectStdoutToPipe(b)
	defer restore()

	cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo, Buffered: true})
	if err := cw.Init(); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = cw.Flush() }()
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cw.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = cw.Flush()
}

// ---------------------------------------------------------------------------
// FileWriter (sync bufio + async drop + async spill)
// ---------------------------------------------------------------------------

// Benchmark_FileWriter_Write measures sync file write throughput (bufio buffered).
func Benchmark_FileWriter_Write(b *testing.B) {
	f := NewFileWriterWithOptions(FileWriterOptions{
		Filename: filepath.Join(b.TempDir(), "bench-%Y%M%D.log"),
		Rotate:   true, Daily: true, MaxDays: 60,
	})
	if err := f.Init(); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = f.Flush() }()
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	_ = f.Flush()
}

// Benchmark_FileWriter_AsyncDrop measures the async FileWriter with a drop
// overflow policy: Write delivers to a bounded channel and returns; a daemon
// goroutine does the bufio write + flush + rotate. This is the production
// high-throughput disk path and isolates disk I/O from the caller.
func Benchmark_FileWriter_AsyncDrop(b *testing.B) {
	defer silenceStdLogger(b)()
	f := NewFileWriterWithOptions(FileWriterOptions{
		Filename: filepath.Join(b.TempDir(), "bench-async-drop-%Y%M%D.log"),
		Rotate:   true, Daily: true, MaxDays: 60,
		Async:           true,
		AsyncBufferSize: 1 << 14,
		OverflowPolicy:  "drop",
	})
	if err := f.Init(); err != nil {
		b.Fatal(err)
	}
	defer f.Stop()
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// Benchmark_FileWriter_AsyncSpill measures the async FileWriter with a ring
// spill overflow policy: when the bounded channel fills (burst), records spill
// to an in-memory ring recovered once the channel drains. OOM-safe fallback.
func Benchmark_FileWriter_AsyncSpill(b *testing.B) {
	defer silenceStdLogger(b)()
	f := NewFileWriterWithOptions(FileWriterOptions{
		Filename: filepath.Join(b.TempDir(), "bench-async-spill-%Y%M%D.log"),
		Rotate:   true, Daily: true, MaxDays: 60,
		Async:           true,
		AsyncBufferSize: 1 << 14,
		OverflowPolicy:  "spill",
		SpillType:       "ring",
		SpillSize:       1 << 14,
	})
	if err := f.Init(); err != nil {
		b.Fatal(err)
	}
	defer f.Stop()
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := f.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// ---------------------------------------------------------------------------
// KafKaWriter (mock producer, no real broker)
// ---------------------------------------------------------------------------

// noopAsyncProducer is a no-op sarama.AsyncProducer for benchmarks/tests. It
// discards every message on Input() and never produces a Success or Error, so
// it drives the full KafKaWriter Write -> channel -> daemon -> producer.Input()
// path WITHOUT a real broker and WITHOUT the sarama mock's expectation-matching
// (the mock requires exact input counts, which is incompatible with b.N being
// unknown until the benchmark loop starts). The KafKaWriter daemon drains
// Successes()/Errors() in goroutines, so Close closes those channels (mirroring
// the real producer) to let those drainers exit cleanly on Stop.
type noopAsyncProducer struct {
	input     chan kafka.Message
	successes chan kafka.Message
	errors    chan *sarama.ProducerError
	quit      chan struct{}
	closeOnce sync.Once
}

func newNoopAsyncProducer() *noopAsyncProducer {
	p := &noopAsyncProducer{
		input:     make(chan kafka.Message, 1<<16),
		successes: make(chan kafka.Message, 1<<16),
		errors:    make(chan *sarama.ProducerError, 1<<16),
		quit:      make(chan struct{}),
	}
	// Drain Input() so the daemon's send never blocks. We deliberately do NOT
	// re-queue onto Successes: the KafKaWriter daemon's success-drainer is a
	// bare `for range Successes()` (no body), so it does not need deliveries to
	// make progress — it just needs the channel closed on shutdown to exit.
	// Re-queueing would race a Close (send-on-closed) for no benefit.
	go func() {
		for {
			select {
			case <-p.input:
				// discard
			case <-p.quit:
				return
			}
		}
	}()
	return p
}

func (p *noopAsyncProducer) Input() chan<- kafka.Message     { return p.input }
func (p *noopAsyncProducer) Successes() <-chan kafka.Message { return p.successes }
func (p *noopAsyncProducer) Errors() <-chan *sarama.ProducerError      { return p.errors }

// AsyncClose signals the Input drainer to stop and closes Successes/Errors so
// the KafKaWriter daemon's range-loop drainers exit (the real producer does the
// same on Close). Idempotent via closeOnce.
func (p *noopAsyncProducer) AsyncClose() {
	p.closeOnce.Do(func() {
		close(p.quit)
		close(p.successes)
		close(p.errors)
	})
}

func (p *noopAsyncProducer) Close() error { p.AsyncClose(); return nil }

// Transaction methods are no-ops: KafKaWriter never uses the transactional API,
// but sarama's AsyncProducer interface requires them. Each returns the safe
// "not transactional" result so any accidental call fails loudly upstream.
var errNoopProducerNotTxn = errors.New("noop producer: transactions not enabled")

func (p *noopAsyncProducer) IsTransactional() bool                   { return false }
func (p *noopAsyncProducer) TxnStatus() sarama.ProducerTxnStatusFlag { return 0 }
func (p *noopAsyncProducer) BeginTxn() error                         { return errNoopProducerNotTxn }
func (p *noopAsyncProducer) CommitTxn() error                        { return errNoopProducerNotTxn }
func (p *noopAsyncProducer) AbortTxn() error                         { return errNoopProducerNotTxn }
func (p *noopAsyncProducer) AddOffsetsToTxn(map[string][]*sarama.PartitionOffsetMetadata, string) error {
	return errNoopProducerNotTxn
}
func (p *noopAsyncProducer) AddOffsetsToTxnWithGroupMetadata(map[string][]*sarama.PartitionOffsetMetadata, *sarama.ConsumerGroupMetadata) error {
	return errNoopProducerNotTxn
}
func (p *noopAsyncProducer) AddMessageToTxn(*sarama.ConsumerMessage, string, *string) error {
	return errNoopProducerNotTxn
}
func (p *noopAsyncProducer) AddMessageToTxnWithGroupMetadata(*sarama.ConsumerMessage, *sarama.ConsumerGroupMetadata, *string) error {
	return errNoopProducerNotTxn
}

// compile-time: noopAsyncProducer is a sarama.AsyncProducer.
var _ sarama.AsyncProducer = (*noopAsyncProducer)(nil)

// Benchmark_KafKaWriter_WriteMock measures end-to-end KafKaWriter throughput
// against a no-op sarama AsyncProducer (no real broker, no mock expectation
// matching — b.N is unknown until the loop starts). Write builds the JSON
// payload, enqueues it on the bounded channel, and returns; the daemon forwards
// to the no-op producer. This is the production hot path minus broker latency.
func Benchmark_KafKaWriter_WriteMock(b *testing.B) {
	defer silenceStdLogger(b)()
	w := NewKafKaWriter(KafKaWriterOptions{
		ProducerTopic: "bench", BufferSize: 1 << 14, Level: LevelFlagInfo,
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		b.Fatal(err)
	}
	defer w.Stop()

	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := w.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// ---------------------------------------------------------------------------
// NetWriter (in-process net.Pipe TCP)
// ---------------------------------------------------------------------------

// startInProcTCPListener opens an in-process TCP listener on a random localhost
// port and drains everything written to it to io.Discard. Returns the dial
// address (host:port) and a stop func. This gives NetWriter a real (but
// loopback, non-blocking) TCP sink without external dependencies.
func startInProcTCPListener(tb testing.TB) (addr string, stop func()) {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _, _ = io.Copy(io.Discard, c); _ = c.Close() }(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

// Benchmark_NetWriter_WriteTCP measures NetWriter (async TCP) throughput against
// an in-process loopback TCP listener drained to io.Discard. Write enqueues a
// private copy of the record on a bounded channel under the drop policy and
// returns; the daemon serializes + writes with a deadline. This isolates the
// caller cost from network RTT (loopback) while still exercising the real
// dial/write/reconnect path.
func Benchmark_NetWriter_WriteTCP(b *testing.B) {
	defer silenceStdLogger(b)()
	addr, stopListener := startInProcTCPListener(b)
	defer stopListener()

	nw := NewNetWriter(NetWriterOptions{
		Network: "tcp", Address: addr, Level: LevelFlagDebug,
		BufferSize: 1 << 14, OverflowPolicy: "drop",
		Timeout: 500 * time.Millisecond,
	})
	if err := nw.Init(); err != nil {
		b.Fatal(err)
	}
	defer nw.Stop()
	// Give the daemon a moment to lazy-dial before the hot loop.
	time.Sleep(20 * time.Millisecond)

	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := nw.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

// ---------------------------------------------------------------------------
// IOWriter (in-memory bytes.Buffer)
// ---------------------------------------------------------------------------

// Benchmark_IOWriter_WriteBytesBuffer measures IOWriter throughput against an
// in-memory bytes.Buffer (the fastest possible io.Writer sink). This is the
// thinnest adapter overhead — useful as a test/capture sink and a ceiling for
// any custom io.Writer-based appender.
func Benchmark_IOWriter_WriteBytesBuffer(b *testing.B) {
	var buf bytes.Buffer
	w := NewIOWriter(&buf, INFO)
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := w.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark_IOWriter_WriteDiscard measures IOWriter against io.Discard — the
// pure adapter + Record.String formatting cost with zero sink overhead.
func Benchmark_IOWriter_WriteDiscard(b *testing.B) {
	w := NewIOWriter(io.Discard, INFO)
	rec := benchRecord()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := w.Write(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Record.JSON (codec comparison) — re-exported here for the single-table view.
// The canonical implementations live in json_codec_test.go; this is the discard
// sink baseline for the writer comparison table.
// ---------------------------------------------------------------------------

// Benchmark_Record_JSON_TextBaseline measures Record.String (the text format)
// — the cost every text-mode writer pays once per record. Compare against the
// JSON codecs to size the FormatJSON overhead.
func Benchmark_Record_JSON_TextBaseline(b *testing.B) {
	r := benchRecordWithFields()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.String()
	}
}

// ---------------------------------------------------------------------------
// deliver pipeline (discard writer) — the Logger-level ceiling.
// ---------------------------------------------------------------------------

// Benchmark_DeliverPipeline_Discard measures the full Logger deliver pipeline
// (format + runtime.Caller + async record delivery) against a discard writer.
// This is the caller-side cost ceiling no matter which writer is registered.
func Benchmark_DeliverPipeline_Discard(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Info("bench info iter=%d", i)
	}
}

// Benchmark_DeliverPipeline_NoCaller measures the pipeline with runtime.Caller
// disabled (no file:line) — the max-throughput mode.
func Benchmark_DeliverPipeline_NoCaller(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(DEBUG)
	lg.WithCaller(false)
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Info("x")
	}
}

// Benchmark_Logger_WithInterfaceInt vs Benchmark_Logger_WithTypedInt isolates the
// boxing cost the typed API removes: With(key, interface{}) boxes the int at the
// call site (one alloc), WithInt never boxes.
func Benchmark_Logger_WithInterfaceInt(b *testing.B) {
	root := newBenchLogger()
	defer root.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = root.With("count", i) // i boxes into interface{}
	}
}

func Benchmark_Logger_WithTypedInt(b *testing.B) {
	root := newBenchLogger()
	defer root.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = root.WithInt("count", i) // no boxing
	}
}

// Benchmark_DeliverPipeline_Filtered measures the cost of a log call filtered
// out by level — should be near-free (early return before formatting).
func Benchmark_DeliverPipeline_Filtered(b *testing.B) {
	lg := newBenchLogger()
	lg.SetLevel(EMERGENCY) // DEBUG filtered out
	lg.Register(discardWriter{})
	defer lg.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Debug("filtered iter=%d", i)
	}
}

// Benchmark_ShardLogger_4 measures multi-shard parallel deliver throughput with
// 4 shards (≈ cores/2 on a 10-core machine — the documented sweet spot).
func Benchmark_ShardLogger_4(b *testing.B) {
	s := NewShardLogger(4)
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

// ---------------------------------------------------------------------------
// Memory: per-writer sustained footprint (100K records each).
// ---------------------------------------------------------------------------

// memSinkFor returns an already-Init'd Writer of the named kind suitable for a
// sustained-footprint run, plus a closer. The sink is chosen to avoid I/O noise
// (pipe/buffer/mock/loopback) so the measured memory reflects writer state, not
// kernel buffers.
func memSinkFor(tb testing.TB, kind string) (Writer, func()) {
	switch kind {
	case "discard":
		w := discardWriter{}
		return w, func() {}
	case "console-buffered":
		restore := redirectStdoutToPipe(tb)
		cw := NewConsoleWriterWithOptions(ConsoleWriterOptions{Level: LevelFlagInfo, Buffered: true})
		_ = cw.Init()
		return cw, func() { _ = cw.Flush(); restore() }
	case "file-async-drop":
		fw := NewFileWriterWithOptions(FileWriterOptions{
			Filename: filepath.Join(tb.TempDir(), "mem-drop-%Y%M%D.log"),
			Rotate:   true, Daily: true,
			Async: true, AsyncBufferSize: 1 << 14, OverflowPolicy: "drop",
		})
		_ = fw.Init()
		return fw, func() { fw.Stop() }
	case "file-async-spill":
		fw := NewFileWriterWithOptions(FileWriterOptions{
			Filename: filepath.Join(tb.TempDir(), "mem-spill-%Y%M%D.log"),
			Rotate:   true, Daily: true,
			Async: true, AsyncBufferSize: 1 << 14, OverflowPolicy: "spill",
			SpillType: "ring", SpillSize: 1 << 14,
		})
		_ = fw.Init()
		return fw, func() { fw.Stop() }
	case "kafka-mock":
		kw := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "mem", BufferSize: 1 << 14, Level: LevelFlagInfo})
		kw.producerFactory = func() (kafka.Producer, error) {
			return newNoopAsyncProducer(), nil
		}
		_ = kw.Start()
		return kw, func() { kw.Stop() }
	case "net-tcp":
		addr, stopListener := startInProcTCPListener(tb)
		nw := NewNetWriter(NetWriterOptions{
			Network: "tcp", Address: addr, Level: LevelFlagDebug,
			BufferSize: 1 << 14, OverflowPolicy: "drop", Timeout: 500 * time.Millisecond,
		})
		_ = nw.Init()
		time.Sleep(20 * time.Millisecond)
		return nw, func() { nw.Stop(); stopListener() }
	case "io-bytes":
		var buf bytes.Buffer
		return NewIOWriter(&buf, INFO), func() {}
	}
	tb.Fatalf("unknown writer kind %q", kind)
	return nil, nil
}

// memDeltaMB returns the signed delta (after-before) in MB, clamped to >= 0.
// uint64 subtraction underflows to a huge value when GC reclaims more than the
// baseline between snapshots, so convert to int64 before dividing.
func memDeltaMB(after, before uint64) float64 {
	d := int64(after) - int64(before)
	if d < 0 {
		return 0
	}
	return float64(d) / 1e6
}

// Test_MemPerWriter logs 100K records through each writer kind and reports the
// process memory footprint (HeapAlloc / HeapInuse) + GC count + goroutine
// count after a full GC. The sink for each writer is I/O-noise-free
// (pipe/buffer/mock/loopback) so the numbers reflect per-writer state, not
// kernel buffer growth. Results are logged as a t.Logf table for PERFORMANCE.md.
func Test_MemPerWriter(k *testing.T) {
	if testing.Short() {
		k.Skip("mem-per-writer is a sustained-footprint measurement; skip in -short")
	}
	const n = 100_000
	kinds := []string{
		"discard",
		"console-buffered",
		"file-async-drop",
		"file-async-spill",
		"kafka-mock",
		"net-tcp",
		"io-bytes",
	}

	type row struct {
		kind        string
		heapAllocMB float64
		heapInuseMB float64
		numGC       uint32
		goroutines  int
	}
	rows := make([]row, 0, len(kinds))

	for _, kind := range kinds {
		ok := k.Run(kind, func(t *testing.T) {
			w, closer := memSinkFor(t, kind)
			cser := closer // mutable: nilled out after the explicit in-body call
			defer func() { cser() }()

			// Feed the writer directly (bypass the Logger deliver path) so the
			// measurement reflects per-writer state, not the Logger pipeline.
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			rec := benchRecord()
			for i := 0; i < n; i++ {
				_ = w.Write(rec)
			}
			// Drain async writers (stop the daemon + flush) before the second GC
			// so the footprint reflects steady state, not in-flight buffers.
			closer()
			// Guard against a second close from the deferred closer (some writers
			// panic on double-Stop).
			cser = func() {}

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			r := row{
				kind:        kind,
				heapAllocMB: memDeltaMB(after.HeapAlloc, before.HeapAlloc),
				heapInuseMB: memDeltaMB(after.HeapInuse, before.HeapInuse),
				numGC:       after.NumGC - before.NumGC,
				goroutines:  runtime.NumGoroutine(),
			}
			rows = append(rows, r)
			t.Logf("kind=%s HeapAlloc=%.2fMB HeapInuse=%.2fMB NumGC=%d Goroutines=%d",
				r.kind, r.heapAllocMB, r.heapInuseMB, r.numGC, r.goroutines)
		})
		if !ok {
			k.Logf("subtest %s failed; continuing", kind)
		}
	}

	// Single consolidated table for PERFORMANCE.md.
	k.Logf("\n=== MemPerWriter summary (100K records each) ===\n%-20s %12s %12s %6s %10s",
		"Writer", "HeapAlloc(MB)", "HeapInuse(MB)", "NumGC", "Goroutines")
	for _, r := range rows {
		k.Logf("%-20s %12.2f %12.2f %6d %10d", r.kind, r.heapAllocMB, r.heapInuseMB, r.numGC, r.goroutines)
	}
}
