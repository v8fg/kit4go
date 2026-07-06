package log4go

import (
	"testing"

	"github.com/v8fg/kit4go/kafka"
)

// Cover the nil-guard branch in each SetOnEvent: passing nil must clear the
// stored hook (Store(nil)+return) rather than take the store-the-func path.
// These three branches (FileWriter / WebhookWriter / KafKaWriter) each drop
// coverage to 50% when only the non-nil path is exercised.

func Test_FileWriter_SetOnEvent_NilClearsHook(t *testing.T) {
	w := NewFileWriter()
	// install a real hook first so the nil branch has something to clear.
	w.SetOnEvent(func(name string, delta int64) {})
	if p := w.onEvent.Load(); p == nil {
		t.Fatal("non-nil SetOnEvent should store the hook")
	}
	w.SetOnEvent(nil) // exercises the nil branch
	if p := w.onEvent.Load(); p != nil {
		t.Fatalf("SetOnEvent(nil) should clear the hook, got %v", p)
	}
}

func Test_WebhookWriter_SetOnEvent_NilClearsHook(t *testing.T) {
	w := NewWebhookWriter(nil, WebhookWriterOptions{})
	w.SetOnEvent(func(name string, delta int64) {})
	if p := w.onEvent.Load(); p == nil {
		t.Fatal("non-nil SetOnEvent should store the hook")
	}
	w.SetOnEvent(nil)
	if p := w.onEvent.Load(); p != nil {
		t.Fatalf("SetOnEvent(nil) should clear the hook, got %v", p)
	}
}

func Test_KafKaWriter_SetOnEvent_NilClearsHook(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 8})
	w.SetOnEvent(func(name string, delta int64) {})
	if p := w.onEvent.Load(); p == nil {
		t.Fatal("non-nil SetOnEvent should store the hook")
	}
	w.SetOnEvent(nil)
	if p := w.onEvent.Load(); p != nil {
		t.Fatalf("SetOnEvent(nil) should clear the hook, got %v", p)
	}
}

// ProducerSnapshot returns the zero value when the producer is nil. Covers the
// `if !k.producerNotNil() { return kafka.ProducerSnapshot{} }` short-circuit
// for a writer that bypassed Start (nil producer).
func Test_KafKaWriter_ProducerSnapshot_NilProducer(t *testing.T) {
	w := NewKafKaWriter(KafKaWriterOptions{ProducerTopic: "t", BufferSize: 8})
	if w.producerNotNil() {
		t.Fatal("precondition: producer should be nil before Start")
	}
	got := w.ProducerSnapshot()
	if got != (kafka.ProducerSnapshot{}) {
		t.Fatalf("ProducerSnapshot on nil producer: want zero value, got %+v", got)
	}
}
