package log4go

import (
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// This file holds the regression test for R20 P0-1: KafkaWriter shutdown race.
//
// Bug: Stop() closed k.messages (line ~793) while send() (the Write hot path,
// lines ~449-470) could be mid-send on k.messages -> send-on-closed-channel
// panic (and a close-vs-send data race under -race). ALL overflow policies were
// affected (Block, Drop default, Spill). The raw `k.messages <- msg` in the
// OverflowBlock branch had no shutdown escape, so a blocked producer could also
// deadlock against a daemon that had stopped consuming.
//
// Fix (ported from FileWriter's proven pattern, file_writer.go:528-568 send +
// 795-812 Stop): add a `closing atomic.Bool` + `stop chan struct{}`; send()
// fast-exits on closing and selects on stop in every branch; Stop() sets
// closing FIRST then closes stop (NEVER closes messages); the daemon exits via
// the stop-channel select after draining pending records.
//
// This test exercises the exact failure mode: 8 goroutines calling Write
// concurrently while one calls Stop, repeated 50x under -race. The OLD code
// panics (send on closed channel) and/or reports a close-vs-send data race
// within the first few iterations; the fixed code completes cleanly.

// startKafkaWriterForRaceTest builds a KafkaWriter wired to a mock producer
// (no real broker) and starts its daemon, returning it ready for concurrent
// Write/Stop stress.
func startKafkaWriterForRaceTest(t *testing.T, policy string, bufSize int) *KafkaWriter {
	t.Helper()
	w := NewKafkaWriter(KafkaWriterOptions{
		ProducerTopic:   "t",
		BufferSize:      bufSize,
		OverflowPolicy:  policy,
		ProducerLinger:  kafka.LingerOff, // flush every record: fastest daemon drain
		BreakerDisabled: true,
	})
	w.producerFactory = func() (kafka.Producer, error) {
		return newMockKafkaProducer(), nil
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// let the daemon + producer hook schedule before the hammer drops
	time.Sleep(20 * time.Millisecond)
	return w
}

// Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Block is the OverflowBlock
// regression: a blocked producer (channel full) races Stop closing messages.
func Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Block(t *testing.T) {
	concurrentWriteStopNoPanic(t, "block", 2) // tiny buffer => frequent block
}

// Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Drop covers the default policy.
func Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Drop(t *testing.T) {
	concurrentWriteStopNoPanic(t, "drop", 8)
}

// Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Spill covers the spill policy
// (drainSpill re-inject path also raced close(messages) on the old code).
func Test_KafkaWriter_ConcurrentWriteStop_NoPanic_Spill(t *testing.T) {
	concurrentWriteStopNoPanic(t, "spill", 8)
}

func concurrentWriteStopNoPanic(t *testing.T, policy string, bufSize int) {
	t.Helper()
	const iterations = 50
	const producers = 8
	for range iterations {
		w := startKafkaWriterForRaceTest(t, policy, bufSize)

		var wg sync.WaitGroup
		stop := make(chan struct{})
		// producers hammer Write until Stop is observed.
		for range producers {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					// Ignore the error: once shutdown begins Write may return nil
					// (drop fast-path) — the point is to race send() vs Stop().
					_ = w.Write(&Record{level: INFO, msg: "concurrent write+stop"})
				}
			})
		}

		// Give producers a moment to get in-flight, then tear down. The race
		// window is the send() select vs Stop() closing — no sleep needed for
		// correctness, the brief yield just raises the probability of overlap.
		time.Sleep(time.Millisecond)
		w.Stop() // OLD CODE: close(messages) panics/races here
		close(stop)
		wg.Wait()
	}
	// Reaching here 50x under -race with no panic and no race report means the
	// shutdown ordering is stable for this policy.
}

// Test_KafkaWriter_StopTwice_NoPanic verifies Stop is idempotent: the
// CompareAndSwap claim prevents a double close(k.stop) / double daemon-drain.
func Test_KafkaWriter_StopTwice_NoPanic(t *testing.T) {
	for range 20 {
		w := startKafkaWriterForRaceTest(t, "drop", 8)
		// concurrent Stops from two goroutines + ongoing Writes.
		var wg sync.WaitGroup
		stop := make(chan struct{})
		for range 4 {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					_ = w.Write(&Record{level: INFO, msg: "x"})
				}
			})
		}
		// two goroutines race Stop
		var stopWg sync.WaitGroup
		for range 2 {
			stopWg.Go(func() { w.Stop() })
		}
		time.Sleep(time.Millisecond)
		close(stop)
		wg.Wait()
		stopWg.Wait()
	}
}
