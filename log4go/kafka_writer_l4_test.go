package log4go

import (
	"testing"

	"github.com/v8fg/kit4go/kafka"
)

// L4 writer-level tests: breaker-open diversion to spill, Metrics exposure,
// disabled-breaker, and closed-breaker normal send. No broker required — sendOne
// is exercised in-process with a ring spiller.

func newSpillTestWriter() *KafkaWriter {
	w := NewKafkaWriter(KafkaWriterOptions{
		OverflowPolicy: OverflowPolicySpill,
		SpillType:      SpillTypeRing,
		SpillSize:      100,
	})
	w.policy = OverflowSpill
	w.spiller = NewRingSpiller[kafka.Message](100)
	w.messages = make(chan kafka.Message, 8)
	return w
}

// When the breaker is open and a spill store exists, sendOne routes the record
// to spill instead of a futile Send, and Metrics surfaces the failover + the
// open circuit. This is the core "kafka down → fail-open to local" guarantee.
func TestKafkaWriter_BreakerOpenDivertsToSpill(t *testing.T) {
	w := newSpillTestWriter()
	w.breaker = newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	driveOpen(t, w.breaker, 4, 3) // closed -> open deterministically

	w.sendOne(kafka.Message{Value: []byte("rec-1")})

	if w.failovered != 1 {
		t.Fatalf("failovered: want 1, got %d", w.failovered)
	}
	if w.spiller.Len() != 1 {
		t.Fatalf("spiller Len: want 1, got %d", w.spiller.Len())
	}
	if w.sent != 0 {
		t.Fatalf("sent should be 0 (diverted, not sent): got %d", w.sent)
	}

	m := w.Metrics()
	if m.Failovered != 1 {
		t.Fatalf("Metrics.Failovered: want 1, got %d", m.Failovered)
	}
	if m.CircuitState != breakerOpen {
		t.Fatalf("Metrics.CircuitState: want open(%d), got %d", breakerOpen, m.CircuitState)
	}
}

// With BreakerDisabled, the writer carries no breaker; Metrics.CircuitState
// reports closed (the no-op state).
func TestKafkaWriter_BreakerDisabled(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{BreakerDisabled: true})
	if w.breaker != nil {
		t.Fatal("breaker should be nil when disabled")
	}
	if got := w.breakerStateCode(); got != breakerClosed {
		t.Fatalf("disabled breaker state: want closed, got %d", got)
	}
	m := w.Metrics()
	if m.CircuitState != breakerClosed {
		t.Fatalf("Metrics.CircuitState: want closed when disabled, got %d", m.CircuitState)
	}
}

// A closed breaker lets sendOne proceed normally (with no producer, a send is
// counted and nothing is failover'd).
func TestKafkaWriter_BreakerClosedSendsNormally(t *testing.T) {
	w := newSpillTestWriter()
	// breaker is the default (closed) from NewKafkaWriter.
	w.sendOne(kafka.Message{Value: []byte("rec-1")})

	if w.sent != 1 {
		t.Fatalf("sent: want 1, got %d", w.sent)
	}
	if w.failovered != 0 {
		t.Fatalf("failovered should be 0 when closed: got %d", w.failovered)
	}
	if w.spiller.Len() != 0 {
		t.Fatalf("spiller should be empty when closed: got %d", w.spiller.Len())
	}
}

// Diversion does NOT happen without a spill store (Drop/Block policies keep
// their semantics): a closed breaker with no spiller counts a normal send, and
// an open breaker with no spiller still attempts the send path (no silent drop
// beyond the existing policy).
func TestKafkaWriter_NoDivertWithoutSpiller(t *testing.T) {
	w := NewKafkaWriter(KafkaWriterOptions{}) // Drop policy, no spiller
	w.policy = OverflowDrop
	w.messages = make(chan kafka.Message, 8)
	// Force the breaker open; with no spiller, sendOne must NOT divert.
	w.breaker = newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	driveOpen(t, w.breaker, 4, 3)

	w.sendOne(kafka.Message{Value: []byte("rec-1")})
	// No producer → counted as a normal send (the diversion gate requires a spiller).
	if w.sent != 1 {
		t.Fatalf("sent: want 1 (no divert without spiller), got %d", w.sent)
	}
	if w.failovered != 0 {
		t.Fatalf("failovered should be 0 without a spiller: got %d", w.failovered)
	}
}
