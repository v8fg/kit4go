package log4go

import (
	"strings"
	"testing"
	"time"
)

// Test_Logger_Sync verifies Sync drains the pipeline: a record logged before
// Sync reaches the writer by the time Sync returns.
func Test_Logger_Sync(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	cw := &captureWriter{}
	lg.Register(cw)
	lg.SetLevel(DEBUG)
	lg.Info("before sync")
	lg.Sync() // drains + closes
	if cw.Len() == 0 {
		t.Fatal("no record after Sync")
	}
}

// Test_Logger_Panic verifies Panic logs at CRITICAL then re-panics. (No defer
// Close: Panic's own Sync already closes the pipeline.)
func Test_Logger_Panic(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	cw := &captureWriter{}
	lg.Register(cw)
	lg.SetLevel(DEBUG)

	var got any
	func() {
		defer func() { got = recover() }()
		lg.Panic("boom %d", 7)
	}()
	if got == nil {
		t.Fatal("Panic should re-panic")
	}
	if !strings.Contains(got.(string), "boom 7") {
		t.Errorf("panic value=%v want 'boom 7'", got)
	}
}

// Test_Recover verifies Recover logs the panic+stack at CRITICAL and re-raises.
func Test_Recover(t *testing.T) {
	lg := newLoggerWithRecords(make(chan *Record, 4))
	cw := &captureWriter{}
	lg.Register(cw)
	lg.SetLevel(DEBUG)

	func() {
		defer func() { _ = recover() }() // swallow the re-raise
		func() {
			defer Recover(func() *Logger { return lg })
			panic("kaboom")
		}()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && cw.Len() == 0 {
	}
	if cw.Len() == 0 {
		t.Fatal("Recover logged nothing")
	}
	cw.mu.Lock()
	r := cw.records[0]
	cw.mu.Unlock()
	if r.level != CRITICAL {
		t.Errorf("recovered record level=%d want CRITICAL", r.level)
	}
	if !strings.Contains(r.msg, "kaboom") {
		t.Errorf("recovered record msg=%q want to contain kaboom", r.msg)
	}
}
