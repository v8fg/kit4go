package log4go

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestSetupLog_FileWriterInitFailure covers applyConfig's file-writer init-error
// branch: a sync FileWriter whose file cannot be created (parent path is a
// regular file -> Rotate's MkdirAll fails) makes SetupLog return an error.
func TestSetupLog_FileWriterInitFailure(t *testing.T) {
	dl := defaultLogger() // ensure the singleton (and its bootstrap) exist first,
	// so the goleak snapshot below captures them rather than a recreation.
	saved := dl.snapshotWriters()
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	defer dl.writers.Store(saved)

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SetupLog(LogConfig{
		Level: "info",
		FileWriter: FileWriterOptions{
			Enable: true,
			// %Y%M%D populates rotation actions so rotateImpl runs on Init and
			// reaches MkdirAll(parent). The parent (blocker) is a regular file,
			// so MkdirAll fails -> Init returns the error.
			Filename: filepath.Join(blocker, "f-%Y%M%D.log"),
			Rotate:   true,
		},
	})
	if err == nil {
		t.Fatal("expected SetupLog to fail when FileWriter cannot init")
	}
}

// drainSlowWriter sleeps in Write so the shutdown drain stays busy long enough for the
// test to close the records channel mid-drain (covering drainAndExit's !ok arm).
type drainSlowWriter struct{}

func (drainSlowWriter) Init() error         { return nil }
func (drainSlowWriter) Write(*Record) error { time.Sleep(time.Millisecond); return nil }

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
	l.Register(drainSlowWriter{})
	records <- &Record{level: INFO, msg: "x"}
	time.Sleep(20 * time.Millisecond) // let the bootstrap enter the main loop
	close(records)
	select {
	case <-l.c:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not exit after records closed (main loop)")
	}

	// (b) drainAndExit !ok: a slow writer keeps the drain busy reaping a backlog
	// while we close records underneath it, so the drain's receive observes the
	// close (ok=false) rather than exiting via default.
	records2 := make(chan *Record, 512)
	l2 := newLoggerWithRecords(records2)
	l2.Register(drainSlowWriter{})
	for i := 0; i < 400; i++ {
		records2 <- &Record{level: INFO, msg: "backlog"}
	}
	time.Sleep(20 * time.Millisecond) // let the bootstrap drain some (slowly)
	close(l2.quit)                    // retire -> drainAndExit reaps the backlog
	time.Sleep(20 * time.Millisecond) // ensure the drain is mid-reap
	close(records2)                   // drain observes closed records -> !ok
	select {
	case <-l2.c:
	case <-time.After(2 * time.Second):
		t.Fatal("bootstrap did not exit after records closed (drain)")
	}
}
