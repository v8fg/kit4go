//go:build franzgo

package kafka

import (
	"strings"
	"testing"
)

// TestFranzgo_ConsumerGroup_SafeHandlerRecovers exercises the franz-go
// consumer-group panic recovery directly (no broker needed). A panicking
// handler must be recovered (not propagated), counted in Recovered() and
// Metrics().Recovered, surfaced as a "handler panic" error (so the caller
// NACKs), and leave the consumer able to process the next message. This
// mirrors the sarama backend's cgHandler.safeHandlerCall behaviour so the
// seamless-switch contract holds for a panicking handler under both backends.
func TestFranzgo_ConsumerGroup_SafeHandlerRecovers(t *testing.T) {
	s := &franzConsumerGroup{}
	processed := 0
	handler := func(m Message) error {
		if string(m.Value) == "boom" {
			panic("handler exploded")
		}
		processed++
		return nil
	}

	// Normal call: no panic, no error.
	if err := s.safeHandler(handler, Message{Value: []byte("ok")}); err != nil {
		t.Fatalf("normal call err=%v, want nil", err)
	}

	// Panicking call: recovered and returned as a handler-panic error.
	err := s.safeHandler(handler, Message{Value: []byte("boom")})
	if err == nil || !strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("panicking call err=%v, want a handler-panic error", err)
	}
	if got := s.Recovered(); got != 1 {
		t.Fatalf("Recovered()=%d, want 1", got)
	}
	if got := s.Metrics().Recovered; got != 1 {
		t.Fatalf("Metrics().Recovered=%d, want 1 (was always 0 before the fix)", got)
	}

	// Post-panic normal call still works: no residual broken state.
	if err := s.safeHandler(handler, Message{Value: []byte("ok2")}); err != nil {
		t.Fatalf("post-panic normal call err=%v, want nil", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d, want 2 (normal calls before and after the panic)", processed)
	}
}

// TestFranzgo_PartitionConsumer_SafeHandlerRecovers is the partition-consumer
// counterpart: the same recover contract applies to ConsumePartition's callback.
func TestFranzgo_PartitionConsumer_SafeHandlerRecovers(t *testing.T) {
	s := &franzPartitionConsumer{}
	handler := func(m Message) error { panic("nope") }

	err := s.safeHandler(handler, Message{Value: []byte("x")})
	if err == nil || !strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("err=%v, want a handler-panic error", err)
	}
	if got := s.Recovered(); got != 1 {
		t.Fatalf("Recovered()=%d, want 1", got)
	}
	if got := s.Metrics().Recovered; got != 1 {
		t.Fatalf("Metrics().Recovered=%d, want 1", got)
	}
}
