package breaker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BreakerMetrics is a best-effort snapshot of a breaker's lifetime counters and
// current state. It is safe to read concurrently; the fields are populated
// atomically but are not mutually consistent across fields (State may be read a
// few nanoseconds apart from Total). Treat it as an observation, not a source
// of truth for transactional decisions.
type BreakerMetrics struct {
	// State is the breaker's state at the instant of the snapshot.
	State BreakerState
	// Total is the cumulative number of Execute calls (including ones rejected
	// with ErrCircuitOpen since they were attempted). It never decreases.
	Total uint64
	// Success is the cumulative number of calls whose fn returned nil.
	Success uint64
	// Failures is the cumulative number of calls whose fn returned a non-nil
	// error (ctx cancellation counts as a failure).
	Failures uint64
	// ConsecutiveFail is the current run of failures since the last success.
	// Reset to 0 on the next success.
	ConsecutiveFail uint32
}

// Breaker is a generic circuit breaker parameterised by the value type returned
// by the wrapped operation. Construct one with [NewBreaker].
//
// Concurrency model: state, expiry, half-open counters and lifetime metrics
// are all stored in atomics for lock-free fast-path reads. The per-instance
// mutex b.mu guards only (a) writes to the sliding-window buckets and their
// running sums, and (b) state-transition critical sections, so a healthy
// Closed breaker pays for one lock/unlock per call plus atomics.
//
// The sliding window is a per-second ring buffer (one int bucket per second of
// Interval) identical in design to log4go.RateAlerter: advance() rolls the
// ring forward and subtracts expired buckets from the running sum, making
// record O(1) amortized with no per-call allocation.
type Breaker[T any] struct {
	opts BreakerOptions

	// state holds a BreakerState value; read/written atomically.
	state atomic.Int32
	// mu guards the sliding-window fields below and serialises state
	// transitions. Held only briefly.
	mu sync.Mutex
	// expiry is the unix-nano timestamp at/after which StateOpen transitions
	// to StateHalfOpen on the next call. Read atomically; written under mu.
	expiry atomic.Int64

	// Sliding window (per-second ring), guarded by mu. counts holds successes,
	// fails holds failures, one slot per second of Interval. base is the unix
	// second of the newest bucket advanced to; sumTotal/sumFail are the running
	// sums of all live buckets.
	counts   []int
	fails    []int
	base     int64
	sumTotal int
	sumFail  int

	// Half-open probe tracking. halfOpenCount is the number of probe slots
	// taken; halfOpenSuccess the number that have succeeded. Both atomic so
	// probes can admit/complete without taking mu. halfOpenGen is the half-open
	// epoch (incremented on each Open→HalfOpen transition); a probe's success or
	// failure credits/trips only if its captured epoch matches the current one, so
	// a probe that outlasts a trip+cooldown cannot bleed into the next epoch.
	halfOpenSuccess atomic.Int32
	halfOpenCount   atomic.Int32
	halfOpenGen     atomic.Int32

	// Lifetime metrics, all atomic.
	total      atomic.Uint64
	success    atomic.Uint64
	failures   atomic.Uint64
	consecFail atomic.Uint32

	// onEvent, when non-nil, is invoked for every notable outcome/transition
	// (success, failure, trip, recover, reject). It is set via SetOnEvent and
	// read with an atomic load on the hot path so the default (nil) costs
	// nothing. Stored as an atomic.Pointer so SetOnEvent can be called before or
	// after traffic starts without a separate lock.
	onEvent atomic.Pointer[func(BreakerEvent)]

	// now is the clock seam. It defaults to time.Now so production behaviour is
	// unchanged; tests substitute a fake clock to make time-window and state
	// transitions deterministic instead of relying on time.Sleep. Read-only on
	// the hot path (never reassigned after construction), so it needs no lock.
	now func() time.Time
}

// SetOnEvent installs a hook invoked for every notable outcome and state
// transition. fn receives a [BreakerEvent] describing what happened and the
// state at that instant. Pass nil to disable a previously-installed hook.
//
// The hook is intended for metrics push (counters/histograms) and observability
// — it must be cheap and must not block, since it fires on the Execute hot path
// (under the same goroutine as the caller). It is invoked after all state
// mutations for the event have settled, so reads of State/Metrics inside fn see
// the post-event view. SetOnEvent is safe for concurrent use with Execute, but
// callers should typically install the hook once at construction time before
// traffic begins to avoid ordering surprises.
func (b *Breaker[T]) SetOnEvent(fn func(evt BreakerEvent)) {
	if fn == nil {
		b.onEvent.Store(nil)
		return
	}
	f := fn // copy to heap
	b.onEvent.Store(&f)
}

