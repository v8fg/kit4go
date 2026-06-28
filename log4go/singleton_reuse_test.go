package log4go

import (
	"sync/atomic"
	"testing"
	"time"
)

// This file holds the regression test for Bug 3 (package singleton orphaned
// after Close). Bug 2's regression test lives in file_writer_spill_shutdown_test.go;
// Bug 1's lives in shard_logger_test.go (RegisterFunc).

// Test_Singleton_ReusableAfterClose verifies the package singleton survives a
// Close+reuse cycle: after Close() the singleton is reset, so a second round of
// Register + log calls rebuilds a live logger (fresh bootstrap goroutine + open
// records channel) instead of delivering to a dead channel and orphaning writer
// daemons.
func Test_Singleton_ReusableAfterClose(t *testing.T) {
	// Reset to a known-fresh singleton for test isolation.
	if old := loggerDefault.Swap(newDefaultLoggerInstance()); old != nil {
		old.Close()
	}

	// Round 1: register a capturing probe and emit.
	lvl1 := int32(0)
	Register(&levelProbe{levelPtr: &lvl1})
	Info("round one")
	// Allow the bootstrap goroutine to process the record.
	time.Sleep(100 * time.Millisecond)
	Close()

	// Round 2: after Close, the singleton must rebuild. Register + emit must not
	// panic (no send on closed channel) and must reach a live writer.
	lvl2 := int32(0)
	Register(&levelProbe{levelPtr: &lvl2})
	Info("round two")
	time.Sleep(100 * time.Millisecond)
	// The round-2 probe must have observed a record; if the singleton were still
	// the closed round-1 logger, no record would arrive (or the send would panic).
	if atomic.LoadInt32(&lvl2) == 0 {
		t.Fatal("singleton not rebuilt after Close: round-2 record never reached a writer")
	}
	Close()
}

// levelProbe is a minimal Writer that records the level of the last record it
// saw. It is stateless across shards/loggers and safe for the singleton reuse
// test where each round registers a fresh instance.
type levelProbe struct {
	levelPtr *int32
}

func (p *levelProbe) Init() error { return nil }
func (p *levelProbe) Write(r *Record) error {
	atomic.StoreInt32(p.levelPtr, int32(r.level))
	return nil
}
