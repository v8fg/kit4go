//go:build !franzgo

package kafka

import (
	"context"
	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
	"testing"
)

func TestSendBatch_MetricsTrackRealRecords(t *testing.T) {
	mp := mocks.NewAsyncProducer(t, mockAsyncCfg())
	const batchSize = 50
	for range batchSize * 3 { // 3 batches of 50
		mp.ExpectInputAndSucceed()
	}
	p, _ := newSaramaProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		func([]string, *sarama.Config) (sarama.AsyncProducer, error) { return mp, nil })
	defer p.Close()

	msgs := make([]Message, batchSize)
	for i := range msgs {
		msgs[i] = Message{Value: []byte("x")}
	}
	_ = p.SendBatch(context.Background(), msgs)
	_ = p.SendBatch(context.Background(), msgs)
	_ = p.SendBatch(context.Background(), msgs)

	waitUntil(t, func() bool { return p.Metrics().Enqueued == 150 }, "Enqueued==150 (3×50 records)")
	waitUntil(t, func() bool { return p.Metrics().Success == 150 }, "Success==150")
	if got := p.Metrics().BatchCount; got != 3 {
		t.Errorf("BatchCount=%d want 3 (3 calls)", got)
	}
	if got := p.Metrics().BatchMax; got != 50 {
		t.Errorf("BatchMax=%d want 50", got)
	}
	t.Logf("Metrics: Enqueued=%d Success=%d Failed=%d BatchCount=%d BatchMax=%d InFlight=%d",
		p.Metrics().Enqueued, p.Metrics().Success, p.Metrics().Failed,
		p.Metrics().BatchCount, p.Metrics().BatchMax, p.Metrics().InFlight)
}
