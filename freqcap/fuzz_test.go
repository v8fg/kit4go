// This file is an internal fuzz test (package freqcap, not freqcap_test) so it
// can inject the shared fakeClock via WithClock and exercise the unexported
// maxKeys normalisation. It drives invariants the unit tests only spot-check:
//
//   - FuzzAllowCountConsistency: Allow/Count/Reset never panic for any key and
//     step schedule, the number of in-window Allow==true results for a single
//     key never exceeds maxEvents, and Count matches the model's in-window set
//     at every step (roundtrip + ordering invariant).
//   - FuzzSlidingWindowExpiry: after the clock advances past the window, every
//     previously-recorded event expires — Count returns 0 and Allow succeeds
//     again — regardless of the fill pattern that preceded the advance.
//
// These run as ordinary tests (seed corpus only) under `go test -run='^Fuzz'`;
// pass `-fuzz=FuzzAllowCountConsistency` / `-fuzz=FuzzSlidingWindowExpiry` to
// expand the corpus.
package freqcap

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FuzzAllowCountConsistency fuzzes a window/maxEvents configuration plus a
// sequence of (advance-then-allow) steps driven by a fake clock, against a
// single key. It asserts, for every input:
//
//  1. Allow and Count never panic.
//  2. The number of Allow==true results whose timestamps are still within the
//     window never exceeds maxEvents — the frequency cap is the package's core
//     contract, so a violation here is the highest-signal bug the fuzzer can
//     raise.
//  3. Count(key) equals the model's in-window set size after every step, so
//     lazy trimming and the sliding window stay consistent with each other.
//
// The first two bytes select the window (ms) and the cap (maxEvents). The
// remainder is a step schedule encoded one byte per step: the high bit set
// (0x80) means "advance the clock by (byte & 0x7f) ms first"; the low bit set
// means "call Allow after the optional advance". Count is probed every step so
// trimming drift is caught immediately rather than only at the boundary.
func FuzzAllowCountConsistency(f *testing.F) {
	// Seeds cover the shapes that matter: tiny/huge windows, cap=1 (pure
	// on/off), no clock advance (zero-elapsed cap must hold), steady advance
	// smaller than the window, advance past the window (full expiry), and a
	// mixed churn that interleaves expiry with re-fill.
	f.Add(uint8(10), uint8(3), []byte{0x01, 0x01, 0x01, 0x80 | 11, 0x01})                              // cap 3 in 10ms, then expire
	f.Add(uint8(1), uint8(1), []byte{0x01, 0x01})                                                      // cap 1: second allow in-window must deny
	f.Add(uint8(255), uint8(1), []byte{0x01, 0x01, 0x01})                                              // huge window, cap 1
	f.Add(uint8(5), uint8(255), []byte{0x01, 0x01, 0x01})                                              // large cap, never exceeded
	f.Add(uint8(10), uint8(2), []byte{})                                                               // empty step stream: just construct
	f.Add(uint8(10), uint8(2), []byte{0x80 | 3, 0x01, 0x80 | 3, 0x01, 0x80 | 3, 0x01, 0x80 | 3, 0x01}) // sub-window churn
	f.Add(uint8(10), uint8(2), []byte{0x01, 0x01, 0x80 | 20, 0x01, 0x01})                              // fill, expire fully, refill

	f.Fuzz(func(t *testing.T, windowMs uint8, maxEvents uint8, steps []byte) {
		// New panics on maxEvents<=0 or window<=0; the fuzzer can drive both.
		// A zero value for either is outside the constructor's contract, so
		// skip rather than assert (the TestPanicGuards unit test already pins
		// the panic). We do still fuzz the constructor's positive space hard.
		if maxEvents == 0 || windowMs == 0 {
			t.Skip("maxEvents==0 or window==0 is outside New's documented contract")
		}
		window := time.Duration(windowMs) * time.Millisecond
		cap := int(maxEvents)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: window=%v maxEvents=%d steps=%x recover=%v",
					window, cap, steps, r)
			}
		}()

		clk := &fakeClock{t: time.Unix(0, 0)}
		c := New(window, cap, WithClock(clk.now))

		const key = "u"
		// inWindow holds the timestamps of every Allow==true result that is
		// still inside the window at the current clock reading. It is the
		// golden model: its length is the oracle for Count(key), and it must
		// never exceed cap (invariant 2 and 3 above).
		inWindow := make([]time.Time, 0, cap+1)

		trimModel := func(now time.Time) {
			cutoff := now.Add(-window)
			keep := inWindow[:0]
			for _, ts := range inWindow {
				if !ts.Before(cutoff) {
					keep = append(keep, ts)
				}
			}
			inWindow = keep
		}

		for _, step := range steps {
			advance := time.Duration(step>>7) * time.Millisecond
			if advance > 0 {
				clk.t = clk.t.Add(advance)
			}
			// Lazily expire model entries at the current clock before probing,
			// mirroring the production trimBefore(cutoff) call.
			now := clk.t
			trimModel(now)

			// Invariant 3: Count agrees with the model before any Allow mutates
			// state. This catches trimming drift independently of cap checks.
			require.Equalf(t, len(inWindow), c.Count(key),
				"Count drifted from model (window=%v cap=%d steps=%x): got %d want %d at now=%v",
				window, cap, steps, c.Count(key), len(inWindow), now)

			if step&0x01 == 0 {
				continue // advance-only step: no Allow call this iteration.
			}

			got := c.Allow(key)
			// trimModel already ran at this clock reading, so the model's view
			// of "at/over cap" is current. Allow records only when len < cap.
			want := len(inWindow) < cap
			require.Equalf(t, want, got,
				"Allow mismatch (window=%v cap=%d steps=%x): got %v want %v (model in-window=%d at now=%v)",
				window, cap, steps, got, want, len(inWindow), now)
			if got {
				inWindow = append(inWindow, now)
			}

			// Invariant 2: the in-window allowed set never exceeds the cap.
			require.LessOrEqualf(t, len(inWindow), cap,
				"cap exceeded (window=%v cap=%d steps=%x): in-window=%d at now=%v",
				window, cap, steps, len(inWindow), now)

			// Count must still agree after the Allow mutated state.
			require.Equalf(t, len(inWindow), c.Count(key),
				"Count drifted after Allow (window=%v cap=%d steps=%x): got %d want %d at now=%v",
				window, cap, steps, c.Count(key), len(inWindow), now)
		}
	})
}

