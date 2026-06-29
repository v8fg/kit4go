package kafka

import (
	"sync"
	"testing"
	"time"
)

func TestEffectiveLinger(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{LingerOff, 0}, // explicit disable → 0
		{0, 0},         // bare 0 passes through (withDefaults already replaced it)
		{DefaultProducerLinger, DefaultProducerLinger},
		{5 * time.Millisecond, 5 * time.Millisecond},
	}
	for _, c := range cases {
		if got := effectiveLinger(c.in); got != c.want {
			t.Errorf("effectiveLinger(%v)=%v want %v", c.in, got, c.want)
		}
	}
}

// mkSnap builds a distinguishable sample: Enqueued==i, Timestamp at unix i.
func mkSnap(i int) ProducerSnapshot {
	return ProducerSnapshot{
		Name:            "t",
		Backend:         "test",
		Timestamp:       time.Unix(int64(1000+i), 0).UTC(),
		ProducerMetrics: ProducerMetrics{Enqueued: uint64(i)},
	}
}

func TestSnapshotHistory_Disabled(t *testing.T) {
	if h := newSnapshotHistory(0); h != nil {
		t.Fatalf("newSnapshotHistory(0)=%v want nil", h)
	}
	if h := newSnapshotHistory(-1); h != nil {
		t.Fatalf("newSnapshotHistory(-1)=%v want nil", h)
	}
	// A nil *snapshotHistory must be safe to call (no-op).
	var nilh *snapshotHistory
	nilh.record(ProducerSnapshot{})
	if got := nilh.snapshot(); got != nil {
		t.Errorf("nil snapshot()=%v want nil", got)
	}
}

func TestSnapshotHistory_BelowCap(t *testing.T) {
	h := newSnapshotHistory(5)
	for i := 1; i <= 3; i++ {
		h.record(mkSnap(i))
	}
	got := h.snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	for i, s := range got { // oldest→newest: 1,2,3
		if s.Enqueued != uint64(i+1) {
			t.Errorf("got[%d].Enqueued=%d want %d (oldest→newest order)", i, s.Enqueued, i+1)
		}
	}
}

func TestSnapshotHistory_OverwriteOldest(t *testing.T) {
	h := newSnapshotHistory(3)
	for i := 1; i <= 5; i++ { // 5 into cap-3 → oldest 1,2 evicted
		h.record(mkSnap(i))
	}
	got := h.snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (capped)", len(got))
	}
	want := []uint64{3, 4, 5}
	for i, s := range got {
		if s.Enqueued != want[i] {
			t.Errorf("got[%d].Enqueued=%d want %d (FIFO overwrite)", i, s.Enqueued, want[i])
		}
	}
}

func TestSnapshotHistory_OrderMonotonic(t *testing.T) {
	h := newSnapshotHistory(4)
	for i := 1; i <= 6; i++ {
		h.record(mkSnap(i))
	}
	got := h.snapshot()
	for i := 1; i < len(got); i++ {
		if !got[i].Timestamp.After(got[i-1].Timestamp) {
			t.Errorf("order break at %d: %v not after %v", i, got[i].Timestamp, got[i-1].Timestamp)
		}
	}
}

func TestSnapshotHistory_Concurrent(t *testing.T) {
	h := newSnapshotHistory(100)
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				h.record(mkSnap(i))
			}
		}()
	}
	for r := 0; r < 5; r++ { // concurrent readers
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = h.snapshot()
			}
		}()
	}
	wg.Wait()
	if got := h.snapshot(); len(got) != 100 {
		t.Errorf("len=%d want 100 (capped, no race)", len(got))
	}
}

func TestSnapshotRate(t *testing.T) {
	t0 := time.Unix(1000, 0).UTC()
	t1 := t0.Add(2 * time.Second)
	prev := ProducerSnapshot{Timestamp: t0, ProducerMetrics: ProducerMetrics{Success: 100, Bytes: 1000}}
	cur := ProducerSnapshot{Timestamp: t1, ProducerMetrics: ProducerMetrics{Success: 300, Bytes: 5000}}

	success := func(s ProducerSnapshot) uint64 { return s.Success }
	if r := SnapshotRate(prev, cur, success); r != 100 { // (300-100)/2s
		t.Errorf("Success rate=%v want 100", r)
	}
	bytes := func(s ProducerSnapshot) uint64 { return s.Bytes }
	if r := SnapshotRate(prev, cur, bytes); r != 2000 { // (5000-1000)/2s
		t.Errorf("Bytes rate=%v want 2000", r)
	}
	if r := SnapshotRate(cur, prev, success); r != 0 { // reverse time
		t.Errorf("reverse-time rate=%v want 0", r)
	}
	if r := SnapshotRate(prev, prev, success); r != 0 { // equal time
		t.Errorf("equal-time rate=%v want 0", r)
	}
	reset := ProducerSnapshot{Timestamp: t1, ProducerMetrics: ProducerMetrics{Success: 50}}
	if r := SnapshotRate(prev, reset, success); r != 0 { // counter decrease (underflow guard)
		t.Errorf("counter-reset rate=%v want 0", r)
	}
}
