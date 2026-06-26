package log4go

import (
	"io"
	"log"
	"testing"
)

func silenceStdLog() func() {
	orig := log.Writer()
	log.SetOutput(io.Discard)
	return func() { log.SetOutput(orig) }
}

// Test_NewShardLoggerWithOptions covers all three knobs.
func Test_NewShardLoggerWithOptions(t *testing.T) {
	restore := silenceStdLog()
	defer restore()

	// Shards pinned + Level set
	sl := NewShardLoggerWithOptions(ShardLoggerOptions{Shards: 3, Level: "info"})
	defer sl.Close()
	if got := sl.ShardCount(); got != 3 {
		t.Fatalf("ShardCount=%d want 3", got)
	}
	if int32(INFO) != sl.loggers[0].level.Load() {
		t.Errorf("level=%d want INFO(%d)", sl.loggers[0].level.Load(), INFO)
	}

	// Shards=0 -> auto (>=2 on any machine)
	sl2 := NewShardLoggerWithOptions(ShardLoggerOptions{})
	defer sl2.Close()
	if got := sl2.ShardCount(); got < 2 {
		t.Fatalf("auto ShardCount=%d want >=2", got)
	}

	// ChannelSize honored
	sl3 := NewShardLoggerWithOptions(ShardLoggerOptions{Shards: 1, ChannelSize: 256})
	defer sl3.Close()
	if cap(sl3.loggers[0].records) != 256 {
		t.Errorf("channel cap=%d want 256", cap(sl3.loggers[0].records))
	}

	// ChannelSize<=0 -> default
	sl4 := NewShardLoggerWithOptions(ShardLoggerOptions{Shards: 1})
	defer sl4.Close()
	if cap(sl4.loggers[0].records) != int(recordChannelSize) {
		t.Errorf("default channel cap=%d want %d", cap(sl4.loggers[0].records), recordChannelSize)
	}
}
