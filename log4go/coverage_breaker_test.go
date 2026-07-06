package log4go

import (
	"testing"
	"time"
)

// Cover the within-window early-return arms of evaluate (still accumulating)
// and the no-op transition (from == to).

// TestBreaker_EvaluateClosedWithinWindow covers the `case breakerClosed` early
// return at kafka_breaker.go:156 — a tick that lands before the window elapses
// must short-circuit (no transition, no window roll).
func TestBreaker_EvaluateClosedWithinWindow(t *testing.T) {
	cfg := breakerTestConfig() // window = 1s
	b := newKafkaBreaker(cfg, breakerBaseTime)
	startNS := b.winStart.Load()
	b.recordSend()
	b.recordSend()

	// A tick 200ms in is still inside the window → return, window unchanged.
	b.evaluate(breakerBaseTime.Add(200 * time.Millisecond))
	if b.stateCode() != breakerClosed {
		t.Fatalf("want closed, got %d", b.stateCode())
	}
	if b.winStart.Load() != startNS {
		t.Fatal("within-window evaluate must not roll the window start")
	}
	if b.winSent.Load() != 2 {
		t.Fatalf("winSent should be preserved (got %d want 2)", b.winSent.Load())
	}
}

// TestBreaker_EvaluateHalfOpenWithinWindow covers the `case breakerHalfOpen`
// early return at kafka_breaker.go:168. After the breaker half-opens, a tick
// inside the probe window must short-circuit rather than decide.
func TestBreaker_EvaluateHalfOpenWithinWindow(t *testing.T) {
	cfg := breakerTestConfig() // cooldown 5s, window 1s
	b := newKafkaBreaker(cfg, breakerBaseTime)
	opened := driveOpen(t, b, 4, 3) // → open
	halfOpenAt := opened.Add(5 * time.Second)
	b.evaluate(halfOpenAt) // → half-open, fresh window starts at halfOpenAt
	if b.stateCode() != breakerHalfOpen {
		t.Fatalf("want half-open, got %d", b.stateCode())
	}
	winStartNS := b.winStart.Load()

	// 300ms into the probe window → still accumulating → return, no decision.
	b.evaluate(halfOpenAt.Add(300 * time.Millisecond))
	if b.stateCode() != breakerHalfOpen {
		t.Fatalf("want still half-open, got %d", b.stateCode())
	}
	if b.winStart.Load() != winStartNS {
		t.Fatal("within-window half-open evaluate must not roll the window")
	}
}

// TestBreaker_TransitionNoOp covers kafka_breaker.go:199 — transitioning to the
// state the breaker is already in is a no-op (Swap returns from==to → return
// without recording the openedAt timestamp or firing any side effect).
func TestBreaker_TransitionNoOp(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	if b.stateCode() != breakerClosed {
		t.Fatal("precondition: new breaker is closed")
	}
	// Closed→Closed: Swap returns breakerClosed == to → early return.
	b.transition(breakerClosed, breakerBaseTime.UnixNano())
	if b.stateCode() != breakerClosed {
		t.Fatalf("no-op transition should leave state closed, got %d", b.stateCode())
	}
	// Also cover the no-op when already open: open→open should not bump openedAt.
	opened := driveOpen(t, b, 4, 3)
	openedAtBefore := b.openedAt.Load()
	b.transition(breakerOpen, opened.Add(time.Second).UnixNano())
	if b.stateCode() != breakerOpen {
		t.Fatalf("want still open, got %d", b.stateCode())
	}
	if b.openedAt.Load() != openedAtBefore {
		t.Fatal("open→open transition must be a no-op (openedAt unchanged)")
	}
}
