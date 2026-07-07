package log4go

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// OverflowPolicy controls what Write does when the async send channel is full.
type OverflowPolicy int

// OverflowDrop / OverflowBlock / OverflowSpill are the channel-full behaviors.
// They correspond to the OverflowPolicyDrop / OverflowPolicyBlock /
// OverflowPolicySpill string config values (see ParseOverflowPolicy).
const (
	OverflowDrop OverflowPolicy = iota
	OverflowBlock
	OverflowSpill
)

// String returns the lowercase policy name ("drop"/"block"/"spill").
func (p OverflowPolicy) String() string {
	switch p {
	case OverflowBlock:
		return "block"
	case OverflowSpill:
		return "spill"
	default:
		return "drop"
	}
}

// ParseOverflowPolicy parses a policy name, defaulting to OverflowDrop.
func ParseOverflowPolicy(s string) OverflowPolicy {
	switch s {
	case OverflowPolicyBlock:
		return OverflowBlock
	case OverflowPolicySpill:
		return OverflowSpill
	default:
		return OverflowDrop
	}
}

// String constants for KafKaWriterOptions.OverflowPolicy (the struct field is a
// string, parsed by ParseOverflowPolicy at construction). Use these instead of
// magic strings for type safety and IDE autocomplete.
const (
	// OverflowPolicyDrop: when the channel is full, the record is silently
	// dropped (IncDropped counter). Zero hot-path overhead. Data is permanently
	// lost. The correct default for ad-tech / RTB logging (logs are lossy; never
	// block the bidding loop).
	OverflowPolicyDrop = "drop"
	// OverflowPolicyBlock: when full, Write blocks until the channel has space.
	// Provides backpressure but can stall the application hot path. Pair with a
	// large BufferSize. Not recommended for latency-critical paths.
	OverflowPolicyBlock = "block"
	// OverflowPolicySpill: when full, the record goes to a bounded recovery store
	// (SpillType: ring in-memory, or file disk-backed, or chain ring→file). The
	// daemon drains the spill back into the channel when it recovers. No data loss
	// (file survives process crash). Use for money/critical-state data. Normal
	// path (channel not full) is identical to drop — zero overhead.
	OverflowPolicySpill = "spill"
)

// String constants for KafKaWriterOptions.SpillType.
const (
	// SpillTypeRing: in-memory circular buffer. Fast (mutex + slice append). Lost
	// on process crash. Overwrites oldest when full. Default.
	SpillTypeRing = "ring"
	// SpillTypeFile: disk-backed append file, bounded by SpillMaxBytes. Slower
	// (disk IO) but survives process crash. Records recovered on next Start().
	SpillTypeFile = "file"
	// SpillTypeChain: ring (hot, in-memory) → file (cold, persistent) → drop.
	// Best of both: ring absorbs brief stalls instantly, file catches prolonged
	// outages. Total space bounded: ring cap + file MaxBytes. Default when
	// SpillDir is set.
	SpillTypeChain = "chain"
)

// String constants for KafKaWriterOptions.DeliveryMode (partition consumer only;
// for KafKaWriter this field is unused — the writer always uses callback mode
// internally).
const (
	DeliveryModeCallback = "callback"
	DeliveryModeChannel  = "channel"
)

// OverflowStats reports overflow accounting (thread-safe) and emits throttled
// alerts (standard log by default; optional AlertSink for webhook/OA push) on
// the first event and every Nth event.
type OverflowStats struct {
	dropped    uint64
	spilled    uint64
	dropEvery  uint64 // log every N drops (0 = only first)
	spillEvery uint64 // log every N spills
	alertSink  AlertSink
}

// SetAlertEvery configures throttled overflow logging (every N events).
func (s *OverflowStats) SetAlertEvery(dropEvery, spillEvery uint64) {
	s.dropEvery = dropEvery
	s.spillEvery = spillEvery
}

// SetAlertSink installs an alert sink (e.g. WebhookAlertSink for lark/dingtalk/
// feishu). Nil disables push (standard log only).
func (s *OverflowStats) SetAlertSink(sink AlertSink) {
	s.alertSink = sink
}

// IncDropped increments the dropped counter and emits an alert on the first
// drop and every dropEvery drops.
func (s *OverflowStats) IncDropped() {
	n := atomic.AddUint64(&s.dropped, 1)
	s.alert("DROP", n, s.dropEvery, "queue full; record lost")
}

