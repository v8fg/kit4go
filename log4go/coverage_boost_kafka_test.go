package log4go

// Targeted coverage tests for kafka_overflow.go + kafka_writer.go.
//
// This file ONLY touches kafka_* code paths (and the shared spill primitives).
// Every test is deterministic (no real broker, no wall-clock precision): error
// branches are forced with temp-dir tricks, bad-codec injection, no-op mock
// producers, and pre-seeded spill files.

import (
	"bufio"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// ---------------------------------------------------------------------------
// helpers local to this file
// ---------------------------------------------------------------------------

// failCodec always fails Encode/Decode — used to drive the codec-error and
// decode-error branches of FileSpiller.Push / decodeSpillFile.
type failCodec struct{}

func (failCodec) Encode(_ kafka.Message) ([]byte, error) {
	return nil, errors.New("failCodec encode")
}

func (failCodec) Decode(_ []byte) (kafka.Message, error) {
	return nil, errors.New("failCodec decode")
}

// errWriter is an io.Writer that always returns an error — wired into a
// FileSpiller's bufio.Writer to force the Write(header)/Write(body) failure
// branches of FileSpiller.Push.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("errWriter") }

// buildFileSpiller constructs a FileSpiller struct without going through
// NewFileSpiller (so the caller can mutate internals to force error paths).
func buildFileSpiller(dir string, codec SpillCodec[kafka.Message]) *FileSpiller[kafka.Message] {
	return &FileSpiller[kafka.Message]{dir: dir, maxBytes: 1 << 20, codec: codec}
}

// waitDaemonRunning polls k.run until the daemon goroutine has marked itself
// running (Start launches daemon in a goroutine, so an immediate Stop could
// otherwise hit Stop's `if !k.run.Load() { return }` guard before the daemon
// sets run=true). Deterministic; no wall-clock precision dependency.
func waitDaemonRunning(t *testing.T, k *KafKaWriter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !k.run.Load() {
		time.Sleep(time.Millisecond)
	}
	if !k.run.Load() {
		t.Fatal("daemon did not mark itself running within 2s")
	}
}

// ===========================================================================
// kafka_overflow.go — NewFileSpiller / open error branches
// ===========================================================================

// Test_NewFileSpiller_OpenError drives the sp.open() failure inside
// NewFileSpiller (lines: open OpenFile error + the NewFileSpiller error return).
// MkdirAll succeeds on the existing dir, but open() cannot OpenFile(spill.log)
// because we pre-create a *directory* named spill.log at that path
// (OpenFile CREATE|WRONLY|APPEND on a directory fails on darwin/linux).
func Test_NewFileSpiller_OpenError(t *testing.T) {
	dir := t.TempDir()
	// Poison the spill.log path so OpenFile(... CREATE|WRONLY|APPEND) fails.
	if err := os.Mkdir(filepath.Join(dir, "spill.log"), 0o700); err != nil {
		t.Fatalf("seed spill.log dir: %v", err)
	}
	if _, err := NewFileSpiller[kafka.Message](dir, 1<<20, ProducerMsgCodec); err == nil {
		t.Fatal("NewFileSpiller: want OpenFile error, got nil")
	}
}

// Test_FileSpiller_open_StatNilAndError exercises open() directly when the
// spill.log path is unopenable (dir is a regular file). It also hits the
// open-error branch independent of NewFileSpiller.
func Test_FileSpiller_open_Error(t *testing.T) {
	// f.dir points at a regular file -> OpenFile on filepath.Join(file,"spill.log")
	// fails with "not a directory".
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f := buildFileSpiller(blocker, ProducerMsgCodec)
	if err := f.open(); err == nil {
		t.Fatal("open: want error (not a directory), got nil")
	}
}

// Test_FileSpiller_open_StatNil covers the `fi == nil` branch of open(): stat
// returns nil for an empty, just-created file is rare, so force it by pointing
// f.dir at a fresh empty dir and asserting open succeeds (fi non-nil in
// practice); the stat-nil arm is reached by opening a file that f.Stat reports
// nil for — we emulate by pre-creating spill.log as an empty regular file so
// fi != nil; to hit the nil arm we instead rely on open() succeeding with a
// truly fresh file (stat may return nil info in edge filesystems). This test
// asserts the happy path keeps written consistent with existing file size.
func Test_FileSpiller_open_HappyWithExistingSize(t *testing.T) {
	dir := t.TempDir()
	// Pre-seed an existing spill.log with some bytes; open() must pick up its size.
	if err := os.WriteFile(filepath.Join(dir, "spill.log"), []byte("preset"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f := buildFileSpiller(dir, ProducerMsgCodec)
	if err := f.open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.f.Close()
	if f.written != int64(len("preset")) {
		t.Errorf("written=%d want %d", f.written, int64(len("preset")))
	}
}

// ===========================================================================
// kafka_overflow.go — FileSpiller.Push error branches
// ===========================================================================

// Test_FileSpiller_Push_CodecError covers the codec.Encode failure branch.
func Test_FileSpiller_Push_CodecError(t *testing.T) {
	dir := t.TempDir()
	f, err := NewFileSpiller[kafka.Message](dir, 1<<20, failCodec{})
	if err != nil {
		t.Fatalf("NewFileSpiller: %v", err)
	}
	defer f.Close()
	if f.Push(spillerMsg("t", "x")) {
		t.Fatal("Push with failing codec must return false")
	}
}

// Test_FileSpiller_Push_WriteError forces the bufio.Writer.Write calls inside
// Push to fail. Two sub-cases cover both error arms:
//   - buffer size 1: the 4-byte header exceeds the buffer -> flush -> errWriter
//     fails on Write(hdr) (covers the header-write error branch).
//   - buffer size 8: the 4-byte header fits (buffered), then the ~32-byte body
//     write exceeds the remaining 4 bytes -> flush -> errWriter fails on
//     Write(body) (covers the body-write error branch).
func Test_FileSpiller_Push_WriteError(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"header_write_error", 1},
		{"body_write_error", 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := NewFileSpiller[kafka.Message](dir, 1<<20, ProducerMsgCodec)
			if err != nil {
				t.Fatalf("NewFileSpiller: %v", err)
			}
			// Inject a bufio over errWriter with the chosen buffer size.
			f.w = bufio.NewWriterSize(errWriter{}, c.size)
			defer f.Close()
			if f.Push(spillerMsg("t", "x")) {
				t.Fatal("Push with failing underlying writer must return false")
			}
		})
	}
}

