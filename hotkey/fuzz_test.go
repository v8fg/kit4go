// This file is an internal fuzz test (package hotkey, not hotkey_test) so it can
// inject the shared fakeClock via WithClock and reach the unexported maxKeys
// field. It drives invariants the unit tests only spot-check:
//
//   - FuzzTouchTopConsistency: Touch/Count/Top/Reset never panic for any key and
//     step schedule; Count(key) matches a golden model's in-window hit set at
//     every step; Top() is sorted by count descending, never longer than topK,
//     and reports no zero-count key (roundtrip + ordering invariant).
//   - FuzzMaxKeysEviction: when WithMaxKeys>0 the tracked key count never exceeds
//     the cap, regardless of how many distinct keys are touched — the cap is the
//     package's signature contract (shared with freqcap).
//
// These run as ordinary tests (seed corpus only) under `go test -run='^Fuzz'`;
// pass `-fuzz=FuzzTouchTopConsistency` / `-fuzz=FuzzMaxKeysEviction` to expand
// the corpus.
package hotkey

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FuzzTouchTopConsistency fuzzes a window/topK configuration plus a sequence of
// (advance-then-touch) steps driven by a fake clock, against a single key. It
// asserts, for every input:
//
//  1. Touch, Count, Top and Reset never panic.
//  2. Count(key) equals the model's in-window hit set size after every step, so
//     lazy trimBefore trimming and the sliding window stay consistent — this is
//     the package's core roundtrip invariant.
//  3. After a Top() call the result is sorted by count descending, contains no
//     more than topK entries, and every entry has Count > 0 (zero-hit keys are
//     excluded). The reported count for the tracked key matches Count(key),
//     pinning the ordering contract.
//
// The first two bytes select the window (ms) and topK. The remainder is a step
// schedule encoded one byte per step: the high bit set (0x80) means "advance the
// clock by (byte & 0x7f) ms first"; the low bit set means "call Touch after the
// optional advance". Count is probed every step so trimming drift is caught
// immediately rather than only at the boundary.
func FuzzTouchTopConsistency(f *testing.F) {
	// Seeds cover the shapes that matter: tiny/huge windows, topK=1 (purest
	// ordering stress), no clock advance (zero-elapsed counts must accumulate),
	// steady advance smaller than the window, advance past the window (full
	// expiry), and a mixed churn that interleaves expiry with re-touching.
	f.Add(uint8(10), uint8(3), []byte{0x01, 0x01, 0x01, 0x80 | 11, 0x01})                              // touch 3x in 10ms, then expire
	f.Add(uint8(1), uint8(1), []byte{0x01, 0x01})                                                      // topK 1: second touch must keep ordering
	f.Add(uint8(255), uint8(1), []byte{0x01, 0x01, 0x01})                                              // huge window, topK 1
	f.Add(uint8(5), uint8(255), []byte{0x01, 0x01, 0x01})                                              // large topK, never truncated
	f.Add(uint8(10), uint8(2), []byte{})                                                               // empty step stream: just construct
	f.Add(uint8(10), uint8(2), []byte{0x80 | 3, 0x01, 0x80 | 3, 0x01, 0x80 | 3, 0x01, 0x80 | 3, 0x01}) // sub-window churn
	f.Add(uint8(10), uint8(2), []byte{0x01, 0x01, 0x80 | 20, 0x01, 0x01})                              // touch, expire fully, re-touch
	f.Add(uint8(0), uint8(2), []byte{0x01})                                                            // window 0: out of contract, must skip
	f.Add(uint8(10), uint8(0), []byte{0x01})                                                           // topK 0: out of contract, must skip

	f.Fuzz(func(t *testing.T, windowMs uint8, topK uint8, steps []byte) {
		// New panics on topK<=0 or window<=0; the fuzzer can drive both. A zero
		// value for either is outside the constructor's contract, so skip rather
		// than assert (the TestPanicGuards unit test already pins the panic). We
		// still fuzz the constructor's positive space hard.
		if windowMs == 0 || topK == 0 {
			t.Skip("windowMs==0 or topK==0 is outside New's documented contract")
		}
		window := time.Duration(windowMs) * time.Millisecond
		k := int(topK)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: window=%v topK=%d steps=%x recover=%v",
					window, k, steps, r)
			}
		}()

		clk := &fakeClock{t: time.Unix(0, 0)}
		d := New(window, k, WithClock(clk.now))

		const key = "u"
		// inWindow holds the timestamps of every Touch call that is still inside
		// the window at the current clock reading. It is the golden model: its
		// length is the oracle for Count(key) (invariant 2).
		inWindow := make([]time.Time, 0, 16)

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

			if step&0x01 == 0 {
				// Advance-only step: still probe Count for trimming drift before
				// any mutation, then continue.
				require.Equalf(t, len(inWindow), d.Count(key),
					"Count drifted from model (window=%v topK=%d steps=%x): got %d want %d at now=%v",
					window, k, steps, d.Count(key), len(inWindow), now)
				continue
			}

			d.Touch(key)
			inWindow = append(inWindow, now)

			// Invariant 2: Count agrees with the model after the Touch mutated
			// state. This catches trimming drift immediately.
			require.Equalf(t, len(inWindow), d.Count(key),
				"Count drifted after Touch (window=%v topK=%d steps=%x): got %d want %d at now=%v",
				window, k, steps, d.Count(key), len(inWindow), now)
		}

		// Invariant 3: ordering of Top(). With a single tracked key the result
		// is either empty (key expired / never touched) or one entry whose count
		// equals Count(key). This pins the sort-descending, topK-bounded,
		// no-zero-count contract on every input.
		top := d.Top()
		require.LessOrEqualf(t, len(top), k,
			"Top must be bounded by topK (window=%v topK=%d steps=%x): got %d",
			window, k, steps, len(top))
		for i := 1; i < len(top); i++ {
			require.GreaterOrEqualf(t, top[i-1].Count, top[i].Count,
				"Top must be sorted by count descending (window=%v topK=%d steps=%x): %v",
				window, k, steps, top)
		}
		for _, hk := range top {
			require.Positivef(t, hk.Count,
				"Top must exclude zero-count keys (window=%v topK=%d steps=%x)",
				window, k, steps)
		}
		// Single-key model: if the key is in-window, Top must report it with the
		// same count Count reports; if it has expired, Top must omit it.
		modelCount := d.Count(key)
		switch modelCount {
		case 0:
			for _, hk := range top {
				require.NotEqualf(t, key, hk.Key,
					"Top must omit an expired/zero key (window=%v topK=%d steps=%x)",
					window, k, steps)
			}
		default:
			require.NotEmptyf(t, top,
				"Top must report an in-window key (window=%v topK=%d steps=%x): modelCount=%d",
				window, k, steps, modelCount)
			require.Equalf(t, key, top[0].Key,
				"single in-window key must lead Top (window=%v topK=%d steps=%x)",
				window, k, steps)
			require.Equalf(t, modelCount, top[0].Count,
				"Top count must match Count (window=%v topK=%d steps=%x): got %d want %d",
				window, k, steps, top[0].Count, modelCount)
		}

		// Reset must not panic and must clear state — roundtrip back to empty.
		d.Reset()
		require.Empty(t, d.Top(), "Reset must clear all tracked keys")
		require.Equal(t, 0, d.Len(), "Reset must leave Len at 0")
	})
}

