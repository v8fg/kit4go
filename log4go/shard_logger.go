package log4go

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"
)

// ShardLogger fans records across N independent Loggers, each with its own
// records channel and bootstrap goroutine. Deliver throughput scales with cores
// (N shards × ~single-shard QPS), so a multi-core machine reaches 1M+ QPS where
// a single Logger is bounded by one channel/bootstrap. Records are distributed
// round-robin; ordering is preserved within each shard.
type ShardLogger struct {
	loggers []*Logger
	n       uint64
	rr      atomic.Uint64
}

// autoShardMin is the floor for the auto-computed shard count. Each shard runs
// a bootstrap goroutine that serially consumes its records channel, so we size
// shards to GOMAXPROCS/2 — leaving the other half of the cores for the producers
// (your app) plus the runtime/OS. There is intentionally NO fixed ceiling: the
// right shard count scales with cores (a 64-core box has a bigger producer pool
// and needs more parallel consumers). A hard cap like 8 would bottleneck big
// machines — the old "8 regresses" measurement was on 10 cores (8 shards = 80%
// of cores to bootstraps, oversubscribed); at GOMAXPROCS/2 it never
// oversubscribes. Pass an explicit n to NewShardLogger to override.
// See PERFORMANCE.md §18.
const autoShardMin = 2

// AutoShardCount returns a recommended shard count for the current machine:
//
//	max(2, GOMAXPROCS/2)
//
// It honors the effective GOMAXPROCS, which on Go 1.25+ respects the cgroup CPU
// quota in containers/k8s (a 64-core host limited to 4 CPUs yields 2 shards, not
// 32). For older Go or misreported GOMAXPROCS, pair with go.uber.org/automaxprocs.
// Override with NewShardLogger(n) when you want a specific count.
func AutoShardCount() int {
	n := runtime.GOMAXPROCS(0) / 2
	if n < autoShardMin {
		return autoShardMin
	}
	return n
}

// NewShardLogger creates n independent shards. n <= 0 selects an automatic count
// via AutoShardCount (recommended for most services); n >= 1 pins the exact
// count for advanced tuning.
func NewShardLogger(n int) *ShardLogger {
	if n <= 0 {
		n = AutoShardCount()
	}
	loggers := make([]*Logger, n)
	for i := range loggers {
		loggers[i] = newLoggerWithRecords(make(chan *Record, recordChannelSize))
	}
	// One startup line so operators can see the effective parallelism log4go
	// settled on (GOMAXPROCS honors cgroup quota on Go 1.25+; pair with the
	// kit4go/maxprocs subpackage on older runtimes). Uses the stdlib logger to
	// avoid bootstrapping log4go into its own startup banner.
	log.Printf("[log4go] ShardLogger started: GOMAXPROCS=%d shards=%d", runtime.GOMAXPROCS(0), n)
	return &ShardLogger{loggers: loggers, n: uint64(n)}
}

// NewShardLoggerAuto is an explicit alias for NewShardLogger(0) — pick the shard
// count from AutoShardCount(). Prefer this when you want the intent to be loud.
func NewShardLoggerAuto() *ShardLogger { return NewShardLogger(0) }

// ShardLoggerOptions configures a ShardLogger. JSON/mapstructure tagged for
// config-file loading (yaml/json/toml), mirroring the XxxWriterOptions pattern.
//
//	sl := log4go.NewShardLoggerWithOptions(log4go.ShardLoggerOptions{
//	    Shards:      0,       // 0 = auto (max(2, GOMAXPROCS/2)); >=1 = pin
//	    Level:       "info",
//	    ChannelSize: 8192,    // per-shard records channel capacity
//	})
type ShardLoggerOptions struct {
	// Shards is the shard count: 0 (default) = auto via AutoShardCount;
	// >=1 pins the exact count (overrides GOMAXPROCS-based sizing).
	Shards int `json:"shards" mapstructure:"shards"`
	// Level is the severity threshold name ("debug".."emergency"); empty = DEBUG.
	Level string `json:"level" mapstructure:"level"`
	// ChannelSize is the per-shard records channel capacity (<=0 -> package
	// default 4096). Larger absorbs bigger bursts at the cost of memory.
	ChannelSize int `json:"channel_size" mapstructure:"channel_size"`
}