// IncSpilled increments the spilled counter and emits an alert on the first
// spill and every spillEvery spills.
func (s *OverflowStats) IncSpilled() {
	n := atomic.AddUint64(&s.spilled, 1)
	s.alert("SPILL", n, s.spillEvery, "overflow buffered to spill store")
}

func (s *OverflowStats) alert(kind string, n, every uint64, reason string) {
	level := AlertWarn
	if kind == "DROP" {
		level = AlertError
	}
	fire := false
	text := ""
	switch {
	case n == 1:
		fire = true
		text = "first event: " + reason
	case every > 0 && n%every == 0:
		fire = true
		text = fmt.Sprintf("total=%d (logged every %d): %s", n, every, reason)
	}
	if !fire {
		return
	}
	log.Printf("[log4go] overflow %s %s", kind, text)
	if s.alertSink != nil {
		s.alertSink.Send(level, kind, text)
	}
}

// Dropped returns the total number of records dropped under the drop policy
// (thread-safe).
func (s *OverflowStats) Dropped() uint64 { return atomic.LoadUint64(&s.dropped) }

// Spilled returns the total number of records buffered to a spill store under
// the spill policy (thread-safe).
func (s *OverflowStats) Spilled() uint64 { return atomic.LoadUint64(&s.spilled) }

// Spiller is a bounded fallback store for type T. Always size-limited so it
// cannot cause OOM. Records are recovered via Drain.
type Spiller[T any] interface {
	Push(msg T) bool // false = at capacity (caller may drop)
	Drain() []T      // recover and remove stored records
	Len() int
	Close() error
}

// SpillerRecoverable is implemented by spillers with a persistent backend
// (e.g. FileSpiller); used to detect resumable spill on startup.
type SpillerRecoverable interface {
	HasPersistent() bool
}

// SpillCodec serializes T to/from bytes for FileSpiller persistence.
type SpillCodec[T any] interface {
	Encode(msg T) ([]byte, error)
	Decode(b []byte) (T, error)
}

// RingSpiller is a fixed-capacity in-memory ring; overwrites oldest when full.
type RingSpiller[T any] struct {
	mu   sync.Mutex
	buf  []T
	head int
	size int
	capv int
}

// NewRingSpiller creates a ring with the given capacity (min 1).
func NewRingSpiller[T any](capacity int) *RingSpiller[T] {
	if capacity < 1 {
		capacity = 1024
	}
	return &RingSpiller[T]{buf: make([]T, capacity), capv: capacity}
}

// Push appends msg, overwriting the oldest entry when the ring is full. It
// always returns true (a ring never refuses). See PushNoOverwrite for the
// non-overwriting variant used by ChainedSpiller.
func (r *RingSpiller[T]) Push(msg T) bool {
	r.mu.Lock()
	r.buf[r.head] = msg
	r.head = (r.head + 1) % r.capv
	if r.size < r.capv {
		r.size++
	}
	r.mu.Unlock()
	return true
}

// Drain recovers and removes all stored records in insertion order, returning
// nil when empty. The ring is reset to empty after the call.
func (r *RingSpiller[T]) Drain() []T {
	r.mu.Lock()
	if r.size == 0 {
		r.mu.Unlock()
		return nil
	}
	out := make([]T, 0, r.size)
	start := (r.head - r.size + r.capv) % r.capv
	var zero T
	for i := range r.size {
		idx := (start + i) % r.capv
		out = append(out, r.buf[idx])
		r.buf[idx] = zero
	}
	r.size = 0
	r.head = 0
	r.mu.Unlock()
	return out
}

// Len returns the number of records currently stored (thread-safe).
func (r *RingSpiller[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}

// PushNoOverwrite pushes without overwriting; returns false if the ring is full.
// Used by ChainedSpiller so a full ring overflows to the next level instead of
// silently dropping the oldest record.
func (r *RingSpiller[T]) PushNoOverwrite(msg T) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size >= r.capv {
		return false
	}
	r.buf[r.head] = msg
	r.head = (r.head + 1) % r.capv
	r.size++
	return true
}

// Close is a no-op for the in-memory ring.
func (r *RingSpiller[T]) Close() error { return nil }

