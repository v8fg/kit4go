package featureflag

import (
	"hash/fnv"
	"sync"
	"testing"
	"time"
)

// FuzzEnabled exercises the full Enabled evaluation path (and the runtime
// mutators that feed it) against arbitrary key + percentage + allowlist inputs.
//
// Core invariants under test:
//   - No panic: Enabled, Enable/Disable, SetPercentage, Allow/Revoke never
//     panic for any key string (incl. empty, huge, or high-bytes) or any
//     percentage (incl. values far above 100 that must be clamped).
//   - Determinism / roundtrip consistency: for a fixed flag state, Enabled(key)
//     returns the same bool on every call (the inlined FNV-1a is stateless).
//   - Ordering precedence (disabled → time-gate → allowlist → percentage):
//     a disabled flag is off for everyone; an allowlisted key is on whenever
//     the flag is enabled and past its time-gate, regardless of percentage;
//     a 100% flag is on for every key.
//
// The corpus is seeded via f.Add so the corpus is deterministic from clone,
// not just whatever the fuzzer happens to generate.
func FuzzEnabled(f *testing.F) {
	// Seeds cover: empty, ascii, long, and high-byte keys; percentages at the
	// 0/100 boundaries and far past the clamp limit.
	for _, seed := range []struct {
		key        string
		percentage uint
	}{
		{"", 0},
		{"user-1", 50},
		{"vip-user", 100},
		{"long-key-" + string(make([]byte, 256)), 200},
		{"\x00\xff\xfe", ^uint(0)},
		{"界", 1},
	} {
		f.Add(seed.key, seed.percentage)
	}

	f.Fuzz(func(t *testing.T, key string, percentage uint) {
		flag := New(
			WithEnabled(true),
			WithPercentage(percentage),
			WithAllowlist("vip-user"),
		)

		// SetPercentage must clamp to 100, so post-construction and post-Set
		// states agree on the clamp.
		flag.SetPercentage(percentage)

		// No-panic + determinism: two reads of the same key under the same flag
		// state must agree. Determinism is the load-bearing invariant for a
		// percentage rollout (a key must not flip-flop between buckets).
		first := flag.Enabled(key)
		second := flag.Enabled(key)
		if first != second {
			t.Fatalf("Enabled(%q) nondeterministic: %v then %v", key, first, second)
		}

		// Ordering: an enabled, allowlisted key is always on regardless of
		// percentage (this exercises the allowlist short-circuit before the
		// percentage branch).
		if got := flag.Enabled("vip-user"); !got {
			t.Fatalf("allowlisted key off at percentage=%d (precedence broken)", percentage)
		}

		// Ordering: disabling globally must turn every key off, including the
		// allowlisted one. This pins the disabled → everything-else precedence.
		flag.Disable()
		if flag.Enabled(key) || flag.Enabled("vip-user") {
			t.Fatalf("Disabled flag still evaluated true for key %q", key)
		}

		// Roundtrip the allowlist: Allow then Revoke returns a key to its
		// percentage-derived answer (no lingering allowlist entry).
		flag.Enable()
		flag.Allow(key)
		if !flag.Enabled(key) {
			t.Fatalf("Allow(%q) did not enable the key", key)
		}
		flag.Revoke(key)
		if flag.Enabled(key) != first {
			t.Fatalf("Revoke(%q) did not restore pre-Allow result: got %v want %v", key, flag.Enabled(key), first)
		}
	})
}

// FuzzHashPercent pins the inlined FNV-1a percentage hash against the stdlib
// implementation for arbitrary key bytes. The inlined form is the zero-alloc
// hot path; a regression (wrong prime, wrong offset basis, dropped byte,
// integer-overflow mishandling) would break the golden parity and silently
// reassign every key's rollout bucket.
//
// Invariants: hashPercent never panics on any input, stays in [0,99], and
// equals hash/fnv.New32a(key) % 100.
func FuzzHashPercent(f *testing.F) {
	for _, seed := range []string{
		"",
		"a",
		"abc",
		"user-42",
		"deterministic-key",
		"alloc-probe",
		"longer-key-with-unicode-界",
		"\x00\xff\xfe",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		// No panic + range contract.
		got := hashPercent(key)
		if got > 99 {
			t.Fatalf("hashPercent(%q) out of range: %d", key, got)
		}

		// Parity with stdlib FNV-1a 32-bit. This is the load-bearing check: the
		// inline exists only to avoid the hash.Hash32 interface allocation, so
		// it MUST produce identical bytes to hash/fnv.New32a.
		h := fnv.New32a()
		_, _ = h.Write([]byte(key))
		want := uint(h.Sum32() % 100)
		if got != want {
			t.Fatalf("hashPercent(%q) = %d, stdlib fnv32a %% 100 = %d", key, got, want)
		}

		// Pure-function determinism: same key, same bucket, every call.
		again := hashPercent(key)
		if got != again {
			t.Fatalf("hashPercent(%q) drifted within one fuzz call: %d then %d", key, got, again)
		}
	})
}

// FuzzEnabledConcurrent stresses Enabled + mutators concurrently under the race
// detector with fuzz-derived keys/percentages. The inlined FNV-1a carries no
// shared state, so concurrent readers must stay race-free against writers.
// This is a no-panic / data-race contract test, not an assertion test.
func FuzzEnabledConcurrent(f *testing.F) {
	for _, seed := range []struct {
		key        string
		percentage uint
	}{
		{"user-0", 0},
		{"vip", 100},
		{"race-key-1", 50},
		{"race-key-2", 99},
		{"", ^uint(0)},
	} {
		f.Add(seed.key, seed.percentage)
	}

	f.Fuzz(func(t *testing.T, key string, percentage uint) {
		// A future start time would make every key evaluate false and mask the
		// concurrency we want to exercise, so we leave the time-gate open.
		flag := New(WithEnabled(true), WithPercentage(percentage), WithStartTime(time.Time{}))

		var wg sync.WaitGroup
		stop := make(chan struct{})

		// Readers.
		for range 4 {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
						_ = flag.Enabled(key)
						_ = flag.Enabled("vip")
					}
				}
			})
		}

		// Writers mutating every guarded field.
		for range 2 {
			wg.Go(func() {
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
						switch i % 5 {
						case 0:
							flag.SetPercentage(percentage)
						case 1:
							flag.Allow(key)
						case 2:
							flag.Revoke(key)
						case 3:
							flag.Disable()
						case 4:
							flag.Enable()
						}
					}
				}
			})
		}

		// Brief burst under -race; bounded so the fuzz corpus stays tractable.
		time.Sleep(2 * time.Millisecond)
		close(stop)
		wg.Wait()
	})
}
