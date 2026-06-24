package log4go

import (
	"fmt"
	"os"
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
var colors = []brush{
	newBrush("1;31"), // Emergency          red
	newBrush("1;36"), // Alert              dark green
	newBrush("1;35"), // Critical           purple
	newBrush("1;31"), // Error              red
	newBrush("1;33"), // Warning            yellow
	newBrush("1;32"), // Notice             green
	newBrush("1;34"), // Informational      blue
	newBrush("2;37"), // Debug              grey
}

func (r *colorRecord) ColorString() string {
	inf := fmt.Sprintf("%s %s %s %s\n", r.time, LevelFlags[r.level], r.file, r.msg)
	return colors[r.level](inf)
}

func (r *colorRecord) String() string {
	inf := ""
	switch r.level {
	case EMERGENCY:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[31m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case ALERT:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[36m%s\033[0m] \033[47;30m%s\033[0m %s\n",
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
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[34m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	case DEBUG:
		inf = fmt.Sprintf("\033[36m%s\033[0m [\033[44m%s\033[0m] \033[47;30m%s\033[0m %s\n",
			r.time, LevelFlags[r.level], r.file, r.msg)
	}

	return inf
}

// ConsoleWriter console writer define
type ConsoleWriter struct {
	level     int
	color     bool
	fullColor bool // line all with color
}

// ConsoleWriterOptions color field options
type ConsoleWriterOptions struct {
	Enable    bool   `json:"enable" mapstructure:"enable"`
	Color     bool   `json:"color" mapstructure:"color"`
	FullColor bool   `json:"full_color" mapstructure:"full_color"`
	Level     string `json:"level" mapstructure:"level"`
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
	}
}

// Write console write
func (w *ConsoleWriter) Write(r *Record) error {
	if r.level > w.level {
		return nil
	}
	if w.color {
		if w.fullColor {
			_, _ = fmt.Fprint(os.Stdout, ((*colorRecord)(r)).ColorString())
		} else {
			_, _ = fmt.Fprint(os.Stdout, ((*colorRecord)(r)).String())
		}
	} else {
		_, _ = fmt.Fprint(os.Stdout, r.String())
	}
	return nil
}

// Init console init without implement
func (w *ConsoleWriter) Init() error {
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
