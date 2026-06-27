package log4go

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// These tests verify Reload/ReloadFile: the singleton swaps atomically, old
// writers are drained+stopped (no goroutine/handle leak via goleak), and a bad
// config leaves the running logger untouched.
//
// Each test uses the defer order: goleak.VerifyNone registered first (runs last),
// Close() registered second (runs before goleak, stopping the singleton's
// writers), and a leading Close() to reset any state a prior test left.

// hasFileWriter reports whether l currently has a *FileWriter registered.
func hasFileWriter(l *Logger) bool {
	for _, w := range l.snapshotWriters() {
		if _, ok := w.(*FileWriter); ok {
			return true
		}
	}
	return false
}

// hasConsoleWriter reports whether l currently has a *ConsoleWriter registered.
func hasConsoleWriter(l *Logger) bool {
	for _, w := range l.snapshotWriters() {
		if _, ok := w.(*ConsoleWriter); ok {
			return true
		}
	}
	return false
}

// baseFieldCount returns the number of base fields set on l (via SetBaseField).
// Reload builds a fresh logger, so base fields — which are not part of LogConfig
// — are reset to zero; this helper verifies that full-replace consequence.
func baseFieldCount(l *Logger) int {
	if p := l.baseFields.v.Load(); p != nil {
		return len(*p)
	}
	return 0
}

// TestReload_SwapsAndStopsOld verifies Reload swaps in a fresh logger with the new
// writer set, and that the previous logger's bootstrap + writers are stopped
// (proven by goleak finding no survivors).
func TestReload_SwapsAndStopsOld(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	old := loggerDefault.Load()
	if old == nil {
		t.Fatal("no logger after SetupLog")
	}

	fwPath := filepath.Join(t.TempDir(), "reload.log")
	err := Reload(LogConfig{
		Level:         "info",
		ConsoleWriter: ConsoleWriterOptions{Enable: true},
		FileWriter:    FileWriterOptions{Enable: true, Filename: fwPath, Async: true, AsyncBufferSize: 8},
	})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	cur := loggerDefault.Load()
	if cur == old {
		t.Fatal("Reload did not swap the singleton")
	}
	if !hasFileWriter(cur) {
		t.Fatal("new logger has no FileWriter after Reload")
	}
	// old logger was swapped out and Close()d inside Reload; the deferred Close()
	// stops `cur`. goleak asserts neither the old nor new daemon survived.
}

// TestReload_BadConfigKeepsOld verifies a failing Reload (a kafka writer that
// cannot start) returns an error and leaves the running singleton untouched.
func TestReload_BadConfigKeepsOld(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	before := loggerDefault.Load()

	// kafka enabled with no brokers -> Start fails -> Reload must keep `before`.
	err := Reload(LogConfig{Level: "info", KafKaWriter: KafKaWriterOptions{Enable: true}})
	if err == nil {
		t.Fatal("expected Reload to fail on a kafka writer that cannot start")
	}
	if loggerDefault.Load() != before {
		t.Fatal("Reload swapped the singleton despite failure; the old logger should be retained")
	}
}

// TestReload_FileWriterInitFailure covers the file-writer init-error branch of
// applyConfig: a sync FileWriter whose file cannot be created (parent path is a
// regular file -> Rotate's OpenFile fails) makes Reload fail and keep the old
// logger. Also asserts the partial fresh logger is cleaned up (no leak).
func TestReload_FileWriterInitFailure(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	before := loggerDefault.Load()

	// Poison the parent dir: a regular file cannot hold a child file.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Reload(LogConfig{
		Level: "info",
		FileWriter: FileWriterOptions{
			Enable: true,
			// %Y%M%D populates rotation actions so rotateImpl runs on Init and
			// reaches MkdirAll(parent). The parent (blocker) is a regular file,
			// so MkdirAll fails with ENOTDIR -> Init returns the error.
			Filename: filepath.Join(blocker, "f-%Y%M%D.log"),
			Rotate:   true,
		},
	})
	if err == nil {
		t.Fatal("expected Reload to fail when FileWriter cannot init")
	}
	if loggerDefault.Load() != before {
		t.Fatal("Reload swapped the singleton despite FileWriter init failure")
	}
}

