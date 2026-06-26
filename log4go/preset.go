package log4go

// NewProduction returns a Logger configured for production: JSON output, INFO
// level, microsecond ISO timestamps, caller capture, and sampling to absorb log
// storms (first 100 records per level pass, then 1-in-100). A console writer is
// registered. This mirrors zap.NewProduction() ergonomics:
//
//	lg := log4go.NewProduction()
//	defer lg.Close()
//	lg.Info("serving")
//
// Register additional writers (Kafka, File, Net, Webhook) on the returned logger
// as needed.
func NewProduction() *Logger {
	l := newDefaultLoggerInstance()
	l.SetLevel(INFO)
	l.SetFormat(FormatJSON)
	l.hasCaller.Store(true)
	l.sampler = newSampler(100, 100)
	l.Register(NewConsoleWriterWithOptions(ConsoleWriterOptions{
		Enable: true,
		Level:  LevelFlagInfo,
	}))
	return l
}

// NewDevelopment returns a Logger configured for development: colored text
// output, DEBUG level, caller + function name. A console writer is registered.
// This mirrors zap.NewDevelopment() ergonomics.
func NewDevelopment() *Logger {
	l := newDefaultLoggerInstance()
	l.SetLevel(DEBUG)
	l.SetFormat(FormatText)
	l.hasCaller.Store(true)
	l.withFuncName.Store(true)
	l.Register(NewConsoleWriterWithOptions(ConsoleWriterOptions{
		Enable: true,
		Color:  true,
		Level:  LevelFlagDebug,
	}))
	return l
}