// FuzzMaxKeysEviction fuzzes a small WithMaxKeys cap plus a stream of distinct
// keys touched under a frozen clock (no expiry). It asserts the tracked key
// count never exceeds the cap once it is reached — the package's signature
// contract (shared with freqcap). Because the clock never advances, no key can
// expire on its own; the only way Len stays bounded is the eviction loop in
// evictIdleLocked firing on every over-cap Touch. Distinct keys are generated
// from the byte stream so the fuzzer can explore both the under-cap and
// over-cap regimes as well as the cap==1 (single-slot) edge.
//
// windowMs selects the window; maxKeys is the cap (0/negative treated as
// unbounded by the package, so skipped); keyBytes is consumed as a sequence of
// distinct keys (one byte -> one key string), letting the fuzzer drive arbitrary
// cardinality against the cap.
func FuzzMaxKeysEviction(f *testing.F) {
	// Seeds cover: cap hit exactly, cap exceeded by one, cap exceeded many,
	// cap==1 (single slot, every new key evicts), single repeated key (cap never
	// reached), and the under-cap regime.
	f.Add(uint8(10), uint8(2), []byte{0, 1, 2, 3, 4})        // 5 distinct keys, cap 2
	f.Add(uint8(10), uint8(1), []byte{0, 1, 2, 3, 4})        // cap 1: every new key evicts the prior
	f.Add(uint8(10), uint8(3), []byte{0, 1, 2})              // exactly cap, no eviction
	f.Add(uint8(10), uint8(3), []byte{0, 0, 0, 0})           // single key repeated, cap never reached
	f.Add(uint8(10), uint8(2), []byte{})                     // no keys: Len must be 0
	f.Add(uint8(10), uint8(0), []byte{0, 1, 2})              // cap 0: unbounded, skipped
	f.Add(uint8(255), uint8(2), []byte{0, 1, 2, 3, 4, 5, 6}) // huge window, cap 2, many distinct

	f.Fuzz(func(t *testing.T, windowMs uint8, maxKeys uint8, keyBytes []byte) {
		// New panics on window<=0; windowMs==0 is outside the contract (skip).
		// maxKeys==0 means unbounded in the package's convention, so the cap
		// invariant under test does not apply (skip). The positive maxKeys space
		// is fuzzed hard.
		if windowMs == 0 {
			t.Skip("windowMs==0 is outside New's documented contract")
		}
		if maxKeys == 0 {
			t.Skip("maxKeys==0 is unbounded; the cap invariant does not apply")
		}
		window := time.Duration(windowMs) * time.Millisecond
		cap := int(maxKeys)

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked: window=%v maxKeys=%d keyBytes=%x recover=%v",
					window, cap, keyBytes, r)
			}
		}()

		// Clock is frozen for the whole run so no key can age out on its own:
		// the only thing keeping Len bounded is the eviction loop.
		clk := &fakeClock{t: time.Unix(0, 0)}
		d := New(window, cap, WithMaxKeys(cap), WithClock(clk.now))

		for i, b := range keyBytes {
			d.Touch(string([]byte{byte(i), b})) // distinct key per (index, byte)
			// Invariant: the cap holds after every Touch. This is the contract.
			require.LessOrEqualf(t, d.Len(), cap,
				"tracked key count must never exceed maxKeys (window=%v maxKeys=%d keyBytes=%x): Len=%d after step %d",
				window, cap, keyBytes, d.Len(), i)
		}

		// Final state must still honour the cap, and Top must be bounded by topK
		// (here == cap, since topK defaults are not exercised).
		require.LessOrEqualf(t, d.Len(), cap,
			"final Len must honour the cap (window=%v maxKeys=%d keyBytes=%x): Len=%d",
			window, cap, keyBytes, d.Len())
		top := d.Top()
		require.LessOrEqualf(t, len(top), cap,
			"Top must be bounded by topK==cap (window=%v maxKeys=%d keyBytes=%x): got %d",
			window, cap, keyBytes, len(top))
	})
}
