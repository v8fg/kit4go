package log4go

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestWriter_PauseResume_Name covers Name/Pause/Resume/Paused and the Write
// pause-check branch for every built-in writer. Write while paused returns nil
// before any I/O, so no daemon/connection/file is needed.
func TestWriter_PauseResume_Name(t *testing.T) {
	cases := []struct {
		kind     string
		w        Writer
		wantName string
	}{
		{"console", &ConsoleWriter{level: DEBUG}, WriterNameConsole},
		{"file", &FileWriter{level: DEBUG}, WriterNameFile},
		{"kafka", &KafKaWriter{level: DEBUG}, WriterNameKafka},
		{"net", &NetWriter{level: DEBUG}, WriterNameNet},
		{"io", &IOWriter{w: io.Discard, level: DEBUG}, WriterNameIO},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			p := c.w.(Pauser)
			n := c.w.(Named)
			if got := n.Name(); got != c.wantName {
				t.Errorf("Name=%q want %q", got, c.wantName)
			}
			if p.Paused() {
				t.Error("new writer should not be paused")
			}
			p.Pause()
			if !p.Paused() {
				t.Error("Pause() did not pause")
			}
			// Write while paused hits the pause-check branch and returns nil
			// without touching any sink.
			if err := c.w.Write(&Record{level: INFO, msg: "dropped"}); err != nil {
				t.Errorf("paused Write returned err: %v", err)
			}
			p.Resume()
			if p.Paused() {
				t.Error("Resume() did not resume")
			}
		})
	}
}

// TestWriter_PauseDrops verifies a paused writer produces no output and Resume
// restores delivery (using IOWriter, whose output is capturable).
func TestWriter_PauseDrops(t *testing.T) {
	buf := &bytes.Buffer{}
	w := &IOWriter{w: buf, level: DEBUG}
	r := &Record{level: INFO, msg: "hi", time: "t"}

	w.Pause()
	for i := 0; i < 10; i++ {
		_ = w.Write(r)
	}
	if buf.Len() != 0 {
		t.Fatalf("paused writer wrote %d bytes, want 0", buf.Len())
	}
	w.Resume()
	_ = w.Write(r)
	if buf.Len() == 0 {
		t.Fatal("resumed writer wrote nothing")
	}
}

// TestLogger_PauseWriterByName covers the by-name convenience on a Logger.
func TestLogger_PauseWriterByName(t *testing.T) {
	l := newLoggerWithRecords(make(chan *Record, 4))
	defer l.Close()
	l.Register(&ConsoleWriter{level: DEBUG})

	if len(l.Writers()) != 1 {
		t.Fatalf("Writers len=%d want 1", len(l.Writers()))
	}
	if !l.PauseWriter(WriterNameConsole) {
		t.Fatal("PauseWriter(console) returned false")
	}
	if !l.WriterPaused(WriterNameConsole) {
		t.Fatal("WriterPaused(console) = false, want true")
	}
	if !l.ResumeWriter(WriterNameConsole) {
		t.Fatal("ResumeWriter(console) returned false")
	}
	if l.WriterPaused(WriterNameConsole) {
		t.Fatal("WriterPaused(console) = true after Resume")
	}
	if l.PauseWriter("does-not-exist") {
		t.Error("PauseWriter(unknown) should return false")
	}
	if l.ResumeWriter("does-not-exist") {
		t.Error("ResumeWriter(unknown) should return false")
	}
	if l.WriterPaused("does-not-exist") {
		t.Error("WriterPaused(unknown) should return false")
	}
}

// TestPackage_PauseWriter covers the package-level by-name control on the
// singleton.
func TestPackage_PauseWriter(t *testing.T) {
	defer Close()
	Close()
	if err := SetupLog(LogConfig{Level: "info", ConsoleWriter: ConsoleWriterOptions{Enable: true}}); err != nil {
		t.Fatal(err)
	}
	if !PauseWriter(WriterNameConsole) {
		t.Fatal("package PauseWriter failed")
	}
	if !WriterPaused(WriterNameConsole) {
		t.Fatal("package WriterPaused = false")
	}
	if !ResumeWriter(WriterNameConsole) {
		t.Fatal("package ResumeWriter failed")
	}
	if len(Writers()) != 1 {
		t.Errorf("package Writers len=%d want 1", len(Writers()))
	}
}

// TestWriter_PauseResume_Concurrent proves Pause/Resume/Write are race-free
// under concurrent access (the ad-tech guarantee).
func TestWriter_PauseResume_Concurrent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	w := &IOWriter{w: io.Discard, level: DEBUG}

	var stop atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for !stop.Load() {
				if i%2 == 0 {
					w.Pause()
				} else {
					w.Resume()
				}
				_ = w.Write(&Record{level: INFO, msg: "x"})
			}
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}
