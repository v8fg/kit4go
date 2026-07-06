package featureflag

import (
	"hash/fnv"
	"strings"
	"sync"
	"testing"
	"time"
)

// goldenFNV32aPercent mirrors the pre-D6 implementation: hash/fnv.New32a over
// the key, modulo 100. Used by TestFlag_HashPercentMatchesStdlib to pin the
// inlined FNV-1a to the stdlib result.
func goldenFNV32aPercent(key string) uint {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return uint(h.Sum32() % 100)
}

func TestFlag_DisabledByDefault(t *testing.T) {
	f := New()
	if f.Enabled("user1") {
		t.Fatal("flag should be off by default")
	}
}

func TestFlag_Enable(t *testing.T) {
	f := New(WithEnabled(true))
	if !f.Enabled("anyone") {
		t.Fatal("enabled flag should be on for everyone")
	}
}

func TestFlag_Percentage(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(50))
	enabled := 0
	total := 1000
	for i := 0; i < total; i++ {
		if f.Enabled("user-" + itoa(i)) {
			enabled++
		}
	}
	// ~50% with some variance; allow 40-60%.
	if enabled < total*40/100 || enabled > total*60/100 {
		t.Fatalf("percentage rollout: %d/%d enabled, expected ~50%%", enabled, total)
	}
}

func TestFlag_PercentageConsistent(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(30))
	for i := 0; i < 100; i++ {
		key := "user-" + itoa(i)
		first := f.Enabled(key)
		second := f.Enabled(key)
		if first != second {
			t.Fatalf("inconsistent result for key %q: %v then %v", key, first, second)
		}
	}
}

func TestFlag_Allowlist(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(0), WithAllowlist("vip-user"))
	if !f.Enabled("vip-user") {
		t.Fatal("allowlisted key should be enabled even at 0%")
	}
	if f.Enabled("regular-user") {
		t.Fatal("non-allowlisted key should be off at 0%")
	}
}

func TestFlag_TimeGate(t *testing.T) {
	future := time.Now().Add(time.Hour)
	f := New(WithEnabled(true), WithPercentage(100), WithStartTime(future))
	if f.Enabled("user1") {
		t.Fatal("flag should be off before start time")
	}
}

// TestFlag_WithPercentageClampsOver100 covers the WithPercentage clamp branch
// (featureflag.go:43): values above 100 must be reduced to 100, so an enabled
// flag rolls out to everyone regardless of how far above 100 the caller went.
// We assert "everyone enabled" both with no allowlist and with keys whose
// hashPercent would otherwise fall outside a sub-100 band.
func TestFlag_WithPercentageClampsOver100(t *testing.T) {
	// Build a few keys that are NOT universally on at low percentages, so a
	// broken clamp (leaving a >100 value, which the rollout math never treats
	// specially) would still be distinguishable from a correctly-clamped 100.
	// At percentage>=100 Enabled short-circuits to true before hashing, so we
	// verify that path is reached for every sampled key.
	keys := []string{"user-0", "user-1", "user-7", "user-42", "vip-x", "alloc-probe"}

	for _, p := range []uint{101, 150, 999, 4294967295} {
		f := New(WithEnabled(true), WithPercentage(p))
		for _, k := range keys {
			if !f.Enabled(k) {
				t.Fatalf("WithPercentage(%d): key %q not enabled; clamp to 100 expected", p, k)
			}
		}
	}
}

// TestFlag_SetPercentageClampsOver100 covers the SetPercentage clamp branch
// (featureflag.go:129): runtime values above 100 are clamped to 100, so after
// the call every key is enabled. We start at 0% (nobody) and confirm the
// post-clamp state turns all sampled keys on.
func TestFlag_SetPercentageClampsOver100(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(0))
	keys := []string{"user-0", "user-1", "user-7", "user-42", "vip-x", "alloc-probe"}
	for _, k := range keys {
		if f.Enabled(k) {
			t.Fatalf("precondition: key %q enabled at 0%%", k)
		}
	}
	for _, p := range []uint{101, 250, 1000, 4294967295} {
		f.SetPercentage(p)
		for _, k := range keys {
			if !f.Enabled(k) {
				t.Fatalf("SetPercentage(%d): key %q not enabled; clamp to 100 expected", p, k)
			}
		}
	}
}

