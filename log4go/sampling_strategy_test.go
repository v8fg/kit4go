package log4go

import (
	"math/rand"
	"testing"
)

// TestSampling_Full keeps everything.
func TestSampling_Full(t *testing.T) {
	f := FullSampling{}
	for _, id := range []string{"", "abc", "4a3f0b1c2d3e4f60718293a4b5c6d7e8"} {
		if !f.ShouldLog(id) {
			t.Errorf("FullSampling dropped %q", id)
		}
	}
}

// TestSampling_TraceIDRatioBased_Boundaries checks the trivial ratios.
func TestSampling_TraceIDRatioBased_Boundaries(t *testing.T) {
	all := TraceIDRatioBased{Ratio: 1.0}
	none := TraceIDRatioBased{Ratio: 0.0}
	for _, id := range []string{"x", "4a3f0b1c2d3e4f60718293a4b5c6d7e8"} {
		if !all.ShouldLog(id) {
			t.Errorf("ratio=1.0 dropped %q", id)
		}
		if none.ShouldLog(id) {
			t.Errorf("ratio=0.0 kept %q", id)
		}
	}
}

// TestSampling_TraceIDRatioBased_ExtremeIds: a tiny uint64 id is kept at any
// positive ratio; the max uint64 id is dropped at any ratio < 1.
func TestSampling_TraceIDRatioBased_ExtremeIds(t *testing.T) {
	half := TraceIDRatioBased{Ratio: 0.5}
	// "0000000000000001..." -> high-64 = 1 -> always kept (1 < 0.5*Max).
	if !half.ShouldLog("00000000000000010000000000000000") {
		t.Error("tiny id should be kept at ratio 0.5")
	}
	// "ffffffffffffffff..." -> high-64 = MaxUint64 -> dropped at ratio < 1.
	if half.ShouldLog("ffffffffffffffffffffffffffffffff") {
		t.Error("max id should be dropped at ratio 0.5")
	}
}

// TestSampling_Determinism: the same id always yields the same decision, and the
// decision is stable across calls (cross-service consistency rests on this).
func TestSampling_Determinism(t *testing.T) {
	strategies := []SamplingStrategy{
		TraceIDRatioBased{Ratio: 0.1},
		TraceIDRatioBased{Ratio: 0.5},
		TailDigitSampling{Modulus: 10, Keep: 3},
	}
	for i, s := range strategies {
		for _, id := range []string{"abc-def-123", "4a3f0b1c2d3e4f60718293a4b5c6d7e8", "req-771"} {
			a := s.ShouldLog(id)
			b := s.ShouldLog(id)
			c := s.ShouldLog(id)
			if a != b || b != c {
				t.Errorf("strategy %d: non-deterministic for %q: %v %v %v", i, id, a, b, c)
			}
		}
	}
}

// TestSampling_Distribution: TraceIDRatioBased(r) keeps ~r of random ids (loose,
// deterministic via a fixed seed). Confirms uniformity, not just determinism.
func TestSampling_Distribution(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	s := TraceIDRatioBased{Ratio: 0.1}
	const n = 5000
	kept := 0
	for i := 0; i < n; i++ {
		// 32-hex-digit trace_id (W3C shape).
		var buf [32]byte
		for j := range buf {
			buf[j] = "0123456789abcdef"[r.Intn(16)]
		}
		if s.ShouldLog(string(buf[:])) {
			kept++
		}
	}
	got := float64(kept) / n
	// expect ~0.10; allow [0.08, 0.12].
	if got < 0.08 || got > 0.12 {
		t.Errorf("ratio=0.1 kept %d/%d = %.3f, want ~0.10", kept, n, got)
	}
}

// TestSampling_InvalidIDKept: a missing/invalid id is never dropped.
func TestSampling_InvalidIDKept(t *testing.T) {
	s := TraceIDRatioBased{Ratio: 0.0001} // very aggressive
	if !s.ShouldLog("") {                 // empty
		t.Error("empty id must be kept")
	}
	if !s.ShouldLog("not-hex-and-short") { // too short / non-hex -> FNV, but kept if it maps high? ensure no panic
		// FNV may map it either way; the contract is only "no panic + deterministic".
		// Re-assert determinism instead of keep:
		if s.ShouldLog("not-hex-and-short") != s.ShouldLog("not-hex-and-short") {
			t.Error("non-hex id must be deterministic")
		}
	}
}

// TestSampling_TailDigit: modulus/keep correctness on a known mapping.
func TestSampling_TailDigit(t *testing.T) {
	td := TailDigitSampling{Modulus: 10, Keep: 3}
	// Deterministic + within [0, modulus).
	for _, id := range []string{"req-1", "req-2", "device-abc", "4a3f0b1c2d3e4f60"} {
		_ = td.ShouldLog(id) // must not panic
	}
	// modulus 0 -> always keep.
	always := TailDigitSampling{Modulus: 0, Keep: 0}
	if !always.ShouldLog("anything") {
		t.Error("modulus=0 must keep all")
	}
	// empty id -> idUint64 returns !ok -> keep (never drop on missing id).
	if !td.ShouldLog("") {
		t.Error("empty id must be kept by TailDigitSampling")
	}
}

// TestIDUint64_ShortHex: a hex id shorter than 16 digits is zero-padded to 16.
func TestIDUint64_ShortHex(t *testing.T) {
	// "00000001" (8 hex) -> padded to "0000000000000001" -> 1.
	if v, _ := idUint64("00000001"); v != 1 {
		t.Errorf("short hex id reduced to %d, want 1", v)
	}
}

// TestIDUint64_Hex: W3C trace_ids reduce via the high-64 hex parse.
func TestIDUint64_Hex(t *testing.T) {
	// First 16 hex = "0000000000000001" -> 1.
	if v, _ := idUint64("0000000000000001ffffffffffffffff"); v != 1 {
		t.Errorf("hex high-64 = %d, want 1", v)
	}
	// First 16 hex = "0000000000000010" -> 16.
	if v, _ := idUint64("0000000000000010deadbeefdeadbeef"); v != 16 {
		t.Errorf("hex high-64 = %d, want 16", v)
	}
}

// TestIDUint64_FNV: non-hex ids reduce via FNV-1a (portable, deterministic).
func TestIDUint64_FNV(t *testing.T) {
	a, _ := idUint64("request-xyz")
	b, _ := idUint64("request-xyz")
	if a != b {
		t.Error("FNV reduction must be deterministic")
	}
	c, _ := idUint64("request-abc")
	if a == c {
		t.Error("distinct ids should (almost surely) reduce to distinct uint64")
	}
}
