# decimal

Arbitrary-precision decimal arithmetic via `math/big`. Immutable, scale-fixed, exact (no float drift). Pure standard library.

## Usage

- `Parse(s string, scale int) (Decimal, error)` / `MustParse(s, scale) Decimal`.
- `New(unscaled int64, scale int) Decimal` / `FromInt(v int64) Decimal`.
- `.Add/Sub/Mul/MulDecimal/Div/Cmp/Negate/Abs/Rescale`.
- `.String()` renders with exactly scale decimal places. Zero-value `Decimal{}` is safe (renders "0").

## Example

```go
price := decimal.MustParse("12.50", 2)
tax := decimal.MustParse("0.08", 2)
total, _ := price.Add(price.MulDecimal(tax).Rescale(2))
fmt.Println(total) // 13.50
```