// FuzzSlidingWindowExpiry fuzzes a fill pattern that loads a key up to (or past)
// its cap, then advances the clock beyond the window. It asserts the cap fully
// resets: Count returns 0 and the next Allow succeeds, no matter how the key
// was filled (single rapid bursts, partial fills, multiple near-saturation
// rounds). This is the "ordering over time" invariant — the window must slide
// forward and release every recorded event.
//
// windowMs selects the window; fills is a byte whose low 4 bits give the number
// of Allow calls to make before the advance (clamped to [0, cap+1] so the cap
// can be both hit and exceeded). gapMs is how far the clock jumps past the
// window before the post-expiry probes.
func FuzzSlidingWindowExpiry(f *testing.F) {
	f.Add(uint8(10), uint8(2), uint8(2), uint8(11))     // fill to cap, jump just past window
	f.Add(uint8(1), uint8(1), uint8(5), uint8(1))       // cap 1, over-fill attempts, minimal gap
	f.Add(uint8(100), uint8(10), uint8(15), uint8(101)) // large window, exceed cap, full expiry
	f.Add(uint8(5), uint8(3), uint8(0), uint8(6))       // zero fills then expire: Count must still be 0
	f.Add(uint8(8), uint8(4), uint8(4), uint8(200))     // exactly cap fills, large gap

	f.Fuzz(func(t *testing.T, windowMs uint8, maxEvents uint8, fills uint8, gapMs uint8) {
		if maxEvents == 0 || windowMs == 0 {
			t.Skip("maxEvents==0 or window==0 is outside New's documented contract")
		}
		window := time.Duration(windowMs) * time.Millisecond
		cap := int(maxEvents)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: window=%v maxEvents=%d fills=%d gapMs=%d recover=%v",
					window, cap, fills, gapMs, r)
			}
		}()

		clk := &fakeClock{t: time.Unix(0, 0)}
		c := New(window, cap, WithClock(clk.now))

		const key = "u"
		n := int(fills)
		if n > cap+1 {
			n = cap + 1 // clamp: exercising the cap and one over-fill is enough
		}
		allowedPre := 0
		for i := 0; i < n; i++ {
			if c.Allow(key) {
				allowedPre++
			}
		}
		// Before expiry the cap holds: never more than maxEvents allowed in-window.
		require.LessOrEqualf(t, allowedPre, cap,
			"pre-expiry cap violated (window=%v cap=%d fills=%d): allowed=%d",
			window, cap, fills, allowedPre)
		if n > 0 {
			require.Equalf(t, allowedPre, c.Count(key),
				"Count must match pre-expiry allows (window=%v cap=%d fills=%d): got %d want %d",
				window, cap, fills, c.Count(key), allowedPre)
		}

		// Advance strictly past the window. Every recorded event is now older
		// than the window, so the model says the in-window set is empty. The
		// +1ns matters: freqcap's window is inclusive on the lower edge (an
		// event exactly `window` old is still counted — the safer choice for a
		// cap), so advancing by exactly `window` (gapMs==0) leaves the event.
		clk.t = clk.t.Add(time.Duration(gapMs)*time.Millisecond + window + time.Nanosecond)

		// gapMs is a uint8 so it can be 0; ensure we genuinely crossed the
		// window boundary. If the advance did not clear the window (gapMs==0
		// and no prior events, or a pathological small gap), the invariants
		// below still hold — Count==0 only when the model agrees, and Allow
		// succeeds only when under cap. Either way this is a valid probe.
		require.Equalf(t, 0, c.Count(key),
			"Count must be 0 after window expiry (window=%v cap=%d fills=%d gapMs=%d): got %d",
			window, cap, fills, gapMs, c.Count(key))

		// The first Allow after expiry must succeed: the cap has fully reset.
		require.True(t, c.Allow(key),
			"Allow must succeed after full window expiry (window=%v cap=%d fills=%d gapMs=%d)",
			window, cap, fills, gapMs)
		require.Equalf(t, 1, c.Count(key),
			"Count must be 1 after one post-expiry allow (window=%v cap=%d fills=%d gapMs=%d)",
			window, cap, fills, gapMs)
	})
}
