//go:build !franzgo

package kafka

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// Unit-level stress coverage (no broker): concurrent correctness + high-volume
// bursts against the sarama mock producer / stub consumer claim. These verify
// Metrics accounting and race-freedom under load. (franz-go Send/Consume
// round-trips need a real broker — covered by the broker-gated integration test.)

// TestStress_AsyncProducer_Concurrent hammers the async producer from many
// goroutines; verifies the Success/Enqueued accounting is exact and race-clean.
func TestStress_AsyncProducer_Concurrent(t *testing.T) {
	const goroutines = 16
	const perG = 500
	const total = goroutines * perG

	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	for range total { // mock requires one expectation per input
		mp.ExpectInputAndSucceed()
	}
	p, err := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			for range perG {
				_ = p.Send(context.Background(), Message{Value: []byte("x")})
			}
		}()
	}
	close(start)
	wg.Wait()

	waitUntil(t, func() bool { return p.Metrics().Enqueued == total }, "all enqueued")
	waitUntil(t, func() bool { return p.Metrics().Success == total }, "all success")
	if got := p.Metrics().Failed; got != 0 {
		t.Errorf("Failed=%d want 0", got)
	}
	if got := p.Metrics().Bytes; got != total { // each value is 1 byte
		t.Errorf("Bytes=%d want %d", got, total)
	}
	_ = p.Close()
}

// TestStress_SyncProducer_Batch issues a large batch of sync sends.
func TestStress_SyncProducer_Batch(t *testing.T) {
	const n = 2000
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	for range n {
		mp.ExpectSendMessageAndSucceed()
	}
	p, _ := newSaramaSyncProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		syncMockFactory(mp))
	for i := range n {
		if _, _, err := p.Send(context.Background(), Message{Value: []byte("v")}); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}
	if got := p.Metrics(); got.Enqueued != n || got.Success != n || got.Failed != 0 {
		t.Errorf("Metrics=%+v want enq=succ=%d fail=0", got, n)
	}
	_ = p.Close()
}

// TestStress_ConsumerGroupHandler_Burst drives the handler adapter with a large
// burst of synthetic messages; verifies ACK accounting is exact.
func TestStress_ConsumerGroupHandler_Burst(t *testing.T) {
	const n = 5000
	parent := &saramaConsumerGroup{}
	h := &cgHandler{parent: parent, handler: func(Message) error { return nil }}
	msgs := make([]*sarama.ConsumerMessage, n)
	for i := range msgs {
		msgs[i] = &sarama.ConsumerMessage{Topic: "t", Value: []byte("x")}
	}
	_, marked := runHandler(t, h, msgs)
	if marked != n {
		t.Errorf("MarkMessage=%d want %d", marked, n)
	}
	if got := parent.Metrics(); got.Received != n || got.Acked != n || got.Failed != 0 || got.Bytes != n {
		t.Errorf("Metrics=%+v want received=acked=%d bytes=%d", got, n, n)
	}
}

// TestStress_ConsumerGroupHandler_NACKRatio verifies a mixed ACK/NACK burst
// keeps the accounting exact.
func TestStress_ConsumerGroupHandler_NACKRatio(t *testing.T) {
	const n = 1000
	parent := &saramaConsumerGroup{}
	var count atomic.Int32
	h := &cgHandler{parent: parent, handler: func(Message) error {
		if count.Add(1)%4 == 0 { // 25% NACK
			return errBoom
		}
		return nil
	}}
	msgs := make([]*sarama.ConsumerMessage, n)
	for i := range msgs {
		msgs[i] = &sarama.ConsumerMessage{Topic: "t", Value: []byte("x")}
	}
	_, _ = runHandler(t, h, msgs)
	acked := uint64(n - n/4) // 750
	if got := parent.Metrics(); got.Received != uint64(n) || got.Acked != acked || got.Failed != uint64(n/4) {
		t.Errorf("Metrics=%+v want received=%d acked=%d failed=%d", got, n, acked, n/4)
	}
}
