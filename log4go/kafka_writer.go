package log4go

import (
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
)

// KafKaMSGFields holds the legacy Kafka→ES message fields.
//
// Field management is unified with the Logger's structured-fields layer:
// SetBaseField/SetBaseFields (global static), With/WithField/WithFields
// (per-scope) and context extractors (per-request) all flow into r.fields, and
// those take PRIORITY over the struct members here. The struct members below are
// kept as a backward-compatible fallback for callers that configure the writer
// directly. New code should prefer:
//
//	log4go.SetBaseField("server_ip", ip)
//	log4go.SetBaseField("es_index", "adx-logs-2026.06")
//
// so the same field is carried by every writer on the logger, not just Kafka.
type KafKaMSGFields struct {
	// ESIndex and ServerIP are routing/static fields. Deprecated as struct
	// members — prefer SetBaseField("es_index",…)/SetBaseField("server_ip",…).
	// They are emitted only when the r.fields layer did not already supply the
	// same key, so a Base Field always wins.
	ESIndex  string `json:"es_index" mapstructure:"es_index"`
	Level    string `json:"level"`     // dynamic, set by logger, mark the record level
	File     string `json:"file"`      // source code file:line_number
	Message  string `json:"message"`   // required, dynamic
	ServerIP string `json:"server_ip"` // init field, set by app
	Timestamp string `json:"timestamp"` // required, dynamic, set by logger
	Now      int64  `json:"now"`       // choice

	// ExtraFields are merged into the top-level JSON on send, below r.fields in
	// priority (a Base/With/Context field of the same key wins). Prefer
	// SetBaseFields for static fields.
	ExtraFields map[string]interface{} `json:"extra_fields" mapstructure:"extra_fields"`
}

// KafKaWriterOptions kafka writer options.
//
// Performance notes: BufferSize bounds the async send channel; OverflowPolicy
// ("drop"|"block"|"spill") decides what happens when it is full and is the
// primary OOM guard. Under "spill", SpillType selects the bounded recovery
// store ("ring" in-memory ring, or "file" disk-backed) whose drained records
// are re-sent once the channel recovers.
type KafKaWriterOptions struct {
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

	// OverflowPolicy: "drop"(default)|"block"|"spill" — behavior when full.
	OverflowPolicy string `json:"overflow_policy" mapstructure:"overflow_policy"`
	// SpillType: "ring"(default)|"file" — recovery store when policy == "spill".
	SpillType string `json:"spill_type" mapstructure:"spill_type"`
	// SpillSize: ring capacity (records) for "ring".
	SpillSize int `json:"spill_size" mapstructure:"spill_size"`
	// SpillDir: directory for "file" spill.
	SpillDir string `json:"spill_dir" mapstructure:"spill_dir"`
	// SpillMaxBytes: byte cap for "file" spill (<=0 -> 64MB).
	SpillMaxBytes int64 `json:"spill_max_bytes" mapstructure:"spill_max_bytes"`

	MSG KafKaMSGFields `json:"msg"`
}

// KafKaWriter kafka writer (async, bounded, overflow-safe).
type KafKaWriter struct {
	level    int
	producer sarama.AsyncProducer
	messages chan *sarama.ProducerMessage
	options  KafKaWriterOptions

	policy  OverflowPolicy
	spiller Spiller[*sarama.ProducerMessage]
	stats   OverflowStats
	sent    uint64
	errored uint64
	// onEvent is an optional real-time metric hook (reserved for monitoring
	// integration). Nil disables it. Must be non-blocking.
	onEvent         func(name string, delta int64)
	producerFactory func(brokers []string, cfg *sarama.Config) (sarama.AsyncProducer, error)
	drainInterval   time.Duration
	wg              sync.WaitGroup

	run  atomic.Bool // set true once the daemon starts
	quit chan struct{}
	stop chan struct{}
}

// NewKafKaWriter new kafka writer
func NewKafKaWriter(options KafKaWriterOptions) *KafKaWriter {
	defaultLevel := DEBUG
	if len(options.Level) > 0 {
		defaultLevel = getLevelDefault(options.Level, defaultLevel, "")
	}
	w := &KafKaWriter{
		options:       options,
		quit:          make(chan struct{}),
		stop:          make(chan struct{}),
		level:         defaultLevel,
		policy:        ParseOverflowPolicy(options.OverflowPolicy),
		drainInterval: 200 * time.Millisecond,
	}
	w.stats.SetAlertEvery(1000, 1000)
	return w
}

