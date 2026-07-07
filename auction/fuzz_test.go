package auction

import (
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"testing"
)

// decodePrices parses a byte slice as a sequence of little-endian int64 prices
// (8 bytes each). Any trailing partial int64 is dropped. An empty or short input
// yields an empty slice — exercising the empty-bids path of Resolve.
func decodePrices(b []byte) []int64 {
	n := len(b) / 8
	if n == 0 {
		return nil
	}
	out := make([]int64, n)
	for i := range n {
		out[i] = int64(binary.LittleEndian.Uint64(b[i*8 : i*8+8]))
	}
	return out
}

// int64Seed packs int64 prices into a little-endian []byte suitable as an
// f.Add seed (native fuzz only accepts []byte slices, not []int64).
func int64Seed(prices ...int64) []byte {
	b := make([]byte, len(prices)*8)
	for i, p := range prices {
		binary.LittleEndian.PutUint64(b[i*8:], uint64(p))
	}
	return b
}

// FuzzResolve exercises the single-slot second-price auction against arbitrary
// bid prices and floors. Prices are decoded from a []byte corpus (each 8-byte
// little-endian chunk is one int64) so the fuzzer can mutate magnitude/sign.
//
// Invariants checked:
//  1. No panic for any input (empty/huge slices, negative prices, overflow).
//  2. Either ErrNoValidBids, or a non-nil winner whose price >= floor and
//     clearing price in [floor, winner.Price].
//  3. The winner is the highest-priced valid bid (>= floor).
//  4. Clearing == max(second-highest valid price, floor).
//  5. ValidCount / BidCount match the input.
func FuzzResolve(f *testing.F) {
	f.Add(int64Seed(300, 500, 200), int64(0))    // classic second-price
	f.Add(int64Seed(500), int64(100))            // single bid → clearing = floor
	f.Add(int64Seed(50), int64(100))             // all below floor → ErrNoValidBids
	f.Add(int64Seed(500, 500), int64(0))         // tie → stable order
	f.Add([]byte{}, int64(0))                    // empty bids
	f.Add(int64Seed(-5, -10, 0), int64(0))       // non-positive prices
	f.Add(int64Seed(math.MaxInt64, 1), int64(0)) // overflow boundary
	f.Add(int64Seed(100, 200, 300), int64(250))  // floor above some bids
	f.Add(int64Seed(1, 2, 3, 4, 5), int64(-1))   // negative floor (no filtering)

	f.Fuzz(func(t *testing.T, priceBlob []byte, floor int64) {
		// Cap decoded size so the fuzzer's large inputs stay tractable.
		if len(priceBlob) > 1<<17 { // 131072 bytes = up to 16384 int64s
			t.Skip("price blob too large for fuzz target")
		}

		prices := decodePrices(priceBlob)
		bids := make([]Bid, len(prices))
		for i, p := range prices {
			bids[i] = Bid{Bidder: "dsp", Price: p, Payload: nil}
		}

		// Invariant 1: never panics.
		r, err := Resolve(bids, floor)

		// Recompute the valid count independently of the impl.
		validCount := 0
		for _, p := range prices {
			if p >= floor {
				validCount++
			}
		}

		if validCount == 0 {
			if !errors.Is(err, ErrNoValidBids) {
				t.Fatalf("validCount=0: want ErrNoValidBids, got err=%v result=%+v", err, r)
			}
			if r.Winner != nil {
				t.Fatalf("validCount=0: expected nil winner, got %v", r.Winner)
			}
			if r.BidCount != len(prices) {
				t.Fatalf("BidCount=%d, want %d", r.BidCount, len(prices))
			}
			return
		}

		// Invariant 2: valid bids → no error, winner present.
		if err != nil {
			t.Fatalf("validCount=%d: unexpected err %v", validCount, err)
		}
		if r.Winner == nil {
			t.Fatalf("validCount=%d: nil winner", validCount)
		}
		if r.ValidCount != validCount {
			t.Fatalf("ValidCount=%d, want %d", r.ValidCount, validCount)
		}
		if r.BidCount != len(prices) {
			t.Fatalf("BidCount=%d, want %d", r.BidCount, len(prices))
		}

		// Invariant 3: winner price is the max valid price.
		maxValid := floor
		for _, p := range prices {
			if p >= floor && p > maxValid {
				maxValid = p
			}
		}
		if r.Winner.Price != maxValid {
			t.Fatalf("winner.Price=%d, want max valid %d", r.Winner.Price, maxValid)
		}
		if r.Winner.Price < floor {
			t.Fatalf("winner.Price=%d < floor %d", r.Winner.Price, floor)
		}

		// Invariant 4: clearing == max(second-highest valid, floor).
		top, second := int64(math.MinInt64), int64(math.MinInt64)
		for _, p := range prices {
			if p < floor {
				continue
			}
			if p > top {
				second = top
				top = p
			} else if p > second {
				second = p
			}
		}
		wantClearing := floor
		if second > floor {
			wantClearing = second
		}
		if r.ClearingPrice != wantClearing {
			t.Fatalf("ClearingPrice=%d, want %d (second=%d floor=%d)",
				r.ClearingPrice, wantClearing, second, floor)
		}
		if r.ClearingPrice < floor || r.ClearingPrice > r.Winner.Price {
			t.Fatalf("ClearingPrice=%d out of [floor=%d, winner=%d]",
				r.ClearingPrice, floor, r.Winner.Price)
		}
	})
}

