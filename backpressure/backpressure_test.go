package backpressure

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestGate_BasicAcquireRelease(t *testing.T) {
	g := New(2)
	if !g.TryAcquire() {
		t.Fatal("first acquire should succeed")
	}
	if !g.TryAcquire() {
		t.Fatal("second acquire should succeed")
	}
	if g.TryAcquire() {
		t.Fatal("third acquire should fail (at capacity)")
	}
	if g.Current() != 2 {
		t.Fatalf("current = %d, want 2", g.Current())
	}
	g.Release()
	if g.Current() != 1 {
		t.Fatalf("after release current = %d, want 1", g.Current())
	}
	if !g.TryAcquire() {
		t.Fatal("acquire after release should succeed")
	}
}

func TestGate_RejectedCounter(t *testing.T) {
	g := New(1)
	g.TryAcquire()
	if g.TryAcquire() {
		t.Fatal("should reject at capacity")
	}
	if g.TryAcquire() {
		t.Fatal("should reject again")
	}
	if g.Rejected() != 2 {
		t.Fatalf("rejected = %d, want 2", g.Rejected())
	}
}

func TestGate_IsOverloaded(t *testing.T) {
	g := New(1)
	if g.IsOverloaded() {
		t.Fatal("empty gate should not be overloaded")
	}
	g.TryAcquire()
	if !g.IsOverloaded() {
		t.Fatal("full gate should be overloaded")
	}
	g.Release()
	if g.IsOverloaded() {
		t.Fatal("after release should not be overloaded")
	}
}

func TestGate_ConcurrentSafe(t *testing.T) {
	g := New(10)
	var wg sync.WaitGroup
	var acquired atomic.Int32
	for range 100 {
		wg.Go(func() {
			if g.TryAcquire() {
				acquired.Add(1)
				g.Release()
			}
		})
	}
	wg.Wait()
	// All 100 should eventually acquire (since each releases immediately).
	if acquired.Load() != 100 {
		t.Fatalf("acquired = %d, want 100", acquired.Load())
	}
	if g.Current() != 0 {
		t.Fatalf("current after all done = %d, want 0", g.Current())
	}
}

func TestGate_ReleaseWithoutAcquire(t *testing.T) {
	g := New(1)
	if g.Release() {
		t.Fatal("Release without Acquire should return false, not panic")
	}
}

func TestGate_Max(t *testing.T) {
	g := New(5)
	if g.Max() != 5 {
		t.Fatalf("Max() = %d, want 5", g.Max())
	}
}

func TestGate_NegativeMax(t *testing.T) {
	g := New(-1)
	if g.Max() != 0 {
		t.Fatalf("negative max should clamp to 0, got %d", g.Max())
	}
	if g.TryAcquire() {
		t.Fatal("max=0 should reject all")
	}
}

func TestGate_SetMax(t *testing.T) {
	g := New(1)
	g.TryAcquire()
	if g.TryAcquire() {
		t.Fatal("should reject at max=1")
	}
	g.SetMax(5)
	// Now 1 in flight, max 5 → 4 more should succeed.
	for i := range 4 {
		if !g.TryAcquire() {
			t.Fatalf("acquire %d should succeed after SetMax(5)", i+1)
		}
	}
}

func TestGate_SetMaxNegativeClampsToZero(t *testing.T) {
	g := New(2)
	// SetMax must clamp a negative capacity to 0 exactly like New does.
	g.SetMax(-3)
	if g.Max() != 0 {
		t.Fatalf("SetMax(-3) should clamp to 0, got %d", g.Max())
	}
	// max=0 means the gate rejects everything.
	if g.TryAcquire() {
		t.Fatal("after SetMax(<0) the gate should reject all attempts")
	}
	if g.Rejected() != 1 {
		t.Fatalf("rejected = %d, want 1", g.Rejected())
	}
	if !g.IsOverloaded() {
		t.Fatal("gate with max=0 and current 0 should report overloaded (0 >= 0)")
	}
}

func TestGate_SetMaxBelowCurrentDoesNotRejectInflight(t *testing.T) {
	g := New(5)
	// Fill 3 in-flight slots.
	for i := range 3 {
		if !g.TryAcquire() {
			t.Fatalf("acquire %d should succeed at max=5", i+1)
		}
	}
	// Hot-reload down to 1. Per the contract, already-in-flight items are NOT
	// evicted; current stays at 3 until they Release.
	g.SetMax(1)
	if g.Current() != 3 {
		t.Fatalf("current after SetMax(1) = %d, want 3 (no eviction)", g.Current())
	}
	// New admissions are now blocked until current drops to 0 (< max).
	if g.TryAcquire() {
		t.Fatal("should reject new acquire while current exceeds new max")
	}
	// Draining one slot is not enough — current(2) is still >= max(1).
	if !g.Release() {
		t.Fatal("Release should decrement and return true")
	}
	if g.TryAcquire() {
		t.Fatal("should still reject while current(2) >= max(1)")
	}
	// Drain to 0; now admissions resume.
	for g.Current() > 0 {
		g.Release()
	}
	if !g.TryAcquire() {
		t.Fatal("should admit again once current drops below max")
	}
}

// TestGate_SetMax_ConcurrentSafe guards the max-as-atomic.Int32 fix: SetMax is
// documented for runtime hot-reload, so it races with TryAcquire/IsOverloaded/
// Max readers. With max as a plain int32 the race detector flagged a data race
// here; with atomic.Int32 it is clean. Run under -race.
func TestGate_SetMax_ConcurrentSafe(t *testing.T) {
	g := New(100)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5000 {
				g.TryAcquire()
				g.Release()
				_ = g.IsOverloaded()
				_ = g.Max()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 5000 {
			g.SetMax(50)
			g.SetMax(150)
		}
	}()
	wg.Wait()
}
