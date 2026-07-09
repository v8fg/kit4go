package log4go

import (
	"context"
	"log"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// KafkaMSGFields holds the legacy Kafka→ES message fields.
//
// Field management is unified with the Logger's structured-fields layer:
// SetBaseField/RemoveBaseField (global static), With/WithField/WithFields
// (per-scope) and context extractors (per-request) all flow into r.fields, and
// those take PRIORITY over the struct members here. The struct members below are
// kept as a backward-compatible fallback for callers that configure the writer
// directly. New code should prefer:
//
//	log4go.SetBaseField("server_ip", ip)
//	log4go.SetBaseField("es_index", "adx-logs-2026.06")
//
// so the same field is carried by every writer on the logger, not just Kafka.
type KafkaMSGFields struct {
	// ESIndex and ServerIP are routing/static fields. Deprecated as struct
	// members — prefer SetBaseField("es_index",…)/SetBaseField("server_ip",…).
	// They are emitted only when the r.fields layer did not already supply the
	// same key, so a Base Field always wins.
	ESIndex   string `json:"es_index" mapstructure:"es_index"`
	Level     string `json:"level"`     // dynamic, set by logger, mark the record level
	File      string `json:"file"`      // source code file:line_number
	Message   string `json:"message"`   // required, dynamic
	ServerIP  string `json:"server_ip"` // init field, set by app
	Timestamp string `json:"timestamp"` // required, dynamic, set by logger
	Now       int64  `json:"now"`       // choice

	// ExtraFields are merged into the top-level JSON on send, below r.fields in
	// priority (a Base/With/Context field of the same key wins). Prefer
	// SetBaseField for static fields.
	ExtraFields map[string]any `json:"extra_fields" mapstructure:"extra_fields"`
}

// KafkaWriterOptions kafka writer options.
//
// Performance notes: BufferSize bounds the async send channel; OverflowPolicy
// ("drop"|"block"|"spill") decides what happens when it is full and is the
// primary OOM guard. Under "spill", SpillType selects the bounded recovery
// store ("ring" in-memory ring, or "file" disk-backed) whose drained records
// are re-sent once the channel recovers.
type KafkaWriterOptions struct {
	Enable                  bool `json:"enable" mapstructure:"enable"`
	Debug                   bool `json:"debug" mapstructure:"debug"`                     // if true, will output the send msg
	SpecifyVersion          bool `json:"specify_version" mapstructure:"specify_version"` // if use the input version, default false
	ProducerReturnSuccesses bool `json:"producer_return_successes" mapstructure:"producer_return_successes"`
	BufferSize              int  `json:"buffer_size" mapstructure:"buffer_size"` // async send channel size (bounded)

	Level      string `json:"level" mapstructure:"level"`
	VersionStr string `json:"version" mapstructure:"version"` // kafka version, ex: 0.10.0.1 or 1.1.1

	Key string `json:"key" mapstructure:"key"` // kafka producer key, choice field

	ProducerTopic   string        `json:"producer_topic" mapstructure:"producer_topic"`
	ProducerTimeout time.Duration `json:"producer_timeout" mapstructure:"producer_timeout"`
	Brokers         []string      `json:"brokers" mapstructure:"brokers"`

	// OverflowPolicy: OverflowPolicyDrop(default)|OverflowPolicyBlock|
	// OverflowPolicySpill — behavior when the channel is full. See the constants
	// in kafka_overflow.go for tradeoffs (drop=fast+lossy, spill=durable).
	OverflowPolicy string `json:"overflow_policy" mapstructure:"overflow_policy"`
	// SpillType: SpillTypeRing(default)|SpillTypeFile|SpillTypeChain — recovery
	// store when policy == spill. See constants in kafka_overflow.go.
	SpillType string `json:"spill_type" mapstructure:"spill_type"`
	// SpillSize: ring capacity (records) for "ring".
	SpillSize int `json:"spill_size" mapstructure:"spill_size"`
	// SpillDir: directory for "file" spill.
	SpillDir string `json:"spill_dir" mapstructure:"spill_dir"`
	// SpillMaxBytes: byte cap for "file" spill (<=0 -> 64MB).
	SpillMaxBytes int64 `json:"spill_max_bytes" mapstructure:"spill_max_bytes"`

	// BatchMode: when true the daemon accumulates records and flushes via
	// producer.SendBatch (fewer producer calls → lower daemon CPU at high QPS)
	// instead of one producer.Send per record. Default false (per-record Send,
	// lowest latency, current behavior). Enable for high-throughput pipelines.
	// Note: the kafka backend already batches at the broker via ProducerLinger
	// (default 10ms), so this mainly cuts daemon-side per-call overhead.
	// DATA-LOSS RISK: records buffered in the un-flushed batch are lost if the
	// process crashes before Stop() flushes them — keep BatchFlushInterval small.
	BatchMode bool `json:"batch_mode" mapstructure:"batch_mode"`
	// BatchSize: max records per batch; flush immediately when reached.
	// <=0 → DefaultKafkaBatchSize (1024). 1024 is the stress-matrix best default:
	// near-peak QPS for franz-go (needs batch ≥1024), flat for sarama, no extra
	// memory (in-flight buffer bounded by the backend MaxBufferedRecords). See
	// kafka/STRESS_MATRIX.md.
	BatchSize int `json:"batch_size" mapstructure:"batch_size"`
	// BatchFlushInterval: max time to hold a partial batch before flushing
	// (linger); flush when elapsed. <=0 → 50ms. Pairs with BatchSize (whichever
	// fires first). Worst-case latency for the last record in a partial batch.
	BatchFlushInterval time.Duration `json:"batch_flush_interval" mapstructure:"batch_flush_interval"`

	// ProducerLinger tunes the kafka BACKEND's batch flush delay — the latency/
	// throughput knob on the underlying producer. 0 (default) = the kafka package
	// default (10ms): the backend batches at the broker level, so log4go BatchMode
	// then adds little (see PERFORMANCE.en.md §22). <0 (e.g. kafka.LingerOff) =
	// disable backend batching (flush every record): pair with BatchMode=true so
	// log4go-level batching becomes the ONLY batching layer — this is where
	// BatchMode actually pays off (amortizes per-record produce cost). >0 =
	// explicit linger (e.g. 5ms).
	ProducerLinger time.Duration `json:"producer_linger" mapstructure:"producer_linger"`

	// Acks controls the kafka producer's required acknowledgments. Empty (default)
	// = leader (unified across both backends). Set "all" for durability (enables
	// franz-go's idempotent producer; slower under RF>1). See kafka.AcksLeader /
	// kafka.AcksAll / kafka.AcksNone constants.
	Acks string `json:"acks" mapstructure:"acks"`

	// BreakerDisabled opts out of the inline producer circuit breaker (L4).
	// Default false (breaker ON): under a sustained broker outage the daemon
	// diverts records to the spill store (OverflowSpill) instead of futile
	// Sends, so a kafka-side failure doesn't lose records or stall the caller.
	// Disable only if you handle downstream errors yourself.
	BreakerDisabled bool `json:"breaker_disabled" mapstructure:"breaker_disabled"`
	// BreakerFailRate is the error/sent ratio that trips the breaker over
	// BreakerWindow. <=0 → 0.5 (50% — trips on real outages, not transient blips).
	BreakerFailRate float64 `json:"breaker_fail_rate" mapstructure:"breaker_fail_rate"`
	// BreakerMinSamples is the minimum sends in a window before the rate is
	// trusted. <=0 → 20. Guards against tripping on tiny low-volume windows.
	BreakerMinSamples uint64 `json:"breaker_min_samples" mapstructure:"breaker_min_samples"`
	// BreakerWindow is the rolling evaluation window. <=0 → 2s.
	BreakerWindow time.Duration `json:"breaker_window" mapstructure:"breaker_window"`
	// BreakerCooldown is how long the breaker stays open before probing
	// recovery (half-open). <=0 → 5s.
	BreakerCooldown time.Duration `json:"breaker_cooldown" mapstructure:"breaker_cooldown"`

	// ProducerSnapshotHistory enables the underlying kafka.Producer's Snapshot
	// history ring (retains the last N Snapshot() samples for trend analysis).
	// 0 (default) = disabled. Set to e.g. 60 for 1 min at 1s scrape cadence.
	// Access via ProducerSnapshot() + type-assert for kafka.SnapshotHistory.
	ProducerSnapshotHistory int `json:"producer_snapshot_history" mapstructure:"producer_snapshot_history"`

	MSG KafkaMSGFields `json:"msg"`
}

// KafkaWriter kafka writer (async, bounded, overflow-safe).
type KafkaWriter struct {
	level    int
	paused   atomic.Bool
	producer kafka.Producer
	messages chan kafka.Message
	options  KafkaWriterOptions

	policy     OverflowPolicy
	spiller    Spiller[kafka.Message]
	breaker    *kafkaBreaker // inline circuit breaker; nil only when disabled (L4)
	stats      OverflowStats
	sent       uint64
	errored    uint64
	failovered uint64 // records diverted to spill on breaker-open / sync send-error (L4)
	// onEvent is an optional real-time metric hook (reserved for monitoring
	// integration). Nil disables it. Must be non-blocking. Stored atomically so
	// SetOnEvent can race safely with the daemon reader.
	onEvent         atomic.Pointer[func(name string, delta int64)]
	producerFactory func() (kafka.Producer, error)
	drainInterval   time.Duration
	// batch-mode tuning (resolved from KafkaWriterOptions in NewKafkaWriter).
	// batchMode=false (default) → per-record Send; true → accumulate + SendBatch.
	batchMode          bool
	batchSize          int
	batchFlushInterval time.Duration
	batches            uint64 // atomic: SendBatch call count (0 in per-record mode)
	batchMax           uint64 // atomic: largest batch size flushed (single writer: the daemon)
	wg                 sync.WaitGroup
	codecMu            sync.RWMutex
	codec              KafkaCodec // default KafkaCodecJSON; swap via SetKafkaCodec (rare)

	run  atomic.Bool // set true once the daemon starts
	quit chan struct{}
	// stop, once closed by Stop, unblocks any producer waiting in send (the
	// OverflowBlock branch) AND wakes the daemon's shutdown branch. Distinct from
	// quit (the daemon→Stop completion signal): stop is the Stop→daemon/producers
	// shutdown trigger. Mirrors FileWriter's stop channel.
	stop chan struct{}
	// closing is set (atomic) BEFORE Stop closes k.stop. Once set, send stops
	// attempting to enqueue (fast-exit drop) and drainSpill stops re-injecting
	// into messages, so nothing sends on messages during shutdown. Stop NEVER
	// closes messages — closing it would race any concurrent send (close-vs-send
	// is a true memory race and send-on-closed panics). The daemon drains all
	// pending records on the stop branch before exiting, so leaving messages open
	// is correct: any record a racing producer slipped in after closing was set is
	// either drained or left for GC (no panic, no race). Mirrors FileWriter's
	// shutdown-safe pattern (file_writer.go:131-139, 795-812).
	closing atomic.Bool
}

// DefaultKafkaBatchSize is the SendBatch size applied when KafkaWriterOptions.
// BatchSize is <= 0. 1024 is the stress-matrix best default (kafka/STRESS_MATRIX.md):
// near-peak QPS for franz-go (which needs batch ≥1024), flat for sarama, and no
// extra memory (in-flight buffer is bounded by the backend's MaxBufferedRecords,
// not the SendBatch size).
const DefaultKafkaBatchSize = 1024

// NewKafkaWriter new kafka writer
func NewKafkaWriter(options KafkaWriterOptions) *KafkaWriter {
	defaultLevel := DEBUG
	if len(options.Level) > 0 {
		defaultLevel = getLevelDefault(options.Level, defaultLevel, "")
	}
	w := &KafkaWriter{
		options:            options,
		quit:               make(chan struct{}),
		level:              defaultLevel,
		policy:             ParseOverflowPolicy(options.OverflowPolicy),
		drainInterval:      200 * time.Millisecond,
		batchMode:          options.BatchMode,
		batchSize:          options.BatchSize,
		batchFlushInterval: options.BatchFlushInterval,
	}
	if w.batchSize <= 0 {
		w.batchSize = DefaultKafkaBatchSize // 1024 — see PERFORMANCE.en.md §22 / kafka STRESS_MATRIX.md
	}
	if w.batchFlushInterval <= 0 {
		w.batchFlushInterval = 50 * time.Millisecond
	}
	w.codec = KafkaCodecJSON{} // default codec
	w.stats.SetAlertEvery(1000, 1000)
	// inline circuit breaker (default ON; opt out via BreakerDisabled). Opens on
	// a sustained broker-error rate so the daemon can fail records over to the
	// spill store instead of losing them (L4: downstream isolation).
	w.breaker = newKafkaBreakerFromOptions(options, time.Now())
	return w
}

// Init service for Record
func (k *KafkaWriter) Init() error {
	return k.Start()
}

// kafkaPayload is the on-the-wire Kafka→ES record shape. Custom MarshalJSON
// avoids map alloc + reflection on the hot path.
//
// Framework fields (unix_nano/seq/level/file/message/timestamp/now) are always
// derived from the Record. unix_nano + seq are the strict-ordering keys an ES
// consumer sorts on (seq primary, unix_nano tie-break) to reconstruct exact log
// order across partitions/cores. Routing fields (es_index/server_ip) and all
// business fields are taken from Fields (= the Record's Base+With+Context
// fields) first, falling back to the legacy KafkaMSGFields struct members only
// when Fields did not already supply them — so SetBaseField is the single source
// of truth and always wins.
type kafkaPayload struct {
	UnixNano   int64
	Seq        uint64
	Level      string
	File       string
	Message    string
	ServerIP   string // legacy fallback; prefer Base Field
	Now        int64
	ESIndex    string  // legacy fallback; prefer Base Field
	userFields []field // merged r.fields (typed) + ExtraFields (typed); r.fields wins
}

// MarshalJSON renders the payload by direct typed byte append: framework fields
// (int64/uint64/escaped strings) append inline (zero reflection, zero map
// alloc), and user fields go through appendFieldJSON (scalars allocation-free;
// kindAny via the active codec). String framework fields are JSON-escaped so a
// message containing quotes can never break the document.
//
// Field priority:
//   - Framework keys (unix_nano/seq/level/file/message/timestamp/now) are always
//     emitted from the record; a same-named user field is ignored (reserved).
//   - Routing keys (server_ip/es_index): a user Field wins; otherwise the legacy
//     struct member is the fallback.
//   - Any other key is emitted from userFields.
func (p kafkaPayload) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 256)
	buf = append(buf, '{')
	// --- fixed framework block (always present) ---
	buf = append(buf, `"unix_nano":`...)
	buf = strconv.AppendInt(buf, p.UnixNano, 10)
	buf = append(buf, `,"seq":`...)
	buf = strconv.AppendUint(buf, p.Seq, 10)
	buf = append(buf, `,"level":`...)
	buf = appendJSONQuoted(buf, p.Level)
	if p.File != "" {
		buf = append(buf, `,"file":`...)
		buf = appendJSONQuoted(buf, p.File)
	}
	buf = append(buf, `,"message":`...)
	buf = appendJSONQuoted(buf, p.Message)
	buf = append(buf, `,"timestamp":"`...)
	buf = appendISOTimeUTC(buf, p.UnixNano)
	buf = append(buf, '"')
	buf = append(buf, `,"now":`...)
	buf = strconv.AppendInt(buf, p.Now, 10)
	// --- routing fields: user Field wins over legacy struct fallback ---
	buf = appendRoutingField(buf, "server_ip", p.userFields, p.ServerIP)
	buf = appendRoutingField(buf, "es_index", p.userFields, p.ESIndex)
	// --- remaining user fields (skip reserved + routing keys already emitted) ---
	for _, f := range p.userFields {
		if kafkaReservedKey(f.key) || f.key == "server_ip" || f.key == "es_index" {
			continue
		}
		buf = append(buf, ',')
		buf = appendFieldJSON(buf, f)
	}
	buf = append(buf, '}')
	return buf, nil
}