// FileSpiller persists overflowed records to disk (length-prefixed framing),
// bounded by MaxBytes. Recover via Drain. Survives process memory pressure.
type FileSpiller[T any] struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	codec    SpillCodec[T]
	w        *bufio.Writer
	f        *os.File
	written  int64
	count    int64
}

// NewFileSpiller creates a disk spiller in dir bounded by maxBytes (<=0 -> 64MB).
func NewFileSpiller[T any](dir string, maxBytes int64, codec SpillCodec[T]) (*FileSpiller[T], error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	sp := &FileSpiller[T]{dir: dir, maxBytes: maxBytes, codec: codec}
	if err := sp.open(); err != nil {
		return nil, err
	}
	return sp, nil
}

func (f *FileSpiller[T]) open() error {
	p := filepath.Join(f.dir, "spill.log")
	fh, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // internal spill file
	if err != nil {
		return err
	}
	if fi, _ := fh.Stat(); fi != nil {
		f.written = fi.Size()
	}
	f.f = fh
	f.w = bufio.NewWriter(fh)
	return nil
}

// Push appends msg as a length-prefixed framed record. Returns false when the
// record would exceed MaxBytes or on a write/encode error (caller may drop).
func (f *FileSpiller[T]) Push(msg T) bool {
	b, err := f.codec.Encode(msg)
	if err != nil {
		return false
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	recLen := int64(4 + len(b))
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.written+recLen > f.maxBytes {
		return false
	}
	if _, err := f.w.Write(hdr[:]); err != nil {
		return false
	}
	if _, err := f.w.Write(b); err != nil {
		return false
	}
	f.written += recLen
	f.count++
	return true
}

// Drain recovers all persisted records: it flushes, renames the active spill
// file aside, decodes it, and removes it. Returns nil on error or emptiness.
// Safe to call repeatedly; each call drains whatever was appended since the
// previous Drain.
func (f *FileSpiller[T]) Drain() []T {
	f.mu.Lock()
	if f.w != nil {
		_ = f.w.Flush()
	}
	if f.f != nil {
		_ = f.f.Close()
	}
	readPath := filepath.Join(f.dir, "spill.log.read")
	writePath := filepath.Join(f.dir, "spill.log")
	if err := os.Rename(writePath, readPath); err != nil {
		_ = f.open()
		f.mu.Unlock()
		return nil
	}
	_ = f.open()
	f.count = 0
	codec := f.codec
	f.mu.Unlock()

	out := decodeSpillFile[T](readPath, codec)
	_ = os.Remove(readPath)
	return out
}

func decodeSpillFile[T any](path string, codec SpillCodec[T]) []T {
	fh, err := os.Open(path) //nolint:gosec // internal spill file
	if err != nil {
		return nil
	}
	defer func() { _ = fh.Close() }() // read-only spill file; close error is irrelevant
	r := bufio.NewReader(fh)
	var out []T
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			break
		}
		l := binary.BigEndian.Uint32(hdr[:])
		buf := make([]byte, l)
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		v, err := codec.Decode(buf)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

// Len returns the number of records currently persisted (thread-safe).
func (f *FileSpiller[T]) Len() int { return int(atomic.LoadInt64(&f.count)) }

// Close flushes the bufio writer and closes the spill file. Idempotent.
func (f *FileSpiller[T]) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.w != nil {
		_ = f.w.Flush()
	}
	if f.f != nil {
		return f.f.Close()
	}
	return nil
}

// HasPersistent marks FileSpiller as resumable on startup.
func (f *FileSpiller[T]) HasPersistent() bool { return true }

// Dir returns the spill directory (for startup-recovery detection).
func (f *FileSpiller[T]) Dir() string { return f.dir }

// ChainedSpiller is multi-level overflow: ring (hot, in-memory) -> file
// (cold, persistent) -> drop. Total space is bounded: ring cap + file MaxBytes.
type ChainedSpiller[T any] struct {
	ring *RingSpiller[T]
	file *FileSpiller[T]
}

// NewChainedSpiller composes a ring (hot) and an optional file (cold) spiller.
func NewChainedSpiller[T any](ring *RingSpiller[T], file *FileSpiller[T]) *ChainedSpiller[T] {
	return &ChainedSpiller[T]{ring: ring, file: file}
}

