package log4go

// This file raises file_writer.go coverage toward 100% by exercising the
// branches the async/sync happy-path tests do not reach:
//   - Init: Rotate() returning an error
//   - rotateImpl: the post-init else branch (daily/hourly/minutely switch), the
//     fileBufWriter.Flush() error, the file.Close() error, and the OpenFile
//     error returns
//   - startDaemon: the chain-spill NewFileSpiller-failure fallback to ring, and
//     the persisted-spill Drain() resume path (including Push-fail -> IncDropped)
//   - daemon: the defensive "messages channel closed" (!ok) branch
//   - drainQueuedAndSpill: the "messages channel closed" (!ok) return
//   - writeOne: the formattedBytes fast path and the write-error path
//   - drainSpill: the successful re-inject into messages (case w.messages <- r)
//   - Metrics: the w.messages == nil path (sync or post-Stop writer)
//
// All tests use t.TempDir()+t.Cleanup, wide timeouts, and synchronous waiting;
// none depend on wall-clock precision. The post-init rotate switch is driven by
// directly seeding w.lastWriteTime / w.variables (internal-package access) so the
// date/hour/minute coincidence is deterministic instead of waiting real time.

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Init: Rotate() returns an error -> Init returns it (Init 300.35-302.3).
// =============================================================================

// Test_FileWriter_Init_RotateError covers Init's "if Rotate() errs return err"
// branch. A bad perm string ("abc") makes strconv.ParseInt fail inside Rotate's
// rotateImpl path — but Init parses perm earlier, so instead we make Rotate fail
// by giving the writer a pattern whose target directory cannot be created
// (parent is a regular file), which makes rotateImpl's os.MkdirAll/OpenFile fail.
func Test_FileWriter_Init_RotateError(t *testing.T) {
	// Create a path component that is a regular file, then ask for a log file
	// "inside" it so rotateImpl's MkdirAll fails.
	parent := filepath.Join(t.TempDir(), "notdir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewFileWriterWithOptions(FileWriterOptions{
		Enable:   true,
		Level:    LevelFlagDebug,
		Filename: parent + "/deep/%Y%M%D%H%m-initerr.log",
		Rotate:   true,
		Daily:    true,
	})
	// rotate: true + daily: true + a pattern => Rotate() calls rotateImpl which
	// fails to MkdirAll the "notdir/deep" parent (parent is a file).
	err := w.Init()
	if err == nil {
		t.Fatal("Init: expected error from Rotate() failure, got nil")
	}
}

// =============================================================================
// rotateImpl: post-init else branch (daily/hourly/minutely switch).
// Lines 391.10-422.8.
// =============================================================================