// NewShardLoggerWithOptions builds a ShardLogger from structured options
// (config-file friendly). Shards<=0 auto-selects; Level and ChannelSize are
// optional tunables.
func NewShardLoggerWithOptions(opts ShardLoggerOptions) *ShardLogger {
	n := opts.Shards
	if n <= 0 {
		n = AutoShardCount()
	}
	chanSize := int(recordChannelSize)
	if opts.ChannelSize > 0 {
		chanSize = opts.ChannelSize
	}
	loggers := make([]*Logger, n)
	for i := range loggers {
		loggers[i] = newLoggerWithRecords(make(chan *Record, chanSize))
	}
	log.Printf("[log4go] ShardLogger started: GOMAXPROCS=%d shards=%d channelSize=%d",
		runtime.GOMAXPROCS(0), n, chanSize)
	sl := &ShardLogger{loggers: loggers, n: uint64(n)}
	if opts.Level != "" {
		sl.SetLevel(getLevelDefault(opts.Level, DEBUG, "shard"))
	}
	return sl
}

// ShardCount returns the number of shards (for metrics/monitoring export).
func (s *ShardLogger) ShardCount() int { return len(s.loggers) }

// Register registers a single writer instance to every shard.
//
// This is only safe for stateless writers (ConsoleWriter, discardWriter, custom
// appenders with no shared mutable file/buffer/channel) at any shard count, OR
// for any single writer when there is exactly one shard (NewShardLogger(1)): a
// single shard calls Init exactly once and owns the writer, so there is no
// cross-shard contention.
//
// Registering an async FileWriter (or any stateful writer) across MORE than one
// shard is a programming error: every shard's Register calls FileWriter.Init
// again, and all shards' bootstrap goroutines then race the SAME
// bufio/*os.File/messages channel, corrupting output. Register panics in that
// case to make the footgun loud rather than silently corrupting under load. For
// stateful writers across multiple shards use RegisterFunc so each shard gets
// its own independent instance.
func (s *ShardLogger) Register(w Writer) {
	if len(s.loggers) > 1 {
		if _, ok := w.(*FileWriter); ok {
			panic("log4go: ShardLogger(n>1).Register(*FileWriter) is not allowed — the " +
				"writer would be shared across shards, racing its bufio/file/daemon and " +
				"corrupting output. Use RegisterFunc(func() Writer { ... }) to build an " +
				"independent FileWriter per shard.")
		}
	}
	for _, l := range s.loggers {
		l.Register(w)
	}
}

// RegisterFunc registers a fresh, independent Writer to every shard by invoking
// make once per shard. Use this for any stateful writer (async FileWriter,
// KafKaWriter, custom writers with their own daemon/buffer) so each shard owns
// a private instance — no bufio/file/channel is shared across shards, which is
// the only correct way to fan disk/kafka writes across cores. Stateless writers
// (ConsoleWriter) may use Register directly.
func (s *ShardLogger) RegisterFunc(make func() Writer) {
	for _, l := range s.loggers {
		l.Register(make())
	}
}

// SetLevel sets the level on every shard.
func (s *ShardLogger) SetLevel(lvl int) {
	for _, l := range s.loggers {
		l.SetLevel(lvl)
	}
}

func (s *ShardLogger) pick() *Logger {
	return s.loggers[s.rr.Add(1)%s.n]
}

// Debug logs at DEBUG on a shard (round-robin).
func (s *ShardLogger) Debug(format string, args ...any) { s.pick().Debug(format, args...) }

// Trace logs at TRACE on a shard (round-robin).
func (s *ShardLogger) Trace(format string, args ...any) { s.pick().Trace(format, args...) }

// Info logs at INFO on a shard (round-robin).
func (s *ShardLogger) Info(format string, args ...any) { s.pick().Info(format, args...) }

// Warn logs at WARNING on a shard (round-robin).
func (s *ShardLogger) Warn(format string, args ...any) { s.pick().Warn(format, args...) }

// Error logs at ERROR on a shard (round-robin).
func (s *ShardLogger) Error(format string, args ...any) { s.pick().Error(format, args...) }

// Close closes all shards in parallel.
func (s *ShardLogger) Close() {
	var wg sync.WaitGroup
	for _, l := range s.loggers {
		wg.Add(1)
		go func(ll *Logger) { defer wg.Done(); ll.Close() }(l)
	}
	wg.Wait()
}
