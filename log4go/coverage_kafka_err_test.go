package log4go

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// Cover the synchronous client-side error branches of sendOne / failover and
// the batch-mode flush error/divert branches, using an erroring producer mock
// (added fields on mockKafkaProducer). These paths are distinct from the async
// OnEvent("error") path the existing tests already cover.

var errSyncKafka = errors.New("sync send failure")

// Test_KafkaWriter_sendOne_SyncErrorSpills covers sendOne lines 490-493: a
// synchronous Send error with a spill store present routes the record to
// failover (durable) rather than the errored/drop counter.
func Test_KafkaWriter_sendOne_SyncErrorSpills(t *testing.T) {
	mp := newMockKafkaProducer()
	mp.sendErr = errSyncKafka
	w := &KafkaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[kafka.Message](4),
		messages: make(chan kafka.Message, 4),
		producer: mp,
	}
	w.sendOne(spillerMsg("t", "boom"))
	if w.stats.Spilled() != 1 {
		t.Errorf("spilled=%d want 1 (sync error -> spill failover)", w.stats.Spilled())
	}
	if w.errored != 0 {
		t.Errorf("errored=%d want 0 (spilled, not errored)", w.errored)
	}
	if atomic.LoadUint64(&w.failovered) != 1 {
		t.Errorf("failovered=%d want 1", w.failovered)
	}
}

// Test_KafkaWriter_sendOne_SyncErrorNoSpiller covers sendOne lines 487-489 and
// 494-497: a synchronous Send error with a breaker but NO spill store records
// the error on the breaker's window and increments errored + fires the "error"
// event (no durable failover available).
func Test_KafkaWriter_sendOne_SyncErrorNoSpiller(t *testing.T) {
	mp := newMockKafkaProducer()
	mp.sendErr = errSyncKafka
	var firedName string
	b := newKafkaBreaker(breakerTestConfig(), time.Now())
	w := &KafkaWriter{
		policy:   OverflowDrop,
		spiller:  nil,
		messages: make(chan kafka.Message, 4),
		producer: mp,
		breaker:  b, // present -> exercises k.breaker.recordError() on the err path
	}
	w.SetOnEvent(func(name string, delta int64) { firedName = name })
	w.sendOne(spillerMsg("t", "boom"))
	if w.errored != 1 {
		t.Errorf("errored=%d want 1", w.errored)
	}
	if firedName != "error" {
		t.Errorf("onEvent=%q want \"error\"", firedName)
	}
	if w.stats.Dropped() != 0 {
		t.Errorf("dropped=%d want 0 (counted as errored, not dropped)", w.stats.Dropped())
	}
	if b.winErr.Load() != 1 {
		t.Errorf("breaker winErr=%d want 1 (recordError should have fired)", b.winErr.Load())
	}
}

// Test_KafkaWriter_sendOne_BreakerOpenSpills covers the breaker-open fast-path
// at sendOne line 475-477: when the breaker is open and a spiller exists, the
// record is diverted to spill WITHOUT calling Send (avoids a futile call that
// would async-fail). Also exercises recordSend on the breaker path via failover.
func Test_KafkaWriter_sendOne_BreakerOpenSpills(t *testing.T) {
	mp := newMockKafkaProducer()
	b := newKafkaBreaker(breakerTestConfig(), time.Now())
	b.transition(breakerOpen, time.Now().UnixNano()) // force open
	w := &KafkaWriter{
		policy:   OverflowSpill,
		spiller:  NewRingSpiller[kafka.Message](4),
		messages: make(chan kafka.Message, 4),
		producer: mp,
		breaker:  b,
	}
	w.sendOne(spillerMsg("t", "divert"))
	if mp.sendCalls != 0 {
		t.Errorf("Send called %d times; want 0 (breaker open must divert)", mp.sendCalls)
	}
	if w.stats.Spilled() != 1 {
		t.Errorf("spilled=%d want 1", w.stats.Spilled())
	}
}

// Test_KafkaWriter_failover_NoSpillerDrops covers failover line 511-513: with
// no spill store (or a full one), failover degrades to a counted drop.
func Test_KafkaWriter_failover_NoSpillerDrops(t *testing.T) {
	w := &KafkaWriter{
		policy:   OverflowDrop,
		spiller:  nil,
		messages: make(chan kafka.Message, 4),
	}
	w.failover(spillerMsg("t", "x"))
	if w.stats.Dropped() != 1 {
		t.Errorf("dropped=%d want 1 (no spiller -> drop)", w.stats.Dropped())
	}
	if w.stats.Spilled() != 0 {
		t.Errorf("spilled=%d want 0", w.stats.Spilled())
	}
}

