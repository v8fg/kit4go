package log4go

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

// GlobalLevel global level
var GlobalLevel = DEBUG

// WriterName* are the stable names returned by each writer's Name(), used for
// by-name control (Logger.PauseWriter / ResumeWriter / WriterPaused, SetWriterLevel).
const (
	WriterNameConsole = "console_writer"
	WriterNameFile    = "file_writer"
	WriterNameKafka   = "kafka_writer"
	WriterNameNet     = "net_writer"
	WriterNameIO      = "io_writer"
)

// LogConfig log config
type LogConfig struct {
	Level    string `json:"level" mapstructure:"level"`
	Debug    bool   `json:"debug" mapstructure:"debug"` // output log info or not for log4go
	FullPath bool   `json:"full_path" mapstructure:"full_path"`
	// Format selects the record serialization: "text" (default, human-readable
	// line) or "json" (one JSON object per record, machine-readable). Unknown
	// values fall back to text. See LogFormat / SetFormat.
	Format        string               `json:"format" mapstructure:"format"`
	ConsoleWriter ConsoleWriterOptions `json:"console_writer" mapstructure:"console_writer"`
	FileWriter    FileWriterOptions    `json:"file_writer" mapstructure:"file_writer"`
	KafKaWriter   KafKaWriterOptions   `json:"kafka_writer" mapstructure:"kafka_writer"`
}

// applyConfig configures l (level, format, full-path, writers) from lc. It is the
// shared core of SetupLog and Reload. Writers are built and registered via
// registerOrFail (no panic) so a failure is returned to the caller rather than
// disturbing the live logger. GlobalLevel is committed only after every enabled
// writer has started, so a mid-config failure leaves the package-level filter and
// the running logger unchanged.
func (l *Logger) applyConfig(lc LogConfig) error {
	if !lc.Debug {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stdout)
	}

	// global + per-writer level aggregation.
	newGlobal := getLevel(lc.Level)
	validGlobalMinLevel := EMERGENCY // default max level
	validGlobalMinLevelBy := "global"

	fileWriterLevelDefault := newGlobal
	consoleWriterLevelDefault := newGlobal
	kafkaWriterLevelDefault := newGlobal

	if lc.ConsoleWriter.Enable {
		consoleWriterLevelDefault = getLevelDefault(lc.ConsoleWriter.Level, newGlobal, WriterNameConsole)
		validGlobalMinLevel = maxInt(consoleWriterLevelDefault, validGlobalMinLevel)
		if validGlobalMinLevel == consoleWriterLevelDefault {
			validGlobalMinLevelBy = WriterNameConsole
		}
	}

	if lc.FileWriter.Enable {
		fileWriterLevelDefault = getLevelDefault(lc.FileWriter.Level, newGlobal, WriterNameFile)
		validGlobalMinLevel = maxInt(fileWriterLevelDefault, validGlobalMinLevel)
		if validGlobalMinLevel == fileWriterLevelDefault {
			validGlobalMinLevelBy = WriterNameFile
		}
	}

	if lc.KafKaWriter.Enable {
		kafkaWriterLevelDefault = getLevelDefault(lc.KafKaWriter.Level, newGlobal, WriterNameKafka)
		validGlobalMinLevel = maxInt(kafkaWriterLevelDefault, validGlobalMinLevel)
		if validGlobalMinLevel == kafkaWriterLevelDefault {
			validGlobalMinLevelBy = WriterNameKafka
		}
	}

	l.WithFullPath(lc.FullPath)
	l.SetLevel(validGlobalMinLevel)
	// Apply the serialization format (text default, json for structured logs).
	// Parsed here so a bad value is reported once at setup rather than per record.
	l.SetFormat(ParseLogLogFormat(lc.Format))

	if lc.ConsoleWriter.Enable {
		w := NewConsoleWriterWithOptions(lc.ConsoleWriter)
		w.level = consoleWriterLevelDefault
		log.Print("[log4go] enable " + WriterNameConsole + " with level " + LevelFlags[consoleWriterLevelDefault])
		// ConsoleWriter.Init is infallible, so Register cannot panic here; file
		// and kafka below use registerOrFail because their Init can fail.
		l.Register(w)
	}

	if lc.FileWriter.Enable {
		w := NewFileWriterWithOptions(lc.FileWriter)
		w.level = fileWriterLevelDefault
		log.Print("[log4go] enable    " + WriterNameFile + " with level " + LevelFlags[fileWriterLevelDefault])
		if err := l.registerOrFail(w); err != nil {
			return fmt.Errorf("file writer init: %w", err)
		}
	}

	if lc.KafKaWriter.Enable {
		w := NewKafKaWriter(lc.KafKaWriter)
		w.level = kafkaWriterLevelDefault
		log.Print("[log4go] enable   " + WriterNameKafka + " with level " + LevelFlags[kafkaWriterLevelDefault])
		if err := l.registerOrFail(w); err != nil {
			return fmt.Errorf("kafka writer init: %w", err)
		}
	}

	log.Printf("[log4go] valid global_level(min:%v, flag:%v, by:%v), default(%v, flag:%v)",
		validGlobalMinLevel, LevelFlags[validGlobalMinLevel], validGlobalMinLevelBy, newGlobal, LevelFlags[newGlobal])

	GlobalLevel = newGlobal // commit only after all enabled writers started
	return nil
}

// SetupLog applies lc to the package singleton. It returns the first writer
// initialization error (e.g. a kafka writer that cannot reach a broker) instead
// of panicking; the writers that did start are registered.
func SetupLog(lc LogConfig) error {
	return defaultLogger().applyConfig(lc)
}

// SetLogWithConf setup log with config file
func SetLogWithConf(file string) (err error) {
	var lc LogConfig
	cnt, err := os.ReadFile(file)
	if err != nil {
		return
	}

	if err = json.Unmarshal(cnt, &lc); err != nil {
		return
	}
	return SetupLog(lc)
}

// SetLog setup log with config []byte
func SetLog(config []byte) (err error) {
	var lc LogConfig
	if err = json.Unmarshal(config, &lc); err != nil {
		return
	}
	return SetupLog(lc)
}

func getLevel(flag string) int {
	return getLevelDefault(flag, DEBUG, "")
}

// maxInt return max int
func maxInt(a, b int) int {
	if a < b {
		return b
	}
	return a
}