// ===========================================================================
// kafka_overflow.go — FileSpiller.Drain rename-error branch
// ===========================================================================

// Test_FileSpiller_Drain_RenameError covers the os.Rename failure branch of
// Drain. Rename(spill.log -> spill.log.read) fails when the source path does
// not exist; Drain then re-opens (creating an empty spill.log) and returns nil.
func Test_FileSpiller_Drain_RenameError(t *testing.T) {
	dir := t.TempDir()
	f, err := NewFileSpiller[kafka.Message](dir, 1<<20, ProducerMsgCodec)
	if err != nil {
		t.Fatalf("NewFileSpiller: %v", err)
	}
	defer f.Close()
	// Delete spill.log so the rename inside Drain fails (no source). f.w is a
	// bufio over a now-deleted file but we never wrote/flushed, so this is safe.
	if err := os.Remove(filepath.Join(dir, "spill.log")); err != nil {
		t.Fatalf("remove spill.log: %v", err)
	}
	// Drain must return nil (rename error path) without panicking.
	out := f.Drain()
	if out != nil {
		t.Errorf("Drain on rename failure want nil, got %d records", len(out))
	}
}

// ===========================================================================
// kafka_overflow.go — decodeSpillFile branches (open err, short body, decode err)
// ===========================================================================

// Test_decodeSpillFile_OpenError covers the os.Open failure branch.
func Test_decodeSpillFile_OpenError(t *testing.T) {
	out := decodeSpillFile[kafka.Message](
		filepath.Join(t.TempDir(), "does-not-exist"), ProducerMsgCodec)
	if out != nil {
		t.Errorf("decodeSpillFile on missing path want nil, got %v", out)
	}
}

