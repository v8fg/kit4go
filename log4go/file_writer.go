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

const (
	// defaultBufferSize is the bufio.Writer size used when BufferSize <= 0.
	// 64KB (up from 8192) cuts flush-to-disk syscall frequency ~8x under high
	// write rates while keeping the worst-case crash-loss window small
	// (<= 64KB of buffered, unflushed bytes). Callers wanting fewer flushes at
	// the cost of a larger loss window can raise BufferSize explicitly.
	defaultBufferSize = 64 << 10
	// rotateCheckInterval bounds how often the async daemon calls rotateImpl.
	// Real rotation only fires at minute/hour/day boundaries, so checking at
	// most once per second preserves daily/hourly/minutely semantics (a rotate
	// may be applied up to ~1s late, well within the finest "minutely" granular-
	// ity) while removing the per-record time.Now()+actions-scan cost.
	rotateCheckInterval = time.Second
	// defaultFlushBatchSize is the number of records the async daemon writes
	// between forced flushSync() calls when FlushBatchSize <= 0. Together with
	// the bufio size this bounds worst-case crash loss: at most
	// (FlushBatchSize records) + (BufferSize bytes) of unflushed data.
	defaultFlushBatchSize = 1000
)

// FileWriter file writer for log record deal
type FileWriter struct {
	// write log order by order and atomic incr
	// maxLinesCurLines and maxSizeCurSize
	level        int
	paused       atomic.Bool
	lock         sync.RWMutex
	initFileOnce sync.Once // init once

	rotatePerm os.FileMode // real used
	perm       string      // input
	bufferSize int         // bufio writer size (<=0 -> defaultBufferSize)
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

	// lastRotateCheck throttles rotateImpl calls in the async daemon. rotateImpl
	// does time.Now() + an actions scan per record, but real rotation only fires
	// at minute/hour/day boundaries. Checking at most once per second removes
	// that per-record overhead from the hot path while preserving rotate
	// semantics (daily/hourly/minutely still trigger within ~1s of the boundary).
	lastRotateCheck time.Time

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
	async         bool
	asyncBufSize  int
	messages      chan *Record
	policy        OverflowPolicy
	spiller       Spiller[*Record]
	spillType     string
	spillSize     int
	spillDir      string
	spillMaxBytes int64
	stats         OverflowStats
	written       uint64
	errored       uint64
	flushInterval time.Duration
	// flushBatchSize forces a flushSync() every N records written by the async
	// daemon (in addition to the time-based flushInterval ticker). <=0 uses
	// defaultFlushBatchSize. Bounds the crash-loss window in the high-rate
	// direction: at most flushBatchSize records + bufferSize bytes are unflushed.
	flushBatchSize int
	// batchSinceFlush counts writeOne records since the last batch flush; daemon-only.
	batchSinceFlush int
	onEvent         func(name string, delta int64)
	quit            chan struct{}
	stop            chan struct{}
	flushSig        chan struct{}
	wg              sync.WaitGroup
	// closing is set (atomic) BEFORE Stop closes the messages channel. Once set,
	// producers (send) stop attempting to enqueue and the daemon's drainSpill
	// stops re-injecting into messages, so nothing sends on messages after it is
	// closed. This closes the spill-policy shutdown race: without it the flush
	// ticker's drainSpill branch could select over the !ok receive and send on a
	// closed channel (panic), and in-flight OverflowBlock producers would block
	// forever or panic. drainSpillAll (the shutdown path) bypasses messages
	// entirely and writes directly, so it is unaffected.
	closing atomic.Bool
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
	// BufferSize is the bufio writer size in bytes (<=0 -> 64KB). A larger value
	// reduces flush-to-disk frequency under high write rates but raises the
	// worst-case crash-loss window (unflushed buffered bytes).
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`
	// FlushBatchSize forces a bufio flush every N records written by the async
	// daemon (<=0 -> 1000), in addition to the time-based flushInterval ticker.
	// Bounds worst-case crash loss in the high-rate direction: at most
	// FlushBatchSize records + BufferSize bytes are unflushed at any time.
	FlushBatchSize int `json:"flush_batch_size" mapstructure:"flush_batch_size"`

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

		async:          options.Async,
		asyncBufSize:   options.AsyncBufferSize,
		policy:         ParseOverflowPolicy(options.OverflowPolicy),
		spillType:      options.SpillType,
		spillSize:      options.SpillSize,
		spillDir:       options.SpillDir,
		spillMaxBytes:  options.SpillMaxBytes,
		flushInterval:  500 * time.Millisecond,
		flushBatchSize: options.FlushBatchSize,
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
//
// In async mode the incoming *Record is owned by the caller (the logger may
// recycle it from a pool right after this returns), while the daemon consumes
// it later. We therefore hand the daemon a private copy so it never races the
// caller's reuse of r. Record holds only immutable value types (strings + int),
// so a shallow copy is sufficient and correct. (In sync mode writeOne runs to
// completion before return, so no copy is needed.)
// Name returns WriterNameFile.
func (w *FileWriter) Name() string { return WriterNameFile }

// Pause drops incoming records without removing the writer or closing the file.
func (w *FileWriter) Pause() { w.paused.Store(true) }

// Resume restores delivery after Pause.
func (w *FileWriter) Resume() { w.paused.Store(false) }

// Paused reports whether the writer is currently paused.
func (w *FileWriter) Paused() bool { return w.paused.Load() }

func (w *FileWriter) Write(r *Record) error {
	if w.paused.Load() {
		return nil
	}
	if r.level > w.level {
		return nil
	}
	if !w.async || w.messages == nil {
		return w.writeSync(r)
	}
	rc := *r // private copy for the daemon
	w.send(&rc)
	return nil
}

// writeSync is the synchronous bufio write path.
func (w *FileWriter) writeSync(r *Record) error {
	if w.fileBufWriter == nil {
		return errors.New("fileWriter no opened file: " + w.filename)
	}
	// FormatJSON fast path: emit pre-serialized bytes verbatim (set by
	// deliverRecordToWriter) instead of re-rendering the text line.
	var err error
	if len(r.formattedBytes) > 0 {
		_, err = w.fileBufWriter.Write(r.formattedBytes)
	} else {
		_, err = w.fileBufWriter.WriteString(r.String())
	}
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
	var v int
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
		bufSize = defaultBufferSize
	}
	// bufio.NewWriterSize never returns nil for a valid size (>=1), and bufSize is
	// guaranteed >= defaultBufferSize above, so the previous nil-check was dead
	// code — removed during coverage hardening.
	w.fileBufWriter = bufio.NewWriterSize(w.file, bufSize)
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
//
// Shutdown safety: Stop sets w.closing and closes w.stop, but NEVER closes
// w.messages (see Stop docs). So send can never panic on a closed channel. The
// closing fast path drops records once shutdown begins (keeping Stop bounded),
// and every send also selects on w.stop so an OverflowBlock producer is
// unblocked when the daemon is winding down instead of waiting forever on a
// channel the daemon has stopped consuming.
func (w *FileWriter) send(r *Record) {
	if w.closing.Load() {
		w.stats.IncDropped()
		w.fire("drop", 1)
		return
	}
	switch w.policy {
	case OverflowBlock:
		select {
		case w.messages <- r:
		case <-w.stop:
			w.stats.IncDropped()
			w.fire("drop", 1)
		}
	case OverflowSpill:
		select {
		case w.messages <- r:
		case <-w.stop:
			w.stats.IncDropped()
			w.fire("drop", 1)
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
		case <-w.stop:
			w.stats.IncDropped()
			w.fire("drop", 1)
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
	w.stop = make(chan struct{})
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
//
// Shutdown is driven by w.stop (closed by Stop), NOT by closing w.messages.
// This is the key to the race-free shutdown: nothing ever closes messages, so
// there is no close-vs-send race. When stop fires the daemon drains everything
// still queued in messages (non-blocking), writes the spiller directly via
// drainSpillAll, flushes, signals quit, and exits. drainSpill (the
// re-inject-from-spiller path) is gated on closing so it never runs during
// shutdown, avoiding any send while the daemon is winding down.
func (w *FileWriter) daemon() {
	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			recordDaemonPanic("file", r)
		}
	}()
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case r, ok := <-w.messages:
			if !ok {
				// Defensive: messages should never be closed in normal operation
				// (Stop does not close it). If it ever is, treat as shutdown.
				w.drainQueuedAndSpill()
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
		case <-w.stop:
			w.drainQueuedAndSpill()
			_ = w.flushSync()
			w.quit <- struct{}{}
			return
		}
	}
}

// drainQueuedAndSpill writes everything still pending at shutdown: all records
// left in the messages channel (non-blocking drain) then the entire spill store
// (directly via writeOne, bypassing messages). Called only from the daemon on
// shutdown, so no concurrent writeOne.
func (w *FileWriter) drainQueuedAndSpill() {
	for {
		select {
		case r, ok := <-w.messages:
			if !ok {
				return
			}
			w.writeOne(r)
		default:
			w.drainSpillAll()
			return
		}
	}
}

// writeOne rotates (by time, throttled to at most one check per second) then
// writes the record to bufio, and every flushBatchSize records forces a
// flushSync() to bound the crash-loss window.
//
// rotateImpl is only called when at least rotateCheckInterval has elapsed since
// the last check; real rotation fires at minute/hour/day boundaries, so a <=1s
// check cadence preserves rotate semantics while removing the per-record
// time.Now()+actions-scan cost (the single biggest hot-path overhead under high
// write rates). The batch-count flush bounds crash loss in the high-rate
// direction; the time-based flushInterval ticker is the low-rate backstop.
func (w *FileWriter) writeOne(r *Record) {
	now := time.Now()
	if now.Sub(w.lastRotateCheck) >= rotateCheckInterval {
		w.lastRotateCheck = now
		_ = w.rotateImpl()
	}
	if w.fileBufWriter == nil {
		atomic.AddUint64(&w.errored, 1)
		w.fire("error", 1)
		return
	}
	// FormatJSON fast path: emit pre-serialized bytes (set by
	// deliverRecordToWriter) instead of re-rendering the text line.
	var writeErr error
	if len(r.formattedBytes) > 0 {
		_, writeErr = w.fileBufWriter.Write(r.formattedBytes)
	} else {
		_, writeErr = w.fileBufWriter.WriteString(r.String())
	}
	if writeErr != nil {
		atomic.AddUint64(&w.errored, 1)
		w.fire("error", 1)
		return
	}
	atomic.AddUint64(&w.written, 1)
	w.fire("written", 1)

	// Batch-count flush. flushBatchSize is resolved here (<=0 -> default) so the
	// struct field stays the raw option value.
	batchSize := w.flushBatchSize
	if batchSize <= 0 {
		batchSize = defaultFlushBatchSize
	}
	w.batchSinceFlush++
	if w.batchSinceFlush >= batchSize {
		w.batchSinceFlush = 0
		_ = w.flushSync()
	}
}

// drainSpill re-injects recovered records into the channel (non-blocking). It
// is called from the daemon's flush-ticker / flush-signal branches. Once Stop
// has set closing, drainSpill is a no-op: the shutdown path is handled by
// drainSpillAll (which writes directly via writeOne, bypassing messages), so
// re-injecting here would race close(w.messages) — the original spill-shutdown
// send-on-closed bug.
func (w *FileWriter) drainSpill() {
	if w.closing.Load() {
		return
	}
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

// Stop gracefully shuts the async daemon.
//
// Race-free ordering (the spill-policy shutdown fix):
//  1. closing=true -> new producers (send) and the daemon's drainSpill stop
//     touching messages immediately; drainSpill returns early so it can never
//     re-inject into messages during shutdown.
//  2. close(stop)  -> unblocks any producer waiting in send, AND wakes the
//     daemon's shutdown branch.
//  3. wait <-quit   -> the daemon has drained every queued message + the entire
//     spill store (written directly via writeOne), flushed bufio, and exited.
//
// Crucially Stop NEVER closes w.messages — nothing does. Closing it would race
// any concurrent send (close-vs-send is a true memory race and send-on-closed
// panics). Since closing=true gates send's fast path and the daemon drains all
// pending records before exiting, leaving messages open is correct: any record a
// racing producer slipped in after closing was set is either drained by the
// daemon or left in an unbuffered-to-GC channel (no panic, no race).
func (w *FileWriter) Stop() {
	if !w.async || w.messages == nil {
		return
	}
	w.closing.Store(true)
	close(w.stop)
	waitQuit("file", w.quit, defaultShutdownTimeout)
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

// CrashLossBound reports the worst-case data loss on a sudden process crash for
// an async FileWriter: at most this many records (plus the bufio buffer bytes)
// can be unflushed at any instant. The bound is the configured FlushBatchSize
// (records between forced flushes); the time-based flushInterval ticker is the
// low-rate backstop, FlushBatchSize is the high-rate bound. The bufio buffer
// adds up to the returned bytes of in-memory, unflushed data on top.
//
// Returned values resolve the <=0 defaults (FlushBatchSize<=0 -> 1000 records;
// BufferSize<=0 -> 64KB). For a sync (non-async) writer this is not meaningful
// (writes flush inline) and returns (0, 0).
func (w *FileWriter) CrashLossBound() (maxRecords int, maxBufferBytes int) {
	if !w.async {
		return 0, 0
	}
	maxRecords = w.flushBatchSize
	if maxRecords <= 0 {
		maxRecords = defaultFlushBatchSize
	}
	maxBufferBytes = w.bufferSize
	if maxBufferBytes <= 0 {
		maxBufferBytes = defaultBufferSize
	}
	return maxRecords, maxBufferBytes
}

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
