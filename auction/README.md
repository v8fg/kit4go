# auction

Second-price (Vickrey) auction resolution for RTB. Winner is the highest bidder; clearing price is max(second-highest, floor). Supports multi-slot and floor filtering. Pure standard library.

## Usage

- `Resolve(bids []Bid, floor int64) (Result, error)` — single-winner auction.
- `ResolveMultiSlot(bids []Bid, floor int64, slots int) ([]Result, error)` — multi-position.
- `Bid{Bidder, Price, Payload}` — carries creative ID / ad markup / deal ID.

## Example

```go
bids := []auction.Bid{
    {"dsp-a", 300, creativeA},
    {"dsp-b", 500, creativeB},
    {"dsp-c", 200, nil},
}
result, _ := auction.Resolve(bids, 100)
// Winner: dsp-b, ClearingPrice: 300 (second-highest), ValidCount: 3
```
