//go:build integration

package log4go

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/v8fg/kit4go/kafka"
)

// measureKafKaThroughput writes n records (OverflowBlock → Write self-paces to
// the daemon+broker ingest rate) against a real broker and returns (sent, wall-
// clock for Write loop + Stop). No producerFactory → a real sarama producer is
// created in Start. Used by TestIntegration_KafKaWriter_Throughput.
func measureKafKaThroughput(t *testing.T, brokers, topic string, batch bool, n int) (uint64, time.Duration) {
	t.Helper()
	opts := KafKaWriterOptions{
		Brokers:        []string{brokers},
		ProducerTopic:  topic,
		BufferSize:     1 << 14,
		Level:          LevelFlagInfo,
		OverflowPolicy: "block", // self-paced → sustained throughput, no drops
	}
	if batch {
		opts.BatchMode = true
		opts.BatchSize = 200
		opts.BatchFlushInterval = 10 * time.Millisecond
	}
	w := NewKafKaWriter(opts)
	if err := w.Start(); err != nil {
		t.Fatalf("Start (batch=%v): %v", batch, err)
	}
	start := time.Now()
	for i := 0; i < n; i++ {
		if err := w.Write(&Record{level: INFO, msg: fmt.Sprintf("tp-%d", i)}); err != nil {
			t.Fatalf("Write[%d] (batch=%v): %v", i, batch, err)
		}
	}
	w.Stop() // flush the final partial batch + close producer
	return w.Metrics().Sent, time.Since(start)
}

// TestIntegration_KafKaWriter_Throughput measures sustained throughput (rec/s)
// of per-record vs batch mode against a real broker, with the kafka backend's
// default 10ms linger. Honest result: ≈ parity (the backend coalesces BOTH
// modes at the broker level, so log4go Send vs SendBatch is invisible there).
// Gated by KAFKA_BROKERS.
func TestIntegration_KafKaWriter_Throughput(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping integration test")
	}
	defer silenceStdLogger(t)()
	const n = 10000
	base := time.Now().UnixNano()

	prSent, prDur := measureKafKaThroughput(t, brokers, fmt.Sprintf("kit4go-l4g-tp-pr-%d", base), false, n)
	bSent, bDur := measureKafKaThroughput(t, brokers, fmt.Sprintf("kit4go-l4g-tp-b-%d", base), true, n)

	t.Logf("per-record: %d rec / %v = %.0f rec/s", prSent, prDur, float64(prSent)/prDur.Seconds())
	t.Logf("batch(200): %d rec / %v = %.0f rec/s", bSent, bDur, float64(bSent)/bDur.Seconds())
	t.Logf("speedup batch/per-record = %.2fx", (float64(bSent)/bDur.Seconds())/(float64(prSent)/prDur.Seconds()))
	if prSent != uint64(n) || bSent != uint64(n) {
		t.Errorf("loss: per-record=%d batch=%d want %d", prSent, bSent, n)
	}
}

// TestIntegration_KafKaWriter_BatchDelivery is the end-to-end proof that batch
// mode delivers EVERY record to a real broker (no loss across batch flushes +
// the shutdown flush). Gated by KAFKA_BROKERS (comma-separated host:port), so it
// is SKIPPED unless a broker is present:
//
//	docker run -d -p 9092:9092 ... (a Kafka broker with auto-topic-create)
//	KAFKA_BROKERS=localhost:9092 go test -tags integration -run Integration_KafKaWriter -v ./...
//
// It writes N records in batch mode, stops (flushing the final partial batch),
// then consumes the topic from the start and asserts all N arrived.
func TestIntegration_KafKaWriter_BatchDelivery(t *testing.T) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		t.Skip("KAFKA_BROKERS unset; skipping integration test")
	}
	const n = 200
	topic := fmt.Sprintf("kit4go-l4g-batch-it-%d", time.Now().UnixNano())
	groupID := fmt.Sprintf("kit4go-l4g-batch-it-group-%d", time.Now().UnixNano())

	// Produce N records in batch mode (BatchSize 50 → 4 count-flushes + a
	// shutdown flush of the remainder).
	w := NewKafKaWriter(KafKaWriterOptions{
		Brokers:            []string{brokers},
		ProducerTopic:      topic,
		BatchMode:          true,
		BatchSize:          50,
		BatchFlushInterval: 100 * time.Millisecond,
		BufferSize:         4096,
		Level:              LevelFlagInfo,
	})
	if err := w.Start(); err != nil {
		t.Fatalf("writer Start: %v", err)
	}
	for i := 0; i < n; i++ {
		if err := w.Write(&Record{level: INFO, msg: fmt.Sprintf("batch-msg-%d", i)}); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	w.Stop() // flush the final partial batch, then close the producer

	m := w.Metrics()
	if m.Sent != uint64(n) {
		t.Fatalf("writer Sent=%d want %d (records lost before reaching the broker)", m.Sent, n)
	}
	if m.Batches < uint64(n/50) {
		t.Errorf("Batches=%d want >= %d (count-flushes)", m.Batches, n/50)
	}

	// Consume the topic from the start; assert every record arrived.
	grp, err := kafka.NewConsumerGroup(
		kafka.WithBrokers(brokers),
		kafka.WithGroupID(groupID),
		kafka.WithConsumerOffsetInitial(kafka.OffsetOldest),
	)
	if err != nil {
		t.Fatalf("NewConsumerGroup: %v", err)
	}
	defer grp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	seen := 0
	reached := make(chan struct{}, 1)
	go func() {
		_ = grp.Consume(ctx, []string{topic}, func(mg kafka.Message) error {
			mu.Lock()
			seen++
			got := seen
			mu.Unlock()
			if got >= n {
				select {
				case reached <- struct{}{}:
				default:
				}
			}
			return nil
		})
	}()

	select {
	case <-reached:
	case <-time.After(20 * time.Second):
	}
	cancel()
	mu.Lock()
	final := seen
	mu.Unlock()
	if final != n {
		t.Fatalf("consumed %d/%d records (batch loss)", final, n)
	}
}
