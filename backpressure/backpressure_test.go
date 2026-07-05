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
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.TryAcquire() {
				acquired.Add(1)
				g.Release()
			}
		}()
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
	for i := 0; i < 4; i++ {
		if !g.TryAcquire() {
			t.Fatalf("acquire %d should succeed after SetMax(5)", i+1)
		}
	}
}
