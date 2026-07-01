package log4go

import (
	"testing"
	"time"
)

// Deterministic breaker state-machine tests. The clock is injected via evaluate,
// so transitions are asserted without real sleeping.

var breakerBaseTime = time.Unix(1700000000, 0)

func breakerTestConfig() breakerConfig {
	return breakerConfig{
		failRate:   0.5,
		minSamples: 4,
		window:     time.Second,
		cooldown:   5 * time.Second,
	}
}

// driveOpen pushes a high-error window and advances the clock past the window so
// the breaker trips to open. Returns the time at which it opened.
func driveOpen(t *testing.T, b *kafkaBreaker, sends, errs int) time.Time {
	t.Helper()
	for i := 0; i < sends; i++ {
		b.recordSend()
	}
	for i := 0; i < errs; i++ {
		b.recordError()
	}
	opened := breakerBaseTime.Add(time.Second)
	b.evaluate(opened) // window elapsed → decide
	if !b.isOpen() {
		t.Fatalf("expected open after %d sends/%d errs", sends, errs)
	}
	return opened
}

func TestBreaker_OpensOnHighErrorRate(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	if b.isOpen() {
		t.Fatal("new breaker should be closed")
	}
	// 4 sends, 3 errors → 75% >= 50%, but window not elapsed yet → still closed.
	for i := 0; i < 4; i++ {
		b.recordSend()
	}
	for i := 0; i < 3; i++ {
		b.recordError()
	}
	if b.isOpen() {
		t.Fatal("should stay closed before the window elapses")
	}
	b.evaluate(breakerBaseTime.Add(time.Second))
	if !b.isOpen() {
		t.Fatal("should open after a high-error window elapses")
	}
}

func TestBreaker_StaysClosedOnLowErrorRate(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	for i := 0; i < 4; i++ {
		b.recordSend()
	}
	b.recordError() // 25% < 50%
	b.evaluate(breakerBaseTime.Add(time.Second))
	if b.stateCode() != breakerClosed {
		t.Fatal("should stay closed on a low error rate")
	}
}

func TestBreaker_MinSamplesGuard(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	// 3 sends, 3 errors → 100% but below minSamples (4) → don't trust it.
	for i := 0; i < 3; i++ {
		b.recordSend()
		b.recordError()
	}
	b.evaluate(breakerBaseTime.Add(time.Second))
	if b.stateCode() != breakerClosed {
		t.Fatal("should not trip below minSamples even at 100% error")
	}
}

func TestBreaker_OpenToHalfOpenAfterCooldown(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	opened := driveOpen(t, b, 4, 3)

	// Before cooldown: stays open.
	b.evaluate(opened.Add(3 * time.Second))
	if b.stateCode() != breakerOpen {
		t.Fatal("should stay open before cooldown")
	}
	// At cooldown: probe → half-open.
	b.evaluate(opened.Add(5 * time.Second))
	if b.stateCode() != breakerHalfOpen {
		t.Fatal("should be half-open after cooldown")
	}
}

func TestBreaker_HalfOpenClosesOnRecovery(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	opened := driveOpen(t, b, 4, 3)
	halfOpenAt := opened.Add(5 * time.Second)
	b.evaluate(halfOpenAt) // → half-open

	// Clean probe window: sends, no errors.
	for i := 0; i < 4; i++ {
		b.recordSend()
	}
	b.evaluate(halfOpenAt.Add(time.Second)) // window elapsed
	if b.stateCode() != breakerClosed {
		t.Fatal("should close after a clean half-open probe window")
	}
}

func TestBreaker_HalfOpenReopensOnContinuedErrors(t *testing.T) {
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	opened := driveOpen(t, b, 4, 3)
	halfOpenAt := opened.Add(5 * time.Second)
	b.evaluate(halfOpenAt) // → half-open

	// Still-bad probe window.
	for i := 0; i < 4; i++ {
		b.recordSend()
	}
	for i := 0; i < 3; i++ {
		b.recordError()
	}
	b.evaluate(halfOpenAt.Add(time.Second))
	if b.stateCode() != breakerOpen {
		t.Fatal("should reopen if the half-open probe window is still bad")
	}
}

func TestBreaker_OnTransitionHookFires(t *testing.T) {
	var transitions []string
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	b.onTransition = func(from, to string) { transitions = append(transitions, from+"->"+to) }

	driveOpen(t, b, 4, 3) // closed -> open
	opened := breakerBaseTime.Add(time.Second)
	b.evaluate(opened.Add(5 * time.Second)) // open -> half_open

	found := false
	for _, tr := range transitions {
		if tr == "closed->open" || tr == "open->half_open" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transition hook to fire; got %v", transitions)
	}
}

func TestBreaker_NoTransitionNoHook(t *testing.T) {
	calls := 0
	b := newKafkaBreaker(breakerTestConfig(), breakerBaseTime)
	b.onTransition = func(string, string) { calls++ }
	// Low error rate: closed -> closed (no real transition).
	for i := 0; i < 4; i++ {
		b.recordSend()
	}
	b.evaluate(breakerBaseTime.Add(time.Second))
	if calls != 0 {
		t.Fatalf("hook should not fire on closed->closed; got %d calls", calls)
	}
}