// fireEvent is the single chokepoint for hook dispatch. It is inlined by the
// compiler; when onEvent is nil (the default) the entire call collapses to a
// single nil compare and no further work, so the no-hook path is zero-overhead.
func (b *Breaker[T]) fireEvent(name string) {
	if p := b.onEvent.Load(); p != nil {
		(*p)(BreakerEvent{Name: name, State: b.State()})
	}
}

// NewBreaker builds a breaker for the given value type T. opts is normalised
// with withDefaults, so the zero BreakerOptions yields a breaker with all
// defaults. Returns a *Breaker ready to use.
func NewBreaker[T any](opts BreakerOptions) *Breaker[T] {
	opts = opts.withDefaults()
	secs := int(opts.Interval.Seconds())
	if secs < 1 {
		secs = 1
	}
	b := &Breaker[T]{
		opts:   opts,
		counts: make([]int, secs),
		fails:  make([]int, secs),
		base:   time.Now().Unix(),
		now:    time.Now,
	}
	b.state.Store(int32(StateClosed))
	return b
}

// State returns the breaker's current state. It is a lock-free atomic read.
func (b *Breaker[T]) State() BreakerState {
	return BreakerState(b.state.Load())
}

// Metrics returns a snapshot of the lifetime counters and current state. See
// BreakerMetrics for the consistency caveat.
func (b *Breaker[T]) Metrics() BreakerMetrics {
	return BreakerMetrics{
		State:           b.State(),
		Total:           b.total.Load(),
		Success:         b.success.Load(),
		Failures:        b.failures.Load(),
		ConsecutiveFail: b.consecFail.Load(),
	}
}

// Execute runs fn under the breaker's protection.
//
// If the breaker is Open and its OpenDuration has not elapsed, or HalfOpen with
// all MaxRequests probe slots taken, Execute returns the zero value of T and
// ErrCircuitOpen without invoking fn. Otherwise fn runs; its result and error
// are propagated unchanged, and the outcome is recorded:
//
//   - Closed: the call is added to the sliding window; if the in-window failure
//     rate then reaches FailRate over >= MinRequests calls, the breaker trips
//     to Open.
//   - HalfOpen: a successful probe increments the half-open success counter,
//     and once it reaches MaxRequests the breaker returns to Closed; a failed
//     probe immediately trips back to Open.
//
// If ctx is already cancelled, Execute counts that as a failure but still
// records it (so a flaky caller cancelling on timeout cannot hide failures
// from the breaker).
//
// Panics from fn are propagated raw to the caller (fn runs on the caller's
// goroutine, so the raw-panic contract of the kit-callback convention holds).
// The one exception is bookkeeping: in StateHalfOpen a panic escapes before
// recordSuccess/recordFailure can release the probe slot, so a half-open slot
// is leaked on every panicking probe and after MaxRequests such panics the
// breaker is wedged in HalfOpen (no slot ever frees, so it can neither recover
// nor re-trip). To stay do-no-harm, a panic that occurs while the caller holds
// a HalfOpen probe slot is treated as a failed probe — recordFailure runs
// (which trips to Open and resets halfOpenCount, the self-healing path) — and
// then the original panic is re-thrown. Non-HalfOpen states are untouched: the
// raw panic propagates with no recover, exactly as before.
func (b *Breaker[T]) Execute(ctx context.Context, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T

	// A nil ctx is tolerated (treated as Background) so a careless caller can't
	// crash the breaker; fn still receives the nil ctx and must itself be
	// nil-safe if it intends to use it.
	if ctx == nil {
		ctx = context.Background()
	}

	// Admit-or-reject. beforeCall returns ErrCircuitOpen when the call must not
	// proceed; otherwise it returns nil (and may have transitioned state).
	if err := b.beforeCall(); err != nil {
		// A rejected call is still a call attempted; count it as a lifetime
		// total but not as success/failure (fn never ran).
		b.total.Add(1)
		b.fireEvent("reject")
		return zero, err
	}

	// Capture the half-open epoch at admission for gen-gating record calls.
	probeGen := b.halfOpenGen.Load()

	// Respect a pre-cancelled context: count it as a failure (callers that
	// cancel on timeout should not be able to mask downstream trouble).
	if err := ctx.Err(); err != nil {
		b.total.Add(1)
		b.recordFailure(probeGen)
		return zero, err
	}

	// A successful admission in StateHalfOpen took a probe slot. That slot is
	// normally released by recordSuccess/recordFailure, but a panicking fn
	// escapes before either runs — leaking the slot and (after MaxRequests such
	// panics) wedging the breaker in HalfOpen. Only HalfOpen needs the safety
	// net: in Closed there is no per-call slot to leak (a panic simply unwinds
	// to the caller, window accounting skipped, which is the pre-existing
	// behaviour), and in Open we never reach here.
	halfOpenSlot := BreakerState(b.state.Load()) == StateHalfOpen

	b.total.Add(1)
	if halfOpenSlot {
		// Run fn under a recover whose only job is to settle the half-open slot
		// accounting before re-throwing, preserving the raw-panic contract. On a
		// panic the deferred recover calls recordFailure (which trips to Open and
		// resets halfOpenCount — the self-healing path) and then re-panics with
		// the original value, so the caller observes the panic exactly as if the
		// recover were not there. On the normal path the closure returns with
		// valid v, err and the surrounding code records the outcome.
		v, err := func() (vv T, e error) {
			defer func() {
				if r := recover(); r != nil {
					b.recordFailure(probeGen)
					panic(r) // re-throw: raw-panic contract preserved.
				}
			}()
			return fn(ctx)
		}()
		if err != nil {
			b.recordFailure(probeGen)
			return v, err
		}
		b.recordSuccess(probeGen)
		return v, nil
	}

	v, err := fn(ctx)
	if err != nil {
		b.recordFailure(probeGen)
		return v, err
	}
	b.recordSuccess(probeGen)
	return v, nil
}