// kafkaReservedKey reports whether k is a framework field always emitted from the
// record (a same-named user field is intentionally ignored so the framework value
// is authoritative). server_ip/es_index are NOT reserved — they are routing
// fields a user Field may legitimately override.
func kafkaReservedKey(k string) bool {
	switch k {
	case "unix_nano", "seq", "level", "file", "message", "timestamp", "now":
		return true
	}
	return false
}

// appendRoutingField emits a routing key (server_ip/es_index) exactly once: from
// the typed user fields if present (Base Field wins), otherwise from the legacy
// struct fallback when non-empty. Omitted entirely when neither supplies a value.
func appendRoutingField(buf []byte, key string, fields []field, fallback string) []byte {
	for _, f := range fields {
		if f.key == key {
			buf = append(buf, ',')
			return appendFieldJSON(buf, f)
		}
	}
	if fallback != "" {
		buf = append(buf, ',', '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		return appendJSONQuoted(buf, fallback)
	}
	return buf
}

// buildPayload serializes the record to a JSON payload once (struct path = zero
// map alloc). It reuses the Record's already-captured time (r.unixNano) rather
// than calling time.Now() again — there is exactly one clock read per record
// (in deliverRecordToWriter), so the Kafka timestamp never drifts from the
// record's own time, and the payload carries the strict-ordering keys
// (unix_nano/seq) the ES consumer sorts on.
func (k *KafkaWriter) buildPayload(r *Record) []byte {
	p := kafkaPayload{
		UnixNano: r.unixNano,
		Seq:      r.seq,
		Level:    LevelFlags[r.level],
		File:     r.file,
		Message:  r.msg,
		ServerIP: k.options.MSG.ServerIP,
		// Timestamp is formatted directly in MarshalJSON from UnixNano via
		// appendISOTimeUTC (RFC3339 micros, UTC) — ES-friendly, no string alloc.
		Now:     r.unixNano / 1e9,
		ESIndex: k.options.MSG.ESIndex,
	}
	// Attach user fields. Three cases avoid a dedup map on the hot single-source
	// paths (the common Base-Fields-only and ExtraFields-only shapes):
	//   - r.fields only: share the slice read-only (duplicate keys keep the last
	//     value, which is the With-overrides-Base semantics already established
	//     when the logger merged them).
	//   - ExtraFields only: map keys are unique, no dedup needed.
	//   - both: dedup by key (r.fields wins) via a small set.
	rf := r.fields
	ef := k.options.MSG.ExtraFields
	switch {
	case len(rf) > 0 && len(ef) == 0:
		p.userFields = rf
	case len(rf) == 0 && len(ef) > 0:
		uf := make([]field, 0, len(ef))
		for fk, fv := range ef {
			uf = append(uf, fieldOf(fk, fv))
		}
		p.userFields = uf
	case len(rf) > 0:
		seen := make(map[string]struct{}, len(rf)+len(ef))
		uf := make([]field, 0, len(rf)+len(ef))
		for _, f := range rf {
			if _, ok := seen[f.key]; ok {
				continue
			}
			seen[f.key] = struct{}{}
			uf = append(uf, f)
		}
		for fk, fv := range ef {
			if _, ok := seen[fk]; ok {
				continue
			}
			seen[fk] = struct{}{}
			uf = append(uf, fieldOf(fk, fv))
		}
		p.userFields = uf
	}
	k.codecMu.RLock()
	c := k.codec
	k.codecMu.RUnlock()
	if c != nil {
		return c.Encode(&p)
	}
	b, _ := p.MarshalJSON()
	return b
}

// SetKafkaCodec swaps the serialization codec (JSON default ↔ Protobuf).
// RWMutex-protected — safe under concurrent logging, takes effect on the next
// record. Codec swaps are rare (config change, not per-record).
func (k *KafkaWriter) SetKafkaCodec(c KafkaCodec) {
	if c == nil {
		c = KafkaCodecJSON{}
	}
	k.codecMu.Lock()
	k.codec = c
	k.codecMu.Unlock()
}

// Name returns WriterNameKafka.
func (k *KafkaWriter) Name() string { return WriterNameKafka }

// Pause drops incoming records without removing the writer or closing the producer.
func (k *KafkaWriter) Pause() { k.paused.Store(true) }

// Resume restores delivery after Pause.
func (k *KafkaWriter) Resume() { k.paused.Store(false) }

// Paused reports whether the writer is currently paused.
func (k *KafkaWriter) Paused() bool { return k.paused.Load() }

// Write writes r by building the Kafka payload (JSON or protobuf, per the
// configured codec) and delivering it to a bounded channel under the configured
// overflow policy. It never spawns a goroutine per record. An empty message is
// skipped (no payload).
func (k *KafkaWriter) Write(r *Record) error {
	if k.paused.Load() {
		return nil
	}
	if r.level > k.level {
		return nil
	}
	if r.msg == "" {
		return nil
	}
	// buildPayload never returns nil (MarshalJSON is infallible), so the former
	// nil-guard was unreachable dead code — removed during coverage hardening.
	payload := k.buildPayload(r)
	key := k.options.Key
	msg := kafka.Message{
		Topic: k.options.ProducerTopic,
		Key:   []byte(key),
		Value: payload,
	}
	if k.options.Debug {
		log.Printf("[log4go] msg [topic: %v, brokers: %v]\nvalue: %s\n", msg.Topic, k.options.Brokers, payload)
	}
	k.send(msg)
	return nil
}

// send delivers msg under the overflow policy (drop / block / spill).
//
// Shutdown safety: Stop sets k.closing and closes k.stop, but NEVER closes
// k.messages (see Stop docs). So send can never panic on a closed channel. The
// closing fast path drops records once shutdown begins (keeping Stop bounded),
// and every send also selects on k.stop so an OverflowBlock producer is unblocked
// when the daemon is winding down instead of blocking forever on a channel the
// daemon has stopped consuming. Mirrors FileWriter.send (file_writer.go:528-568).
func (k *KafkaWriter) send(msg kafka.Message) {
	if k.closing.Load() {
		// Shutdown in progress: drop instead of racing a send against the daemon
		// winding down. Keeps Stop bounded and avoids any send-after-stop hazard.
		k.stats.IncDropped()
		return
	}
	switch k.policy {
	case OverflowBlock:
		select {
		case k.messages <- msg:
		case <-k.stop:
			k.stats.IncDropped()
		}
	case OverflowSpill:
		select {
		case k.messages <- msg:
		case <-k.stop:
			k.stats.IncDropped()
		default:
			if k.spiller != nil && k.spiller.Push(msg) {
				k.stats.IncSpilled()
			} else {
				k.stats.IncDropped()
			}
		}
	default: // OverflowDrop
		select {
		case k.messages <- msg:
		case <-k.stop:
			k.stats.IncDropped()
		default:
			k.stats.IncDropped()
		}
	}
}

// sendOne delivers a single message under the breaker. When the breaker is open
// and a spill store exists (OverflowSpill), the record is diverted to spill
// instead of a futile Send that would async-fail and be lost. A sync client-side
// Send error is also failover'd to spill under Spill policy (at-least-once).
func (k *KafkaWriter) sendOne(msg kafka.Message) {
	if k.breaker != nil && k.breaker.isOpen() && k.spiller != nil {
		k.failover(msg)
		return
	}
	var err error
	if k.producerNotNil() {
		err = k.producer.Send(context.Background(), msg)
	}
	if k.breaker != nil {
		k.breaker.recordSend()
	}
	if err != nil {
		if k.breaker != nil {
			k.breaker.recordError()
		}
		if k.spiller != nil {
			k.failover(msg) // sync client-side error → durable failover
			return
		}
		atomic.AddUint64(&k.errored, 1)
		k.fireEvent("error", 1)
		log.Printf("[log4go] kafka send err: %v", err)
		return
	}
	atomic.AddUint64(&k.sent, 1)
	k.fireEvent("sent", 1)
}

// failover routes a record to the spill store when the producer path is
// unavailable (breaker open) or a sync send errored. The record is recovered via
// drainSpill once the breaker closes. With no spill store it degrades to a
// counted drop — Drop/Block policies keep their existing behavior.
func (k *KafkaWriter) failover(msg kafka.Message) {
	atomic.AddUint64(&k.failovered, 1)
	if k.spiller != nil && k.spiller.Push(msg) {
		k.stats.IncSpilled()
	} else {
		k.stats.IncDropped()
	}
	k.fireEvent("failover", 1)
}

// drainSpill re-injects recovered records into the channel (non-blocking).
//
// Once Stop has set closing, drainSpill is a no-op: the shutdown path is handled
// by drainSpillToProducer (which sends straight to the producer, bypassing
// messages), so re-injecting here while the daemon winds down is unnecessary and
// would race a producer that closing has not yet observed. Gated on closing to
// match FileWriter.drainSpill.
func (k *KafkaWriter) drainSpill() {
	if k.closing.Load() {
		return
	}
	if k.spiller == nil || k.spiller.Len() == 0 {
		return
	}
	for _, msg := range k.spiller.Drain() {
		select {
		case k.messages <- msg:
		default:
			// channel full again; keep the rest in the spiller.
			_ = k.spiller.Push(msg)
			return
		}
	}
}

// producerNotNil reports whether the producer is set to a non-nil value,
// guarding against the typed-nil-interface gotcha (a non-nil interface holding
// a nil pointer).
func (k *KafkaWriter) producerNotNil() bool {
	if k.producer == nil {
		return false
	}
	v := reflect.ValueOf(k.producer)
	// Only pointer and interface kinds can be nil; a struct value is non-nil
	// by construction. reflect.Value.IsNil panics on non-nilable kinds.
	return v.Kind() != reflect.Ptr && v.Kind() != reflect.Interface || !v.IsNil()
}

// daemon forwards channel messages to the producer and periodically drains the
// spiller so recovered records get re-sent. Success/error accounting is via the
// kafka.Producer OnEvent hook (installed at daemon start, below).
//
// In batch mode (KafkaWriterOptions.BatchMode) it accumulates records into a
// reusable slice and flushes via producer.SendBatch on BatchSize /
// BatchFlushInterval / shutdown — cutting daemon-side per-call overhead at high
// QPS. Per-record mode (default) Sends each immediately (lowest latency).
func (k *KafkaWriter) daemon() {
	defer func() {
		if r := recover(); r != nil {
			recordDaemonPanic("kafka", r)
		}
	}()
	k.run.Store(true)

	// install the error-accounting hook (replaces the old sarama Errors() drain
	// goroutine). Done here rather than Start() so any caller that runs the
	// daemon (incl. tests that bypass Start) gets the hook wired. Guard against
	// nil producer for test setups that exercise overflow without a real daemon.
	if k.producerNotNil() {
		k.producer.SetOnEvent(func(e kafka.ProducerEvent) {
			if e.Name == "error" {
				atomic.AddUint64(&k.errored, 1)
				if k.breaker != nil {
					k.breaker.recordError() // feeds the error-rate window (L4)
				}
				k.fireEvent("error", 1)
				log.Printf("[log4go] kafka producer err: %v", e.Err)
			}
		})
	}

	ticker := time.NewTicker(k.drainInterval)
	defer ticker.Stop()

	// Batch mode state. flushC is nil unless batch mode → its select case never
	// fires. flush captures `batch` by reference; batch[:0] reuses the backing
	// array (the backend copies records out of SendBatch before it returns, so
	// reuse is safe — no aliasing of Value []bytes across flushes).
	var batch []kafka.Message
	var flushC <-chan time.Time
	flush := func() {}
	if k.batchMode {
		batch = make([]kafka.Message, 0, k.batchSize)
		ft := time.NewTicker(k.batchFlushInterval)
		flushC = ft.C
		defer ft.Stop()
		flush = func() {
			if len(batch) == 0 {
				return
			}
			// Broker down (breaker open) with a spill store: divert the whole
			// batch to spill rather than a futile SendBatch that async-fails.
			if k.breaker != nil && k.breaker.isOpen() && k.spiller != nil {
				for _, m := range batch {
					k.failover(m)
				}
				batch = batch[:0]
				return
			}
			var batchErr error
			if k.producerNotNil() {
				batchErr = k.producer.SendBatch(context.Background(), batch)
			}
			if batchErr != nil && k.spiller != nil {
				// client-side failure: requeue the batch to spill (at-least-once).
				for _, m := range batch {
					k.failover(m)
				}
				batch = batch[:0]
				return
			}
			n := uint64(len(batch))
			atomic.AddUint64(&k.sent, n)
			if k.breaker != nil {
				k.breaker.recordSendN(n)
			}
			if batchErr != nil {
				// client-side error, no spill store: count errored, drop the batch.
				atomic.AddUint64(&k.errored, n)
				k.fireEvent("error", int64(n))
			} else {
				atomic.AddUint64(&k.batches, 1)
				if n > atomic.LoadUint64(&k.batchMax) { // daemon sole writer: Load/Store safe
					atomic.StoreUint64(&k.batchMax, n)
				}
				k.fireEvent("sent", int64(n))
			}
			batch = batch[:0]
		}
	}

	for {
		select {
		case mes, ok := <-k.messages:
			if !ok {
				// Defensive: messages should never be closed in normal operation
				// (Stop does not close it). If it ever is, treat as shutdown.
				k.drainOnShutdown(&batch, flush)
				k.drainSpillToProducer()
				k.quit <- struct{}{}
				return
			}
			if k.batchMode {
				batch = append(batch, mes)
				if len(batch) >= k.batchSize {
					flush()
				}
			} else {
				k.sendOne(mes)
			}
		case <-flushC:
			flush()
		case <-ticker.C:
			k.drainSpill()
			if k.breaker != nil {
				k.breaker.evaluate(time.Now()) // cold-path state machine (L4)
			}
		case <-k.stop:
			// Shutdown (Stop closed stop; messages is intentionally NOT closed).
			// Drain everything still queued + any buffered batch to the producer,
			// then send the spill store straight through. A racing producer that
			// slipped a record past closing.Load() is drained here too, so nothing
			// is lost and nothing sends on a closed channel.
			k.drainOnShutdown(&batch, flush)
			k.drainSpillToProducer()
			k.quit <- struct{}{}
			return
		}
	}
}

// drainOnShutdown non-blockingly drains every record still pending in the
// messages channel into the batch (growing *batch, which points at the daemon's
// slice variable the flush closure also reads — so the two stay in sync), then
// flushes the whole accumulated batch in one SendBatch. This preserves batch
// semantics on the shutdown tail (one flush, not N per-record Sends) and is
// race-free: nothing else touches messages during shutdown. Called only from the
// daemon, so no concurrent consumer. Mirrors FileWriter.drainQueuedAndSpill.
func (k *KafkaWriter) drainOnShutdown(batch *[]kafka.Message, flush func()) {
	for {
		select {
		case mes, ok := <-k.messages:
			if !ok {
				// channel closed defensively; flush what was accumulated.
				flush()
				return
			}
			*batch = append(*batch, mes)
			if len(*batch) >= k.batchSize {
				flush()
			}
			continue
		default:
			flush() // send the accumulated (partial) batch in one SendBatch
			return
		}
	}
}

// drainSpillToProducer sends recovered records straight to the producer on shutdown.
func (k *KafkaWriter) drainSpillToProducer() {
	if k.spiller == nil {
		return
	}
	for _, msg := range k.spiller.Drain() {
		_ = k.producer.Send(context.Background(), msg)
	}
}

// kafkaProducerOpts builds the kafka.Option list for the underlying producer from
// KafkaWriterOptions. Extracted from Start so the option wiring (notably
// ProducerLinger) is unit-testable without a broker. ProducerLinger is forwarded
// only when the caller set it (!=0); a bare 0 is left to the kafka package
// default (10ms), and a negative value (kafka.LingerOff) disables backend
// batching.
func (k *KafkaWriter) kafkaProducerOpts() []kafka.Option {
	opts := []kafka.Option{
		kafka.WithBrokers(k.options.Brokers...),
		kafka.WithTopic(k.options.ProducerTopic),
		kafka.WithReturnSuccesses(true),
	}
	if k.options.SpecifyVersion && k.options.VersionStr != "" {
		opts = append(opts, kafka.WithVersion(k.options.VersionStr))
	}
	if k.options.ProducerLinger != 0 {
		opts = append(opts, kafka.WithProducerLinger(k.options.ProducerLinger))
		if k.options.ProducerLinger == kafka.LingerOff {
			opts = append(opts, kafka.WithMaxBufferedRecords(1))
		}
	}
	if k.options.ProducerSnapshotHistory > 0 {
		opts = append(opts, kafka.WithSnapshotHistory(k.options.ProducerSnapshotHistory))
	}
	if k.options.Acks != "" {
		opts = append(opts, kafka.WithAcks(k.options.Acks))
	}
	return opts
}

// Start start the kafka writer
func (k *KafkaWriter) Start() (err error) {
	log.Printf("[log4go] kafka writer starting (policy=%s)", k.policy)

	// create the producer via the kafka package (sarama default, franz-go via
	// -tags franzgo — the build tag selects the backend, log4go is agnostic).
	if k.producerFactory != nil {
		k.producer, err = k.producerFactory()
	} else {
		k.producer, err = kafka.NewProducer(k.kafkaProducerOpts()...)
	}
	if err != nil {
		log.Printf("[log4go] kafka.NewProducer err, message=%s", err.Error())
		return err
	}

	size := k.options.BufferSize
	if size <= 1 {
		size = 1024
	}
	k.messages = make(chan kafka.Message, size)
	// stop is created here (with the daemon) rather than in NewKafkaWriter so a
	// never-Started writer has no goroutine to signal. Close(stop) is the
	// shutdown trigger for send() and the daemon; quit is the daemon→Stop
	// completion ack. See the closing field doc.
	k.stop = make(chan struct{})

	// init the recovery store for spill policy.
	if k.policy == OverflowSpill {
		switch k.options.SpillType {
		case SpillTypeFile:
			k.spiller, err = NewFileSpiller[kafka.Message](k.options.SpillDir, k.options.SpillMaxBytes, ProducerMsgCodec)
			if err != nil {
				return err
			}
		case SpillTypeRing:
			k.spiller = NewRingSpiller[kafka.Message](k.options.SpillSize)
		default: // SpillTypeChain or "": ring (hot) -> file (cold, persistent)
			ring := NewRingSpiller[kafka.Message](k.options.SpillSize)
			if k.options.SpillDir != "" {
				if file, ferr := NewFileSpiller[kafka.Message](k.options.SpillDir, k.options.SpillMaxBytes, ProducerMsgCodec); ferr == nil {
					k.spiller = NewChainedSpiller[kafka.Message](ring, file)
				} else {
					k.spiller = ring
				}
			} else {
				k.spiller = ring
			}
		}
	}

	// resume any persisted spill from a previous (interrupted) run.
	if k.spiller != nil {
		for _, msg := range k.spiller.Drain() {
			select {
			case k.messages <- msg:
			default:
				if !k.spiller.Push(msg) {
					k.stats.IncDropped()
				}
			}
		}
	}

	// Mark running BEFORE launching the goroutine so Stop() works immediately
	// after Start returns — otherwise a Stop issued before the daemon schedules
	// would no-op (k.run still false) and drop the un-flushed batch (a race the
	// shutdown-flush test exercises under -cover instrumentation).
	k.run.Store(true)
	go k.daemon()
	linger := "10ms (default)"
	if k.options.ProducerLinger == kafka.LingerOff {
		linger = "off"
	} else if k.options.ProducerLinger > 0 {
		linger = k.options.ProducerLinger.String()
	}
	acks := k.options.Acks
	if acks == "" {
		acks = kafka.AcksLeader + "(default)"
	}
	backend := "sarama(default)"
	if k.producerNotNil() {
		backend = k.producer.Backend()
	}
	log.Printf("[log4go] kafka writer started (backend=%s, buffer=%d, spill=%v, batchMode=%v batchSize=%d "+
		"producerLinger=%s acks=%s) — for durability set Acks=kafka.AcksAll",
		backend, size, k.spiller != nil, k.batchMode, k.batchSize, linger, acks)
	return err
}

// Stop stops the kafka writer gracefully: it drains every record still queued
// in the channel and the entire spill store to the producer before closing it.
//
// Race-free ordering (the shutdown-race fix, R20 P0-1):
//  1. closing=true -> new producers (send) and the daemon's drainSpill stop
//     touching messages immediately; drainSpill returns early so it can never
//     re-inject into messages during shutdown.
//  2. close(stop)  -> unblocks any producer waiting in send's OverflowBlock
//     branch, AND wakes the daemon's shutdown branch.
//  3. wait <-quit  -> the daemon has drained every queued message + the entire
//     spill store (sent straight to the producer via drainSpillToProducer),
//     flushed any buffered batch, and exited.
//  4. producer.Close / spiller.Close -> only after the daemon is done sending.
//
// Crucially Stop NEVER closes k.messages — nothing does. Closing it would race
// any concurrent send (close-vs-send is a true memory race and send-on-closed
// panics). Since closing=true gates send's fast path and the daemon drains all
// pending records before exiting, leaving messages open is correct. Mirrors
// FileWriter.Stop (file_writer.go:778-812).
func (k *KafkaWriter) Stop() {
	if !k.run.CompareAndSwap(true, false) {
		return // already stopped, or another Stop in flight — atomic claim avoids a double close
	}
	k.closing.Store(true)
	close(k.stop)
	waitQuit("kafka", k.quit, defaultShutdownTimeout)
	if k.producerNotNil() {
		if err := k.producer.Close(); err != nil {
			log.Printf("[log4go] kafkaWriter stop error: %v", err.Error())
		}
	}
	if k.spiller != nil {
		_ = k.spiller.Close()
	}
}

// Stats returns a snapshot of overflow counters.
func (k *KafkaWriter) Stats() (dropped, spilled uint64) {
	return k.stats.Dropped(), k.stats.Spilled()
}

// WriterMetrics is a point-in-time snapshot of KafkaWriter operational counters,
// suitable for scraping by a monitoring system (Prometheus, etc.).
type WriterMetrics struct {
	Sent     uint64 // records handed to the producer
	Errored  uint64 // producer errors
	Dropped  uint64 // dropped on full (drop policy)
	Spilled  uint64 // moved to the spill store (spill policy)
	Queued   int    // records currently buffered in the channel
	SpillLen int    // records currently held in the spill store
	Batches  uint64 // SendBatch flush count (0 in per-record mode)
	BatchMax int    // largest batch size flushed (0 in per-record mode)
	// Producer-derived (bridged from the underlying kafka.Producer, nil-safe =
	// zero when no producer). These surface the backend's real-time buffer
	// depth — the linger backlog + batch memory — which Queued (channel depth)
	// alone does NOT show. For full depth (Timestamp/Linger/MaxBufferedRecs/
	// Bytes counters/History) use ProducerSnapshot().
	InFlight      uint64 // producer in-flight records (Enqueued−Success−Failed) — linger backlog depth
	BufferedBytes uint64 // producer buffer memory (BytesEnqueued−Bytes−BytesFailed)
	Backend       string // underlying client ("sarama"/"franz-go"); "" if no producer
	Failovered    uint64 // records diverted to spill on breaker-open / sync send-error (L4)
	CircuitState  int32  // 0 closed, 1 open, 2 half-open — the breaker's current state (L4)
}

// Metrics returns a snapshot of operational counters for monitoring.
func (k *KafkaWriter) Metrics() WriterMetrics {
	queued := 0
	if k.messages != nil {
		queued = len(k.messages)
	}
	spillLen := 0
	if k.spiller != nil {
		spillLen = k.spiller.Len()
	}
	m := WriterMetrics{
		Sent:         atomic.LoadUint64(&k.sent),
		Errored:      atomic.LoadUint64(&k.errored),
		Dropped:      k.stats.Dropped(),
		Spilled:      k.stats.Spilled(),
		Queued:       queued,
		SpillLen:     spillLen,
		Batches:      atomic.LoadUint64(&k.batches),
		BatchMax:     int(atomic.LoadUint64(&k.batchMax)),
		Failovered:   atomic.LoadUint64(&k.failovered),
		CircuitState: k.breakerStateCode(),
	}
	if k.producerNotNil() {
		pm := k.producer.Metrics()
		m.InFlight = pm.InFlight
		m.BufferedBytes = pm.BufferedBytes
		m.Backend = k.producer.Backend()
	}
	return m
}

// breakerStateCode returns the breaker's int state for Metrics, or closed when
// the breaker is disabled (nil).
func (k *KafkaWriter) breakerStateCode() int32 {
	if k.breaker == nil {
		return breakerClosed
	}
	return k.breaker.stateCode()
}

// ProducerSnapshot returns the underlying kafka.Producer's full monitoring
// snapshot — Timestamp (UTC), all ProducerMetrics (Enqueued/Success/Failed/
// Bytes/BytesFailed/BytesEnqueued/BatchCount/BatchMax/InFlight/BufferedBytes),
// Linger, MaxBufferedRecs, BatchMaxBytesCfg, Backend. Use this for deep
// monitoring / point-in-time inspection beyond the scrape-level WriterMetrics.
// Returns the zero value if the producer is nil (e.g. a test that bypassed
// Start). For trend samples, type-assert the producer against kafka.SnapshotHistory.
func (k *KafkaWriter) ProducerSnapshot() kafka.ProducerSnapshot {
	if !k.producerNotNil() {
		return kafka.ProducerSnapshot{}
	}
	return k.producer.Snapshot()
}

// SetOnEvent installs a real-time metric hook (reserved for monitoring
// integration). The callback receives an event name ("sent"/"error") and a
// delta. It must be set before Start and must be non-blocking.
func (k *KafkaWriter) SetOnEvent(fn func(name string, delta int64)) {
	if fn == nil {
		k.onEvent.Store(nil)
		return
	}
	k.onEvent.Store(&fn)
}

// fireEvent invokes the onEvent hook if installed (atomic load; zero-overhead
// when nil). Called from the daemon goroutine.
func (k *KafkaWriter) fireEvent(name string, delta int64) {
	if p := k.onEvent.Load(); p != nil {
		(*p)(name, delta)
	}
}

// SetAlertSink installs an alert sink (e.g. WebhookAlertSink for lark/dingtalk/
// feishu) for overflow push notifications. Set before Start.
func (k *KafkaWriter) SetAlertSink(sink AlertSink) { k.stats.SetAlertSink(sink) }

// --- Deprecated aliases (backward compatibility for the old KafKa spelling) ---
//
// The canonical names use "Kafka" (first-letter capital, matching Go's
// proper-noun convention and the package's existing KafkaCodec/SetKafkaCodec).
// These aliases keep the old "KafKa" spelling compiling for external callers
// and will not be removed within the v0.x line.

// Deprecated: use KafkaWriter. The KafKa spelling is retained only for backward compatibility.
type KafKaWriter = KafkaWriter

// Deprecated: use KafkaWriterOptions. The KafKa spelling is retained only for backward compatibility.
type KafKaWriterOptions = KafkaWriterOptions

// Deprecated: use KafkaMSGFields. The KafKa spelling is retained only for backward compatibility.
type KafKaMSGFields = KafkaMSGFields

// Deprecated: use NewKafkaWriter. The KafKa spelling is retained only for backward compatibility.
func NewKafKaWriter(options KafKaWriterOptions) *KafKaWriter { return NewKafkaWriter(options) }
