# stats

Basic descriptive statistics over `[]float64`. Pure standard library.

Go's standard library has no statistics package. This fills the gap with the
everyday aggregates used in metrics, monitoring, and finance: mean, median,
mode, variance, standard deviation, percentile, min, max, range, sum.

## Quick start

```go
import "github.com/v8fg/kit4go/stats"

latency := []float64{12.1, 14.3, 11.8, 13.0, 99.2}

stats.Mean(latency)            // 30.08 — dragged by the 99.2 outlier
stats.Median(latency)          // 13.00 — robust to outliers
stats.Percentile(latency, 95)  // p95
stats.StdDev(latency)          // dispersion
stats.Min(latency)             // 11.8
stats.Max(latency)             // 99.2
```

## API

| Function | Description |
|----------|-------------|
| `Sum(s)` | Sum of all values (0 for empty) |
| `Mean(s)` | Arithmetic mean — NaN if empty |
| `Median(s)` | Middle value (avg of two middle for even length); sorts a copy |
| `Mode(s)` | Most frequent value; ties → smallest among modes |
| `Variance(s)` | Population variance (÷N) — NaN if empty |
| `StdDev(s)` | Population standard deviation — NaN if empty |
| `Percentile(s, p)` | p-th percentile (0–100), linear interpolation |
| `Min(s)` / `Max(s)` | Extreme values — NaN if empty |
| `Range(s)` | `Max - Min` — NaN if empty |

## Notes

- **Empty input**: every aggregate except `Sum` returns `NaN`; `Sum` returns `0`
  (a sum over no elements is the additive identity).
- **Variance** is the *population* variance (divides by N). For sample variance
  (÷(N−1)) multiply by `N/(N−1)`.
- `Median` and `Percentile` sort a **copy** — the input slice is never mutated.
- `Mode` on a tie returns the **smallest** value among the most frequent, for
  deterministic output.
- `Percentile` uses **linear interpolation** between the two closest ranks
  (the method used by NumPy's default and most spreadsheet engines).
- Inputs containing NaN/Inf will propagate NaN through the aggregate; sanitize
  upstream if that is undesired.