// beforeCall enforces the state-dependent admission policy. It returns
// ErrCircuitOpen when the call must not proceed, and nil otherwise. Open→HalfOpen
// and the HalfOpen probe-slot accounting happen here.
func (b *Breaker[T]) beforeCall() error {
	switch BreakerState(b.state.Load()) {
	case StateClosed:
		return nil
	case StateOpen:
		// Transition to HalfOpen once the cooldown has elapsed. Compare
		// atomically; if it hasn't, reject without taking the lock.
		if b.now().UnixNano() < b.expiry.Load() {
			return ErrCircuitOpen
		}
		return b.toHalfOpenOrReject()
	case StateHalfOpen:
		// Admit up to MaxRequests concurrent probes. Extra callers are rejected
		// so the probe set stays bounded.
		if uint32(b.halfOpenCount.Load()) >= b.opts.MaxRequests {
			return ErrCircuitOpen
		}
		if b.halfOpenCount.Add(1) > int32(b.opts.MaxRequests) {
			// Lost the admission race: undo and reject.
			b.halfOpenCount.Add(-1)
			return ErrCircuitOpen
		}
		return nil
	default:
		return nil
	}
}

// toHalfOpenOrReject is the Open→HalfOpen transition under contention: the
// first caller that observes the elapsed expiry flips the state to HalfOpen and
// takes the first probe slot; losers see HalfOpen and go through the normal
// half-open admission path.
func (b *Breaker[T]) toHalfOpenOrReject() error {
	b.mu.Lock()
	// Re-check under lock: another goroutine may have transitioned already, or
	// the clock may read differently now.
	if BreakerState(b.state.Load()) != StateOpen {
		b.mu.Unlock()
		// Already moved (likely HalfOpen); re-evaluate via beforeCall's path.
		if BreakerState(b.state.Load()) == StateHalfOpen {
			if b.halfOpenCount.Add(1) <= int32(b.opts.MaxRequests) {
				return nil
			}
			b.halfOpenCount.Add(-1)
		}
		return ErrCircuitOpen
	}
	if b.now().UnixNano() < b.expiry.Load() {
		b.mu.Unlock()
		return ErrCircuitOpen
	}
	// Flip to HalfOpen and arm the probe counters.
	b.state.Store(int32(StateHalfOpen))
	b.halfOpenGen.Add(1) // new epoch — stale probes from the previous HalfOpen can't credit this one
	b.halfOpenSuccess.Store(0)
	b.halfOpenCount.Store(1) // current call takes the first slot
	b.mu.Unlock()
	return nil
}

// recordSuccess updates counters for a successful call and may transition
// HalfOpen→Closed.
func (b *Breaker[T]) recordSuccess(gen int32) {
	b.success.Add(1)
	b.consecFail.Store(0)

	switch BreakerState(b.state.Load()) {
	case StateHalfOpen:
		// Credit only if the probe belongs to the current half-open epoch: a
		// probe admitted in a previous epoch that outlasted a trip+cooldown
		// must not bleed its success into this epoch's recovery count.
		if b.halfOpenGen.Load() == gen {
			if uint32(b.halfOpenSuccess.Add(1)) >= b.opts.MaxRequests {
				b.toClosed()
			}
		}
	case StateClosed:
		b.recordWindow(true)
	}
	b.fireEvent("success")
}

// recordFailure updates counters for a failed call and may trip the breaker.
func (b *Breaker[T]) recordFailure(gen int32) {
	b.failures.Add(1)
	b.consecFail.Add(1)

	switch BreakerState(b.state.Load()) {
	case StateHalfOpen:
		// Trip only if the probe belongs to the current epoch — a stale failure
		// from a previous half-open must not trip this epoch's fresh probes.
		if b.halfOpenGen.Load() == gen {
			b.toOpen()
		}
	case StateClosed:
		b.recordWindow(false)
		// Re-read state in case another goroutine tripped while we held the
		// window lock; if still Closed, evaluate the trip condition ourselves.
		if BreakerState(b.state.Load()) == StateClosed {
			b.maybeTrip()
		}
	}
	b.fireEvent("failure")
}

