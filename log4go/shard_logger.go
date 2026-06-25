package log4go

import (
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

// NewShardLogger creates n independent shards (min 1).
func NewShardLogger(n int) *ShardLogger {
	if n < 1 {
		n = 1
	}
	loggers := make([]*Logger, n)
	for i := range loggers {
		loggers[i] = newLoggerWithRecords(make(chan *Record, recordChannelSize))
	}
	return &ShardLogger{loggers: loggers, n: uint64(n)}
}

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
func (s *ShardLogger) Debug(format string, args ...interface{}) { s.pick().Debug(format, args...) }

// Info logs at INFO on a shard (round-robin).
func (s *ShardLogger) Info(format string, args ...interface{}) { s.pick().Info(format, args...) }

// Warn logs at WARNING on a shard (round-robin).
func (s *ShardLogger) Warn(format string, args ...interface{}) { s.pick().Warn(format, args...) }

// Error logs at ERROR on a shard (round-robin).
func (s *ShardLogger) Error(format string, args ...interface{}) { s.pick().Error(format, args...) }

// Close closes all shards in parallel.
func (s *ShardLogger) Close() {
	var wg sync.WaitGroup
	for _, l := range s.loggers {
		wg.Add(1)
		go func(ll *Logger) { defer wg.Done(); ll.Close() }(l)
	}
	wg.Wait()
}
