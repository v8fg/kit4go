package log4go

import (
	"os"
	"path/filepath"
	"testing"

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

// TestReload_ResetsRuntimeState proves Reload resets state that is NOT part of
// LogConfig: base fields set via the API (SetBaseField) do not carry over to the
// fresh logger. The host must re-apply them after Reload if still wanted.
func TestReload_ResetsRuntimeState(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer Close()
	Close()

	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	SetBaseField("hostname", "host-1")
	if got := baseFieldCount(loggerDefault.Load()); got != 1 {
		t.Fatalf("base field not set: got %d want 1", got)
	}

	if err := Reload(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := baseFieldCount(loggerDefault.Load()); got != 0 {
		t.Fatalf("Reload must reset base fields (not in LogConfig): got %d want 0", got)
	}
}
