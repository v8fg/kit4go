package auction_test

import (
	"fmt"

	"github.com/v8fg/kit4go/auction"
)

func ExampleResolve() {
	bids := []auction.Bid{
		{"dsp-a", 300, nil},
		{"dsp-b", 500, nil},
		{"dsp-c", 200, nil},
	}
	r, _ := auction.Resolve(bids, 100)
	fmt.Println(r.Winner.Bidder, r.ClearingPrice)
	// Output: dsp-b 300
}