// Test_decodeSpillFile_DecodeError covers the codec.Decode failure branch
// (records with undecodable payloads are skipped via `continue`). It also hits
// the truncated-body ReadFull branch by writing a length header whose body is
// shorter than declared.
func Test_decodeSpillFile_DecodeAndTruncatedBody(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "spill.log")

	// Build a spill file with: one valid record, one undecodable payload, and a
	// trailing record whose declared body length exceeds the remaining bytes
	// (truncated-body ReadFull -> break).
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	writeFramed := func(payload []byte) {
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
		_, _ = fh.Write(hdr[:])
		_, _ = fh.Write(payload)
	}
	// 1) valid record
	good, _ := ProducerMsgCodec.Encode(spillerMsg("t", "ok"))
	writeFramed(good)
	// 2) undecodable payload (json unmarshal of garbage into spillRecord fails)
	writeFramed([]byte("{not-json"))
	// 3) truncated record: header claims 1<<20 bytes but we write only a few.
	var big [4]byte
	binary.BigEndian.PutUint32(big[:], uint32(1<<20))
	_, _ = fh.Write(big[:])
	_, _ = fh.Write([]byte("only-a-few")) // < declared length
	// 4) a short header (1 byte) at the very end -> ReadFull(hdr) breaks.
	_, _ = fh.Write([]byte{0x00})
	if err := fh.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out := decodeSpillFile[kafka.Message](p, ProducerMsgCodec)
	if len(out) != 1 {
		t.Fatalf("decodeSpillFile want 1 valid record (bad+truncated skipped), got %d", len(out))
	}
	got, _ := out[0].Value.Encode()
	if string(got) != "ok" {
		t.Errorf("decoded value=%q want %q", got, "ok")
	}
}

// ===========================================================================
// kafka_overflow.go — FileSpiller.Close with nil underlying file
// ===========================================================================

// Test_FileSpiller_Close_NilFile covers Close's `f.f == nil -> return nil`
// branch: a struct built directly without ever opening a file.
func Test_FileSpiller_Close_NilFile(t *testing.T) {
	f := buildFileSpiller(t.TempDir(), ProducerMsgCodec)
	if err := f.Close(); err != nil {
		t.Errorf("Close on never-opened spiller want nil, got %v", err)
	}
}

// ===========================================================================
// kafka_overflow.go — ChainedSpiller.Close with nil file backend
// ===========================================================================

// Test_ChainedSpiller_Close_NilFile covers ChainedSpiller.Close's `c.file ==
// nil -> return nil` branch (ring-only chain).
func Test_ChainedSpiller_Close_NilFile(t *testing.T) {
	c := NewChainedSpiller[kafka.Message](NewRingSpiller[kafka.Message](4), nil)
	if err := c.Close(); err != nil {
		t.Errorf("Close on ring-only chain want nil, got %v", err)
	}
}

// ===========================================================================
// kafka_overflow.go — DrainFileRecover rename-error branch
// ===========================================================================

