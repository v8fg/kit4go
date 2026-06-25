package log4go

import "sync/atomic"

// Sampler implements level-aware log sampling to protect against high-frequency
// log storms. The policy mirrors zap's Sampler: for each level, the first
// `Initial` records are all emitted, and after that one record is emitted every
// `Thereafter` records.
//
// Counting is per-level so that a flood of DEBUG records does not starve
// occasional ERROR records (and vice-versa). Each level has its own atomic
// counter, so sampling is lock-free on the hot path.
//
// A Sampler is created by Logger.WithSampling and stored on the Logger. It is
// safe for concurrent use by multiple goroutines emitting through the same
// Logger (the records channel serializes delivery, but sampling is checked
// before the channel send, so the atomic counter is the only shared state).
type Sampler struct {
	// Initial is the number of leading records emitted at full rate per level
	// before sampling kicks in.
	Initial int
	// Thereafter is the sampling period after Initial: one record is emitted
	// every Thereafter records. Must be >= 1.
	Thereafter int

	// counts[level] is the per-level counter incremented on every delivery
	// attempt. Sized for DEBUG+1 to index directly by level.
	counts [DEBUG + 1]uint64
}

// newSampler builds a Sampler. initial may be 0 (sample from the first record);
// thereafter must be >= 1 (enforced by WithSampling before calling).
func newSampler(initial, thereafter int) *Sampler {
	return &Sampler{Initial: initial, Thereafter: thereafter}
}

// allow reports whether the record at the given level should be emitted under
// the sampling policy. It is the hot-path entry point called from
// deliverRecordToWriter. The decision is deterministic given the counter value:
//
//	count < Initial                  -> emit
//	(count - Initial) % Thereafter == 0 -> emit
//
// so the first Initial records pass, then exactly one in every Thereafter
// thereafter. The counter is incremented unconditionally on every call so the
// sampling decision stays consistent across concurrent emitters.
func (s *Sampler) allow(level int) bool {
	if level < 0 || level >= len(s.counts) {
		return true // defensive: unknown level is never sampled
	}
	c := atomic.AddUint64(&s.counts[level], 1)
	// 1-based c: emit records 1..Initial, then every Thereafter-th after that.
	if c <= uint64(s.Initial) {
		return true
	}
	// Offset by Initial so the period starts cleanly after the burst.
	return (c-uint64(s.Initial))%uint64(s.Thereafter) == 0
}
