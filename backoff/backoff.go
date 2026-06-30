// Package backoff generates retry delays with exponential growth and optional
// jitter, with an optional attempt cap and context-aware sleep.
//
// Jitter modes follow the well-known AWS retry shapes: None (pure exponential),
// Full (uniform in [0, exp]), Equal (exp/2 + uniform[0, exp/2]), and
// Decorrelated (next uniform in [base, last*3]). Jitter prevents the
// thundering-herd retry storms that pure exponential backoff produces when many
// clients fail in sync.
//
// Ad-tech uses: retrying a transient SSP/broker failure, backing off after a
// rate-limit (429), and re-establishing a dropped connection — all without
// hammering the upstream in lockstep.
package backoff

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// Jitter selects the randomization applied to each exponential delay.
type Jitter int

const (
	// JitterNone is pure exponential backoff (no jitter). Predictable but
	// synchronization-prone; only use in tests or single-client code.
	JitterNone Jitter = iota
	// JitterFull returns a uniform random value in [0, exp].
	JitterFull
	// JitterEqual returns exp/2 + uniform[0, exp/2] (centered on exp/2).
	JitterEqual
	// JitterDecorrelated keeps the next delay in [base, last*3] (AWS shape).
	JitterDecorrelated
)

// ErrMaxAttempts is returned by Wait when the attempt cap has been reached.
var ErrMaxAttempts = errors.New("backoff: max attempts reached")

// Backoff produces a sequence of retry delays. The zero value is NOT usable;
// construct with New. Safe for concurrent use (each call advances the shared
// counter — for per-call isolation, use separate Backoff instances).
type Backoff struct {
	mu          sync.Mutex
	base        time.Duration
	factor      float64
	max         time.Duration
	jitter      Jitter
	maxAttempts int // 0 = unlimited
	attempt     int
	raw         time.Duration // un-jittered exponential value (None/Full/Equal)
	last        time.Duration // last returned delay (Decorrelated)
}

// Option configures a Backoff.
type Option func(*Backoff)

// WithBase sets the initial delay (default 100ms).
func WithBase(d time.Duration) Option { return func(b *Backoff) { b.base = d } }

// WithFactor sets the exponential growth factor (default 2.0).
func WithFactor(f float64) Option { return func(b *Backoff) { b.factor = f } }

// WithMax caps any single delay (default 10s).
func WithMax(d time.Duration) Option { return func(b *Backoff) { b.max = d } }

// WithJitter selects the jitter mode (default JitterFull).
func WithJitter(j Jitter) Option { return func(b *Backoff) { b.jitter = j } }

// WithMaxAttempts caps the number of attempts; Next returns ok=false (and Wait
// returns ErrMaxAttempts) once exceeded. 0 = unlimited.
func WithMaxAttempts(n int) Option { return func(b *Backoff) { b.maxAttempts = n } }

// New builds a Backoff with the given options applied.
func New(opts ...Option) *Backoff {
	b := &Backoff{
		base:   100 * time.Millisecond,
		factor: 2.0,
		max:    10 * time.Second,
		jitter: JitterFull,
	}
	for _, opt := range opts {
		opt(b)
	}
	b.raw = b.base
	b.last = b.base
	return b
}

// Next returns the next delay and advances the counter, or ok=false when the
// attempt cap is reached. Reset restarts the sequence.
func (b *Backoff) Next() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxAttempts > 0 && b.attempt >= b.maxAttempts {
		return 0, false
	}
	b.attempt++
	d := b.computeLocked()
	return d, true
}

// computeLocked derives the current delay and advances internal state. The
// current exponential value is used to derive the jittered result, then advanced
// for the following call.
func (b *Backoff) computeLocked() time.Duration {
	switch b.jitter {
	case JitterDecorrelated:
		hi := b.last * 3
		if hi < b.base {
			hi = b.base
		}
		d := randRange(b.base, hi)
		if d > b.max {
			d = b.max
		}
		b.last = d
		return d
	case JitterFull:
		cur := b.raw
		b.advance()
		return randRange(0, cur)
	case JitterEqual:
		cur := b.raw
		b.advance()
		return cur/2 + randRange(0, cur/2)
	default: // JitterNone
		cur := b.raw
		b.advance()
		return cur
	}
}

// advance grows the un-jittered exponential value, capped at b.max.
func (b *Backoff) advance() {
	if b.raw >= b.max {
		b.raw = b.max
		return
	}
	nf := float64(b.raw) * b.factor
	if nf >= float64(b.max) || math.IsInf(nf, 1) {
		b.raw = b.max
		return
	}
	b.raw = time.Duration(nf)
}

// Attempt returns the number of Next/Wait calls since the last Reset.
func (b *Backoff) Attempt() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempt
}

// Reset restarts the delay sequence and attempt counter.
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempt = 0
	b.raw = b.base
	b.last = b.base
}

// Wait blocks for the next delay (respecting ctx) and returns nil to signal
// "retry now". It returns ErrMaxAttempts when the cap is reached (stop retrying)
// and ctx.Err() when the context is cancelled.
func (b *Backoff) Wait(ctx context.Context) error {
	d, ok := b.Next()
	if !ok {
		return ErrMaxAttempts
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// randRange returns a uniform random duration in [lo, hi]; hi when lo>=hi.
func randRange(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(rand.Int64N(int64(hi-lo+1)))
}
