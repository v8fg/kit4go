// Package budget paces campaign spend: distribute a total budget across a period
// (a day) according to a target curve, then decide — given actual cumulative
// spend — whether to throttle (you're ahead of plan) or push (you're behind).
//
// Three knobs compose:
//   - averaging: the default even curve (budget spread uniformly over the period)
//   - time-of-day weights: a per-bucket curve (e.g. prime hours get more share),
//     so spend is shaped to expected traffic instead of flat
//   - smoothing: an exponential moving average of the spend rate that damps
//     spikes (a burst of wins does not instantly flip pacing to full-throttle).
//
// Pure standard library. The decisions are O(1); the only state is total budget,
// the normalized weight curve, and an EMA. Ad-tech uses: bid pacing per campaign,
// daily-budget protection against early overspend, prime-hour shaping.
package budget

import (
	"errors"
	"math"
	"sync/atomic"
	"time"
)

// ErrBudget is returned for a non-positive total budget or period.
var ErrBudget = errors.New("budget: total and period must be > 0")

// isFinite reports whether x is a finite float (not NaN, not +/-Inf). Used to
// sanitize API-boundary inputs so a bad spend counter can never make the
// protection primitives fail open (throttle-bypass / full-pace).
func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Pacer distributes a total budget across a period and paces spend against a
// weighted target curve. Safe for concurrent read of decision methods
// (TargetSpend, Deviation, OnPlan, ShouldThrottle, PaceRatio, SmoothedRate,
// Total, Period); record spend (Smooth) from a single accounting path (or guard
// externally). SmoothedRate is safe to call concurrently with Smooth: the
// smoothed rate is stored atomically.
type Pacer struct {
	total      float64       // total budget for the period
	period     time.Duration // period length (e.g. 24h)
	buckets    int           // number of weight buckets (e.g. 24 hourly)
	weight     []float64     // cumulative normalized weights (len buckets+1, [0,1])
	tolerance  float64       // over/under-spend tolerance fraction
	emaAlpha   float64       // EMA smoothing factor for spend rate (0=off, 1=no smoothing)
	emaRate    atomic.Uint64 // smoothed spend rate (budget/sec), stored as Float64bits for lock-free reads
	lastSpend  float64       // last cumulative spend seen by Smooth (writer-only)
	lastTime   time.Time     // last time Smooth was called (writer-only)
	rawWeights []float64     // staged by WithWeights, normalized in buildCurve
}

// Option configures a Pacer.
type Option func(*Pacer)

// WithBuckets sets the number of weight buckets spanning the period (default 24,
// one per hour for a daily period).
func WithBuckets(n int) Option {
	return func(p *Pacer) {
		if n > 0 {
			p.buckets = n
		}
	}
}

// WithWeights sets a per-bucket weight curve (e.g. time-of-day shape). It is
// normalized to sum to 1. If len < buckets, missing buckets are weight 1 (even).
// WithWeights(nil) or all-equal weights reproduces the averaging (even) strategy.
func WithWeights(w []float64) Option {
	return func(p *Pacer) { p.rawWeights = w }
}

// WithTolerance sets the over/under-spend tolerance as a fraction of the target
// (default 0.05 = 5%). Within tolerance, spend is considered on-plan.
func WithTolerance(f float64) Option {
	return func(p *Pacer) {
		if f >= 0 {
			p.tolerance = f
		}
	}
}

// WithSmoothing enables EMA smoothing of the spend rate with the given alpha
// (0 < alpha <= 1; smaller = more smoothing). alpha <= 0 disables smoothing.
func WithSmoothing(alpha float64) Option {
	return func(p *Pacer) { p.emaAlpha = alpha }
}

