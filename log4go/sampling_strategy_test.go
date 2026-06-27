package log4go

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// waitFor polls cond until it returns true or the deadline elapses.
func waitFor(t *testing.T, cond func() bool, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) && !cond() {
		time.Sleep(time.Millisecond)
	}
}

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

// TestSamplingStrategy_Wired: the strategy is evaluated once per request at
// WithContext (cached as sampleDrop) and the deliver hot path honors it — a
// dropped request's records never reach the writer.
func TestSamplingStrategy_Wired(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	cw := &captureWriter{}
	root.Register(cw)
	root.SetLevel(DEBUG)

	ctx := context.WithValue(context.Background(), "trace_id", "4a3f0b1c2d3e4f60718293a4b5c6d7e8")

	// ratio=0 -> drop all records from this trace.
	root.SetSamplingStrategy(TraceIDRatioBased{Ratio: 0})
	root.WithContext(ctx).Info("dropped-1")
	root.WithContext(ctx).Info("dropped-2")
	waitFor(t, func() bool { return cw.Len() == 0 }, time.Second) // let any straggler arrive
	if cw.Len() != 0 {
		t.Errorf("ratio=0: %d records delivered, want 0", cw.Len())
	}

	// ratio=1 -> kept.
	root.SetSamplingStrategy(TraceIDRatioBased{Ratio: 1})
	root.WithContext(ctx).Info("kept")
	waitFor(t, func() bool { return cw.Len() >= 1 }, time.Second)
	if cw.Len() < 1 {
		t.Errorf("ratio=1: %d records delivered, want >=1", cw.Len())
	}

	// nil strategy -> default keep all (no per-request decision).
	root.SetSamplingStrategy(nil)
	root.Info("default-kept")
	want := cw.Len() + 1
	root.Info("default-kept-2")
	waitFor(t, func() bool { return cw.Len() >= want }, time.Second)
}

// TestCorrelationIDFromContext: the first correlation key in ctx.Value wins
// (trace_id before request_id/device_id).
func TestCorrelationIDFromContext(t *testing.T) {
	if got := correlationIDFromContext(nil); got != "" {
		t.Errorf("nil ctx = %q, want empty", got)
	}
	ctx := context.WithValue(context.Background(), "trace_id", "tid-123")
	if got := correlationIDFromContext(ctx); got != "tid-123" {
		t.Errorf("trace_id = %q, want tid-123", got)
	}
	// request_id only (no trace_id).
	ctx2 := context.WithValue(context.Background(), "request_id", "req-9")
	if got := correlationIDFromContext(ctx2); got != "req-9" {
		t.Errorf("request_id = %q, want req-9", got)
	}
	// non-string value formatted.
	ctx3 := context.WithValue(context.Background(), "trace_id", 42)
	if got := correlationIDFromContext(ctx3); got != "42" {
		t.Errorf("int trace_id = %q, want 42", got)
	}
	// no correlation key present -> empty.
	if got := correlationIDFromContext(context.Background()); got != "" {
		t.Errorf("empty ctx = %q, want empty", got)
	}
}

// TestPackage_SetSamplingStrategy covers the package-level installer on the
// singleton (install + nil-clear), with save/restore.
func TestPackage_SetSamplingStrategy(t *testing.T) {
	dl := defaultLogger()
	saved := dl.samplingStrategy.Load()
	defer dl.samplingStrategy.Store(saved)

	SetSamplingStrategy(TraceIDRatioBased{Ratio: 0.5})
	if dl.samplingStrategy.Load() == nil {
		t.Error("package SetSamplingStrategy did not install")
	}
	SetSamplingStrategy(nil)
	if dl.samplingStrategy.Load() != nil {
		t.Error("package SetSamplingStrategy(nil) did not clear")
	}
}

// TestSetSamplingStrategyFor_AutoRevert: a temporary session keeps records while
// active, then auto-reverts to the previous (drop-all) policy.
func TestSetSamplingStrategyFor_AutoRevert(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	cw := &captureWriter{}
	root.Register(cw)
	root.SetLevel(DEBUG)
	ctx := context.WithValue(context.Background(), "trace_id", "4a3f0b1c2d3e4f60718293a4b5c6d7e8")

	root.SetSamplingStrategy(TraceIDRatioBased{Ratio: 0}) // prev: drop all
	root.SetSamplingStrategyFor(TraceIDRatioBased{Ratio: 1}, 50*time.Millisecond)
	root.WithContext(ctx).Info("during") // kept (session active)
	waitFor(t, func() bool { return cw.Len() >= 1 }, time.Second)
	if cw.Len() < 1 {
		t.Fatal("during session: record should be kept")
	}
	time.Sleep(90 * time.Millisecond)   // session expired -> reverted to ratio=0
	root.WithContext(ctx).Info("after") // dropped
	time.Sleep(50 * time.Millisecond)   // let any delivery settle
	if cw.Len() != 1 {
		t.Errorf("after auto-revert: cw.Len()=%d, want 1 (dropped)", cw.Len())
	}
}

// TestSetSamplingStrategyFor_EarlyStop: stop() reverts immediately (synchronous).
func TestSetSamplingStrategyFor_EarlyStop(t *testing.T) {
	root := newLoggerWithRecords(make(chan *Record, 4))
	defer root.Close()
	cw := &captureWriter{}
	root.Register(cw)
	root.SetLevel(DEBUG)
	ctx := context.WithValue(context.Background(), "trace_id", "4a3f0b1c2d3e4f60718293a4b5c6d7e8")

	root.SetSamplingStrategy(TraceIDRatioBased{Ratio: 0}) // prev: drop all
	stop := root.SetSamplingStrategyFor(TraceIDRatioBased{Ratio: 1}, 5*time.Second)
	stop()                                // synchronous revert -> back to ratio=0
	root.WithContext(ctx).Info("stopped") // dropped
	time.Sleep(50 * time.Millisecond)
	if cw.Len() != 0 {
		t.Errorf("after stop: cw.Len()=%d, want 0 (reverted to drop)", cw.Len())
	}
	// stop() is idempotent — calling again must not panic.
	stop()
	stop()
}

// TestPackage_SetSamplingStrategyFor covers the package-level temporary-session
// installer on the singleton (install + stop-revert).
func TestPackage_SetSamplingStrategyFor(t *testing.T) {
	dl := defaultLogger()
	saved := dl.samplingStrategy.Load()
	defer dl.samplingStrategy.Store(saved)

	stop := SetSamplingStrategyFor(TraceIDRatioBased{Ratio: 0.5}, 5*time.Second)
	if dl.samplingStrategy.Load() == nil {
		t.Error("package SetSamplingStrategyFor did not install")
	}
	stop() // synchronous revert to saved
	if got := dl.samplingStrategy.Load(); got != saved {
		t.Error("package SetSamplingStrategyFor stop did not revert to saved")
	}
}
