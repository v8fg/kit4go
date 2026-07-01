# breaker: circuit breaker

A generic circuit breaker with closed / open / half-open states, rolling-window
failure counting, bounded half-open admission, and an event hook. Wrap any call
in `Execute`; the breaker short-circuits when the downstream is unhealthy.

## States

- **Closed**: requests pass; failures counted per `Interval` window.
- **Open**: requests rejected for `OpenDuration`; trips when the failure rate
  crosses `FailRate` and at least `MinRequests` samples have been seen.
- **Half-Open**: at most `MaxRequests` trial calls are admitted; success closes,
  failure re-opens.

## Usage

- `NewBreaker[T any](opts BreakerOptions) *Breaker[T]` (generic over the result
  type so `Execute` is allocation-free).
- `(*Breaker).Execute(ctx, func(ctx) (T, error)) (T, error)` run a call under it.
- `(*Breaker).SetOnEvent(func(BreakerEvent))` observe transitions / outcomes.
- `BreakerMetrics` `{ State, Total, Success, Failures, ConsecutiveFail }`.

## BreakerOptions

- `Name`, `MaxRequests` (half-open trial cap), `Interval` (rolling window),
  `OpenDuration` (cool-down), `FailRate` (0-1 trip threshold),
  `MinRequests` (minimum samples before tripping).

## Example

```go
import (
    "context"
    "github.com/v8fg/kit4go/breaker"
)

br := breaker.NewBreaker[string](breaker.BreakerOptions{
    Interval:     10 * time.Second,
    OpenDuration: 30 * time.Second,
    FailRate:     0.5,
    MinRequests:  20,
    MaxRequests:  5,
})
res, err := br.Execute(ctx, func(ctx context.Context) (string, error) {
    return callDownstream(ctx)
})
```
