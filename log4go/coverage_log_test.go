package log4go

import (
	"testing"
	"time"
)

// Cover log.go:1495 — the `<-l.quit` arm of enqueue's select. enqueue normally
// sends on records (blocking), but when the logger is retired (quit closed) and
// the records channel is full, the quit case fires and the record is dropped
// instead of deadlocking. Construct a Logger with a full records channel and a
// closed quit so the quit case is the only ready one.
func Test_Logger_enqueue_QuitDropsRecordWhenFull(t *testing.T) {
	l := &Logger{
		records: make(chan *Record, 1), // capacity 1
		quit:    make(chan struct{}),
	}
	l.records <- &Record{msg: "filler"} // fill the channel so the send cannot proceed
	close(l.quit)                       // retire the logger -> quit case is ready

	// enqueue must select the quit case (non-blocking) rather than block on the
	// full records channel.
	done := make(chan struct{})
	go func() {
		l.enqueue(&Record{msg: "dropped"})
		close(done)
	}()
	select {
	case <-done:
		// expected: enqueue returned via the quit case
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue blocked instead of selecting the quit case")
	}
	// the dropped record never reached the channel
	if len(l.records) != 1 {
		t.Fatalf("records len=%d want 1 (dropped via quit)", len(l.records))
	}
}

// NOTE on log.go:277 — the `return ""` fallback inside the log4goPkgDir init
// closure is unreachable defensive code: runtime.FuncForPC().FileLine always
// returns a file path containing a path separator ('/'), so the
// `strings.LastIndexByte(file, '/')` guard at log.go:274 never misses on any
// supported platform. Verified empirically; left in place as a guard against a
// degenerate runtime. Not covered by design.