// Init service for Record
func (k *KafKaWriter) Init() error {
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
// fields) first, falling back to the legacy KafKaMSGFields struct members only
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
	ESIndex    string // legacy fallback; prefer Base Field
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
func (k *KafKaWriter) buildPayload(r *Record) []byte {
	p := kafkaPayload{
		UnixNano:  r.unixNano,
		Seq:       r.seq,
		Level:     LevelFlags[r.level],
		File:      r.file,
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
	b, err := p.MarshalJSON()
	if err != nil {
		log.Printf("[log4go] kafka writer json marshal err: %v", err.Error())
		return nil
	}
	return b
}

// Write service for Record. It never spawns a goroutine per record; the record
// is delivered to a bounded channel under the configured overflow policy.
func (k *KafKaWriter) Write(r *Record) error {
	if r.level > k.level {
		return nil
	}
	if r.msg == "" {
		return nil
	}
	payload := k.buildPayload(r)
	if payload == nil {
		return nil
	}
	key := k.options.Key
	msg := &sarama.ProducerMessage{
		Topic: k.options.ProducerTopic,
		Key:   sarama.ByteEncoder(key),
		Value: sarama.ByteEncoder(payload),
	}
	if k.options.Debug {
		log.Printf("[log4go] msg [topic: %v, brokers: %v]\nvalue: %s\n", msg.Topic, k.options.Brokers, payload)
	}
	k.send(msg)
	return nil
}

// send delivers msg under the overflow policy (drop / block / spill).
func (k *KafKaWriter) send(msg *sarama.ProducerMessage) {
	switch k.policy {
	case OverflowBlock:
		k.messages <- msg
	case OverflowSpill:
		select {
		case k.messages <- msg:
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
		default:
			k.stats.IncDropped()
		}
	}
}

// drainSpill re-injects recovered records into the channel (non-blocking).
func (k *KafKaWriter) drainSpill() {
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

// daemon forwards channel messages to the async producer and periodically
// drains the spiller so recovered records get re-sent.
func (k *KafKaWriter) daemon() {
	k.run.Store(true)

	// drain successes (required when Producer.Return.Successes = true)
	k.wg.Add(2)
	go func() {
		defer k.wg.Done()
		for range k.producer.Successes() {
		}
	}()
	// drain errors
	go func() {
		defer k.wg.Done()
		for pe := range k.producer.Errors() {
			atomic.AddUint64(&k.errored, 1)
			if k.onEvent != nil {
				k.onEvent("error", 1)
			}
			log.Printf("[log4go] kafka producer err: %v", pe.Error())
		}
	}()

	ticker := time.NewTicker(k.drainInterval)
	defer ticker.Stop()

	for {
		select {
		case mes, ok := <-k.messages:
			if !ok {
				k.drainSpillToProducer()
				k.quit <- struct{}{}
				return
			}
			k.producer.Input() <- mes
			atomic.AddUint64(&k.sent, 1)
			if k.onEvent != nil {
				k.onEvent("sent", 1)
			}
		case <-ticker.C:
			k.drainSpill()
		case <-k.stop:
			k.drainSpillToProducer()
			k.quit <- struct{}{}
			return
		}
	}
}

// drainSpillToProducer sends recovered records straight to the producer on shutdown.
func (k *KafKaWriter) drainSpillToProducer() {
	if k.spiller == nil {
		return
	}
	for _, msg := range k.spiller.Drain() {
		k.producer.Input() <- msg
	}
}

// Start start the kafka writer
func (k *KafKaWriter) Start() (err error) {
	log.Printf("[log4go] kafka writer starting (policy=%s)", k.policy)
	cfg := sarama.NewConfig()
	// AsyncProducer requires Successes handling when Return.Successes is true.
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.Timeout = k.options.ProducerTimeout

	// default to V2_5_0; allow override via SpecifyVersion + VersionStr.
	kafkaVer := sarama.V2_5_0_0
	if k.options.SpecifyVersion && k.options.VersionStr != "" {
		if v, e := sarama.ParseKafkaVersion(k.options.VersionStr); e == nil {
			kafkaVer = v
		}
	}
	cfg.Version = kafkaVer
	cfg.Producer.Partitioner = sarama.NewRoundRobinPartitioner

	factory := sarama.NewAsyncProducer
	if k.producerFactory != nil {
		factory = k.producerFactory
	}
	k.producer, err = factory(k.options.Brokers, cfg)
	if err != nil {
		log.Printf("[log4go] sarama.NewAsyncProducer err, message=%s", err.Error())
		return err
	}

	size := k.options.BufferSize
	if size <= 1 {
		size = 1024
	}
	k.messages = make(chan *sarama.ProducerMessage, size)

	// init the recovery store for spill policy.
	if k.policy == OverflowSpill {
		switch k.options.SpillType {
		case "file":
			k.spiller, err = NewFileSpiller[*sarama.ProducerMessage](k.options.SpillDir, k.options.SpillMaxBytes, ProducerMsgCodec)
			if err != nil {
				return err
			}
		case "ring":
			k.spiller = NewRingSpiller[*sarama.ProducerMessage](k.options.SpillSize)
		default: // "" or "chain": ring (hot) -> file (cold, persistent)
			ring := NewRingSpiller[*sarama.ProducerMessage](k.options.SpillSize)
			if k.options.SpillDir != "" {
				if file, ferr := NewFileSpiller[*sarama.ProducerMessage](k.options.SpillDir, k.options.SpillMaxBytes, ProducerMsgCodec); ferr == nil {
					k.spiller = NewChainedSpiller[*sarama.ProducerMessage](ring, file)
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

	go k.daemon()
	log.Printf("[log4go] kafka writer started (buffer=%d, spill=%v)", size, k.spiller != nil)
	return err
}

// Stop stop the kafka writer gracefully (flushes spiller to producer first).
func (k *KafKaWriter) Stop() {
	if !k.run.Load() {
		return
	}
	close(k.messages)
	<-k.quit
	if err := k.producer.Close(); err != nil {
		log.Printf("[log4go] kafkaWriter stop error: %v", err.Error())
	}
	k.wg.Wait() // ensure all success/error accounting completes before returning
	if k.spiller != nil {
		_ = k.spiller.Close()
	}
}

// Stats returns a snapshot of overflow counters.
func (k *KafKaWriter) Stats() (dropped, spilled uint64) {
	return k.stats.Dropped(), k.stats.Spilled()
}

// WriterMetrics is a point-in-time snapshot of KafKaWriter operational counters,
// suitable for scraping by a monitoring system (Prometheus, etc.).
type WriterMetrics struct {
	Sent     uint64 // records handed to the producer
	Errored  uint64 // producer errors
	Dropped  uint64 // dropped on full (drop policy)
	Spilled  uint64 // moved to the spill store (spill policy)
	Queued   int    // records currently buffered in the channel
	SpillLen int    // records currently held in the spill store
}

// Metrics returns a snapshot of operational counters for monitoring.
func (k *KafKaWriter) Metrics() WriterMetrics {
	queued := 0
	if k.messages != nil {
		queued = len(k.messages)
	}
	spillLen := 0
	if k.spiller != nil {
		spillLen = k.spiller.Len()
	}
	return WriterMetrics{
		Sent:     atomic.LoadUint64(&k.sent),
		Errored:  atomic.LoadUint64(&k.errored),
		Dropped:  k.stats.Dropped(),
		Spilled:  k.stats.Spilled(),
		Queued:   queued,
		SpillLen: spillLen,
	}
}

// SetOnEvent installs a real-time metric hook (reserved for monitoring
// integration). The callback receives an event name ("sent"/"error") and a
// delta. It must be set before Start and must be non-blocking.
func (k *KafKaWriter) SetOnEvent(fn func(name string, delta int64)) {
	k.onEvent = fn
}

// SetAlertSink installs an alert sink (e.g. WebhookAlertSink for lark/dingtalk/
// feishu) for overflow push notifications. Set before Start.
func (k *KafKaWriter) SetAlertSink(sink AlertSink) { k.stats.SetAlertSink(sink) }
