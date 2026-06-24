package log4go

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var pathVariableTable map[byte]func(*time.Time) int

// FileWriter file writer for log record deal
type FileWriter struct {
	// write log order by order and atomic incr
	// maxLinesCurLines and maxSizeCurSize
	level        int
	lock         sync.RWMutex
	initFileOnce sync.Once // init once

	rotatePerm  os.FileMode // real used
	perm        string      // input
	bufferSize  int         // bufio writer size (default 8192)
	// input filename
	filename string
	// The opened file
	file          *os.File
	fileBufWriter *bufio.Writer
	// like "xwi88.log", xwi88 is filenameOnly and .log is suffix
	filenameOnly, suffix string

	pathFmt   string // Rotate when, use actions
	actions   []func(*time.Time) int
	variables []interface{}

	// // Rotate at file lines
	// maxLines         int // Rotate at line
	// maxLinesCurLines int

	// // Rotate at size
	// maxSize        int
	// maxSizeCurSize int

	lastWriteTime time.Time

	initFileOk bool
	rotate     bool
	// Rotate daily
	daily bool
	// Rotate hourly
	hourly bool
	// Rotate minutely
	minutely bool

	maxDays       int
	dailyOpenDate int
	dailyOpenTime time.Time

	// Rotate hourly
	maxHours       int
	hourlyOpenDate int
	hourlyOpenTime time.Time

	// Rotate minutely
	maxMinutes       int
	minutelyOpenDate int
	minutelyOpenTime time.Time

	// async mode (optional; default false = synchronous, backward compatible).
	async        bool
	asyncBufSize int
	messages     chan *Record
	policy        OverflowPolicy
	spiller       Spiller[*Record]
	spillType     string
	spillSize     int
	spillDir      string
	spillMaxBytes int64
	stats         OverflowStats
	written      uint64
	errored      uint64
	flushInterval time.Duration
	onEvent      func(name string, delta int64)
	quit         chan struct{}
	stop         chan struct{}
	flushSig     chan struct{}
	wg           sync.WaitGroup
}

