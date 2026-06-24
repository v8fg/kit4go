package log4go

import (
	"bytes"
	"fmt"
	"log"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LevelFlag log level flags
const (
	LevelFlagEmergency = "EMERGENCY"
	LevelFlagAlert     = "ALERT"
	LevelFlagCritical  = "CRITICAL"
	LevelFlagError     = "ERROR"
	LevelFlagWarning   = "WARNING" // compatible WARN
	LevelFlagWarn      = "WARN"
	LevelFlagNotice    = "NOTICE"
	LevelFlagInfo      = "INFO"
	LevelFlagDebug     = "DEBUG"
)

// RFC5424 log message levels.
// ref: https://tools.ietf.org/html/draft-ietf-syslog-protocol-23
const (
	EMERGENCY = iota // Emergency: system is unusable
	ALERT            // Alert: action must be taken immediately
	CRITICAL         // Critical: critical conditions
	ERROR            // Error: error conditions
	WARNING          // Warning: warning conditions
	NOTICE           // Notice: normal but significant condition
	INFO             // Informational: informational messages
	DEBUG            // Debug: debug-level messages
)

const (
	// default size or min size for record channel
	recordChannelSizeDefault = uint(4096)
	// default time layout
	defaultLayout = "2006/01/02 15:04:05"
	// timestamp with zone info
	timestampLayout = "2006-01-02T15:04:05.000+0800"
)

// LevelFlags level Flags set
var (
	LevelFlags = []string{
		LevelFlagEmergency,
		LevelFlagAlert,
		LevelFlagCritical,
		LevelFlagError,
		LevelFlagWarning,
		LevelFlagNotice,
		LevelFlagInfo,
		LevelFlagDebug,
	}
	DefaultLayout = defaultLayout
)

// default logger
var (
	loggerDefault     *Logger
	recordPool        *sync.Pool
	bufPool           *sync.Pool
	recordChannelSize = recordChannelSizeDefault // log chan size
)

// Record log record
type Record struct {
	level int
	time  string
	file  string
	msg   string
}

func (r *Record) String() string {
	return fmt.Sprintf("%s [%s] <%s> %s\n", r.time, LevelFlags[r.level], r.file, r.msg)
}

// Writer record writer
type Writer interface {
	Init() error
	Write(*Record) error
}

// Flusher record flusher
type Flusher interface {
	Flush() error
}

// Rotater record rotater
type Rotater interface {
	Rotate() error
	SetPathPattern(string) error
}

// Logger logger define
type Logger struct {
	writers         atomic.Value // []Writer, copy-on-write for lock-free reads
	records         chan *Record
	recordsChanSize uint
	lastTime        atomic.Int64
	lastTimeStr     atomic.Pointer[string]

	flushTimer  time.Duration // timer to flush logger record to chan
	rotateTimer time.Duration // timer to rotate logger record for writer

	c chan bool

	layout         atomic.Pointer[string]
	level          atomic.Int32
	recordsByLevel [DEBUG + 1]uint64 // per-level record counters (monitoring)
	fullPath       atomic.Bool       // show full path, default only show file:line_number
	withFuncName   atomic.Bool       // show caller func name
	hasCaller      atomic.Bool       // capture caller (file:line); disable for max throughput
	lock           sync.RWMutex
}

// NewLogger create the logger
func NewLogger() *Logger {
	if loggerDefault != nil {
		return loggerDefault
	}
	records := make(chan *Record, recordChannelSize)

	return newLoggerWithRecords(records)
}

// newLoggerWithRecords is useful for go test
func newLoggerWithRecords(records chan *Record) *Logger {
	l := new(Logger)
	l.writers.Store(make([]Writer, 0, 1)) // normal least has console writer
	if l.recordsChanSize == 0 {
		recordChannelSize = recordChannelSizeDefault
	}

	l.records = records
	l.c = make(chan bool, 1)
	l.level.Store(int32(DEBUG))
	lp := DefaultLayout
	l.layout.Store(&lp)
	l.hasCaller.Store(true)

	go bootstrapLogWriter(l)

	return l
}

// Register register writer
// the writer should be register once for writers by kind
func (l *Logger) Register(w Writer) {
	if err := w.Init(); err != nil {
		panic(err)
	}

	// copy-on-write so the bootstrap goroutine can read writers lock-free.
	cur := l.writers.Load().([]Writer)
	next := make([]Writer, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = w
	l.writers.Store(next)
}

// snapshotWriters returns the current writers slice for lock-free iteration
// by the bootstrap goroutine; Register replaces the slice copy-on-write so
// this view stays valid.
func (l *Logger) snapshotWriters() []Writer {
	return l.writers.Load().([]Writer)
}

// LoggerMetrics is a snapshot of per-level record counters for monitoring.
type LoggerMetrics struct {
	Records [DEBUG + 1]uint64 // indexed by level (EMERGENCY..DEBUG)
}

// Metrics returns per-level record counters of this logger for monitoring.
func (l *Logger) Metrics() LoggerMetrics {
	var m LoggerMetrics
	for i := range m.Records {
		m.Records[i] = atomic.LoadUint64(&l.recordsByLevel[i])
	}
	return m
}

// Metrics returns the default logger's per-level record counters for monitoring.
func Metrics() LoggerMetrics { return loggerDefault.Metrics() }

// Close close logger
func (l *Logger) Close() {
	close(l.records)
	<-l.c

	for _, w := range l.snapshotWriters() {
		if f, ok := w.(Flusher); ok {
			if err := f.Flush(); err != nil {
				log.Println(err)
			}
		}
	}
}

// SetLayout set the logger time layout
func (l *Logger) SetLayout(layout string) {
	v := layout
	l.layout.Store(&v)
}

// SetLevel set the logger level
func (l *Logger) SetLevel(lvl int) {
	l.level.Store(int32(lvl))
}

// WithFullPath set the logger with full path
func (l *Logger) WithFullPath(show bool) {
	l.fullPath.Store(show)
}

// WithFuncName set the logger with func name
func (l *Logger) WithFuncName(show bool) {
	l.withFuncName.Store(show)
}

// WithCaller toggles runtime.Caller capture (file:line). Disable for maximum
// throughput when line info is not needed.
func (l *Logger) WithCaller(enable bool) {
	l.hasCaller.Store(enable)
}

// Debug level debug
func (l *Logger) Debug(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(DEBUG, fmt, args...)
}

// Info level info
func (l *Logger) Info(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(INFO, fmt, args...)
}

// Notice level notice
func (l *Logger) Notice(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(NOTICE, fmt, args...)
}

// Warn level warn
func (l *Logger) Warn(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(WARNING, fmt, args...)
}

// Error level error
func (l *Logger) Error(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(ERROR, fmt, args...)
}

// Critical level critical
func (l *Logger) Critical(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(CRITICAL, fmt, args...)
}

// Alert level alert
func (l *Logger) Alert(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(ALERT, fmt, args...)
}

// Emergency level emergency
func (l *Logger) Emergency(fmt string, args ...interface{}) {
	l.deliverRecordToWriter(EMERGENCY, fmt, args...)
}

func (l *Logger) deliverRecordToWriter(level int, f string, args ...interface{}) {
	var msg string
	var fileStr string

	if level > int(l.level.Load()) {
		return
	}
	atomic.AddUint64(&l.recordsByLevel[level], 1)

	msg = f
	sz := len(args)
	if sz != 0 {
		if strings.Contains(msg, "%") && !strings.Contains(msg, "%%") {
		} else {
			msg += strings.Repeat("%v", len(args))
		}
	}
	msg = fmt.Sprintf(msg, args...)

	// source code, file and line num
	if l.hasCaller.Load() {
		fi := bufPool.Get().(*bytes.Buffer)
		fi.Reset()
		pc, file, line, ok := runtime.Caller(2)
		if ok {
			fileName := path.Base(file)
			if l.fullPath.Load() {
				fileName = file
			}
			fi.WriteString(fileName)
			fi.WriteByte(':')
			fi.WriteString(strconv.Itoa(line))

			if l.withFuncName.Load() {
				funcName := runtime.FuncForPC(pc).Name()
				funcName = path.Base(funcName)
				fi.WriteByte(' ')
				fi.WriteString(funcName)
			}
		}
		fileStr = fi.String()
		bufPool.Put(fi)
	}

	// format time
	now := time.Now()
	sec := now.Unix()
	lpStr := l.lastTimeStr.Load()
	var lastTimeStr string
	if lpStr != nil && l.lastTime.Load() == sec {
		lastTimeStr = *lpStr
	} else {
		lpLayout := l.layout.Load()
		layout := defaultLayout
		if lpLayout != nil {
			layout = *lpLayout
		}
		lastTimeStr = now.Format(layout)
		l.lastTime.Store(sec)
		l.lastTimeStr.Store(&lastTimeStr)
	}

	r := recordPool.Get().(*Record)
	r.msg = msg
	r.file = fileStr
	r.time = lastTimeStr
	r.level = level

	l.records <- r
}

func bootstrapLogWriter(logger *Logger) {
	var (
		r  *Record
		ok bool
	)

	if r, ok = <-logger.records; !ok {
		logger.c <- true
		return
	}

	for _, w := range logger.snapshotWriters() {
		if err := w.Write(r); err != nil {
			log.Printf("%v\n", err)
		}
	}

	flushTimer := time.NewTimer(logger.flushTimer)
	rotateTimer := time.NewTimer(logger.rotateTimer)

	for {
		select {
		case r, ok = <-logger.records:
			if !ok {
				logger.c <- true
				return
			}

			for _, w := range logger.snapshotWriters() {
				if err := w.Write(r); err != nil {
					log.Printf("%v\n", err)
				}
			}

			recordPool.Put(r)

		case <-flushTimer.C:
			for _, w := range logger.snapshotWriters() {
				if f, ok := w.(Flusher); ok {
					if err := f.Flush(); err != nil {
						log.Printf("%v\n", err)
					}
				}
			}
			flushTimer.Reset(logger.flushTimer)

		case <-rotateTimer.C:
			for _, w := range logger.snapshotWriters() {
				if r, ok := w.(Rotater); ok {
					if err := r.Rotate(); err != nil {
						log.Printf("%v\n", err)
					}
				}
			}
			rotateTimer.Reset(logger.rotateTimer)
		}
	}
}

func init() {
	loggerDefault = NewLogger()
	loggerDefault.flushTimer = time.Millisecond * 500
	loggerDefault.rotateTimer = time.Second * 10
	recordPool = &sync.Pool{New: func() interface{} {
		return &Record{}
	}}
	bufPool = &sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
}

// Register register writer
func Register(w Writer) {
	loggerDefault.Register(w)
}

// Close close logger
func Close() {
	loggerDefault.Close()
}

// SetLayout set the logger time layout, should call before logger real use
func SetLayout(layout string) {
	loggerDefault.SetLayout(layout)
}

// SetLevel set the logger level, should call before logger real use
func SetLevel(lvl int) {
	loggerDefault.level.Store(int32(lvl))
}

// WithFullPath set the logger with full path, should call before logger real use
func WithFullPath(show bool) {
	loggerDefault.fullPath.Store(show)
}

// WithFuncName set the logger with func name, should call before logger real use
func WithFuncName(show bool) {
	loggerDefault.withFuncName.Store(show)
}

// Debug level debug
func Debug(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(DEBUG, fmt, args...)
}

// Info level info
func Info(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(INFO, fmt, args...)
}

// Notice level notice
func Notice(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(NOTICE, fmt, args...)
}

// Warn level warn
func Warn(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(WARNING, fmt, args...)
}

// Error level error
func Error(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(ERROR, fmt, args...)
}

// Critical level critical
func Critical(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(CRITICAL, fmt, args...)
}

// Alert level alert
func Alert(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(ALERT, fmt, args...)
}

// Emergency level emergency
func Emergency(fmt string, args ...interface{}) {
	loggerDefault.deliverRecordToWriter(EMERGENCY, fmt, args...)
}

// The method is put here, so it's easy to test
func getLevelDefault(flag string, defaultFlag int, writer string) int {
	// level WARN == WARNING
	if strings.EqualFold(flag, LevelFlagWarn) {
		flag = LevelFlagWarning
	}

	for i, f := range LevelFlags {
		if strings.TrimSpace(strings.ToUpper(flag)) == f {
			return i
		}
	}
	log.Printf("[log4go] no matching level for writer(%v, flag:%v), use default level(%d, flag:%v)", writer, flag, defaultFlag, LevelFlags[defaultFlag])
	return defaultFlag
}