func TestFlag_RuntimeChanges(t *testing.T) {
	f := New()
	f.Enable()
	f.SetPercentage(0)
	if f.Enabled("user1") {
		t.Fatal("0% should be off for non-allowlisted")
	}
	f.Allow("user1")
	if !f.Enabled("user1") {
		t.Fatal("allowlisted user should be on")
	}
	f.Revoke("user1")
	if f.Enabled("user1") {
		t.Fatal("revoked user should be off at 0%")
	}
	f.Disable()
	if f.Enabled("vip") {
		t.Fatal("disabled flag should be off for everyone")
	}
}

// TestFlag_HashPercentDeterministic drives hashPercent through many calls for
// the same key. The inlined FNV-1a is a pure function of the key, so the value
// must be identical on every call (a regression that introduces per-call state
// would surface here).
func TestFlag_HashPercentDeterministic(t *testing.T) {
	const key = "deterministic-key"
	first := hashPercent(key)
	for i := 0; i < 10000; i++ {
		if got := hashPercent(key); got != first {
			t.Fatalf("hashPercent(%q) drifted: first=%d got=%d at i=%d", key, first, got, i)
		}
	}
}

// TestFlag_HashPercentMatchesStdlib pins the inlined FNV-1a to hash/fnv so the
// zero-alloc shortcut cannot silently drift from the canonical implementation.
// A key that previously hashed to one bucket under hash/fnv must keep doing so.
func TestFlag_HashPercentMatchesStdlib(t *testing.T) {
	keys := []string{
		"", "a", "abc", "user-1", "user-42", "vip-user",
		"deterministic-key", "alloc-probe", "regular-user",
		"longer-key-with-unicode-界", strings.Repeat("x", 64),
	}
	for _, key := range keys {
		want := goldenFNV32aPercent(key)
		if got := hashPercent(key); got != want {
			t.Fatalf("hashPercent(%q) = %d, stdlib fnv32a %% 100 = %d", key, got, want)
		}
	}
}

// TestFlag_HashPercentDistinctKeys covers the full distribution: a sane hash
// spreads 1000 distinct keys across most of the 0-99 buckets, and the value is
// always within range.
func TestFlag_HashPercentDistinctKeys(t *testing.T) {
	seen := make(map[uint]int)
	for i := 0; i < 1000; i++ {
		p := hashPercent(itoa(i))
		if p > 99 {
			t.Fatalf("hashPercent out of range: %d", p)
		}
		seen[p]++
	}
	// A uniform hash over 1000 keys into 100 buckets should fill almost all of
	// them; require at least 80 distinct buckets to catch a degenerate hash.
	if len(seen) < 80 {
		t.Fatalf("poor hash distribution: only %d/100 buckets used", len(seen))
	}
}

// TestFlag_HotPathAllocs verifies the D6 fix: a percentage-rollout Enabled call
// allocates nothing on the heap. Before the fix the FNV hasher was constructed
// (and its key slice escaped through the hash.Hash32 interface) on every call
// while the read lock was held; the inlined FNV-1a plus state-snapshot-outside-
// the-lock pattern removes every allocation.
func TestFlag_HotPathAllocs(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(50))
	key := "alloc-probe"
	// Prime any internal caches so steady state is measured.
	for i := 0; i < 100; i++ {
		_ = f.Enabled(key)
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = f.Enabled(key)
	})
	if allocs != 0 {
		t.Fatalf("Enabled hot path allocates %v objects; expected 0", allocs)
	}
}

// TestFlag_ConcurrentEnabledAndMutators hammers Enabled from many goroutines
// while writers mutate the flag, primarily to exercise the -race detector:
// the inlined hash holds no shared state, so concurrent readers must stay
// race-free against writers mutating enabled/percentage/allowlist.
func TestFlag_ConcurrentEnabledAndMutators(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(50), WithAllowlist("vip"))
	done := make(chan struct{})
	var wg sync.WaitGroup

	// Readers.
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = f.Enabled("user-" + itoa(n))
					_ = f.Enabled("vip")
				}
			}
		}(r)
	}

	// Writers.
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-done:
					return
				default:
					switch i % 4 {
					case 0:
						f.SetPercentage(uint(i % 101))
					case 1:
						f.Allow("k-" + itoa(i))
					case 2:
						f.Revoke("k-" + itoa(i))
					case 3:
						f.Disable()
						f.Enable()
					}
				}
			}
		}(w)
	}

	// Let them run briefly under -race.
	time.Sleep(50 * time.Millisecond)
	close(done)
	wg.Wait()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