// recordWindow advances the sliding window to the current second and records
// one call in it. success selects the success vs failure bucket. Caller holds
// b.mu. Only the matching bucket (counts for every call, fails for failures
// only) is bumped, and the running sums are updated to match.
func (b *Breaker[T]) recordWindow(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Read the second UNDER the lock: a value read before acquiring the lock can
	// be older than base (advanced by a concurrent caller while we waited), which
	// would otherwise trip advance's backward path and silently drop failure
	// counts — the breaker could then fail to trip.
	sec := b.now().Unix()
	b.advance(sec)
	if sec < b.base {
		sec = b.base // wall clock regressed: charge the current bucket
	}
	n := int64(len(b.counts))
	idx := sec % n
	b.counts[idx]++
	b.sumTotal++
	if !success {
		b.fails[idx]++
		b.sumFail++
	}
}

// maybeTrip evaluates the Closed→Open condition under b.mu and trips if met.
// Reads the live sliding-window sums; called only from recordFailure.
func (b *Breaker[T]) maybeTrip() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if BreakerState(b.state.Load()) != StateClosed {
		return
	}
	sec := b.now().Unix()
	b.advance(sec)
	if uint32(b.sumTotal) < b.opts.MinRequests {
		return
	}
	// sumFail/sumTotal >= FailRate. FailRate <= 0 trips as soon as MinRequests
	// failures land; FailRate > 1 never trips.
	if b.opts.FailRate <= 0 {
		if b.sumFail > 0 {
			b.tripLocked()
		}
		return
	}
	if float64(b.sumFail)/float64(b.sumTotal) >= b.opts.FailRate {
		b.tripLocked()
	}
}

// tripLocked moves the breaker from Closed to Open. Caller holds b.mu.
func (b *Breaker[T]) tripLocked() {
	b.state.Store(int32(StateOpen))
	b.expiry.Store(b.now().Add(b.opts.OpenDuration).UnixNano())
	b.mu.Unlock()
	b.fireEvent("trip")
	b.mu.Lock()
}

// toOpen moves the breaker to Open from any state. Resets the half-open
// counters so a future HalfOpen phase starts clean.
func (b *Breaker[T]) toOpen() {
	b.mu.Lock()
	b.state.Store(int32(StateOpen))
	b.expiry.Store(b.now().Add(b.opts.OpenDuration).UnixNano())
	b.halfOpenSuccess.Store(0)
	b.halfOpenCount.Store(0)
	b.mu.Unlock()
	b.fireEvent("trip")
}

// toClosed moves the breaker from HalfOpen back to Closed and resets the
// sliding window so the freshly-recovered breaker is not re-tripped by stale
// failures from before the outage.
func (b *Breaker[T]) toClosed() {
	b.mu.Lock()
	if BreakerState(b.state.Load()) != StateHalfOpen {
		b.mu.Unlock()
		return
	}
	for i := range b.counts {
		b.counts[i] = 0
		b.fails[i] = 0
	}
	b.sumTotal = 0
	b.sumFail = 0
	b.base = b.now().Unix()
	b.halfOpenSuccess.Store(0)
	b.halfOpenCount.Store(0)
	b.state.Store(int32(StateClosed))
	b.mu.Unlock()
	b.fireEvent("recover")
}

// advance rolls the bucket ring forward to sec, zeroing buckets that have
// fallen out of the window and subtracting them from sumTotal/sumFail. After it
// returns, the bucket at index sec%n is cleared and ready for a fresh count.
// This is the same algorithm as log4go.RateAlerter.advance, applied to two
// rings in lockstep. Caller holds b.mu.
func (b *Breaker[T]) advance(sec int64) {
	n := int64(len(b.counts))
	if sec <= b.base {
		// Stale read or wall-clock regression (NTP). Do NOT clear: destroying
		// live failure counts on a backward timestamp could make the breaker fail
		// to trip. The caller clamps its write to base, so leave the window alone.
		return
	}
	if sec-b.base >= n {
		// A full window (or more) has elapsed: every bucket is expired.
		for i := range b.counts {
			b.sumTotal -= b.counts[i]
			b.sumFail -= b.fails[i]
			b.counts[i] = 0
			b.fails[i] = 0
		}
		b.base = sec
		return
	}
	for b.base < sec {
		b.base++
		i := b.base % n
		b.sumTotal -= b.counts[i]
		b.sumFail -= b.fails[i]
		b.counts[i] = 0
		b.fails[i] = 0
	}
}