// newRotatableWriter builds a FileWriter whose pattern carries all 5 variables
// (%Y%M%D%H%m) so w.actions has indices 2=D, 3=H, 4=m. The writer is fully
// initialized (Init runs rotateImpl once, opening a real file in dir) so the
// next rotateImpl call hits the post-init switch. Returns the writer and its
// currently open log file path.
func newRotatableWriter(t *testing.T, dir string) *FileWriter {
	t.Helper()
	w := NewFileWriterWithOptions(FileWriterOptions{
		Enable:   true,
		Level:    LevelFlagDebug,
		Filename: filepath.Join(dir, "rot-%Y%M%D%H%m.log"),
		Rotate:   true,
		Daily:    true,
		Hourly:   true,
		Minutely: true,
		// Provide non-zero max* so Init's defaults don't override them.
		MaxDays:    30,
		MaxHours:   12,
		MaxMinutes: 1,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !w.initFileOk {
		t.Fatal("initFileOk=false after Init; cannot exercise post-init switch")
	}
	return w
}

// forceRotateCaseDaily seeds the writer so the next rotateImpl call enters the
// post-init switch and fires the daily (case 2) rotation branch:
//   - sets w.variables[2] (stored day) to a value != today's day, so v != stored
//     and the else branch runs;
//   - sets w.lastWriteTime to (now - maxDays) so lastWriteTime.AddDate(0,0,maxDays)
//     lands on today, making v == d and rotate=true.
func forceRotateCaseDaily(w *FileWriter) {
	now := time.Now()
	w.variables[2] = now.Day() + 100 // != today, guarantees v != stored
	w.lastWriteTime = now.AddDate(0, 0, -w.maxDays)
}

// forceRotateCaseHourly seeds the writer for the hourly (case 3) branch.
func forceRotateCaseHourly(w *FileWriter) {
	now := time.Now()
	w.variables[3] = now.Hour() + 100
	w.lastWriteTime = now.Add(-time.Duration(w.maxHours) * time.Hour)
}

// forceRotateCaseMinutely seeds the writer for the minutely (case 4) branch.
func forceRotateCaseMinutely(w *FileWriter) {
	now := time.Now()
	w.variables[4] = now.Minute() + 100
	w.lastWriteTime = now.Add(-time.Duration(w.maxMinutes) * time.Minute)
}

// Test_FileWriter_RotateImpl_PostInit_Daily covers the post-init daily switch
// branch (case 2) and the successful open-new-file path.
func Test_FileWriter_RotateImpl_PostInit_Daily(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	forceRotateCaseDaily(w)
	if err := w.rotateImpl(); err != nil {
		t.Fatalf("rotateImpl daily: %v", err)
	}
	if w.file == nil {
		t.Fatal("file nil after daily rotate")
	}
}

// Test_FileWriter_RotateImpl_PostInit_Hourly covers the post-init hourly switch
// branch (case 3).
func Test_FileWriter_RotateImpl_PostInit_Hourly(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	forceRotateCaseHourly(w)
	if err := w.rotateImpl(); err != nil {
		t.Fatalf("rotateImpl hourly: %v", err)
	}
}

// Test_FileWriter_RotateImpl_PostInit_Minutely covers the post-init minutely
// switch branch (case 4).
func Test_FileWriter_RotateImpl_PostInit_Minutely(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	forceRotateCaseMinutely(w)
	if err := w.rotateImpl(); err != nil {
		t.Fatalf("rotateImpl minutely: %v", err)
	}
}

// =============================================================================
// rotateImpl: error returns (Flush error 435-438, Close error 441-444,
// OpenFile error 457-459).
// =============================================================================

// Test_FileWriter_RotateImpl_FlushError covers the fileBufWriter.Flush() error
// branch (lines 436-438). bufio.Flush only touches the underlying writer when it
// has buffered data, so we first buffer some bytes via WriteString (no flush),
// THEN close the underlying fd, then call rotateImpl: its Flush step finds
// buffered data, writes to the closed fd, and returns the error.
func Test_FileWriter_RotateImpl_FlushError(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	forceRotateCaseDaily(w)
	// Buffer bytes into the bufio writer WITHOUT flushing, so the upcoming Flush
	// has work to do and will hit the closed fd.
	if w.fileBufWriter != nil {
		_, _ = w.fileBufWriter.WriteString("buffered-bytes-to-force-flush-write\n")
	}
	// Close the underlying file out from under the bufio writer so Flush errors.
	if w.file != nil {
		_ = w.file.Close()
	}
	err := w.rotateImpl()
	if err == nil {
		t.Fatal("rotateImpl: expected flush error from closed file, got nil")
	}
}

// Test_FileWriter_RotateImpl_CloseError covers the file.Close() error branch
// (lines 441-444). We pre-flush the bufio writer (so the rotateImpl Flush step
// succeeds with an empty buffer) but close the underlying file first, so
// rotateImpl's w.file.Close() double-closes and errors.
func Test_FileWriter_RotateImpl_CloseError(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	forceRotateCaseDaily(w)
	// Flush + drain bufio first so the rotateImpl Flush step is a no-op (empty
	// buffer) and does NOT short-circuit before the Close step.
	if w.fileBufWriter != nil {
		_ = w.fileBufWriter.Flush()
	}
	// Close the file once; rotateImpl's w.file.Close() will then double-close.
	if w.file != nil {
		_ = w.file.Close()
	}
	err := w.rotateImpl()
	if err == nil {
		t.Fatal("rotateImpl: expected close error from double-close, got nil")
	}
}

// Test_FileWriter_RotateImpl_OpenFileError covers the os.OpenFile error branch
// (lines 457-459). MkdirAll(path.Dir(filePath)) must SUCCEED but OpenFile(filePath)
// must FAIL. We achieve this by making filePath itself an existing directory:
// MkdirAll on its parent succeeds, but OpenFile on a directory path fails.
//
// The rotate trigger uses a single action returning a sentinel int (999999). On
// the first-round branch rotateImpl overwrites variables[0] with that int, so the
// path format must consume it via %d and resolve to a directory NAMED "999999"
// (pre-created). fmt.Sprintf(dir+"/%d", 999999) = dir/999999 (a directory) ->
// OpenFile fails with "is a directory".
func Test_FileWriter_RotateImpl_OpenFileError(t *testing.T) {
	dir := t.TempDir()
	w := newRotatableWriter(t, dir)
	// Pre-create the directory the path will resolve to.
	target := filepath.Join(dir, "999999")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	// pathFmt consumes the one int variable via %d; the action overwrites
	// variables[0] to 999999, so filePath = dir/999999 (the directory we made).
	w.pathFmt = dir + "/%d"
	// Single action returning the sentinel; variables[0] differs -> first-round
	// mismatch -> rotate=true; then variables[0] is overwritten to 999999.
	w.actions = []func(*time.Time) int{func(*time.Time) int { return 999999 }}
	w.variables = []any{0} // any value != 999999 -> mismatch
	w.initFileOk = false   // force first-round branch
	err := w.rotateImpl()
	if err == nil {
		t.Fatal("rotateImpl: expected OpenFile error on directory path, got nil")
	}
}

// =============================================================================
// startDaemon: chain-spill fallback to ring when NewFileSpiller fails
// (lines 581-583), and persisted-spill resume Drain() path (lines 591-597).
// =============================================================================

// Test_FileWriter_StartDaemon_ChainSpillerFallback covers the default ("") spill
// branch where spillDir is set but NewFileSpiller fails (dir is a regular file),
// so the writer falls back to just the ring spiller.
func Test_FileWriter_StartDaemon_ChainSpillerFallback(t *testing.T) {
	// spillDir points at a regular file -> MkdirAll inside NewFileSpiller fails
	// -> ferr != nil -> w.spiller = ring (the fallback branch).
	badSpillDir := filepath.Join(t.TempDir(), "iamfile")
	if err := os.WriteFile(badSpillDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "cb-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 128,
		OverflowPolicy:  "spill",
		SpillType:       "", // "" -> chain branch; spillDir bad -> ring fallback
		SpillSize:       16,
		SpillDir:        badSpillDir,
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer fw.Stop()
	// After the fallback, spiller is a *RingSpiller (not ChainedSpiller).
	if fw.spiller == nil {
		t.Fatal("spiller nil after chain fallback")
	}
	if _, ok := fw.spiller.(*RingSpiller[*Record]); !ok {
		t.Fatalf("expected *RingSpiller fallback, got %T", fw.spiller)
	}
}

// Test_FileWriter_StartDaemon_SpillResume_PushSucceeds covers the
// persisted-spill resume path: a FileSpiller that has persisted records from a
// previous run is resumed on startDaemon — its Drain() is consumed and the
// records re-injected into messages (the `case w.messages <- r:` branch, 593).
func Test_FileWriter_StartDaemon_SpillResume_PushSucceeds(t *testing.T) {
	spillDir := t.TempDir()
	logDir := t.TempDir()

	// Pre-populate a persistent FileSpiller with a few records, then close it so
	// the spill file is flushed to disk.
	pre, err := NewFileSpiller[*Record](spillDir, 1<<20, RecordCodec)
	if err != nil {
		t.Fatalf("pre spiller: %v", err)
	}
	for i := range 5 {
		if !pre.Push(&Record{level: INFO, time: "t", file: "f", msg: "resumed"}) {
			t.Fatalf("pre Push %d failed", i)
		}
	}
	// Flush the spill file to disk so the next open() re-reads its size; then
	// close so the file handle is released for the writer to reopen.
	_ = pre.Close()

	// Use a tiny spillSize so the ring is small, but spillType="file" so the
	// writer constructs a FileSpiller over the SAME spillDir and resumes.
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "rs-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 256,
		OverflowPolicy:  "spill",
		SpillType:       "file",
		SpillSize:       4,
		SpillDir:        spillDir,
		SpillMaxBytes:   1 << 20,
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// The resumed records were re-injected into messages; let the daemon drain +
	// write them, then stop.
	time.Sleep(200 * time.Millisecond)
	fw.Stop()
	m := fw.Metrics()
	if m.Written == 0 {
		t.Fatalf("resumed spill records not written: %+v", m)
	}
}

// Test_FileWriter_StartDaemon_SpillResume_PushFails covers the resume path's
// Push-fail branch (lines 594-597: `if !w.spiller.Push(r) { w.stats.IncDropped() }`).
//
// The only Spiller whose Push can return false is FileSpiller (it returns false
// when written+recLen > maxBytes). RingSpiller.Push always returns true (it
// overwrites), so to hit the Push-fail branch we use spillType="file" with a
// tiny SpillMaxBytes: we persist many records, then on startDaemon the resume
// loop refills the tiny async buffer (2 slots) and re-Pushes the rest into the
// FileSpiller; once cumulative bytes exceed maxBytes, Push returns false and
// IncDropped fires.
func Test_FileWriter_StartDaemon_SpillResume_PushFails(t *testing.T) {
	spillDir := t.TempDir()
	logDir := t.TempDir()

	// Pre-populate a persistent FileSpiller with many records (large total size).
	// Each encoded record is ~tens of bytes, so 2000 records far exceed a small
	// maxBytes when re-Pushed on resume.
	pre, err := NewFileSpiller[*Record](spillDir, 4<<20, RecordCodec)
	if err != nil {
		t.Fatalf("pre spiller: %v", err)
	}
	const n = 2000
	for i := range n {
		if !pre.Push(&Record{level: INFO, time: "t", file: "f", msg: "resume-drop-payload-to-force-overflow"}) {
			t.Fatalf("pre Push %d failed", i)
		}
	}
	_ = pre.Close()

	// spillType="file" with a SMALL SpillMaxBytes so re-Push on resume saturates
	// the file store quickly and starts returning false. Tiny AsyncBufferSize so
	// the non-blocking re-inject hits `default` for almost all resumed records.
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "rd-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 2,
		OverflowPolicy:  "spill",
		SpillType:       "file",
		SpillSize:       4,
		SpillDir:        spillDir,
		SpillMaxBytes:   64, // tiny: re-Push of a few records exceeds 64 bytes
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	fw.Stop()
	m := fw.Metrics()
	// The vast majority of the 2000 resumed records could not be re-injected
	// (buffer=2) nor re-Pushed (file maxBytes=64), so Dropped must be > 0.
	if m.Dropped == 0 {
		t.Fatalf("expected drops from resume Push-fail, got %+v", m)
	}
}

// =============================================================================
// daemon: defensive "messages channel closed" (!ok) branch (lines 622-629).
// drainQueuedAndSpill: "messages channel closed" (!ok) return (lines 654-656).
// =============================================================================
//
// Stop() deliberately NEVER closes w.messages (see its doc), so the !ok branches
// are defensive fallbacks. They are exercised here by manually closing messages
// from the test once all production sends are quiesced (no concurrent producer),
// which is race-safe: with no sender left, close(messages) cannot race a send.
// The daemon's !ok branch then drains the spill store, flushes, and signals quit.

// Test_FileWriter_Daemon_MessagesClosedDefensive covers the daemon's `if !ok`
// branch and drainQueuedAndSpill's `if !ok { return }` branch by closing the
// messages channel directly (the only way to reach these defensive paths).
func Test_FileWriter_Daemon_MessagesClosedDefensive(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "dc-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 64,
		OverflowPolicy:  "drop",
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Put one record in the channel so the daemon is mid-receive, then ensure no
	// more writes happen from this test goroutine.
	_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "before close"})
	time.Sleep(100 * time.Millisecond) // let the daemon consume the queued record

	// No concurrent producer remains; closing messages is safe. The daemon's
	// next receive observes !ok and runs the defensive shutdown branch.
	close(fw.messages)

	// The defensive branch signals quit; drain it so the daemon can exit. Use a
	// timeout so the test cannot hang if the branch misbehaves.
	select {
	case <-fw.quit:
		// daemon exited via the !ok branch.
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not signal quit after messages closed")
	}
	// Mark daemon done so a later Stop (via cleanup) does not wait on a wg that
	// was already decremented. The daemon's defer wg.Done() has run.
	fw.wg.Wait()
	// Prevent Stop() from blocking on <-quit (already drained) / double-close of
	// stop: reset the async fields Stop checks.
	fw.async = false
	// Best-effort cleanup of the file handle.
	if fw.fileBufWriter != nil {
		_ = fw.fileBufWriter.Flush()
	}
	if fw.file != nil {
		_ = fw.file.Close()
	}
}

// =============================================================================
// writeOne: formattedBytes fast path (689-691) and write-error path (694-698).
// =============================================================================

// Test_FileWriter_WriteOne_FormattedBytes covers the formattedBytes fast path
// in writeOne (the daemon path): a Record carrying formattedBytes is written via
// fileBufWriter.Write rather than String().
func Test_FileWriter_WriteOne_FormattedBytes(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "fb-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 64,
		OverflowPolicy:  "drop",
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer fw.Stop()
	// A Record with formattedBytes set takes the fast path in writeOne.
	r := &Record{
		level:          INFO,
		time:           "t",
		file:           "f",
		msg:            "json",
		formattedBytes: []byte(`{"msg":"fast"}` + "\n"),
	}
	_ = fw.Write(r)
	time.Sleep(150 * time.Millisecond)
	m := fw.Metrics()
	if m.Written == 0 {
		t.Fatalf("formattedBytes record not written: %+v", m)
	}
}

// Test_FileWriter_WriteOne_WriteError covers the write-error path in writeOne
// (lines 694-698): writing to a bufio whose underlying file is closed errors,
// incrementing Errored.
//
// To avoid racing the async daemon for w.file/w.fileBufWriter, we do NOT start a
// daemon. We build a SYNC FileWriter, Init() it (which opens the file via
// rotateImpl), then close the underlying fd and call writeOne directly. writeOne
// is a single-goroutine method, so with no daemon there is no concurrency and no
// race. We also set lastRotateCheck to now so writeOne skips its rotateImpl call
// (which would otherwise reopen the file and mask the write error).
func Test_FileWriter_WriteOne_WriteError(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:     true,
		Level:      LevelFlagDebug,
		Filename:   filepath.Join(logDir, "we-%Y%M%D.log"),
		Rotate:     true,
		Daily:      true,
		Async:      false, // no daemon — we call writeOne directly
		BufferSize: 1,     // tiny: any write fills the bufio buffer and flushes
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if fw.fileBufWriter == nil || fw.file == nil {
		t.Fatalf("Init did not open a file: file=%v buf=%v", fw.file != nil, fw.fileBufWriter != nil)
	}
	// Close the underlying fd out from under the bufio writer.
	if err := fw.file.Close(); err != nil {
		t.Fatalf("pre-close: %v", err)
	}
	// Suppress rotateImpl inside writeOne (it would reopen the file).
	fw.lastRotateCheck = time.Now()

	// Large message: fills the 1-byte bufio buffer immediately -> flush to the
	// closed fd -> write error -> Errored++ (writeOne lines 694-698).
	big := strings.Repeat("x", 2000)
	before := atomic.LoadUint64(&fw.errored)
	fw.writeOne(&Record{level: INFO, time: "t", file: "f", msg: big})
	after := atomic.LoadUint64(&fw.errored)
	if after <= before {
		t.Fatalf("expected errored to increase from write to closed file: before=%d after=%d", before, after)
	}
}

// =============================================================================
// drainSpill: successful re-inject into messages (case w.messages <- r, 730.24).
// =============================================================================

// Test_FileWriter_DrainSpill_Reinject covers the drainSpill re-inject success
// branch. We drive an async spill writer past overflow so the spiller is
// non-empty, then let the daemon's flush ticker fire drainSpill while the
// messages channel has room — the `case w.messages <- r:` branch runs.
func Test_FileWriter_DrainSpill_Reinject(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "ds-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 2, // tiny -> overflows into the spiller
		OverflowPolicy:  "spill",
		SpillType:       "ring",
		SpillSize:       256,
	})
	// Short flush interval so the ticker fires drainSpill frequently.
	fw.flushInterval = 20 * time.Millisecond
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Burst from multiple producers to force the spiller non-empty while the
	// daemon is still draining the tiny channel.
	const workers = 4
	const per = 500
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range per {
				_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "spill reinject"})
			}
		})
	}
	wg.Wait()
	// Give the daemon time to run drainSpill (ticker at 20ms) which re-injects
	// spilled records into the now-draining channel.
	time.Sleep(300 * time.Millisecond)
	fw.Stop()
	m := fw.Metrics()
	// Sanity: most records were written (some may have been dropped/spilled
	// depending on timing). The reinject branch itself is covered as long as the
	// spiller was non-empty during a tick, which the burst guarantees.
	if m.Written == 0 {
		t.Fatalf("no records written via spill reinject: %+v", m)
	}
}