// Test_DrainFileRecover_RenameError covers the os.Rename failure branch of
// DrainFileRecover. The source spill.log exists (so Stat succeeds) but the
// rename to spill.log.recover is made to fail by turning the destination into
// an existing directory whose parent blocks the move; simplest deterministic
// trigger is to make spill.log itself a directory (Stat ok, Rename of a dir
// across itself still works on some OS — so instead use a read-only parent).
func Test_DrainFileRecover_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Pre-create spill.log so Stat passes.
	if err := os.WriteFile(filepath.Join(dir, "spill.log"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Make the directory read-only so the rename (which needs write/creat in
	// the dir) fails. Restore perms via t.Cleanup so cleanup can delete.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	out := DrainFileRecover[kafka.Message](dir, ProducerMsgCodec)
	if out != nil {
		t.Errorf("DrainFileRecover on rename failure want nil, got %d", len(out))
	}
}

// ===========================================================================
// kafka_writer.go — buildPayload dedup branch (duplicate r.fields key)
// ===========================================================================

// Test_KafKaWriter_buildPayload_DedupDuplicateField covers the
// `if _, ok := seen[f.key]; ok { continue }` branch inside buildPayload's
// r.fields+ExtraFields dedup path: the same key appears twice in r.fields, so
// the second occurrence is skipped.
func Test_KafKaWriter_buildPayload_DedupDuplicateField(t *testing.T) {
	w := &KafKaWriter{options: KafKaWriterOptions{
		ProducerTopic: "t",
		MSG: KafKaMSGFields{
			ExtraFields: map[string]interface{}{"extra": "e"}, // forces dedup path
		},
	}}
	r := &Record{
		level: INFO,
		msg:   "m",
		fields: []field{
			fld("dup", "first"),
			fld("dup", "second"), // skipped by the seen-map continue
			fld("uniq", "u"),
		},
	}
	b := w.buildPayload(r)
	if b == nil {
		t.Fatal("nil payload")
	}
	s := string(b)
	if !containsStr(s, `"dup":"first"`) {
		t.Errorf("dedup kept wrong value or dropped key; payload=%s", s)
	}
	if containsStr(s, `"dup":"second"`) {
		t.Errorf("dedup failed to skip duplicate; payload=%s", s)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ===========================================================================
// kafka_writer.go — Write: Debug log path + payload-nil guard
// ===========================================================================

// Test_KafKaWriter_Write_Debug covers the Debug=true log branch of Write.
func Test_KafKaWriter_Write_Debug(t *testing.T) {
	w := &KafKaWriter{
		level:    INFO,
		policy:   OverflowDrop,
		messages: make(chan kafka.Message, 4),
		options: KafKaWriterOptions{
			ProducerTopic: "t",
			Debug:         true,
			Brokers:       []string{"127.0.0.1:9092"},
		},
	}
	if err := w.Write(&Record{level: INFO, msg: "debug-me"}); err != nil {
		t.Fatalf("Write debug: %v", err)
	}
	if len(w.messages) != 1 {
		t.Errorf("Debug Write enqueued=%d want 1", len(w.messages))
	}
}

// ===========================================================================
// kafka_writer.go — send: spill success-path + spill-with-nil-spiller drop
// ===========================================================================

// Test_KafKaWriter_send_SpillSuccessPath covers the `case k.messages <- msg:`
// success arm under OverflowSpill (the spill policy's fast path when the
// channel still has room).
func Test_KafKaWriter_send_SpillSuccessPath(t *testing.T) {
	w := &KafKaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[kafka.Message](4),
		messages: make(chan kafka.Message, 4),
	}
	w.send(spillerMsg("t", "ok")) // channel has room -> direct send, no spill
	if got := w.stats.Spilled(); got != 0 {
		t.Errorf("spilled=%d want 0 on successful channel send", got)
	}
	if len(w.messages) != 1 {
		t.Errorf("messages len=%d want 1", len(w.messages))
	}
}

// Test_KafKaWriter_send_SpillNilSpiller covers the OverflowSpill branch where
// the channel is full and no spiller is wired: the record is dropped (not
// spilled) via stats.IncDropped.
func Test_KafKaWriter_send_SpillNilSpiller(t *testing.T) {
	w := &KafKaWriter{
		policy:   OverflowSpill,
		spiller:  nil, // no spill store -> drop on full
		messages: make(chan kafka.Message, 1),
	}
	w.messages <- spillerMsg("t", "1") // fill the channel
	w.send(spillerMsg("t", "2"))       // full + nil spiller -> drop
	if got := w.stats.Dropped(); got != 1 {
		t.Errorf("dropped=%d want 1", got)
	}
	if got := w.stats.Spilled(); got != 0 {
		t.Errorf("spilled=%d want 0 (nil spiller)", got)
	}
}

// ===========================================================================
// kafka_writer.go — daemon error goroutine + onEvent("error") hook
// ===========================================================================

// Test_KafKaWriter_daemon_ErrorOnEvent covers the daemon's error-drainer
// `if k.onEvent != nil { k.onEvent("error", 1) }` branch by driving a real
// (mock) producer that fails every input, with an onEvent hook installed.
func Test_KafKaWriter_daemon_ErrorOnEvent(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	mp := newMockFailingProducer() // local failing producer (no broker)

	var mu sync.Mutex
	seen := map[string]int64{}
	w := &KafKaWriter{
		level:    INFO,
		policy:   OverflowDrop,
		messages: make(chan kafka.Message, 16),
		options:  KafKaWriterOptions{ProducerTopic: "t"},
		producer: mp,
		quit:     make(chan struct{}, 1),
		// daemon calls time.NewTicker(drainInterval); a zero interval panics, so
		// mirror NewKafKaWriter's default.
		drainInterval: 200 * time.Millisecond,
	}
	w.SetOnEvent(func(name string, delta int64) {
		mu.Lock()
		seen[name] += delta
		mu.Unlock()
	})

	w.run.Store(true) // mark running so Stop() proceeds below
	// daemon itself does k.wg.Add(2) for its success/error drainers; no pre-Add.
	go w.daemon()

	// feed a record; the failing producer emits one error.
	if err := w.Write(&Record{level: INFO, msg: "boom"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// wait for the error to be accounted (deterministic polling).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		errs := seen["error"]
		mu.Unlock()
		if errs > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(w.messages)
	<-w.quit // daemon exits via messages-closed path
	_ = mp.Close()
	w.wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if seen["error"] < 1 {
		t.Errorf("onEvent error=%d want >=1", seen["error"])
	}
}

// mockFailingProducer is a minimal sarama.AsyncProducer that emits a
// ProducerError for every message on Input(). No real broker. It exists purely
// to drive the daemon's error-drainer + onEvent hook deterministically.
type mockFailingProducer struct {
	input     chan kafka.Message
	successes chan kafka.Message
	errors    chan *sarama.ProducerError
	quit      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newMockFailingProducer() *mockFailingProducer {
	p := &mockFailingProducer{
		input:     make(chan kafka.Message, 1<<12),
		successes: make(chan kafka.Message, 1<<12),
		errors:    make(chan *sarama.ProducerError, 1<<12),
		quit:      make(chan struct{}),
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case m := <-p.input:
				select {
				case p.errors <- &sarama.ProducerError{Msg: m, Err: errors.New("mock fail")}:
				case <-p.quit:
					return
				}
			case <-p.quit:
				return
			}
		}
	}()
	return p
}

func (p *mockFailingProducer) Input() chan<- kafka.Message     { return p.input }
func (p *mockFailingProducer) Successes() <-chan kafka.Message { return p.successes }
func (p *mockFailingProducer) Errors() <-chan *sarama.ProducerError      { return p.errors }
func (p *mockFailingProducer) Close() error {
	p.closeOnce.Do(func() {
		close(p.quit)
		p.wg.Wait()
		close(p.successes)
		close(p.errors)
	})
	return nil
}
func (p *mockFailingProducer) AsyncClose()                             { _ = p.Close() }
func (p *mockFailingProducer) IsTransactional() bool                   { return false }
func (p *mockFailingProducer) TxnStatus() sarama.ProducerTxnStatusFlag { return 0 }
func (p *mockFailingProducer) BeginTxn() error                         { return errNoopProducerNotTxn }
func (p *mockFailingProducer) CommitTxn() error                        { return errNoopProducerNotTxn }
func (p *mockFailingProducer) AbortTxn() error                         { return errNoopProducerNotTxn }
func (p *mockFailingProducer) AddOffsetsToTxn(map[string][]*sarama.PartitionOffsetMetadata, string) error {
	return errNoopProducerNotTxn
}
func (p *mockFailingProducer) AddOffsetsToTxnWithGroupMetadata(map[string][]*sarama.PartitionOffsetMetadata, *sarama.ConsumerGroupMetadata) error {
	return errNoopProducerNotTxn
}
func (p *mockFailingProducer) AddMessageToTxn(*sarama.ConsumerMessage, string, *string) error {
	return errNoopProducerNotTxn
}
func (p *mockFailingProducer) AddMessageToTxnWithGroupMetadata(*sarama.ConsumerMessage, *sarama.ConsumerGroupMetadata, *string) error {
	return errNoopProducerNotTxn
}

// compile-time: mockFailingProducer is a sarama.AsyncProducer.
var _ sarama.AsyncProducer = (*mockFailingProducer)(nil)

// ===========================================================================
// kafka_writer.go — Start branches: BufferSize default + spill chain fallbacks
// ===========================================================================

// Test_KafKaWriter_Start_ValidVersionOverride covers the SpecifyVersion success
// branch: a sarama-parseable VersionStr sets cfg.Version to the parsed value
// (sarama only accepts certain dotted forms; "0.10.0.1" is one that parses).
func Test_KafKaWriter_Start_ValidVersionOverride(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		SpecifyVersion: true, VersionStr: "0.10.0.1",
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	w.Stop()
}

// Test_KafKaWriter_Start_BufferSizeDefault covers the `if size <= 1 { size =
// 1024 }` default in Start (BufferSize<=1 -> default buffer size).
func Test_KafKaWriter_Start_BufferSizeDefault(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 0, // <= 1 -> default 1024
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	if cap(w.messages) != 1024 {
		t.Errorf("messages cap=%d want 1024 (default)", cap(w.messages))
	}
	w.Stop()
}

// Test_KafKaWriter_Start_SpillChain_FileErrorFallback covers the chain branch
// where NewFileSpiller fails (dir under a regular file) and the spiller falls
// back to ring-only (k.spiller = ring).
func Test_KafKaWriter_Start_SpillChain_FileErrorFallback(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		OverflowPolicy: "spill", SpillType: "", // chain path
		SpillSize: 4, SpillDir: blocker + "/sub", // file spiller fails -> ring fallback
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	if w.spiller == nil {
		t.Fatal("spiller nil after chain-file-error fallback")
	}
	if _, ok := w.spiller.(*RingSpiller[kafka.Message]); !ok {
		t.Errorf("fallback spiller type=%T want *RingSpiller", w.spiller)
	}
	w.Stop()
}

// Test_KafKaWriter_Start_SpillChain_NoDir covers the chain branch where
// SpillDir is empty -> ring-only (k.spiller = ring) without attempting a file
// spiller.
func Test_KafKaWriter_Start_SpillChain_NoDir(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		OverflowPolicy: "spill", SpillType: "", // chain path
		SpillSize: 4, // no SpillDir -> ring only
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	if _, ok := w.spiller.(*RingSpiller[kafka.Message]); !ok {
		t.Errorf("no-dir spiller type=%T want *RingSpiller", w.spiller)
	}
	w.Stop()
}

// ===========================================================================
// kafka_writer.go — Start resume-spilled branch (channel-full push/drop)
// ===========================================================================

// Test_KafKaWriter_Start_ResumeSpillFullChannel covers the Start "resume
// persisted spill" loop, including the channel-full `default` branch where the
// record is re-pushed to the spiller (and dropped if the spiller rejects it).
//
// We pre-seed a FileSpiller's spill.log with more records than the channel can
// hold, so the resume loop overflows the channel and exercises both the
// re-Push success path and (by shrinking the spiller) the IncDropped path.
func Test_KafKaWriter_Start_ResumeSpillFullChannel(t *testing.T) {
	dir := t.TempDir()

	// Sub-test A: channel overflows but the spiller can re-absorb (Push ok).
	t.Run("repush_when_channel_full", func(t *testing.T) {
		sub := t.TempDir()
		// Seed a spill.log directly with framed records (more than BufferSize).
		spillPath := filepath.Join(sub, "spill.log")
		fh, err := os.OpenFile(spillPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		var hdr [4]byte
		oneRec, _ := ProducerMsgCodec.Encode(spillerMsg("t", "seeded"))
		binary.BigEndian.PutUint32(hdr[:], uint32(len(oneRec)))
		for i := 0; i < 8; i++ { // 8 records, BufferSize=2 -> channel fills fast
			_, _ = fh.Write(hdr[:])
			_, _ = fh.Write(oneRec)
		}
		_ = fh.Close()

		w := NewKafKaWriter(KafKaWriterOptions{
			Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
			ProducerTopic: "cov", BufferSize: 2,
			OverflowPolicy: "spill", SpillType: "file", SpillDir: sub, SpillMaxBytes: 1 << 20,
		})
		w.producerFactory = func() (kafka.Producer, error) {
			return newNoopAsyncProducer(), nil
		}
		if err := w.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		waitDaemonRunning(t, w)
		// The resume loop ran: some records went to the channel, the rest were
		// re-pushed to the (large) file spiller -> none dropped.
		if got := w.stats.Dropped(); got != 0 {
			t.Errorf("dropped=%d want 0 (file spiller re-absorbs)", got)
		}
		w.Stop()
	})

	// Sub-test B: channel overflows AND the (tiny) spiller cannot re-absorb ->
	// the IncDropped branch fires. BufferSize=2 avoids the Start "size<=1 ->
	// 1024" default, so 20 seeded records overflow the 2-slot channel; the tiny
	// spiller (maxBytes = exactly one record) accepts one re-Push then rejects
	// the rest, forcing drops.
	t.Run("drop_when_channel_and_spiller_full", func(t *testing.T) {
		sub := t.TempDir()
		spillPath := filepath.Join(sub, "spill.log")
		fh, err := os.OpenFile(spillPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		var hdr [4]byte
		oneRec, _ := ProducerMsgCodec.Encode(spillerMsg("t", "seeded"))
		binary.BigEndian.PutUint32(hdr[:], uint32(len(oneRec)))
		// Write many records into the seeded spill.log.
		for i := 0; i < 20; i++ {
			_, _ = fh.Write(hdr[:])
			_, _ = fh.Write(oneRec)
		}
		_ = fh.Close()

		w := NewKafKaWriter(KafKaWriterOptions{
			Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
			ProducerTopic: "cov", BufferSize: 2, // >1 so no default; small enough to overflow
			OverflowPolicy: "spill", SpillType: "file", SpillDir: sub,
			// maxBytes = exactly one record (4-byte hdr + payload). First re-Push
			// fits; every subsequent re-Push is rejected -> IncDropped.
			SpillMaxBytes: int64(len(hdr) + len(oneRec)),
		})
		w.producerFactory = func() (kafka.Producer, error) {
			return newNoopAsyncProducer(), nil
		}
		if err := w.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		waitDaemonRunning(t, w)
		if got := w.stats.Dropped(); got == 0 {
			t.Errorf("dropped=0 want >0 (channel + tiny spiller both full on resume)")
		}
		w.Stop()
	})

	_ = dir // (kept for symmetry / future shared fixtures)
}

// ===========================================================================
// kafka_writer.go — Stop branches: !running guard + nil spiller
// ===========================================================================

// Test_KafKaWriter_Stop_NotRunning covers the `if !k.run.Load() { return }`
// early-return of Stop (Stop before Start is a no-op).
func Test_KafKaWriter_Stop_NotRunning(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t"})
	// Never started -> run is false. Stop must be a no-op without panicking.
	w.Stop()
	if w.run.Load() {
		t.Error("run should remain false after Stop on never-started writer")
	}
}

// Test_KafKaWriter_Stop_NilSpiller covers the `if k.spiller != nil` false branch
// of Stop: a started writer with OverflowDrop has a nil spiller.
func Test_KafKaWriter_Stop_NilSpiller(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 8, OverflowPolicy: "drop",
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newNoopAsyncProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	if w.spiller != nil {
		t.Fatalf("spiller want nil for drop policy, got %T", w.spiller)
	}
	w.Stop() // exercises the k.spiller == nil guard
}

// Test_KafKaWriter_Stop_ProducerCloseError covers the producer.Close error-log
// branch of Stop, using a producer whose Close always fails.
func Test_KafKaWriter_Stop_ProducerCloseError(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 8,
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newCloseErrorProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	// Must not panic on producer.Close error; it is logged.
	w.Stop()
}

// closeErrorProducer is a no-op AsyncProducer whose Close always returns an
// error, to drive the Stop producer.Close-error log branch.
type closeErrorProducer struct {
	*noopAsyncProducer
}

func newCloseErrorProducer() *closeErrorProducer {
	return &closeErrorProducer{noopAsyncProducer: newNoopAsyncProducer()}
}

func (p *closeErrorProducer) Close() error {
	p.noopAsyncProducer.AsyncClose() // still drain so daemon exits cleanly
	return errors.New("close failed")
}

// compile-time: closeErrorProducer is a sarama.AsyncProducer.
var _ sarama.AsyncProducer = (*closeErrorProducer)(nil)

// ===========================================================================
// kafka_writer.go — Metrics with non-nil spiller
// ===========================================================================

// Test_KafKaWriter_Metrics_WithSpiller covers the `k.spiller != nil ->
// spillLen = k.spiller.Len()` branch of Metrics (previously only the nil path
// was hit). It also confirms queued/spillLen reflect a populated writer.
func Test_KafKaWriter_Metrics_WithSpiller(t *testing.T) {
	ring := NewRingSpiller[kafka.Message](8)
	w := &KafKaWriter{
		level:    INFO,
		policy:   OverflowSpill,
		spiller:  ring,
		messages: make(chan kafka.Message, 4),
	}
	ring.Push(spillerMsg("t", "a"))
	ring.Push(spillerMsg("t", "b"))
	m := w.Metrics()
	if m.SpillLen != 2 {
		t.Errorf("SpillLen=%d want 2", m.SpillLen)
	}
	if m.Queued != 0 {
		t.Errorf("Queued=%d want 0", m.Queued)
	}
}