// Test_KafkaWriter_batchFlush_BreakerOpenSpillDivert covers the batch-mode flush
// branch at daemon lines 594-599: when the breaker is open with a spiller, the
// whole buffered batch is diverted to spill rather than a futile SendBatch.
// Exercised via a direct daemon run in batch mode.
func Test_KafkaWriter_batchFlush_BreakerOpenSpillDivert(t *testing.T) {
	mp := newMockKafkaProducer()
	b := newKafkaBreaker(breakerTestConfig(), time.Now())
	b.transition(breakerOpen, time.Now().UnixNano())

	w := &KafkaWriter{
		level:              INFO,
		policy:             OverflowSpill,
		spiller:            NewRingSpiller[kafka.Message](64),
		messages:           make(chan kafka.Message, 64),
		producer:           mp,
		breaker:            b,
		batchMode:          true,
		batchSize:          2,
		batchFlushInterval: 10 * time.Second, // never auto-flush; we close to trigger flush
		drainInterval:      200 * time.Millisecond,
		quit:               make(chan struct{}, 1),
	}
	w.run.Store(true)
	go w.daemon()

	// feed 2 records (== batchSize) so flush fires immediately on the 2nd append.
	_ = w.Write(&Record{level: INFO, msg: "a"})
	_ = w.Write(&Record{level: INFO, msg: "b"})

	// wait for spill accounting (deterministic polling).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.stats.Spilled() >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(w.messages)
	<-w.quit
	w.wg.Wait()

	if w.stats.Spilled() < 2 {
		t.Errorf("spilled=%d want >=2 (batch diverted on breaker-open)", w.stats.Spilled())
	}
	if mp.sendBatchCalls != 0 {
		t.Errorf("SendBatch called %d times; want 0 (breaker open must divert batch)", mp.sendBatchCalls)
	}
}

