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

// Register registers a writer to every shard.
func (s *ShardLogger) Register(w Writer) {
	for _, l := range s.loggers {
		l.Register(w)
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
