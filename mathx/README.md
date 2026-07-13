# mathx

Numeric helpers missing from the standard library: `Sum`, `Product`, `Clamp`,
`Map`, `Lerp`. Pure standard library.

## Quick start

```go
import "github.com/v8fg/kit4go/mathx"

mathx.Sum(1, 2, 3)              // 6
mathx.Product(2, 3, 4)          // 24
mathx.Clamp(75.5, 0.0, 50.0)    // 50
mathx.Map(0.5, 0, 1, 0, 256)   // 128
mathx.Lerp(0.0, 100.0, 0.5)     // 50
```

## API

| Function | Constraint | Description |
|----------|-----------|-------------|
| `Sum(vals...)` | `Numeric` | Sum, empty → 0 |
| `Product(vals...)` | `Numeric` | Product, empty → 1 |
| `Clamp(v, lo, hi)` | `cmp.Ordered` | Clamp to [lo, hi] |
| `Map(v, in..., out...)` | `Float` | Remap between ranges |
| `Lerp(a, b, t)` | `Float` | Linear interpolation |