// Push tries the ring (without overwriting) first, then the file. Returns
// false only when both are at capacity.
func (c *ChainedSpiller[T]) Push(msg T) bool {
	if c.ring != nil && c.ring.PushNoOverwrite(msg) {
		return true
	}
	if c.file != nil && c.file.Push(msg) {
		return true
	}
	return false
}

// Drain recovers and removes records from both the ring and the file (ring
// records first, then file records).
func (c *ChainedSpiller[T]) Drain() []T {
	var out []T
	if c.ring != nil {
		out = append(out, c.ring.Drain()...)
	}
	if c.file != nil {
		out = append(out, c.file.Drain()...)
	}
	return out
}

// Len returns the total record count across the ring and the file.
func (c *ChainedSpiller[T]) Len() int {
	n := 0
	if c.ring != nil {
		n += c.ring.Len()
	}
	if c.file != nil {
		n += c.file.Len()
	}
	return n
}

// Close closes both backends, returning the file backend's error (if any).
func (c *ChainedSpiller[T]) Close() error {
	if c.ring != nil {
		_ = c.ring.Close()
	}
	if c.file != nil {
		return c.file.Close()
	}
	return nil
}

// HasPersistent reports whether a file backend is present (resumable).
func (c *ChainedSpiller[T]) HasPersistent() bool { return c.file != nil }

// File returns the file backend (may be nil), for startup recovery.
func (c *ChainedSpiller[T]) File() *FileSpiller[T] { return c.file }

// ---- shared codecs ----

type spillRecord struct {
	Topic     string `json:"topic"`
	Key       []byte `json:"key,omitempty"`
	Value     []byte `json:"value,omitempty"`
	Timestamp int64  `json:"ts,omitempty"`
}

// producerMsgCodec is the shared SpillCodec for kafka.Message.
type producerMsgCodec struct{}

// Encode marshals msg to the spillRecord JSON framing.
func (producerMsgCodec) Encode(msg kafka.Message) ([]byte, error) {
	rec := spillRecord{Topic: msg.Topic}
	if len(msg.Key) > 0 {
		rec.Key = msg.Key
	}
	if len(msg.Value) > 0 {
		rec.Value = msg.Value
	}
	if !msg.Timestamp.IsZero() {
		rec.Timestamp = msg.Timestamp.UnixNano()
	}
	return json.Marshal(rec)
}

// Decode unmarshals spillRecord JSON back into a kafka.Message.
func (producerMsgCodec) Decode(b []byte) (kafka.Message, error) {
	var rec spillRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return kafka.Message{}, err
	}
	msg := kafka.Message{Topic: rec.Topic, Key: rec.Key, Value: rec.Value}
	if rec.Timestamp != 0 {
		msg.Timestamp = time.Unix(0, rec.Timestamp)
	}
	return msg, nil
}

// ProducerMsgCodec is the shared codec for kafka.Message spill.
var ProducerMsgCodec SpillCodec[kafka.Message] = producerMsgCodec{}

type spillRecordData struct {
	Level int    `json:"level"`
	Time  string `json:"time"`
	File  string `json:"file"`
	Msg   string `json:"msg"`
}

// recordCodec is the shared SpillCodec for *Record.
type recordCodec struct{}

// Encode marshals the record's level/time/file/msg to JSON.
func (recordCodec) Encode(r *Record) ([]byte, error) {
	return json.Marshal(spillRecordData{Level: r.level, Time: r.time, File: r.file, Msg: r.msg})
}

// Decode unmarshals the JSON back into a *Record.
func (recordCodec) Decode(b []byte) (*Record, error) {
	var d spillRecordData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &Record{level: d.Level, time: d.Time, file: d.File, msg: d.Msg}, nil
}

// RecordCodec is the shared codec for *Record spill (FileWriter).
var RecordCodec SpillCodec[*Record] = recordCodec{}

// DrainFileRecover reads any persisted spill.log in dir without holding an
// open spiller — used at startup to resume from an interrupted process.
func DrainFileRecover[T any](dir string, codec SpillCodec[T]) []T {
	readPath := filepath.Join(dir, "spill.log")
	if _, err := os.Stat(readPath); err != nil {
		return nil
	}
	// move aside so a concurrently-opening spiller won't fight us
	tmp := filepath.Join(dir, "spill.log.recover")
	if err := os.Rename(readPath, tmp); err != nil {
		return nil
	}
	out := decodeSpillFile[T](tmp, codec)
	_ = os.Remove(tmp)
	return out
}