// FuzzResolveMultiSlotConsistency cross-checks the multi-slot resolver against
// the single-slot resolver and against the GSP ordering invariants.
//
// Invariants checked:
//  1. No panic for any slots value (negative, zero, huge).
//  2. slots <= 0 → (nil, nil).
//  3. winners are non-increasing in price (ties resolved by stable order).
//  4. slot 0 winner == Resolve() winner.
//  5. Each slot's clearing == max(next sorted valid price, floor), or floor
//     when no further valid bid exists (mirrors ResolveMultiSlot's loop).
//  6. result count == min(slots, validCount).
func FuzzResolveMultiSlotConsistency(f *testing.F) {
	f.Add(int64Seed(100, 300, 200, 50), int64(0), 2)
	f.Add(int64Seed(100, 300), int64(50), 2)
	f.Add(int64Seed(500, 100), int64(100), 2)
	f.Add(int64Seed(500), int64(0), 5)
	f.Add(int64Seed(700), int64(250), 1)
	f.Add(int64Seed(300, 200, 100), int64(150), 3)
	f.Add(int64Seed(50), int64(100), 2)        // below floor
	f.Add([]byte{}, int64(0), 1)               // empty bids
	f.Add(int64Seed(10, 20, 30), int64(0), 0)  // zero slots
	f.Add(int64Seed(10, 20, 30), int64(0), -1) // negative slots

	f.Fuzz(func(t *testing.T, priceBlob []byte, floor int64, slots int) {
		if len(priceBlob) > 1<<17 {
			t.Skip("price blob too large for fuzz target")
		}

		prices := decodePrices(priceBlob)
		bids := make([]Bid, len(prices))
		for i, p := range prices {
			bids[i] = Bid{Bidder: "dsp", Price: p, Payload: nil}
		}

		// Invariant 1: never panics.
		results, err := ResolveMultiSlot(bids, floor, slots)

		// Invariant 2: slots <= 0 → nil, nil regardless of bids.
		if slots <= 0 {
			if err != nil {
				t.Fatalf("slots=%d: unexpected err %v", slots, err)
			}
			if results != nil {
				t.Fatalf("slots=%d: expected nil results, got %v", slots, results)
			}
			return
		}

		validCount := 0
		for _, p := range prices {
			if p >= floor {
				validCount++
			}
		}

		if validCount == 0 {
			if !errors.Is(err, ErrNoValidBids) {
				t.Fatalf("validCount=0: want ErrNoValidBids, got err=%v", err)
			}
			if results != nil {
				t.Fatalf("validCount=0: expected nil results, got %v", results)
			}
			return
		}

		if err != nil {
			t.Fatalf("validCount=%d: unexpected err %v", validCount, err)
		}

		// Invariant 6: result count == min(slots, validCount).
		wantLen := slots
		if validCount < wantLen {
			wantLen = validCount
		}
		if len(results) != wantLen {
			t.Fatalf("len(results)=%d, want %d (slots=%d valid=%d)",
				len(results), wantLen, slots, validCount)
		}
		if wantLen == 0 {
			return
		}

		// Invariant 4: slot 0 winner == Resolve() winner.
		single, singleErr := Resolve(bids, floor)
		if singleErr != nil {
			t.Fatalf("single Resolve err where multi succeeded: %v", singleErr)
		}
		if single.Winner == nil || results[0].Winner == nil {
			t.Fatalf("nil winner: single=%v results[0]=%v", single.Winner, results[0].Winner)
		}
		if results[0].Winner.Price != single.Winner.Price ||
			results[0].Winner.Bidder != single.Winner.Bidder {
			t.Fatalf("slot0 winner=%+v != Resolve winner=%+v",
				results[0].Winner, single.Winner)
		}

		// Independently reconstruct the descending-sorted valid prices so we can
		// check per-slot clearing without aliasing the implementation's sort.
		validSorted := make([]int64, 0, validCount)
		for _, p := range prices {
			if p >= floor {
				validSorted = append(validSorted, p)
			}
		}
		sort.SliceStable(validSorted, func(i, j int) bool {
			return validSorted[i] > validSorted[j]
		})

		// Invariant 3: non-increasing winner prices; clearing within [floor, winner].
		// Invariant 5: slot i clearing == max(validSorted[i+1], floor), or floor
		// when i+1 exceeds the valid set (mirrors ResolveMultiSlot's logic).
		for i, res := range results {
			if res.Winner == nil {
				t.Fatalf("slot %d: nil winner", i)
			}
			if res.Winner.Price < floor {
				t.Fatalf("slot %d: winner.Price=%d < floor %d", i, res.Winner.Price, floor)
			}
			if res.Winner.Price != validSorted[i] {
				t.Fatalf("slot %d: winner.Price=%d, want sorted[%d]=%d",
					i, res.Winner.Price, i, validSorted[i])
			}
			if i > 0 && results[i-1].Winner.Price < res.Winner.Price {
				t.Fatalf("ordering broken: slot[%d].Price=%d < slot[%d].Price=%d",
					i-1, results[i-1].Winner.Price, i, res.Winner.Price)
			}
			if res.ClearingPrice < floor || res.ClearingPrice > res.Winner.Price {
				t.Fatalf("slot %d: clearing=%d out of [floor=%d, winner=%d]",
					i, res.ClearingPrice, floor, res.Winner.Price)
			}
			wantClearing := floor
			if i+1 < len(validSorted) && validSorted[i+1] > floor {
				wantClearing = validSorted[i+1]
			}
			if res.ClearingPrice != wantClearing {
				t.Fatalf("slot %d: clearing=%d, want %d (nextSorted=%v floor=%d)",
					i, res.ClearingPrice, wantClearing, validSorted, floor)
			}
			if res.BidCount != len(prices) || res.ValidCount != validCount {
				t.Fatalf("slot %d: meta mismatch BidCount=%d/%d ValidCount=%d/%d",
					i, res.BidCount, len(prices), res.ValidCount, validCount)
			}
		}
	})
}
