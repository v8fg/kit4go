package log4go

import (
	"fmt"
	"os"
	"testing"
)

var deleteTempLogs = true

func generateNewFileWriterWithOptions(level string, filename string) (*FileWriter, error) {
	options := FileWriterOptions{
		Level:    level,
		Filename: filename,
	}
	w := NewFileWriterWithOptions(options)
	err := w.SetPathPattern(w.filename)
	return w, err
}

func generateRegisterFileWriter(lg *Logger, w *FileWriter, fullPath, funcName bool, layout string) {
	lg.Register(w)
	if layout == "" {
		lg.SetLayout("2006-01-02 15:04:05")
	} else {
		lg.SetLayout(layout)
	}
	lg.WithFullPath(fullPath)
	lg.WithFuncName(funcName)
}

func deleteGenerateLogFile(filename string) {
	if deleteTempLogs {
		os.Remove(filename)
	}
}
func Test_NewFileWriterWithStruct(t *testing.T) {
	w := &FileWriter{}
	t.Logf("%#v", w)
}

func Test_NewFileWriter(t *testing.T) {
	NewFileWriter()
}

func Test_NewFileWriterWithoutSuffixFilename(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(128))
	loggerDefaultTest := newLoggerWithRecords(records)
	loggerDefaultTest.SetLevel(DEBUG)

	filename := "./test/xwi88-log4go%Y%M%D%H%m-withoutSuffixFilename"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}
	w.perm = "0999"
	var name = "filename without suffix"
	defer func() {
		if err := recover(); err != nil {
			w.perm = "0755"
			loggerDefaultTest = newLoggerWithRecords(records)
			generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
			curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
			defer deleteGenerateLogFile(curFilename)
			defer loggerDefaultTest.Close()
			loggerDefaultTest.Debug("log4go by %s", name)
			loggerDefaultTest.Info("log4go by %s", name)
			loggerDefaultTest.Alert("%#v", loggerDefaultTest)
		}
	}()
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
}

func Test_NewFileWriterWithErrorPattern(t *testing.T) {
	var fullPath, funcName bool
	var layout string
	records := make(chan *Record, uint(128))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%X-error-pattern.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Log(err)
		return
	}
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
}

func Test_NewFileWriterWithNilLogger(t *testing.T) {
	var fullPath, funcName bool
	var layout string
	records := make(chan *Record, uint(0))
	close(records)
	loggerDefaultTest := newLoggerWithRecords(records)

	filename := "./test/xwi88-log4go%Y%M%D%H%m-nil.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}
	var name = "file nil logger"
	defer func() {
		if err := recover(); err != nil {
			t.Errorf("error occur: %v", err)
			loggerDefaultTest = newLoggerWithRecords(records)
			generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
			curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
			defer deleteGenerateLogFile(curFilename)
			defer loggerDefaultTest.Close()
			loggerDefaultTest.Debug("log4go by %s", name)
			loggerDefaultTest.Info("log4go by %s", name)
			loggerDefaultTest.Alert("%#v", loggerDefaultTest)
		}
	}()
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
}

func Test_NewFileWriterWithLevel(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	loggerDefaultTest.SetLevel(DEBUG)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%M%D%H%m-level.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagInfo, filename)
	if err != nil {
		t.Error(err)
	}

	w.rotate = true
	w.daily = true
	w.maxDays = 0
	w.hourly = true
	w.maxHours = 0
	w.minutely = true
	w.maxMinutes = 0
	var name = "file level"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Info("")
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithRotate(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	loggerDefaultTest.SetLevel(DEBUG)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%M%D%H%m-level.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagInfo, filename)
	if err != nil {
		t.Error(err)
	}
	// w.initFileOk = true // forbidden manual set initFileOk

	w.rotate = true
	w.daily = true
	w.maxDays = 0
	w.hourly = true
	w.maxHours = 0
	w.minutely = true
	w.maxMinutes = 0
	var name = "file level"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Info("")
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}
func Test_NewFileWriterWithLevel2(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	loggerDefaultTest.SetLevel(NOTICE)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%M%D%H%m-level2.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagInfo, filename)
	if err != nil {
		t.Error(err)
	}

	var name = "file level2"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithEmptyPath(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	filename := ""
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}

	var name = "file with empty path"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithNilFileBufWriter(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	filename := ""
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}
	var name = "file color"
	w.fileBufWriter = nil
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
}

func Test_NewFileWriterWithFullColor(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%M%D%H%m-fullColor.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}

	var name = "file full color"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithFullPath(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	fullPath = true
	filename := "./test/xwi88-log4go%Y%M%D%H%m-fullPath.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}
	var name = "file full path"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithFuncName(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	funcName = true
	filename := "./test/xwi88-log4go%Y%M%D%H%m-funcName.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}

	var name = "file func name"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Test_NewFileWriterWithLayout(t *testing.T) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	defer loggerDefaultTest.Close()

	layout = "20060102T150405.000-0700"
	filename := "./test/xwi88-log4go%Y%M%D%H%m-layout.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		t.Error(err)
	}

	var name = "file layout"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}

func Benchmark_NewFileWriter(b *testing.B) {
	var fullPath, funcName bool
	var layout string

	records := make(chan *Record, uint(256))
	loggerDefaultTest := newLoggerWithRecords(records)
	loggerDefaultTest.SetLevel(DEBUG)
	defer loggerDefaultTest.Close()

	filename := "./test/xwi88-log4go%Y%M%D%H%m-benchmark.log"
	w, err := generateNewFileWriterWithOptions(LevelFlagDebug, filename)
	if err != nil {
		b.Error(err)
	}

	var name = "file benchmark test"
	generateRegisterFileWriter(loggerDefaultTest, w, fullPath, funcName, layout)
	curFilename := fmt.Sprintf("%s%s", w.filenameOnly, w.suffix)
	defer deleteGenerateLogFile(curFilename)
	loggerDefaultTest.Debug("log4go by %s", name)
	loggerDefaultTest.Info("log4go by %s", name)
	loggerDefaultTest.Notice("log4go by %s", name)
	loggerDefaultTest.Warn("log4go by %s", name)
	loggerDefaultTest.Error("log4go by %s", name)
	loggerDefaultTest.Critical("log4go by %s", name)
	loggerDefaultTest.Alert("log4go by %s", name)
	loggerDefaultTest.Emergency("log4go by %s", name)
	loggerDefaultTest.Alert("%#v", loggerDefaultTest)
}