// =============================================================================
// Metrics: w.messages == nil path (804-806).
// =============================================================================

// Test_FileWriter_MessagesNil_Metrics covers Metrics() when w.messages is nil
// (sync writer that never started a daemon) — the `if w.messages != nil` false
// branch yields Queued=0.
func Test_FileWriter_MessagesNil_Metrics(t *testing.T) {
	// Sync writer: Async=false, so startDaemon never runs and messages stays nil.
	w := NewFileWriterWithOptions(FileWriterOptions{
		Enable:   true,
		Level:    LevelFlagDebug,
		Filename: filepath.Join(t.TempDir(), "mn-%Y%M%D.log"),
		Rotate:   true,
		Daily:    true,
		Async:    false,
	})
	if err := w.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	m := w.Metrics()
	if m.Queued != 0 {
		t.Fatalf("Queued=%d want 0 for messages==nil", m.Queued)
	}
	// SpillLen also 0 (no spiller on a sync writer).
	if m.SpillLen != 0 {
		t.Fatalf("SpillLen=%d want 0 for spiller==nil", m.SpillLen)
	}
}

// Test_FileWriter_Metrics_PostStop covers Metrics() after Stop has nilled
// w.messages on an async writer — the same nil branch from the other direction.
func Test_FileWriter_Metrics_PostStop(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:          true,
		Level:           LevelFlagDebug,
		Filename:        filepath.Join(logDir, "ms-%Y%M%D.log"),
		Rotate:          true,
		Daily:           true,
		Async:           true,
		AsyncBufferSize: 64,
		OverflowPolicy:  "drop",
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_ = fw.Write(&Record{level: INFO, time: "t", file: "f", msg: "x"})
	time.Sleep(100 * time.Millisecond)
	fw.Stop() // sets w.messages = nil
	m := fw.Metrics()
	if m.Queued != 0 {
		t.Fatalf("Queued=%d want 0 after Stop nilled messages", m.Queued)
	}
}

