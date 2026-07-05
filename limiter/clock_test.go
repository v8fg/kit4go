package limiter

import "time"

// fakeClock is a deterministic clock for time-window correctness tests. Instead
// of sleeping past a window boundary (flaky under CPU contention), tests inject
// a *fakeClock into an algorithm struct's `now` field and advance time with add.
//
// All fields are accessed from a single goroutine in the tests that use it; the
// algorithm's atomic hot path is exercised separately under -race by the
// concurrent tests, which keep the real clock.
type fakeClock struct {
	t time.Time
}

// newFakeClock returns a fake clock anchored at a fixed, reproducible instant
// (well clear of the epoch so per-second bucket math is realistic).
func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Unix(1_000_000, 0)}
}

func (f *fakeClock) now() time.Time { return f.t }

// add advances the fake clock by d. Negative durations are not used by the
// correctness tests but are tolerated (the production code clamps backward
// clocks defensively).
func (f *fakeClock) add(d time.Duration) { f.t = f.t.Add(d) }
