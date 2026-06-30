# budget

Paces campaign spend: distributes a total budget across a period according to a
target curve, then decides — given actual cumulative spend — whether to throttle
(ahead of plan) or push (behind). Pure standard library.

## Three strategies compose

- **Averaging** (default): budget spread uniformly → linear target curve.
- **Time-of-day weights** (`WithWeights`): a per-hour curve so spend is shaped to
  expected traffic (prime hours get more share) instead of flat.
- **Smoothing** (`WithSmoothing`): an EMA of the spend rate that damps spikes — a
  burst of wins does not instantly flip pacing to full-throttle.

## API

```go
p, _ := budget.New(240.0, 24*time.Hour,
    budget.WithWeights(primeHourWeights),   // 24-element hourly curve
    budget.WithTolerance(0.05),              // ±5% of plan = on-plan
    budget.WithSmoothing(0.3),               // EMA alpha
)
plan   := p.TargetSpend(time.Now())          // planned cumulative spend
throt  := p.ShouldThrottle(actualSpend, time.Now()) // true = slow down
ratio  := p.PaceRatio(actualSpend, time.Now())      // [0,1] bid-pacing multiplier
smoothed := p.Smooth(actualSpend, time.Now())       // updates + returns EMA rate
```

| Symbol | Behavior |
|---|---|
| `New(total, period, opts...)` | Build a Pacer |
| `TargetSpend(t)` | Planned cumulative spend at time t (the weighted curve) |
| `Deviation(actual, t)` | `(actual - planned) / planned` (positive = ahead) |
| `OnPlan(actual, t)` | Within tolerance of the plan |
| `ShouldThrottle(actual, t)` | Ahead of plan beyond tolerance (or budget exhausted) |
| `PaceRatio(actual, t)` | [0,1] bid multiplier (1 = full pace, 0 = stop) |
| `Smooth(actual, t)` | Update EMA spend rate; returns smoothed rate (budget/sec) |

## Ad-tech uses

- **Bid pacing**: throttle bid participation when a campaign is ahead of its
  daily spend plan; open up when behind.
- **Daily-budget protection**: prevents early-morning overspend that leaves
  nothing for prime hours.
- **Prime-hour shaping**: weight the curve so more budget is reserved for
  high-value hours.
- **Spend-rate smoothing**: EMA damps short-term win bursts so pacing decisions
  are stable.

## Testing

91% statement coverage, `-race` clean. Covers the even (linear) curve, prime-hour
weighted shaping, throttle decisions (ahead / on-plan / behind / exhausted),
PaceRatio tapering, deviation sign, tolerance band, EMA damping (two-tick spike
test), smoothing-off (instantaneous), non-daily periods, all-zero-weight fallback
to even, and invalid-input guards.

```bash
go test -race -cover ./budget/...
```
