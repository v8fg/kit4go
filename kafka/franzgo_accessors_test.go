//go:build franzgo

package kafka

import (
	"testing"
	"time"
)

// TestFranzgo_ProducerAccessors exercises the franzProducer methods that don't
// need a broker (Metrics, Snapshot, History, Name, Backend, SetOnEvent, fire)
// by constructing the struct directly with a nil client.
func TestFranzgo_ProducerAccessors(t *testing.T) {
	p := &franzProducer{
		opts: Options{
			Name: "test-prod", Topic: "fallback",
			ProducerLinger:     10 * time.Millisecond,
			MaxBufferedRecords: 1000,
			BatchMaxBytes:      2048,
		},
		history: newSnapshotHistory(3),
	}
	p.enqueued.Store(100)
	p.success.Store(80)
	p.failed.Store(5)
	p.bytes.Store(8000)
	p.bytesEnqueued.Store(10000)
	p.bytesFailed.Store(500)
	p.batchCount.Store(10)
	p.batchMax.Store(20)

	m := p.Metrics()
	if m.Enqueued != 100 || m.Success != 80 || m.Failed != 5 {
		t.Errorf("Metrics: Enqueued=%d Success=%d Failed=%d", m.Enqueued, m.Success, m.Failed)
	}
	if m.InFlight != 15 {
		t.Errorf("InFlight=%d want 15", m.InFlight)
	}

	s := p.Snapshot()
	if s.Name != "test-prod" || s.Backend != "franz-go" || s.Timestamp.IsZero() {
		t.Errorf("Snapshot: Name=%q Backend=%q ts0=%v", s.Name, s.Backend, s.Timestamp.IsZero())
	}
	if s.Linger != 10*time.Millisecond {
		t.Errorf("Linger=%v want 10ms", s.Linger)
	}

	// History: Snapshot() records; call again + retrieve
	p.Snapshot()
	hist := p.History()
	if len(hist) != 2 {
		t.Errorf("History len=%d want 2", len(hist))
	}

	// SetOnEvent + fire
	var fired ProducerEvent
	p.SetOnEvent(func(e ProducerEvent) { fired = e })
	p.fire(ProducerEvent{Name: "test-event"})
	if fired.Name != "test-event" {
		t.Errorf("fire: got %q want test-event", fired.Name)
	}

	if p.Name() != "test-prod" || p.Backend() != "franz-go" {
		t.Errorf("Name=%q Backend=%q", p.Name(), p.Backend())
	}
}

// TestFranzgo_SyncProducerAccessors exercises franzSyncProducer accessors (nil client).
func TestFranzgo_SyncProducerAccessors(t *testing.T) {
	sp := &franzSyncProducer{opts: Options{Name: "sync-prod", Topic: "t"}}
	sp.enqueued.Store(50)
	sp.success.Store(45)
	sp.failed.Store(3)
	sp.bytes.Store(4500)

	m := sp.Metrics()
	if m.Enqueued != 50 || m.Success != 45 {
		t.Errorf("sync Metrics: %+v", m)
	}
	s := sp.Snapshot()
	if s.Name != "sync-prod" || s.Backend != "franz-go" || s.Timestamp.IsZero() {
		t.Errorf("sync Snapshot: %+v", s)
	}
	// sync must NOT satisfy SnapshotHistory
	if _, ok := any(sp).(SnapshotHistory); ok {
		t.Error("sync producer should NOT satisfy SnapshotHistory")
	}
	sp.SetOnEvent(func(ProducerEvent) {})
	sp.fire(ProducerEvent{Name: "x"})
}

// TestFranzgo_OptsBuilders covers all 4 kgo opts builders (branches: acks,
// linger, buffer, BatchMaxBytes, idempotency-disable).
func TestFranzgo_OptsBuilders(t *testing.T) {
	base := Options{
		Brokers: []string{"localhost:9092"}, Topic: "t", GroupID: "g",
		Partition: 0, Offset: OffsetNewest,
		ProducerLinger: 5 * time.Millisecond, MaxBufferedRecords: 500,
		Acks: AcksLeader, BatchMaxBytes: 4096,
	}

	// kgoProducerOpts with all tuning set (acks=leader → DisableIdempotentWrite)
	if len(kgoProducerOpts(base)) < 5 {
		t.Error("kgoProducerOpts(base): too few opts")
	}
	// kgoProducerOpts with BatchMaxBytes=0 + Acks="" (native all, no DisableIdempotentWrite)
	o2 := base
	o2.BatchMaxBytes = 0
	o2.Acks = ""
	if len(kgoProducerOpts(o2)) < 4 {
		t.Error("kgoProducerOpts(o2): too few opts")
	}

	if len(kgoSyncProducerOpts(base)) < 3 {
		t.Error("kgoSyncProducerOpts: too few opts")
	}
	if len(kgoConsumerGroupOpts(base)) < 4 {
		t.Error("kgoConsumerGroupOpts: too few opts")
	}
	if len(kgoPartitionConsumerOpts(base)) < 2 {
		t.Error("kgoPartitionConsumerOpts: too few opts")
	}
}

// TestFranzgo_ConsumerGroupAccessors exercises franzConsumerGroup accessors (nil client).
func TestFranzgo_ConsumerGroupAccessors(t *testing.T) {
	cg := &franzConsumerGroup{opts: Options{Name: "cg", GroupID: "fallback-g"}}
	cg.received.Store(100)
	cg.acked.Store(90)
	cg.failed.Store(5)
	cg.rebalance.Store(2)
	cg.bytes.Store(5000)

	m := cg.Metrics()
	if m.Received != 100 || m.Acked != 90 || m.Failed != 5 || m.Rebalance != 2 {
		t.Errorf("cg Metrics: %+v", m)
	}
	if cg.Name() != "cg" || cg.Backend() != "franz-go" {
		t.Errorf("cg Name=%q Backend=%q", cg.Name(), cg.Backend())
	}
	cg.SetOnEvent(func(ConsumerEvent) {})
	cg.fire(ConsumerEvent{Name: "x"})
	_ = cg.Errors() // lazy channel init, no panic
}

// TestFranzgo_PartitionConsumerAccessors exercises franzPartitionConsumer accessors.
func TestFranzgo_PartitionConsumerAccessors(t *testing.T) {
	pc := &franzPartitionConsumer{opts: Options{Name: "pc", Topic: "fallback-t"}}
	pc.received.Store(50)
	pc.acked.Store(40)
	pc.failed.Store(3)
	pc.bytes.Store(2000)

	m := pc.Metrics()
	if m.Received != 50 || m.Acked != 40 {
		t.Errorf("pc Metrics: %+v", m)
	}
	if pc.Name() != "pc" || pc.Backend() != "franz-go" {
		t.Errorf("pc Name=%q Backend=%q", pc.Name(), pc.Backend())
	}
	pc.SetOnEvent(func(ConsumerEvent) {})
	pc.fire(ConsumerEvent{Name: "x"})
	_ = pc.Errors()
	_ = pc.Messages()
}