// Test_KafkaWriter_batchFlush_SyncErrorSpillRequeue covers daemon lines 605-611:
// a synchronous SendBatch error with a spill store present requeues the whole
// batch to spill (at-least-once) rather than dropping it.
func Test_KafkaWriter_batchFlush_SyncErrorSpillRequeue(t *testing.T) {
	mp := newMockKafkaProducer()
	mp.sendBatchErr = errSyncKafka

	w := &KafkaWriter{
		level:              INFO,
		policy:             OverflowSpill,
		spiller:            NewRingSpiller[kafka.Message](64),
		messages:           make(chan kafka.Message, 64),
		producer:           mp,
		batchMode:          true,
		batchSize:          2,
		batchFlushInterval: 10 * time.Second,
		drainInterval:      200 * time.Millisecond,
		quit:               make(chan struct{}, 1),
	}
	w.run.Store(true)
	go w.daemon()

	_ = w.Write(&Record{level: INFO, msg: "a"})
	_ = w.Write(&Record{level: INFO, msg: "b"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.stats.Spilled() >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(w.messages)
	<-w.quit
	w.wg.Wait()

	if w.stats.Spilled() < 2 {
		t.Errorf("spilled=%d want >=2 (batch requeued to spill on sync error)", w.stats.Spilled())
	}
}

// Test_KafkaWriter_batchFlush_SyncErrorNoSpillerDrops covers daemon lines
// 618-622: a synchronous SendBatch error with NO spill store counts the batch
// as errored and drops it (fires "error" event with the batch delta).
func Test_KafkaWriter_batchFlush_SyncErrorNoSpillerDrops(t *testing.T) {
	mp := newMockKafkaProducer()
	mp.sendBatchErr = errSyncKafka
	var errDelta int64

	w := &KafkaWriter{
		level:              INFO,
		policy:             OverflowDrop,
		spiller:            nil,
		messages:           make(chan kafka.Message, 64),
		producer:           mp,
		batchMode:          true,
		batchSize:          2,
		batchFlushInterval: 10 * time.Second,
		drainInterval:      200 * time.Millisecond,
		quit:               make(chan struct{}, 1),
	}
	w.SetOnEvent(func(name string, delta int64) {
		if name == "error" {
			atomic.AddInt64(&errDelta, delta)
		}
	})
	w.run.Store(true)
	go w.daemon()

	_ = w.Write(&Record{level: INFO, msg: "a"})
	_ = w.Write(&Record{level: INFO, msg: "b"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&errDelta) >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(w.messages)
	<-w.quit
	w.wg.Wait()

	if got := atomic.LoadInt64(&errDelta); got < 2 {
		t.Errorf("error delta=%d want >=2 (batch errored, no spill)", got)
	}
	if w.errored < 2 {
		t.Errorf("errored=%d want >=2", w.errored)
	}
}

// ===========================================================================
// kafkaProducerOpts branches + Start linger/acks log branches
// ===========================================================================

// Test_KafkaWriter_kafkaProducerOpts_AllBranches covers the three conditional
// append branches in kafkaProducerOpts (lines 683/692/695): SpecifyVersion,
// ProducerSnapshotHistory, and Acks. Each adds exactly one option; the bare
// base has 3 options, so the full-config case must yield 6.
func Test_KafkaWriter_kafkaProducerOpts_AllBranches(t *testing.T) {
	w := &KafkaWriter{
		options: KafkaWriterOptions{
			Brokers:                 []string{"localhost:9092"},
			ProducerTopic:           "t",
			SpecifyVersion:          true,
			VersionStr:              "0.10.0.1",
			ProducerSnapshotHistory: 10,
			Acks:                    kafka.AcksAll,
		},
	}
	opts := w.kafkaProducerOpts()
	if len(opts) != 6 {
		t.Errorf("opts len=%d want 6 (3 base + version + snapshotHistory + acks)", len(opts))
	}

	// and the no-extra-branch path: bare options → exactly 3 (the base opts).
	w2 := &KafkaWriter{options: KafkaWriterOptions{Brokers: []string{"localhost:9092"}, ProducerTopic: "t"}}
	if got := len(w2.kafkaProducerOpts()); got != 3 {
		t.Errorf("bare opts len=%d want 3", got)
	}
}

// Test_KafkaWriter_Start_LingerPositive covers the Start log branch at line 769
// (ProducerLinger > 0 → linger = d.String()) and line 770-ish acks default path.
// It also implicitly exercises kafkaProducerOpts's ProducerLinger>0 branch.
func Test_KafkaWriter_Start_LingerPositive(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		ProducerLinger: 5 * time.Millisecond, // > 0 → the d.String() branch
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	w.Stop()
}

// Test_KafkaWriter_Start_LingerOff covers the Start log branch at line 767-768
// (ProducerLinger == kafka.LingerOff → linger = "off"), distinct from the >0
// branch covered by Test_KafkaWriter_Start_LingerPositive. Also exercises
// kafkaProducerOpts's LingerOff sub-branch (appends WithMaxBufferedRecords(1)).
func Test_KafkaWriter_Start_LingerOff(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{
		Enable: true, Level: LevelFlagInfo, Brokers: []string{"localhost:9092"},
		ProducerTopic: "cov", BufferSize: 16,
		ProducerLinger: kafka.LingerOff, // == -1 → the linger="off" branch
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDaemonRunning(t, w)
	w.Stop()
}

// Test_KafkaWriter_daemon_TickerBreakerEvaluate covers the daemon's ticker arm
// (lines 652-656): on each drain tick it calls drainSpill() and, when a breaker
// is wired, breaker.evaluate(now). Runs the daemon in per-record mode with a
// short drainInterval so at least one tick lands, then shuts down.
func Test_KafkaWriter_daemon_TickerBreakerEvaluate(t *testing.T) {
	mp := newMockKafkaProducer()
	b := newKafkaBreaker(breakerTestConfig(), time.Now())
	w := &KafkaWriter{
		level:         INFO,
		policy:        OverflowDrop,
		spiller:       NewRingSpiller[kafka.Message](4),
		messages:      make(chan kafka.Message, 4),
		producer:      mp,
		breaker:       b,
		drainInterval: 15 * time.Millisecond, // short so the ticker fires quickly
		quit:          make(chan struct{}, 1),
	}
	w.run.Store(true)
	go w.daemon()

	// Let at least one drain tick fire (drainSpill + breaker.evaluate).
	time.Sleep(80 * time.Millisecond)

	close(w.messages)
	<-w.quit
	w.wg.Wait()
	// If we got here the ticker arm executed without deadlock; evaluate ran at
	// least once on a live breaker (cold-path state machine).
}
