package log4go

import (
	"sync/atomic"
	"time"
)

// breakerState values for kafkaBreaker.stateVal. Kept unexported and numeric so
// the daemon's hot path is a single atomic load, and so Metrics can surface the
// integer without importing an enum type.
const (
	breakerClosed   int32 = 0 // normal: Send proceeds
	breakerOpen     int32 = 1 // broker down: daemon diverts to spill (Spill policy)
	breakerHalfOpen int32 = 2 // probing: Send resumes to test recovery
)

// BreakerStateClosed/Open/HalfOpen are the public string labels for Metrics.
const (
	BreakerStateClosed   = "closed"
	BreakerStateOpen     = "open"
	BreakerStateHalfOpen = "half_open"
)

// breakerStateName maps the internal int state to its public label.
func breakerStateName(s int32) string {
	switch s {
	case breakerOpen:
		return BreakerStateOpen
	case breakerHalfOpen:
		return BreakerStateHalfOpen
	default:
		return BreakerStateClosed
	}
}

// default breaker tuning. Conservative: it trips only on a sustained high error
// rate over a real sample window, not on transient blips, and probes recovery
// every cooldown. All overridable via KafKaWriterOptions.
const (
	defaultBreakerFailRate   = 0.5
	defaultBreakerMinSamples = 20
	defaultBreakerWindow     = 2 * time.Second
	defaultBreakerCooldown   = 5 * time.Second
)

// kafkaBreaker is an inline circuit breaker around producer.Send. It opens when
// the broker-error rate crosses failRate over a rolling window, so the daemon
// can divert records to the spill store instead of futile Sends that would
// async-fail and be lost; it half-opens after cooldown to probe recovery, and
// closes when the probe window is clean.
//
// The hot path (recordSend/recordError from the daemon + the async error hook)
// is two atomic adds. The state machine runs on the daemon's drain ticker (a
// single goroutine), so transitions are race-free without a mutex. No external
// dependency — log4go stays self-contained (L4: downstream isolation).
//
// Trade-off (at-least-once): during half-open the daemon Sends to probe; a probe
// record whose delivery async-fails is not recoverable (the backend's error
// event carries no message). This loss window is bounded to one cooldown. Under
// the open state (the actual outage) records go to spill and are recovered on
// close. Set acks=all + idempotent producer to dedup re-drained records.
type kafkaBreaker struct {
	stateVal atomic.Int32 // breakerClosed | breakerOpen | breakerHalfOpen
	openedAt atomic.Int64 // unixNano when opened; 0 while closed

	// rolling-window accumulators. Touched by the daemon (recordSend) and the
	// producer's async error goroutine (recordError), so atomic. Reset by
	// evaluate on each window rollover (approximate; a straddling sample is
	// benign for a breaker decision).
	winStart atomic.Int64 // unixNano of the current window's start
	winSent  atomic.Uint64
	winErr   atomic.Uint64

	// config (immutable after newKafkaBreaker).
	failRate   float64
	minSamples uint64
	window     time.Duration
	cooldown   time.Duration

	// onTransition is an optional non-blocking hook (from→to state labels),
	// fired only on a real state change.
	onTransition func(from, to string)
}

// breakerConfig carries caller-tunable breaker settings, normalised to defaults.
type breakerConfig struct {
	failRate   float64
	minSamples uint64
	window     time.Duration
	cooldown   time.Duration
}

func newKafkaBreaker(c breakerConfig, now time.Time) *kafkaBreaker {
	b := &kafkaBreaker{
		failRate:   c.failRate,
		minSamples: c.minSamples,
		window:     c.window,
		cooldown:   c.cooldown,
	}
	b.winStart.Store(now.UnixNano())
	return b
}

func (b *kafkaBreaker) recordSend() { b.winSent.Add(1) }
func (b *kafkaBreaker) recordSendN(n uint64) {
	if n != 0 {
		b.winSent.Add(n)
	}
}
func (b *kafkaBreaker) recordError() { b.winErr.Add(1) }

// newKafkaBreakerFromOptions resolves the breaker config from KafKaWriterOptions
// (applying conservative defaults for zero values) and returns a breaker, or nil
// if the breaker is disabled. now seeds the first window start.
func newKafkaBreakerFromOptions(o KafKaWriterOptions, now time.Time) *kafkaBreaker {
	if o.BreakerDisabled {
		return nil
	}
	fr := o.BreakerFailRate
	if fr <= 0 {
		fr = defaultBreakerFailRate
	}
	ms := o.BreakerMinSamples
	if ms == 0 {
		ms = defaultBreakerMinSamples
	}
	win := o.BreakerWindow
	if win <= 0 {
		win = defaultBreakerWindow
	}
	cd := o.BreakerCooldown
	if cd <= 0 {
		cd = defaultBreakerCooldown
	}
	return newKafkaBreaker(breakerConfig{
		failRate:   fr,
		minSamples: ms,
		window:     win,
		cooldown:   cd,
	}, now)
}

// isOpen is the daemon's hot-path diversion gate (open → spill under Spill
// policy). A single atomic load.
func (b *kafkaBreaker) isOpen() bool { return b.stateVal.Load() == breakerOpen }

// stateCode returns the internal int state for Metrics export.
func (b *kafkaBreaker) stateCode() int32 { return b.stateVal.Load() }

// evaluate runs the state machine on the daemon's drain ticker. now is injected
// for deterministic tests. It is the only writer of stateVal (single goroutine).
func (b *kafkaBreaker) evaluate(now time.Time) {
	nowNS := now.UnixNano()
	switch b.stateVal.Load() {
	case breakerClosed:
		if nowNS-b.winStart.Load() < int64(b.window) {
			return // still accumulating this window
		}
		b.decideAndRoll(nowNS) // trip to open, or roll a fresh window
	case breakerOpen:
		if nowNS-b.openedAt.Load() >= int64(b.cooldown) {
			b.winStart.Store(nowNS)
			b.winSent.Store(0)
			b.winErr.Store(0)
			b.transition(breakerHalfOpen, nowNS)
		}
	case breakerHalfOpen:
		if nowNS-b.winStart.Load() < int64(b.window) {
			return // let the probe window accumulate
		}
		b.decideAndRoll(nowNS) // close on recovery, or reopen
	}
}

// decideAndRoll reads the just-elapsed window, transitions based on the error
// rate, and starts a fresh window. Called only from evaluate (single goroutine).
func (b *kafkaBreaker) decideAndRoll(nowNS int64) {
	sent := b.winSent.Load()
	err := b.winErr.Load()
	b.winStart.Store(nowNS)
	b.winSent.Store(0)
	b.winErr.Store(0)
	if sent < b.minSamples {
		return // too few samples to trust the rate; keep the current state
	}
	rate := float64(err) / float64(sent)
	cur := b.stateVal.Load()
	if rate >= b.failRate {
		if cur != breakerOpen {
			b.transition(breakerOpen, nowNS)
		}
	} else if cur != breakerClosed {
		b.transition(breakerClosed, nowNS)
	}
}

func (b *kafkaBreaker) transition(to int32, nowNS int64) {
	from := b.stateVal.Swap(to)
	if from == to {
		return
	}
	if to == breakerOpen {
		b.openedAt.Store(nowNS)
	}
	if b.onTransition != nil {
		b.onTransition(breakerStateName(from), breakerStateName(to))
	}
}
