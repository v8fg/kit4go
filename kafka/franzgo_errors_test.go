//go:build franzgo

package kafka

import (
	"context"
	"errors"
	"testing"
)

// TestFranzgoErrors_PushErr covers the 0% pushErr methods on both consumer types.
func TestFranzgoErrors_PushErr(t *testing.T) {
	cg := &franzConsumerGroup{opts: Options{GroupID: "g"}}
	cg.pushErr(errors.New("cg-err"))
	select {
	case <-cg.Errors():
	default:
		t.Error("pushErr didn't deliver to Errors()")
	}

	pc := &franzPartitionConsumer{opts: Options{Topic: "t"}}
	pc.pushErr(errors.New("pc-err"))
	select {
	case <-pc.Errors():
	default:
		t.Error("pushErr didn't deliver to Errors()")
	}
}

// TestFranzgoErrors_SetOnEventNil covers the nil-path (the 50% gap on all types).
func TestFranzgoErrors_SetOnEventNil(t *testing.T) {
	p := &franzProducer{opts: Options{Topic: "t"}}
	p.SetOnEvent(nil)
	p.fire(ProducerEvent{Name: "x"})

	sp := &franzSyncProducer{opts: Options{Topic: "t"}}
	sp.SetOnEvent(nil)
	sp.fire(ProducerEvent{Name: "x"})

	cg := &franzConsumerGroup{opts: Options{GroupID: "g"}}
	cg.SetOnEvent(nil)
	cg.fire(ConsumerEvent{Name: "x"})

	pc := &franzPartitionConsumer{opts: Options{Topic: "t"}}
	pc.SetOnEvent(nil)
	pc.fire(ConsumerEvent{Name: "x"})
}

// TestFranzgoErrors_ClosedProducers covers Close idempotency + closed guards.
func TestFranzgoErrors_ClosedProducers(t *testing.T) {
	// async producer closed guard
	p := &franzProducer{opts: Options{Topic: "t"}}
	p.closed.Store(true)
	if err := p.Send(context.Background(), Message{Value: []byte("x")}); err != ErrProducerClosed {
		t.Errorf("Send after close: %v want ErrProducerClosed", err)
	}
	if err := p.SendBatch(context.Background(), []Message{{Value: []byte("y")}}); err != ErrProducerClosed {
		t.Errorf("SendBatch after close: %v want ErrProducerClosed", err)
	}
	if err := p.Close(); err != nil { // second close is no-op (CAS fails)
		t.Errorf("double Close: %v", err)
	}

	// sync producer closed guard
	sp := &franzSyncProducer{opts: Options{Topic: "t"}}
	sp.closed.Store(true)
	if _, _, err := sp.Send(context.Background(), Message{Value: []byte("z")}); err != ErrProducerClosed {
		t.Errorf("sync Send after close: %v want ErrProducerClosed", err)
	}
	if err := sp.Close(); err != nil {
		t.Errorf("sync double Close: %v", err)
	}
}

// TestFranzgoErrors_ConsumerClosedGuards covers consumer Close idempotency.
func TestFranzgoErrors_ConsumerClosedGuards(t *testing.T) {
	cg := &franzConsumerGroup{opts: Options{GroupID: "g"}}
	if err := cg.Close(); err != nil {
		t.Errorf("cg Close: %v", err)
	}

	pc := &franzPartitionConsumer{opts: Options{Topic: "t"}}
	if err := pc.Close(); err != nil {
		t.Errorf("pc Close: %v", err)
	}
}
