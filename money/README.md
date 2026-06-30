# money

Exact, immutable fixed-point monetary arithmetic in an ISO 4217 currency. Pure
standard library (`math/big` concepts over int64, no third-party decimal dep).

## Why

Floating-point money is a billing bug waiting to ship. `0.1 + 0.2 != 0.3`, and
across millions of auctions / payments those lost sub-cents add up — or worse,
an exchange round-trips through a value it never held. `money` stores amounts
as int64 **minor units** (cents), so add / sub / mul-by-int / compare are exact
and allocation-free, and any non-integer result (tax, FX, split) goes through an
explicit rounding mode.

## Currency

A curated ISO 4217 registry ships built-in (USD/EUR/JPY/KWD/…, with the right
minor decimals — 0 for JPY, 3 for KWD). `RegisterCurrency` adds custom or token
currencies. Lookups are case-insensitive.

```go
usd, _ := money.Lookup("USD")        // Currency{Code:"USD", Numeric:"840", Decimals:2}
money.RegisterCurrency(money.Currency{Code:"FOO", Numeric:"000", Decimals:2})
money.MustCurrency("jpy")            // panics if unknown
```

## Construct

```go
a, _ := money.FromMinor(1234, "USD")     // 1234 cents == $12.34 (no parsing)
b, _ := money.FromMajor(12, 34, "USD")   // 12 dollars + 34 cents
c, _ := money.Parse("USD", "12.34")      // from a decimal string
c, _ = money.Parse("USD", "-0.05")

a.String()  // "12.34 USD" (always Decimals digits, code-suffixed)
a.Amount()  // 1234 (minor units)
```

## Arithmetic (exact, immutable)

```go
sum, _   := a.Add(b)     // same currency required (else ErrCurrencyMismatch)
diff, _  := a.Sub(b)
prod, _  := a.Mul(7)     // integer scalar, exact
neg       = a.Negate()
abs       = a.Abs()
cmp, _   := a.Cmp(b)     // -1/0/+1
```

## Rounding (non-integer results)

```go
tax,  _ := price.Scale(1.07, money.RoundHalfUp)   // 7% tax
take, _ := price.Scale(0.15, money.RoundHalfUp)   // 15% take rate
each, _ := total.Div(3, money.RoundHalfEven)       // split, banker's rounding
```

Modes: `RoundHalfUp` (commercial default), `RoundHalfEven` (banker's),
`RoundDown` (truncate), `RoundUp`.

## Allocate (lossless split)

```go
shares, _ := dollar.Allocate([]int{50, 30, 20})   // proportional, total preserved
thirds, _  := dollar.Allocate([]int{1, 1, 1})      // remainder -> largest fractions
// $1.00 into thirds => 34/33/33 cents, never 33/33/33 (+1 lost)
```

## Safety

- `ErrCurrencyMismatch` on cross-currency add/sub/compare (no silent conversion).
- `ErrOverflow` if a result exceeds int64 minor units (the floor, ~9.2e18, is
  far above any fiat amount; for token-scale values use a `math/big` type).
- All operations are immutable and safe for concurrent use (values don't mutate).

## Ad-tech / finance uses

- Exact **eCPM/CPM** rounding, **spend / budget** accounting, **payout splits**.
- Applying **tax / take-rate / FX** with an auditable rounding mode.
- **Allocate** for splitting revenue or cost across parties without losing a cent.

## Testing

92% statement coverage, `-race` clean. Covers currency registry + custom
registration, parse/format (incl. 0- and 3-decimal currencies, negatives),
exact arithmetic, currency-mismatch errors, all four rounding modes (Scale and
Div), Allocate losslessness + remainder distribution, overflow guards, and sign
checks.

```bash
go test -race -cover ./money/...
```