// FileWriterOptions file writer options
type FileWriterOptions struct {
	Level    string `json:"level" mapstructure:"level"`
	Filename string `json:"filename" mapstructure:"filename"`
	Enable   bool   `json:"enable" mapstructure:"enable"`

	Rotate bool `json:"rotate" mapstructure:"rotate"`
	// Rotate daily
	Daily bool `json:"daily" mapstructure:"daily"`
	// Rotate hourly
	Hourly bool `json:"hourly" mapstructure:"hourly"`
	// Rotate minutely
	Minutely bool `json:"minutely" mapstructure:"minutely"`

	MaxDays    int `json:"max_days" mapstructure:"max_days"`
	MaxHours   int `json:"max_hours" mapstructure:"max_hours"`
	MaxMinutes int `json:"max_minutes" mapstructure:"max_minutes"`
	// BufferSize is the bufio writer size in bytes (<=0 -> 8192). A larger value
	// reduces flush-to-disk frequency under high write rates.
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`

	// Async enables the async pipeline: Write delivers to a bounded channel and
	// a daemon goroutine does bufio write + flush + rotate, isolating disk I/O
	// from the caller. Default false (synchronous, backward compatible).
	Async bool `json:"async" mapstructure:"async"`
	// AsyncBufferSize bounds the async channel (<=0 -> 4096).
	AsyncBufferSize int `json:"async_buffer_size" mapstructure:"async_buffer_size"`
	// OverflowPolicy for async mode: "drop"(default)|"block"|"spill".
	OverflowPolicy string `json:"overflow_policy" mapstructure:"overflow_policy"`
	// SpillType under "spill": "ring"(default)|"file".
	SpillType string `json:"spill_type" mapstructure:"spill_type"`
	// SpillSize: ring capacity (records) for "ring".
	SpillSize int `json:"spill_size" mapstructure:"spill_size"`
	// SpillDir/SpillMaxBytes for "file" spill.
	SpillDir      string `json:"spill_dir" mapstructure:"spill_dir"`
	SpillMaxBytes int64  `json:"spill_max_bytes" mapstructure:"spill_max_bytes"`
}

// NewFileWriter create new file writer
func NewFileWriter() *FileWriter {
	return &FileWriter{}
}

// NewFileWriterWithOptions create new file writer with options
func NewFileWriterWithOptions(options FileWriterOptions) *FileWriter {
	defaultLevel := DEBUG
	if len(options.Level) > 0 {
		defaultLevel = getLevelDefault(options.Level, defaultLevel, "")
	}
	fileWriter := &FileWriter{
		level:      defaultLevel,
		filename:   options.Filename,
		rotate:     options.Rotate,
		daily:      options.Daily,
		maxDays:    options.MaxDays,
		hourly:     options.Hourly,
		maxHours:   options.MaxHours,
		minutely:   options.Minutely,
		maxMinutes: options.MaxMinutes,
		bufferSize: options.BufferSize,

		async:         options.Async,
		asyncBufSize:  options.AsyncBufferSize,
		policy:        ParseOverflowPolicy(options.OverflowPolicy),
		spillType:     options.SpillType,
		spillSize:     options.SpillSize,
		spillDir:      options.SpillDir,
		spillMaxBytes: options.SpillMaxBytes,
		flushInterval: 500 * time.Millisecond,
	}
	if err := fileWriter.SetPathPattern(options.Filename); err != nil {
		log.Printf("[log4go] file writer init err: %v", err.Error())
	}
	fileWriter.stats.SetAlertEvery(1000, 1000)
	return fileWriter
}

// Write file write. In async mode it delivers to a bounded channel under the
// configured overflow policy (never spawns a goroutine per record); in sync
// mode it writes bufio directly (backward compatible).
func (w *FileWriter) Write(r *Record) error {
	if r.level > w.level {
		return nil
	}
	if !w.async || w.messages == nil {
		return w.writeSync(r)
	}
	w.send(r)
	return nil
}

// writeSync is the synchronous bufio write path.
func (w *FileWriter) writeSync(r *Record) error {
	if w.fileBufWriter == nil {
		return errors.New("fileWriter no opened file: " + w.filename)
	}
	_, err := w.fileBufWriter.WriteString(r.String())
	if err == nil {
		atomic.AddUint64(&w.written, 1)
	}
	return err
}

// Init file writer init
func (w *FileWriter) Init() error {
	filename := w.filename
	defaultPerm := "0755"
	if len(filename) != 0 {
		w.suffix = filepath.Ext(filename)
		w.filenameOnly = strings.TrimSuffix(filename, w.suffix)
		w.filename = filename
		if w.suffix == "" {
			w.suffix = ".log"
		}
	}
	if w.perm == "" {
		w.perm = defaultPerm
	}

	perm, err := strconv.ParseInt(w.perm, 8, 64)
	if err != nil {
		return err
	}
	w.rotatePerm = os.FileMode(perm)

	if w.rotate {
		if w.daily && w.maxDays <= 0 {
			w.maxDays = 60
		}
		if w.hourly && w.maxHours <= 0 {
			w.maxHours = 12
		}
		if w.minutely && w.maxMinutes <= 0 {
			w.maxMinutes = 1
		}
	}

	if err := w.Rotate(); err != nil {
		return err
	}
	if w.async {
		w.startDaemon()
	}
	return nil
}

// Flush writes any buffered data to file. In async mode it signals the daemon
// to flush (non-blocking); use Stop for a synchronous flush on close.
func (w *FileWriter) Flush() error {
	if w.async && w.flushSig != nil {
		select {
		case w.flushSig <- struct{}{}:
		default:
		}
		return nil
	}
	if w.fileBufWriter != nil {
		return w.fileBufWriter.Flush()
	}
	return nil
}

// SetPathPattern for file writer
func (w *FileWriter) SetPathPattern(pattern string) error {
	n := 0
	for _, c := range pattern {
		if c == '%' {
			n++
		}
	}

	if n == 0 {
		w.pathFmt = pattern
		return nil
	}

	w.actions = make([]func(*time.Time) int, 0, n)
	w.variables = make([]interface{}, n, n)
	tmp := []byte(pattern)

	variable := 0
	for _, c := range tmp {
		if variable == 1 {
			act, ok := pathVariableTable[c]
			if !ok {
				return errors.New("invalid rotate pattern (" + pattern + ")")
			}
			w.actions = append(w.actions, act)
			variable = 0
			continue
		}
		if c == '%' {
			variable = 1
		}
	}

	w.pathFmt = convertPatternToFmt(tmp)

	return nil
}

func (w *FileWriter) initFile() {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.initFileOk = true
}

// Rotate file writer rotate. In async mode rotation is driven by the daemon
// (by time), so this is a no-op; in sync mode it performs the original rotate.
func (w *FileWriter) Rotate() error {
	if w.async {
		return nil
	}
	return w.rotateImpl()
}

// rotateImpl performs the real rotate; called by Rotate (sync) and by the
// async daemon (single goroutine, no extra locking needed).
func (w *FileWriter) rotateImpl() error {
	now := time.Now()
	v := 0
	rotate := false
	for i, act := range w.actions {
		v = act(&now)
		if v != w.variables[i] {
			if !w.initFileOk {
				w.variables[i] = v
				rotate = true
			} else {
				// only exec except the first round
				switch i {
				case 2:
					if w.daily {
						w.dailyOpenDate = v
						w.dailyOpenTime = now
						_, _, d := w.lastWriteTime.AddDate(0, 0, w.maxDays).Date()
						if v == d {
							rotate = true
							w.variables[i] = v
						}
					}
				case 3:
					if w.hourly {
						w.hourlyOpenDate = v
						w.hourlyOpenTime = now
						h := w.lastWriteTime.Add(time.Hour * time.Duration(w.maxHours)).Hour()
						if v == h {
							rotate = true
							w.variables[i] = v
						}
					}
				case 4:
					if w.minutely {
						w.minutelyOpenDate = v
						w.minutelyOpenTime = now
						m := w.lastWriteTime.Add(time.Minute * time.Duration(w.maxMinutes)).Minute()
						if v == m {
							rotate = true
							w.variables[i] = v
						}
					}
				}
			}
		}
	}
	// must init file first!
	if rotate == false {
		return nil
	}
	w.initFileOnce.Do(w.initFile)
	w.lastWriteTime = now

	if w.fileBufWriter != nil {
		if err := w.fileBufWriter.Flush(); err != nil {
			return err
		}
	}

	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}

	filePath := fmt.Sprintf(w.pathFmt, w.variables...)

	if err := os.MkdirAll(path.Dir(filePath), w.rotatePerm); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}

	if file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, w.rotatePerm); err == nil {
		w.file = file
	} else {
		return err
	}

	bufSize := w.bufferSize
	if bufSize <= 0 {
		bufSize = 8192
	}
	if w.fileBufWriter = bufio.NewWriterSize(w.file, bufSize); w.fileBufWriter == nil {
		return errors.New("fileWriter new fileBufWriter failed")
	}
	w.suffix = filepath.Ext(filePath)
	w.filenameOnly = strings.TrimSuffix(filePath, w.suffix)
	return nil
}

func getYear(now *time.Time) int {
	return now.Year()
}

func getMonth(now *time.Time) int {
	return int(now.Month())
}

func getDay(now *time.Time) int {
	return now.Day()
}

func getHour(now *time.Time) int {
	return now.Hour()
}

func getMin(now *time.Time) int {
	return now.Minute()
}

func convertPatternToFmt(pattern []byte) string {
	pattern = bytes.Replace(pattern, []byte("%Y"), []byte("%d"), -1)
	pattern = bytes.Replace(pattern, []byte("%M"), []byte("%02d"), -1)
	pattern = bytes.Replace(pattern, []byte("%D"), []byte("%02d"), -1)
	pattern = bytes.Replace(pattern, []byte("%H"), []byte("%02d"), -1)
	pattern = bytes.Replace(pattern, []byte("%m"), []byte("%02d"), -1)
	return string(pattern)
}

// send delivers a record under the overflow policy (drop/block/spill).
func (w *FileWriter) send(r *Record) {
	switch w.policy {
	case OverflowBlock:
		w.messages <- r
	case OverflowSpill:
		select {
		case w.messages <- r:
		default:
			if w.spiller != nil && w.spiller.Push(r) {
				w.stats.IncSpilled()
				w.fire("spill", 1)
			} else {
				w.stats.IncDropped()
				w.fire("drop", 1)
			}
		}
	default: // OverflowDrop
		select {
		case w.messages <- r:
		default:
			w.stats.IncDropped()
			w.fire("drop", 1)
		}
	}
}

func (w *FileWriter) fire(name string, delta int64) {
	if w.onEvent != nil {
		w.onEvent(name, delta)
	}
}

// startDaemon initializes the async channel/spiller and launches the daemon.
func (w *FileWriter) startDaemon() {
	size := w.asyncBufSize
	if size <= 0 {
		size = 4096
	}
	w.messages = make(chan *Record, size)
	w.quit = make(chan struct{})
	w.flushSig = make(chan struct{}, 1)
	if w.policy == OverflowSpill {
		switch w.spillType {
		case "file":
			if sp, err := NewFileSpiller[*Record](w.spillDir, w.spillMaxBytes, RecordCodec); err == nil {
				w.spiller = sp
			}
		case "ring":
			w.spiller = NewRingSpiller[*Record](w.spillSize)
		default: // "" or "chain": ring (hot) -> file (cold, persistent)
			ring := NewRingSpiller[*Record](w.spillSize)
			if w.spillDir != "" {
				if file, ferr := NewFileSpiller[*Record](w.spillDir, w.spillMaxBytes, RecordCodec); ferr == nil {
					w.spiller = NewChainedSpiller[*Record](ring, file)
				} else {
					w.spiller = ring
				}
			} else {
				w.spiller = ring
			}
		}
	}
	// resume any persisted spill from a previous (interrupted) run.
	if w.spiller != nil {
		for _, r := range w.spiller.Drain() {
			select {
			case w.messages <- r:
			default:
				if !w.spiller.Push(r) {
					w.stats.IncDropped()
				}
			}
		}
	}
	w.wg.Add(1)
	go w.daemon()
}

// daemon consumes the channel: writes bufio, flushes on tick/signal, rotates by
// time, and drains the spiller.
func (w *FileWriter) daemon() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case r, ok := <-w.messages:
			if !ok {
				w.drainSpillAll()
				_ = w.flushSync()
				w.quit <- struct{}{}
				return
			}
			w.writeOne(r)
		case <-ticker.C:
			_ = w.flushSync()
			w.drainSpill()
		case <-w.flushSig:
			_ = w.flushSync()
			w.drainSpill()
		}
	}
}

// writeOne rotates (by time) then writes the record to bufio.
func (w *FileWriter) writeOne(r *Record) {
	_ = w.rotateImpl()
	if w.fileBufWriter == nil {
		atomic.AddUint64(&w.errored, 1)
		w.fire("error", 1)
		return
	}
	if _, err := w.fileBufWriter.WriteString(r.String()); err != nil {
		atomic.AddUint64(&w.errored, 1)
		w.fire("error", 1)
		return
	}
	atomic.AddUint64(&w.written, 1)
	w.fire("written", 1)
}

// drainSpill re-injects recovered records into the channel (non-blocking).
func (w *FileWriter) drainSpill() {
	if w.spiller == nil || w.spiller.Len() == 0 {
		return
	}
	for _, r := range w.spiller.Drain() {
		select {
		case w.messages <- r:
		default:
			_ = w.spiller.Push(r)
			return
		}
	}
}

// drainSpillAll writes all spilled records directly (used on shutdown).
func (w *FileWriter) drainSpillAll() {
	if w.spiller == nil {
		return
	}
	for _, r := range w.spiller.Drain() {
		w.writeOne(r)
	}
}

func (w *FileWriter) flushSync() error {
	if w.fileBufWriter != nil {
		return w.fileBufWriter.Flush()
	}
	return nil
}

// Stop gracefully shuts the async daemon: closes the channel, waits for the
// daemon to drain + flush, then closes the spiller and file.
func (w *FileWriter) Stop() {
	if !w.async || w.messages == nil {
		return
	}
	close(w.messages)
	<-w.quit
	w.messages = nil
	if w.spiller != nil {
		_ = w.spiller.Close()
	}
	if w.fileBufWriter != nil {
		_ = w.fileBufWriter.Flush()
	}
	if w.file != nil {
		_ = w.file.Close()
	}
}

// FileWriterMetrics is a point-in-time snapshot of async FileWriter counters.
type FileWriterMetrics struct {
	Written  uint64
	Errored  uint64
	Dropped  uint64
	Spilled  uint64
	Queued   int
	SpillLen int
}

// Metrics returns a snapshot of async FileWriter counters for monitoring.
func (w *FileWriter) Metrics() FileWriterMetrics {
	queued := 0
	if w.messages != nil {
		queued = len(w.messages)
	}
	spillLen := 0
	if w.spiller != nil {
		spillLen = w.spiller.Len()
	}
	return FileWriterMetrics{
		Written:  atomic.LoadUint64(&w.written),
		Errored:  atomic.LoadUint64(&w.errored),
		Dropped:  w.stats.Dropped(),
		Spilled:  w.stats.Spilled(),
		Queued:   queued,
		SpillLen: spillLen,
	}
}

// SetOnEvent installs a real-time metric hook (reserved for monitoring).
func (w *FileWriter) SetOnEvent(fn func(name string, delta int64)) { w.onEvent = fn }

// SetAlertSink installs an alert sink (e.g. WebhookAlertSink for lark/dingtalk/
// feishu) for overflow push notifications. Set before Start.
func (w *FileWriter) SetAlertSink(sink AlertSink) { w.stats.SetAlertSink(sink) }

func init() {
	pathVariableTable = make(map[byte]func(*time.Time) int, 5)
	pathVariableTable['Y'] = getYear
	pathVariableTable['M'] = getMonth
	pathVariableTable['D'] = getDay
	pathVariableTable['H'] = getHour
	pathVariableTable['m'] = getMin
}
