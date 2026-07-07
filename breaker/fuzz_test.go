// This file is an internal fuzz test (package breaker, not breaker_test) so it
// can reuse newFakeBreaker and the unexported fakeClock to drive state
// transitions deterministically.
//
// FuzzBreakerExecute feeds arbitrary success/failure patterns (plus occasional
// fake-clock advances that simulate OpenDuration expiry and sliding-window
// ageing) into Breaker.Execute and asserts two invariants for every input:
//
//  1. Execute never panics, regardless of the outcome pattern, the breaker's
//     current state, or concurrent state transitions it triggers itself.
//  2. After each call the breaker is in a valid state (Closed/Open/HalfOpen)
//     and the lifetime metrics stay self-consistent:
//       Total == Success + Failures + rejected
//     where rejected is the number of calls that returned ErrCircuitOpen
//     without fn running (those bump Total but neither Success nor Failures).
//
// The fake clock keeps the test instantaneous (no real time.Sleep) so the fuzz
// loop stays tight, and it lets us observe the full Closed -> Open -> HalfOpen
// -> Closed cycle from the input stream alone.
package breaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fuzzOpts is a fixed option set tuned for fast tripping and recovery so the
// fuzzer exercises all three states within a short byte stream. MinRequests is
// small and OpenDuration short (advanced via the fake clock, never slept).
var fuzzOpts = BreakerOptions{
	Name:         "fuzz",
	MaxRequests:  2,
	Interval:     1 * time.Second,
	OpenDuration: 10 * time.Millisecond,
	FailRate:     0.5,
	MinRequests:  3,
}

// FuzzBreakerExecute fuzzes a stream of call outcomes derived from a byte slice.
// Each byte contributes: its low bit as the success/failure decision when the
// call actually runs, and every 7th byte also triggers a fake-clock advance
// (past OpenDuration, occasionally past the sliding window) so the Open ->
// HalfOpen -> Closed transitions are reachable from the input alone.
//
// Seed corpus covers the canonical paths: all-success, all-failure (trip),
// trip-then-recover, and an interleaved mix.
func FuzzBreakerExecute(f *testing.F) {
	// All successes: breaker stays Closed.
	f.Add([]byte{0x00, 0x02, 0x04, 0x06, 0x08, 0x0a})
	// All failures: trips to Open and then rejects.
	f.Add([]byte{0x01, 0x03, 0x05, 0x07, 0x09, 0x0b, 0x0d})
	// Trip (3 fails) then recover (clock advance + 2 successful probes).
	f.Add([]byte{0x01, 0x03, 0x05, 0xff, 0xff, 0x00, 0x02})
	// Interleaved mix with embedded clock advances.
	f.Add([]byte{0x01, 0x00, 0xff, 0x01, 0x01, 0xff, 0x00, 0x02, 0x01, 0x00})

	failErr := errors.New("fuzz failure")

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		b, clock := newFakeBreaker(fuzzOpts)

		var (
			rejected uint64
			prevOpen bool // track whether we have ever observed Open, for coverage only
		)

		for i, in := range data {
			// Every 7th byte (and any byte with the high bit set) advances the
			// fake clock past OpenDuration so an Open breaker can transition to
			// HalfOpen and recover. Occasionally age the whole window too.
			if i%7 == 0 || in&0x80 != 0 {
				clock.add(fuzzOpts.OpenDuration * 2)
			}
			if i%11 == 0 {
				clock.add(fuzzOpts.Interval + 100*time.Millisecond)
			}

			shouldFail := in&0x01 == 1
			v, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
				if shouldFail {
					return 0, failErr
				}
				return int(in), nil
			})

			// Invariant 1: state is always one of the three defined states.
			state := b.State()
			switch state {
			case StateClosed, StateOpen, StateHalfOpen:
			default:
				t.Fatalf("byte %d: invalid state %d", i, state)
			}

			if errors.Is(err, ErrCircuitOpen) {
				// Rejected before fn ran: value is the zero value, and the call
				// is counted in Total but not in Success or Failures.
				if v != 0 {
					t.Fatalf("byte %d: rejected call returned non-zero value %d", i, v)
				}
				rejected++
			} else if shouldFail {
				if !errors.Is(err, failErr) {
					t.Fatalf("byte %d: fn failed but err=%v want failErr", i, err)
				}
			} else {
				if err != nil {
					t.Fatalf("byte %d: unexpected err=%v", i, err)
				}
				if v != int(in) {
					t.Fatalf("byte %d: value=%d want %d", i, v, int(in))
				}
			}

			// Invariant 2: lifetime metrics are self-consistent after every call.
			m := b.Metrics()
			if m.State != state {
				t.Fatalf("byte %d: Metrics.State=%s want %s", i, m.State, state)
			}
			if m.Total != m.Success+m.Failures+rejected {
				t.Fatalf("byte %d: Total=%d != Success=%d+Failures=%d+rejected=%d",
					i, m.Total, m.Success, m.Failures, rejected)
			}
			if m.State == StateOpen {
				prevOpen = true
			}
			// ConsecutiveFail must be 0 whenever the most recent outcome was a
			// success that actually ran; on a success path it is always reset.
			if !shouldFail && !errors.Is(err, ErrCircuitOpen) && m.ConsecutiveFail != 0 {
				t.Fatalf("byte %d: success did not reset ConsecutiveFail=%d",
					i, m.ConsecutiveFail)
			}
		}

		_ = prevOpen
	})
}