// New builds a Pacer for total budget over period. Panics-free: invalid input
// returns ErrBudget.
func New(total float64, period time.Duration, opts ...Option) (*Pacer, error) {
	if total <= 0 || period <= 0 {
		return nil, ErrBudget
	}
	p := &Pacer{
		total:     total,
		period:    period,
		buckets:   24,
		tolerance: 0.05,
		emaAlpha:  0, // off by default
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.buckets <= 0 {
		p.buckets = 24
	}
	p.buildCurve()
	return p, nil
}

// buildCurve normalizes the raw weights into a cumulative [0,1] curve of length
// buckets+1 (curve[0]=0, curve[buckets]=1). Missing weights default to even.
func (p *Pacer) buildCurve() {
	w := make([]float64, p.buckets)
	for i := range w {
		v := 1.0
		if i < len(p.rawWeights) {
			v = p.rawWeights[i]
			if v < 0 {
				v = 0
			}
		}
		w[i] = v
	}
	sum := 0.0
	for _, v := range w {
		sum += v
	}
	if sum <= 0 { // all-zero weights -> even
		for i := range w {
			w[i] = 1
		}
		sum = float64(len(w))
	}
	p.weight = make([]float64, p.buckets+1)
	cum := 0.0
	for i, v := range w {
		cum += v / sum
		p.weight[i+1] = cum
	}
	p.weight[p.buckets] = 1 // guard against float drift
}

// dailyPeriod is the exact period length that triggers the wall-clock
// time-of-day daily shape (a 24h campaign aligned to midnight). Non-24h
// periods (including 48h/72h multi-day flights) are NOT shaped as a repeating
// daily curve; they fall through to the arbitrary-period branch so the plan
// advances monotonically across the whole flight instead of resetting every
// midnight. A repeating-daily shape over a multi-day flight is a different,
// opt-in policy and must not be the silent default.
const dailyPeriod = 24 * time.Hour

// fractionOfPeriod returns the elapsed fraction [0,1] of the period at t. For
// an exactly-24h period the wall-clock time-of-day is used so the curve aligns
// to local midnight (the daily shape). For every other period length (sub-day
// or multi-day), the fraction is computed as UnixNano() % period, which makes
// the plan advance linearly across the whole period.
func (p *Pacer) fractionOfPeriod(t time.Time) float64 {
	// Daily shape ONLY for an exactly-24h period. >= 24h multiples (48h, 72h)
	// must NOT reuse the daily curve — they'd reset to 0 every midnight and
	// over-pace the first 24h.
	if p.period == dailyPeriod {
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		f := float64(t.Sub(startOfDay)) / float64(dailyPeriod)
		if f < 0 {
			f = 0
		}
		if f >= 1 {
			f = 0.999999
		}
		return f
	}
	f := float64(t.UnixNano()%int64(p.period)) / float64(p.period)
	if f < 0 {
		f = 0
	}
	return f
}

// targetFraction returns the planned cumulative spend fraction [0,1] at time t
// (the weighted curve value).
func (p *Pacer) targetFraction(t time.Time) float64 {
	f := p.fractionOfPeriod(t)
	idx := f * float64(p.buckets)
	i := int(idx)
	if i >= p.buckets {
		i = p.buckets - 1
	}
	// Linear interpolation between curve[i] and curve[i+1].
	lo := p.weight[i]
	hi := p.weight[i+1]
	frac := idx - float64(i)
	return lo + (hi-lo)*frac
}

// TargetSpend returns the planned cumulative spend at time t.
func (p *Pacer) TargetSpend(t time.Time) float64 { return p.targetFraction(t) * p.total }

// sanitizeSpend normalizes an actualSpend value at the API boundary. A budget
// pacing system is a PROTECTION primitive: it must never fail open (let bids
// through unsuppressed) because a caller handed it a bad spend counter. NaN and
// +/-Inf — which a corrupted/missing spend feed can produce — are mapped to 0 so
// the downstream deviation math is conservative (treated as "no spend seen yet"
// -> behind plan). Negative cumulative spend is undefined for a monotonic
// counter and is likewise clamped to 0. Without this guard, NaN spend made every
// comparison false, so ShouldThrottle(NaN) returned false (full pace) and
// PaceRatio(NaN) returned 1.0 — the exact opposite of safe.
func (p *Pacer) sanitizeSpend(actualSpend float64) float64 {
	if !isFinite(actualSpend) || actualSpend < 0 {
		return 0
	}
	return actualSpend
}

// Deviation returns (actual - planned) / planned at time t. Positive = ahead of
// plan (overspending); negative = behind. NaN/Inf/negative actualSpend is
// clamped to 0 (see sanitizeSpend).
func (p *Pacer) Deviation(actualSpend float64, t time.Time) float64 {
	planned := p.TargetSpend(t)
	if planned <= 0 {
		return 0
	}
	return (p.sanitizeSpend(actualSpend) - planned) / planned
}

// OnPlan reports whether actualSpend is within tolerance of the plan at t.
// NaN/Inf/negative spend is clamped to 0 first, so a bad feed never reads as
// "on plan" by accident (it reads as behind plan).
func (p *Pacer) OnPlan(actualSpend float64, t time.Time) bool {
	return math.Abs(p.Deviation(actualSpend, t)) <= p.tolerance
}

// ShouldThrottle reports whether spend is ahead of plan beyond tolerance (true =
// slow down / shade bids). Near end-of-period with budget left it returns false
// (spend the remainder). NaN/Inf/negative spend is clamped to 0 (behind plan ->
// no throttle); spend at/over total throttles regardless of feed corruption.
func (p *Pacer) ShouldThrottle(actualSpend float64, t time.Time) bool {
	// Non-finite spend is a corrupt feed; throttle-safe is to slow down rather
	// than let the comparison silently evaluate to false (full pace).
	if !isFinite(actualSpend) {
		return true
	}
	spend := p.sanitizeSpend(actualSpend)
	if spend >= p.total {
		return true // budget exhausted
	}
	return p.Deviation(spend, t) > p.tolerance
}

// PaceRatio returns a [0,1] bid-pacing multiplier derived from the deviation:
// 1.0 when on/behind plan, tapering toward 0 as overspend grows. Use it to shade
// bid probability or cap the bid rate. NaN/Inf spend yields 0 (no pace); a
// negative spend is clamped to 0 (behind plan -> full pace).
func (p *Pacer) PaceRatio(actualSpend float64, t time.Time) float64 {
	// Non-finite spend is a corrupt feed; fail closed (0 pace) rather than
	// letting the NaN path fall through to the full-pace branch.
	if !isFinite(actualSpend) {
		return 0
	}
	spend := p.sanitizeSpend(actualSpend)
	if spend >= p.total {
		return 0
	}
	dev := p.Deviation(spend, t)
	if dev <= p.tolerance {
		return 1 // on/behind plan -> full pace
	}
	// Linear taper from tolerance to tolerance+1 (100% over plan -> 0 pace).
	over := dev - p.tolerance
	r := 1 - over
	if r < 0 {
		r = 0
	}
	return r
}

// loadEmaRate returns the smoothed spend rate (budget/sec), reading the
// atomically-stored bits. Safe for concurrent use with Smooth.
func (p *Pacer) loadEmaRate() float64 {
	return math.Float64frombits(p.emaRate.Load())
}

// storeEmaRate publishes the smoothed spend rate atomically. Smooth (the single
// writer) calls this; concurrent SmoothedRate readers observe a consistent
// (non-torn) float64.
func (p *Pacer) storeEmaRate(v float64) {
	p.emaRate.Store(math.Float64bits(v))
}

// Smooth updates the EMA spend rate from the current cumulative spend and time,
// and returns the smoothed spend rate (budget units / second). Call it
// periodically (e.g. each spend tick). With WithSmoothing off it returns the
// instantaneous rate since the last call. Smooth is single-writer; concurrent
// SmoothedRate reads are safe (the rate is stored atomically).
func (p *Pacer) Smooth(actualSpend float64, t time.Time) float64 {
	if p.lastTime.IsZero() {
		p.lastTime = t
		p.lastSpend = actualSpend
		return 0
	}
	dt := t.Sub(p.lastTime).Seconds()
	if dt <= 0 {
		return p.loadEmaRate()
	}
	inst := (actualSpend - p.lastSpend) / dt
	// Sanitize: a non-finite input (NaN/Inf spend, or a pathological dt) must
	// never poison the smoothed rate or leak to callers. Keep the previous rate
	// on bad input rather than propagating NaN/Inf.
	cur := p.loadEmaRate()
	var next float64
	switch {
	case !isFinite(inst):
		next = cur // ignore non-finite instantaneous rate
	case p.emaAlpha <= 0:
		next = inst
	case cur == 0:
		next = inst // seed the EMA on the first finite tick
	default:
		next = p.emaAlpha*inst + (1-p.emaAlpha)*cur
	}
	p.storeEmaRate(next)
	p.lastSpend = actualSpend
	p.lastTime = t
	return next
}

// SmoothedRate returns the last smoothed spend rate without updating it. Safe
// for concurrent use with Smooth.
func (p *Pacer) SmoothedRate() float64 { return p.loadEmaRate() }

// Total returns the configured total budget.
func (p *Pacer) Total() float64 { return p.total }

// Period returns the configured period.
func (p *Pacer) Period() time.Duration { return p.period }
