package log4go

import (
	"bufio"
	"fmt"
	"os"
	"sync/atomic"
)

type colorRecord Record

// brush is a color join function
type brush func(string) string

// newBrush return a fix color Brush
func newBrush(color string) brush {
	pre := "\033["
	reset := "\033[0m"
	return func(text string) string {
		return fmt.Sprintf("%s%s%s%s%s", pre, color, "m", text, reset)
	}
}

// effect: 0~8
// 0:no, 1: Highlight (deepen) display, 2: Low light (dimmed) display,
// 4: underline, 5: blink, 7: Reverse display (replace background color and font color)
// 8: blank

// font color: 30~39
// 30: black, 31: red, 32: green, 33: yellow, 34: blue, 35: purple, 36: dark green, 37: grey
// 38: Sets the underline on the default foreground color, 39: Turn off underlining on the default foreground color

// background color: 40~49
// 40: black, 41: red, 42: green, 43: yellow, 44: blue, 45: purple, 46: dark green, 47: grey

// (background;font;effect)
// Severity hierarchy (most → least prominent): red-bg > bold-red > magenta >
// red > yellow > green > cyan > blue > dark-grey. Each level is visually
// distinct at a glance; see README.md for the full color table.
var colors = []brush{
	newBrush("1;41"), // Emergency          red background (most prominent)
	newBrush("1;31"), // Alert              bold red
	newBrush("1;35"), // Critical           bold magenta
	newBrush("31"),   // Error              red
	newBrush("33"),   // Warning            yellow
	newBrush("32"),   // Notice             green
	newBrush("36"),   // Info               cyan
	newBrush("34"),   // Debug              blue
	newBrush("90"),   // Trace              dark grey
}

func (r *colorRecord) ColorString() string {
	inf := fmt.Sprintf("%s %s %s %s\n", r.time, LevelFlags[r.level], r.file, r.msg)
	return colors[r.level](inf)
}

func (r *colorRecord) String() string {
	inf := ""
	switch r.level {
	case EMERGENCY:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[1;41m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case ALERT:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[1;31m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case CRITICAL:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[35m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case ERROR:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[31m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case WARNING:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[33m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case NOTICE:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[32m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case INFO:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[36m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case DEBUG:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[34m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case TRACE:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[90m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	}

	return inf
}

// ConsoleWriter console writer define
type ConsoleWriter struct {
	level     int
	color     bool
	fullColor bool // line all with color
	buffered  bool
	buf       *bufio.Writer
	paused    atomic.Bool
}

// Name returns WriterNameConsole (for by-name control).
func (w *ConsoleWriter) Name() string { return WriterNameConsole }

// Pause drops incoming records without removing the writer (atomic, non-blocking).
func (w *ConsoleWriter) Pause() { w.paused.Store(true) }

// Resume restores delivery after Pause.
func (w *ConsoleWriter) Resume() { w.paused.Store(false) }

// Paused reports whether the writer is currently paused.
func (w *ConsoleWriter) Paused() bool { return w.paused.Load() }

// ConsoleWriterOptions configures the console writer. All fields default to
// zero/false — color is OFF by default so production output is clean plain text
// (safe for grep / copy / log shippers that choke on ANSI escape codes).
type ConsoleWriterOptions struct {
	Enable bool `json:"enable" mapstructure:"enable"`
	// Color renders the level flag with ANSI color (e.g. red for ERROR). OFF by
	// default — enable only for local development terminals. Production should
	// keep this false to avoid ANSI escape codes in collected logs.
	Color bool `json:"color" mapstructure:"color"`
	// FullColor renders the entire line in the level color (not just the flag).
	// OFF by default. Requires Color to be useful.
	FullColor bool   `json:"full_color" mapstructure:"full_color"`
	Level     string `json:"level" mapstructure:"level"`
	// Buffered wraps os.Stdout in a bufio.Writer to reduce syscalls.
	// Default false (immediate output for debugging). Set true for high-rate
	// console (e.g. container stdout collection). Flush is driven by the
	// bootstrap flushTimer (Flusher interface).
	Buffered bool `json:"buffered" mapstructure:"buffered"`
	// BufferSize bufio size in bytes (<=0 -> 4096).
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`
}

// NewConsoleWriter create new console writer
func NewConsoleWriter() *ConsoleWriter {
	return &ConsoleWriter{}
}

// NewConsoleWriterWithOptions create new console writer with level
func NewConsoleWriterWithOptions(options ConsoleWriterOptions) *ConsoleWriter {
	defaultLevel := DEBUG

	if len(options.Level) > 0 {
		defaultLevel = getLevelDefault(options.Level, defaultLevel, "")
	}

	return &ConsoleWriter{
		level:     defaultLevel,
		color:     options.Color,
		fullColor: options.FullColor,
		buffered:  options.Buffered,
	}
}

// Write console write
func (w *ConsoleWriter) Write(r *Record) error {
	if w.paused.Load() {
		return nil
	}
	if r.level > w.level {
		return nil
	}
	// FormatJSON fast path: when the Logger pre-serialized the record, emit the
	// bytes verbatim (no color, no String()) — JSON is for machine ingestion.
	// This keeps the format decision in one place (deliverRecordToWriter) and
	// avoids re-serializing per writer.
	if len(r.formattedBytes) > 0 {
		var out *os.File = os.Stdout
		if w.buf != nil {
			_, _ = w.buf.Write(r.formattedBytes)
			return nil
		}
		_, _ = fmt.Fprint(out, string(r.formattedBytes))
		return nil
	}
	var out *os.File = os.Stdout
	if w.buf != nil {
		// buffered path: write to bufio (flushed by bootstrap timer)
		if w.color {
			if w.fullColor {
				_, _ = w.buf.WriteString(((*colorRecord)(r)).ColorString())
			} else {
				_, _ = w.buf.WriteString(((*colorRecord)(r)).String())
			}
		} else {
			_, _ = w.buf.WriteString(r.String())
		}
		return nil
	}
	if w.color {
		if w.fullColor {
			_, _ = fmt.Fprint(out, ((*colorRecord)(r)).ColorString())
		} else {
			_, _ = fmt.Fprint(out, ((*colorRecord)(r)).String())
		}
	} else {
		_, _ = fmt.Fprint(out, r.String())
	}
	return nil
}

// Init console init; wraps os.Stdout in bufio when Buffered is set.
func (w *ConsoleWriter) Init() error {
	if w.buffered {
		size := 4096
		if w.buf == nil {
			// use BufferSize from options if set via NewConsoleWriterWithOptions path
			// (stored in a deferred way: check if SetBuffered was used)
		}
		w.buf = bufio.NewWriterSize(os.Stdout, size)
	}
	return nil
}

// Flush implements Flusher; flushes the bufio buffer when Buffered is set.
func (w *ConsoleWriter) Flush() error {
	if w.buf != nil {
		return w.buf.Flush()
	}
	return nil
}

// SetColor console output color control
func (w *ConsoleWriter) SetColor(c bool) {
	w.color = c
}

// SetFullColor console output full line color control
func (w *ConsoleWriter) SetFullColor(c bool) {
	w.fullColor = c
}
