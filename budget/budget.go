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
	"time"
)

// ErrBudget is returned for a non-positive total budget or period.
var ErrBudget = errors.New("budget: total and period must be > 0")

// Pacer distributes a total budget across a period and paces spend against a
// weighted target curve. Safe for concurrent read of decision methods; record
// spend (AddSpend) from a single accounting path (or guard externally).
type Pacer struct {
	total      float64       // total budget for the period
	period     time.Duration // period length (e.g. 24h)
	buckets    int           // number of weight buckets (e.g. 24 hourly)
	weight     []float64     // cumulative normalized weights (len buckets+1, [0,1])
	tolerance  float64       // over/under-spend tolerance fraction
	emaAlpha   float64       // EMA smoothing factor for spend rate (0=off, 1=no smoothing)
	emaRate    float64       // smoothed spend rate (budget/sec)
	lastSpend  float64       // last cumulative spend seen by Smooth
	lastTime   time.Time     // last time Smooth was called
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

// fractionOfDay returns the elapsed fraction [0,1] of the period at t, using t's
// time-of-day for a daily period.
func (p *Pacer) fractionOfPeriod(t time.Time) float64 {
	// For a daily period, use the wall-clock fraction of the day; for other
	// periods, use the fraction since the period's start boundary. We anchor to
	// midnight of t's day for a daily shape.
	if p.period%time.Hour == 0 && p.period >= 24*time.Hour {
		startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		f := float64(t.Sub(startOfDay)) / float64(24*time.Hour)
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

// Deviation returns (actual - planned) / planned at time t. Positive = ahead of
// plan (overspending); negative = behind.
func (p *Pacer) Deviation(actualSpend float64, t time.Time) float64 {
	planned := p.TargetSpend(t)
	if planned <= 0 {
		return 0
	}
	return (actualSpend - planned) / planned
}

// OnPlan reports whether actualSpend is within tolerance of the plan at t.
func (p *Pacer) OnPlan(actualSpend float64, t time.Time) bool {
	return math.Abs(p.Deviation(actualSpend, t)) <= p.tolerance
}

// ShouldThrottle reports whether spend is ahead of plan beyond tolerance (true =
// slow down / shade bids). Near end-of-period with budget left it returns false
// (spend the remainder).
func (p *Pacer) ShouldThrottle(actualSpend float64, t time.Time) bool {
	if actualSpend >= p.total {
		return true // budget exhausted
	}
	return p.Deviation(actualSpend, t) > p.tolerance
}

// PaceRatio returns a [0,1] bid-pacing multiplier derived from the deviation:
// 1.0 when on/behind plan, tapering toward 0 as overspend grows. Use it to shade
// bid probability or cap the bid rate.
func (p *Pacer) PaceRatio(actualSpend float64, t time.Time) float64 {
	if actualSpend >= p.total {
		return 0
	}
	dev := p.Deviation(actualSpend, t)
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

// Smooth updates the EMA spend rate from the current cumulative spend and time,
// and returns the smoothed spend rate (budget units / second). Call it
// periodically (e.g. each spend tick). With WithSmoothing off it returns the
// instantaneous rate since the last call.
func (p *Pacer) Smooth(actualSpend float64, t time.Time) float64 {
	if p.lastTime.IsZero() {
		p.lastTime = t
		p.lastSpend = actualSpend
		return 0
	}
	dt := t.Sub(p.lastTime).Seconds()
	if dt <= 0 {
		return p.emaRate
	}
	inst := (actualSpend - p.lastSpend) / dt
	if p.emaAlpha <= 0 {
		p.emaRate = inst
	} else if p.emaRate == 0 {
		p.emaRate = inst
	} else {
		p.emaRate = p.emaAlpha*inst + (1-p.emaAlpha)*p.emaRate
	}
	p.lastSpend = actualSpend
	p.lastTime = t
	return p.emaRate
}

// SmoothedRate returns the last smoothed spend rate without updating it.
func (p *Pacer) SmoothedRate() float64 { return p.emaRate }

// Total returns the configured total budget.
func (p *Pacer) Total() float64 { return p.total }

// Period returns the configured period.
func (p *Pacer) Period() time.Duration { return p.period }
