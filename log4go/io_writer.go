package log4go

import (
	"fmt"
	"io"
	"sync/atomic"
)

// IOWriter adapts any io.Writer (bytes.Buffer, *os.File, a network conn, an
// io.Pipe writer, a test buffer, ...) to the log4go Writer interface. It is the
// thinnest possible adapter: Write renders the record (formattedBytes when the
// Logger pre-serialized for FormatJSON, else Record.String()) with fmt.Fprint.
//
// IOWriter is SYNCHRONOUS — the bootstrap goroutine calls the underlying
// io.Writer directly on each record. This is fine for fast sinks (an in-memory
// buffer, a local pipe) but will back-pressure the whole logger pipeline if the
// io.Writer is slow (a blocking network conn, a full pipe). For slow/remote
// sinks wrap the writer in an async buffer yourself, or use NetWriter / Kafka
// which are async + bounded by design.
//
// Typical use: testing (capture output into a bytes.Buffer), or wiring log4go
// into an existing io.Writer-based sink owned by the application.
type IOWriter struct {
	w      io.Writer
	level  int
	paused atomic.Bool
}

// Name returns WriterNameIO.
func (i *IOWriter) Name() string { return WriterNameIO }

// Pause drops incoming records without removing the writer.
func (i *IOWriter) Pause() { i.paused.Store(true) }

// Resume restores delivery after Pause.
func (i *IOWriter) Resume() { i.paused.Store(false) }

// Paused reports whether the writer is currently paused.
func (i *IOWriter) Paused() bool { return i.paused.Load() }

// NewIOWriter wraps w so records at or below level are written to it. The
// caller retains ownership of w; IOWriter does not close it (closing is the
// caller's responsibility, matching io.Writer conventions).
func NewIOWriter(w io.Writer, level int) *IOWriter {
	return &IOWriter{w: w, level: level}
}

// Init is a no-op (the io.Writer is already open). Present to satisfy the
// Writer interface.
func (i *IOWriter) Init() error { return nil }

// Write renders r and writes it to the underlying io.Writer. It honors the
// Logger's format: when r.formattedBytes is set (FormatJSON) the pre-serialized JSON
// is written verbatim, otherwise the text String() form is written.
func (i *IOWriter) Write(r *Record) error {
	if i.paused.Load() {
		return nil
	}
	if r.level > i.level {
		return nil
	}
	if len(r.formattedBytes) > 0 {
		_, err := i.w.Write(r.formattedBytes)
		return err
	}
	_, err := fmt.Fprint(i.w, r.String())
	return err
}

// compile-time: IOWriter implements Writer.
var _ Writer = (*IOWriter)(nil)