// TestReloadFile_AppliesAndRejectsBad verifies ReloadFile applies a good config
// and rejects a missing file / malformed JSON without swapping the singleton.
func TestReloadFile_AppliesAndRejectsBad(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	good := filepath.Join(t.TempDir(), "log.json")
	if err := os.WriteFile(good, []byte(`{"level":"info","console_writer":{"enable":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReloadFile(good); err != nil {
		t.Fatalf("ReloadFile good: %v", err)
	}
	before := loggerDefault.Load()

	if err := ReloadFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected ReloadFile to fail on a missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReloadFile(bad); err == nil {
		t.Fatal("expected ReloadFile to fail on malformed JSON")
	}
	if loggerDefault.Load() != before {
		t.Fatal("ReloadFile failure swapped the singleton; the old logger should be retained")
	}
}

// TestReload_FullReplace_NotAMerge proves Reload is a FULL replace of the writer
// set, not a merge: reloading with a subset of writers removes the ones no longer
// enabled and stops their daemons. (console+file) -> console-only must leave the
// file writer gone and its daemon stopped (goleak), with console retained.
func TestReload_FullReplace_NotAMerge(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	fwPath := filepath.Join(t.TempDir(), "full.log")
	if err := SetupLog(LogConfig{
		Level:         "info",
		ConsoleWriter: ConsoleWriterOptions{Enable: true},
		FileWriter:    FileWriterOptions{Enable: true, Filename: fwPath, Async: true, AsyncBufferSize: 8},
	}); err != nil {
		t.Fatal(err)
	}
	before := loggerDefault.Load()
	if !hasConsoleWriter(before) || !hasFileWriter(before) {
		t.Fatal("setup: console+file expected")
	}

	// Reload with console ONLY — file must disappear (full replace, not merge).
	if err := Reload(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	cur := loggerDefault.Load()
	if cur == before {
		t.Fatal("Reload did not swap the singleton")
	}
	if !hasConsoleWriter(cur) {
		t.Fatal("console writer missing after reload")
	}
	if hasFileWriter(cur) {
		t.Fatal("Reload merged writers: file writer should be gone after a full replace")
	}
	// goleak (deferred) asserts the old file daemon was stopped, not orphaned.
}

// TestReload_PreservesRuntimeState proves Reload PRESERVES state that is NOT part
// of LogConfig: base fields (SetBaseField) and the caller/func toggles set at init
// or for debugging survive a config reload (inheritRuntimeState). Only the writer
// set, level, format and full-path are re-applied from the config.
func TestReload_PreservesRuntimeState(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	SetBaseField("hostname", "host-1")
	defaultLogger().WithCaller(false) // a throughput/debug toggle set at runtime
	if got := baseFieldCount(loggerDefault.Load()); got != 1 {
		t.Fatalf("base field not set: got %d want 1", got)
	}

	if err := Reload(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	cur := loggerDefault.Load()
	if got := baseFieldCount(cur); got != 1 {
		t.Fatalf("Reload must preserve base fields: got %d want 1", got)
	}
	if cur.hasCaller.Load() {
		t.Fatal("Reload must preserve WithCaller(false)")
	}
}

// TestBootstrap_LegacyRecordsClose covers the defensive `!ok` arms of
// bootstrapLogWriter for an externally-closed records channel (records is never
// closed by log4go itself — it retires via quit — but the arms guard legacy
// callers). (a) main loop: records closed mid-stream. (b) drainAndExit: records
// closed while the shutdown drain is reaping a backlog.
func TestBootstrap_LegacyRecordsClose(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// (a) main-loop !ok: close records while the bootstrap is running.
	records := make(chan *Record, 4)
	l := newLoggerWithRecords(records)
	records <- &Record{level: INFO, msg: "x"}
	time.Sleep(20 * time.Millisecond) // let the bootstrap enter the main loop
	close(records)
	select {
	case <-l.c:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not exit after records closed (main loop)")
	}

	// (b) drainAndExit !ok: fill a backlog, retire via quit, then close records
	// while the drain is reaping — the drain's receive observes the close.
	records2 := make(chan *Record, 512)
	l2 := newLoggerWithRecords(records2)
	for i := 0; i < 400; i++ {
		records2 <- &Record{level: INFO, msg: "backlog"}
	}
	time.Sleep(20 * time.Millisecond) // let the bootstrap drain some
	close(l2.quit)                    // retire -> drainAndExit reaps the backlog
	close(records2)                   // drain observes closed records -> !ok
	select {
	case <-l2.c:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not exit after records closed (drain)")
	}
}
