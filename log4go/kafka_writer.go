package log4go

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
)

// KafKaMSGFields kafka msg fields
type KafKaMSGFields struct {
	ESIndex   string `json:"es_index" mapstructure:"es_index"` // optional, init field, can set if want send data to es
	Level     string `json:"level"`                            // dynamic, set by logger, mark the record level
	File      string `json:"file"`                             // source code file:line_number
	Message   string `json:"message"`                          // required, dynamic
	ServerIP  string `json:"server_ip"`                        // required, init field, set by app
	Timestamp string `json:"timestamp"`                        // required, dynamic, set by logger
	Now       int64  `json:"now"`                              // choice

	ExtraFields map[string]interface{} `json:"extra_fields" mapstructure:"extra_fields"` // hoisted to top level on send
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

	policy        OverflowPolicy
	spiller       Spiller[*sarama.ProducerMessage]
	stats         OverflowStats
	sent          uint64
	errored       uint64
	// onEvent is an optional real-time metric hook (reserved for monitoring
	// integration). Nil disables it. Must be non-blocking.
	onEvent         func(name string, delta int64)
	producerFactory func(brokers []string, cfg *sarama.Config) (sarama.AsyncProducer, error)
	drainInterval time.Duration
	wg            sync.WaitGroup

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

// buildPayload serializes the record to a JSON payload once (no double marshal).
func (k *KafKaWriter) buildPayload(r *Record) []byte {
	now := time.Now()
	m := map[string]interface{}{
		"es_index":  k.options.MSG.ESIndex,
		"level":     LevelFlags[r.level],
		"file":      r.file,
		"message":   r.msg,
		"server_ip": k.options.MSG.ServerIP,
		"timestamp": now.Format(timestampLayout),
		"now":       now.Unix(),
	}
	// hoist ExtraFields to the top level, never overriding built-in fields.
	for fk, fv := range k.options.MSG.ExtraFields {
		if _, ok := m[fk]; !ok {
			m[fk] = fv
		}
	}
	b, err := json.Marshal(m)
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
