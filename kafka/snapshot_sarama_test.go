//go:build !franzgo

package kafka

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// asyncMockProducer builds a saramaProducer backed by a mock (no broker) with
// the given SnapshotHistory capacity.
func asyncMockProducer(t *testing.T, snapshotHistory int) *saramaProducer {
	t.Helper()
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	p, err := newSaramaProducer(
		Options{Brokers: []string{"x"}, Topic: "t", SnapshotHistory: snapshotHistory}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProducer_SnapshotHistory(t *testing.T) {
	p := asyncMockProducer(t, 3)
	defer p.Close()

	for i := 0; i < 5; i++ {
		s := p.Snapshot()
		if s.Timestamp.IsZero() {
			t.Fatal("Snapshot.Timestamp is zero")
		}
		if s.Backend != "sarama" {
			t.Errorf("Backend=%q want sarama", s.Backend)
		}
		// B4 normalization: Linger reflects the resolved default (10ms), MaxBufferedRecs 1000.
		if s.Linger != DefaultProducerLinger {
			t.Errorf("Linger=%v want %v", s.Linger, DefaultProducerLinger)
		}
		if s.MaxBufferedRecs != DefaultMaxBufferedRecords {
			t.Errorf("MaxBufferedRecs=%d want %d", s.MaxBufferedRecs, DefaultMaxBufferedRecords)
		}
	}
	hist := p.History()
	if len(hist) != 3 { // capped at 3
		t.Errorf("History len=%d want 3", len(hist))
	}
	for i := 1; i < len(hist); i++ { // oldest→newest
		if hist[i].Timestamp.Before(hist[i-1].Timestamp) {
			t.Errorf("history not ordered at %d", i)
		}
	}
}

func TestProducer_SnapshotHistory_Disabled(t *testing.T) {
	p := asyncMockProducer(t, 0) // disabled
	defer p.Close()
	_ = p.Snapshot()
	if got := p.History(); got != nil {
		t.Errorf("disabled History()=%v want nil", got)
	}
	// Async producers always satisfy SnapshotHistory (History returns nil when
	// disabled — check len, not the assertion, for enablement).
	if _, ok := any(p).(SnapshotHistory); !ok {
		t.Error("async producer must satisfy SnapshotHistory even when disabled")
	}
}

func TestProducer_SnapshotTimestampUTC(t *testing.T) {
	p := asyncMockProducer(t, 0)
	defer p.Close()

	before := time.Now().UTC().Add(-time.Second)
	snap := p.Snapshot()
	after := time.Now().UTC().Add(time.Second)

	if snap.Timestamp.Before(before) || snap.Timestamp.After(after) {
		t.Errorf("Timestamp %v outside call window [%v,%v]", snap.Timestamp, before, after)
	}
	if snap.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location=%v want UTC", snap.Timestamp.Location())
	}
	b, err := json.Marshal(snap.Timestamp)
	if err != nil {
		t.Fatalf("marshal Timestamp: %v", err)
	}
	re := regexp.MustCompile(`^"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z"$`)
	if !re.Match(b) {
		t.Errorf("Timestamp JSON=%q not RFC3339-UTC (trailing Z)", string(b))
	}
}

func TestSyncProducer_SnapshotTimestamp(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	sp, err := newSaramaSyncProducer(
		Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	snap := sp.Snapshot()
	if snap.Timestamp.IsZero() {
		t.Error("sync Snapshot.Timestamp is zero")
	}
	// Sync does NOT implement SnapshotHistory (no History method) → assertion fails.
	if _, ok := any(sp).(SnapshotHistory); ok {
		t.Error("sync producer must NOT satisfy SnapshotHistory")
	}
}

// B5: async applies the resolved linger/buffer; sync forces Flush off (so a
// sarama SyncProducer's SendMessage is never stalled by Flush.Frequency).
func TestBuildSaramaConfig_AsyncVsSyncFlush(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults()

	asyncCfg, err := buildSaramaConfig(o, false)
	if err != nil {
		t.Fatal(err)
	}
	if asyncCfg.Producer.Flush.Frequency != DefaultProducerLinger {
		t.Errorf("async Flush.Frequency=%v want %v", asyncCfg.Producer.Flush.Frequency, DefaultProducerLinger)
	}
	if asyncCfg.Producer.Flush.Messages != DefaultMaxBufferedRecords {
		t.Errorf("async Flush.Messages=%d want %d", asyncCfg.Producer.Flush.Messages, DefaultMaxBufferedRecords)
	}

	syncCfg, err := buildSaramaConfig(o, true)
	if err != nil {
		t.Fatal(err)
	}
	if syncCfg.Producer.Flush.Frequency != 0 {
		t.Errorf("sync Flush.Frequency=%v want 0 (no linger stall)", syncCfg.Producer.Flush.Frequency)
	}
	if syncCfg.Producer.Flush.Messages != 0 {
		t.Errorf("sync Flush.Messages=%d want 0 (no wait-for-N stall)", syncCfg.Producer.Flush.Messages)
	}
	// ChannelBufferSize tracks MaxBufferedRecords regardless of sync/async.
	if syncCfg.ChannelBufferSize != DefaultMaxBufferedRecords {
		t.Errorf("ChannelBufferSize=%d want %d", syncCfg.ChannelBufferSize, DefaultMaxBufferedRecords)
	}
}

// LingerOff survives withDefaults and maps to Flush.Frequency=0 (batching off).
func TestBuildSaramaConfig_LingerOff(t *testing.T) {
	o := Options{Brokers: []string{"x"}, Topic: "t", ProducerLinger: LingerOff}.withDefaults()
	if o.ProducerLinger != LingerOff {
		t.Fatalf("ProducerLinger=%v want LingerOff (survives withDefaults)", o.ProducerLinger)
	}
	cfg, err := buildSaramaConfig(o, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Producer.Flush.Frequency != 0 {
		t.Errorf("LingerOff Flush.Frequency=%v want 0", cfg.Producer.Flush.Frequency)
	}
}
