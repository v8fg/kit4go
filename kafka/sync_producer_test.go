//go:build !franzgo

package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
)

// syncMockFactory wraps a *mocks.SyncProducer into the syncProducerFactory seam.
func syncMockFactory(mp *mocks.SyncProducer) syncProducerFactory {
	return func([]string, *sarama.Config) (sarama.SyncProducer, error) { return mp, nil }
}

func TestSyncProducer_SendSuccess(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	mp.ExpectSendMessageAndSucceed()
	p, err := newSaramaSyncProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		syncMockFactory(mp))
	if err != nil {
		t.Fatal(err)
	}
	part, off, err := p.Send(context.Background(), Message{Value: []byte("hi")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if part < 0 || off < 0 {
		t.Errorf("got (%d,%d) want >=0", part, off)
	}
	if got := p.Metrics(); got.Enqueued != 1 || got.Success != 1 || got.Failed != 0 {
		t.Errorf("Metrics=%+v want enq=1 succ=1 fail=0", got)
	}
	_ = p.Close()
}

func TestSyncProducer_SendError(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	mp.ExpectSendMessageAndFail(errBoom)
	p, _ := newSaramaSyncProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		syncMockFactory(mp))
	if _, _, err := p.Send(context.Background(), Message{Value: []byte("x")}); !errors.Is(err, errBoom) {
		t.Errorf("got %v want errBoom", err)
	}
	if got := p.Metrics().Failed; got != 1 {
		t.Errorf("Failed=%d want 1", got)
	}
	_ = p.Close()
}

func TestSyncProducer_CloseIdempotent(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaSyncProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		syncMockFactory(mp))
	_ = p.Close()
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestSyncProducer_SendAfterClose(t *testing.T) {
	mp := mocks.NewSyncProducer(t, mockAsyncCfg())
	p, _ := newSaramaSyncProducer(Options{Brokers: []string{"x"}, Topic: "t"}.withDefaults(),
		syncMockFactory(mp))
	_ = p.Close()
	if _, _, err := p.Send(context.Background(), Message{Value: []byte("x")}); err != ErrProducerClosed {
		t.Errorf("Send after Close: got %v want ErrProducerClosed", err)
	}
}
