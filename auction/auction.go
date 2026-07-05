// Package auction implements second-price (Vickrey) auction resolution for
// ad-tech bidding. Given a set of bids and an optional floor price, it selects
// the winner and computes the clearing price (second-highest bid, or floor if
// only one bid meets the floor). Pure standard library.
//
// Ad-tech uses: RTB auction resolution — the highest bidder wins but pays the
// second-highest bid price (generalized second-price auction), which is the
// standard mechanism in most ad exchanges.
package auction

import (
	"errors"
	"sort"
)

// Bid represents a single bid in an auction.
type Bid struct {
	Bidder  string // bidder identifier (DSP name, seat ID)
	Price   int64  // bid price in minor units (cents, micros, etc.)
	Payload any    // optional: creative ID, ad markup, deal ID
}

// ErrNoValidBids is returned when no bid meets the floor price.
var ErrNoValidBids = errors.New("auction: no bids above floor")

// Result is the outcome of an auction.
type Result struct {
	Winner        *Bid  // the winning bid (nil if no valid bids)
	ClearingPrice int64 // the price the winner pays (second-highest or floor)
	BidCount      int   // total bids evaluated
	ValidCount    int   // bids that met the floor
}

// Resolve runs a second-price auction on the given bids. Only bids with Price >=
// floor participate. The winner is the highest bidder; the clearing price is
// max(second-highest valid bid, floor). If only one bid is valid, clearing =
// floor. Returns ErrNoValidBids if no bid meets the floor.
func Resolve(bids []Bid, floor int64) (Result, error) {
	// Filter by floor.
	valid := make([]Bid, 0, len(bids))
	for _, b := range bids {
		if b.Price >= floor {
			valid = append(valid, b)
		}
	}
	if len(valid) == 0 {
		return Result{BidCount: len(bids)}, ErrNoValidBids
	}
	// Sort descending by price (stable to preserve input order for ties).
	sort.SliceStable(valid, func(i, j int) bool {
		return valid[i].Price > valid[j].Price
	})

	winner := valid[0] // copy to avoid aliasing local slice
	clearing := floor
	if len(valid) > 1 && valid[1].Price > floor {
		clearing = valid[1].Price
	}
	return Result{
		Winner:        &winner,
		ClearingPrice: clearing,
		BidCount:      len(bids),
		ValidCount:    len(valid),
	}, nil
}

// ResolveMultiSlot runs a multi-slot auction: selects the top N winners and
// their clearing prices. Slot i's clearing price is the (i+1)-th highest bid,
// or floor if fewer than i+2 valid bids. Returns ErrNoValidBids if no bid meets
// the floor.
func ResolveMultiSlot(bids []Bid, floor int64, slots int) ([]Result, error) {
	if slots <= 0 {
		return nil, nil
	}
	valid := make([]Bid, 0, len(bids))
	for _, b := range bids {
		if b.Price >= floor {
			valid = append(valid, b)
		}
	}
	if len(valid) == 0 {
		return nil, ErrNoValidBids
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return valid[i].Price > valid[j].Price
	})

	results := make([]Result, 0, slots)
	for i := 0; i < slots && i < len(valid); i++ {
		clearing := floor
		if i+1 < len(valid) && valid[i+1].Price > floor {
			clearing = valid[i+1].Price
		}
		w := valid[i] // copy to avoid aliasing
		results = append(results, Result{
			Winner:        &w,
			ClearingPrice: clearing,
			BidCount:      len(bids),
			ValidCount:    len(valid),
		})
	}
	return results, nil
}