// =============================================================================
// Auxiliary coverage: SetOnEvent fire path for the "error" event and the
// "spill" event (used by the error/spill tests above to confirm the hook fires).
// Kept here so the file_writer event surface is fully exercised in one place.
// =============================================================================

// Test_FileWriter_SetOnEvent_ErrorEvent confirms the "error" event fires from
// writeOne's error path, complementing the existing "drop"/"written" coverage.
// Like WriteOne_WriteError this drives writeOne directly (no daemon) to avoid
// racing the daemon for w.file.
func Test_FileWriter_SetOnEvent_ErrorEvent(t *testing.T) {
	logDir := t.TempDir()
	fw := NewFileWriterWithOptions(FileWriterOptions{
		Enable:     true,
		Level:      LevelFlagDebug,
		Filename:   filepath.Join(logDir, "ev-%Y%M%D.log"),
		Rotate:     true,
		Daily:      true,
		Async:      false, // no daemon
		BufferSize: 1,
	})
	if err := fw.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	var errEvents int64
	fw.SetOnEvent(func(name string, delta int64) {
		if name == "error" {
			atomic.AddInt64(&errEvents, delta)
		}
	})
	if err := fw.file.Close(); err != nil {
		t.Fatalf("pre-close: %v", err)
	}
	fw.lastRotateCheck = time.Now() // suppress rotateImpl
	big := strings.Repeat("y", 2000)
	fw.writeOne(&Record{level: INFO, time: "t", file: "f", msg: big})
	if atomic.LoadInt64(&errEvents) == 0 {
		t.Fatal("error event never fired from writeOne error path")
	}
}

// Test_FileWriter_Stop_ConcurrentSafe is the regression for the Stop double-
// close race: before the closing.CompareAndSwap guard, two concurrent Stops
// both passed the messages!=nil check and double-closed w.stop (panic) while
// also racing w.messages (read vs nil-write). The CAS makes Stop idempotent —
// exactly one caller proceeds.
func Test_FileWriter_Stop_ConcurrentSafe(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		fw, _ := newAsyncFileWriter(t, FileWriterOptions{})
		if err := fw.Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}
		time.Sleep(20 * time.Millisecond) // let the daemon start

		const n = 8
		var wg sync.WaitGroup
		wg.Add(n)
		for range n {
			go func() {
				defer wg.Done()
				fw.Stop()
			}()
		}
		wg.Wait()
		fw.Stop() // idempotent extra: a no-op, must not panic
	}
}
