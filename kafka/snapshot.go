package kafka

import (
	"sync"
	"time"
)

// effectiveLinger resolves the ProducerLinger option to the value handed to the
// backend: LingerOff (-1) → 0 (batching disabled, flush every record); any other
// value passes through unchanged (withDefaults has already replaced a bare 0 with
// DefaultProducerLinger, and a user-set positive value is honored). Shared by
// both backends so the linger mapping is symmetric.
func effectiveLinger(d time.Duration) time.Duration {
	if d == LingerOff {
		return 0
	}
	return d
}

// snapshotHistory is a bounded ring of recent ProducerSnapshot samples, opt-in
// via the SnapshotHistory option. It is the cold/scrape-path counterpart to
// Snapshot(): every Snapshot() call records one sample; History() copies them
// out oldest→newest for trend analysis (e.g. via SnapshotRate).
//
// Design (industry scrape-driven model — Prometheus scrape interval = one
// timestamped sample; OpenTelemetry MetricReader periodic collection;
// Micrometer StepMeter sliding window):
//   - PULL, not push: history grows only when monitored (each Snapshot() call).
//     No background ticker/goroutine → no leak, zero idle overhead.
//   - Bounded: cap samples (configurable); overwrites the oldest when full.
//   - Lock-free hot path: Send/SendBatch never touch this struct — only
//     Snapshot() (record) and History() (read) do, both under mu. Both are
//     scrape-cadence (rare), so a plain sync.Mutex suffices (matches
//     log4go.RingSpiller and latency.Histogram).
//   - No dedup: every scrape records (Prometheus semantics — an idle period
//     still consumes slots; boundedness is guaranteed by cap).
//
// A nil *snapshotHistory is safe to call (record/snapshot are no-ops), so a
// producer with history disabled pays zero memory and zero overhead.
type snapshotHistory struct {
	mu   sync.Mutex
	buf  []ProducerSnapshot // len == capv; ring buffer
	head int                // next write position
	size int                // current sample count (≤ capv)
	capv int                // configured capacity (≥1 when non-nil)
}

// newSnapshotHistory returns a ring of the given capacity, or nil if capacity ≤ 0
// (history disabled). Callers store the *snapshotHistory directly and treat nil
// as "off" — no wrapper struct needed.
func newSnapshotHistory(capacity int) *snapshotHistory {
	if capacity <= 0 {
		return nil
	}
	return &snapshotHistory{
		buf:  make([]ProducerSnapshot, capacity),
		capv: capacity,
	}
}

// record appends snap, overwriting the oldest sample when full. No-op when h is
// nil (history disabled). The ProducerSnapshot is stored by value copy; its
// string fields (Name/Backend) share immutable backing arrays, so later producer
// state changes cannot corrupt stored samples.
func (h *snapshotHistory) record(snap ProducerSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.buf[h.head] = snap
	h.head = (h.head + 1) % h.capv
	if h.size < h.capv {
		h.size++
	}
	h.mu.Unlock()
}

// snapshot returns a copy of the recorded samples in oldest→newest order (index 0
// is the oldest, last is the newest), so a forward walk diffs consecutive
// samples in time order. Returns nil when empty or disabled (len(nil)==0 and
// range-nil are safe). The caller owns the returned slice.
func (h *snapshotHistory) snapshot() []ProducerSnapshot {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.size == 0 {
		return nil
	}
	out := make([]ProducerSnapshot, h.size)
	start := (h.head - h.size + h.capv) % h.capv
	for i := range h.size {
		out[i] = h.buf[(start+i)%h.capv]
	}
	return out
}

// SnapshotRate returns the per-second rate of a counter between two samples:
// (metric(cur) - metric(prev)) / cur.Timestamp.Sub(prev.Timestamp). metric
// selects the counter (e.g. func(s ProducerSnapshot) uint64 { return s.Success }
// for records-acked/sec, or s.Bytes for bytes/sec). Returns 0 if the timestamps
// are not strictly increasing (cur must be after prev), or if the counter went
// backwards (uint64 underflow guard — counter reset after producer restart, or a
// transient inconsistency in a derived field).
//
// Typical use with History():
//
//	if h, ok := producer.(kafka.SnapshotHistory); ok {
//	    s := h.History()
//	    if len(s) >= 2 {
//	        rps := kafka.SnapshotRate(s[len(s)-2], s[len(s)-1],
//	            func(p kafka.ProducerSnapshot) uint64 { return p.Success })
//	        _ = rps // records/sec over the last scrape window
//	    }
//	}
func SnapshotRate(prev, cur ProducerSnapshot, metric func(ProducerSnapshot) uint64) float64 {
	dt := cur.Timestamp.Sub(prev.Timestamp)
	if dt <= 0 {
		return 0
	}
	pc, cc := metric(prev), metric(cur)
	if cc < pc {
		return 0
	}
	return float64(cc-pc) / dt.Seconds()
}
