package topk

import "testing"

// FuzzTouchNHeavyHitterSurfaces encodes the documented-WORKING heavy-hitter
// path (the counterpart to the per-event Touch starvation caveat): a key whose
// count, set via a single batched TouchN, strictly exceeds every other key's
// MUST surface in Top. Guards the recommended detection path against regression.
// E10 invariant-encoding fuzz target.
func FuzzTouchNHeavyHitterSurfaces(f *testing.F) {
	f.Add("heavy", "a", "b", int64(100), int64(1), int64(1))
	f.Fuzz(func(t *testing.T, heavy, a, b string, heavySeed, aSeed, bSeed int64) {
		// heavy is always the strict max: [50,1049] vs a,b in [0,39].
		hs := heavySeed % 1000
		if hs < 0 {
			hs += 1000
		}
		heavyN := hs + 50
		as := aSeed % 40
		if as < 0 {
			as += 40
		}
		bs := bSeed % 40
		if bs < 0 {
			bs += 40
		}
		tr := New(2)
		tr.TouchN(heavy, heavyN)
		tr.TouchN(a, as)
		tr.TouchN(b, bs)
		for _, e := range tr.Top() {
			if e.Key == heavy {
				return // surfaced — invariant holds
			}
		}
		t.Errorf("heavy %q (count %d, > a=%d b=%d) not in Top after batched TouchN", heavy, heavyN, as, bs)
	})
}
