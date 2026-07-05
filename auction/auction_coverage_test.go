package auction

import (
	"errors"
	"testing"
)

// TestResolveMultiSlot_SlotsNonPositive covers the `slots <= 0` early-return
// branch (returns nil, nil) that the existing TestResolveMultiSlot does not hit.
func TestResolveMultiSlot_SlotsNonPositive(t *testing.T) {
	bids := []Bid{{"a", 100, nil}, {"b", 300, nil}}
	res, err := ResolveMultiSlot(bids, 0, 0)
	if err != nil {
		t.Fatalf("slots=0: unexpected err %v", err)
	}
	if res != nil {
		t.Fatalf("slots=0: expected nil results, got %v", res)
	}
	// Negative slots take the same path.
	res, err = ResolveMultiSlot(bids, 0, -3)
	if err != nil {
		t.Fatalf("slots=-3: unexpected err %v", err)
	}
	if res != nil {
		t.Fatalf("slots=-3: expected nil results, got %v", res)
	}
}

// TestResolveMultiSlot_NoValidBids covers the empty-valid branch (returns
// ErrNoValidBids) for the multi-slot variant.
func TestResolveMultiSlot_NoValidBids(t *testing.T) {
	bids := []Bid{{"a", 50, nil}} // below floor
	_, err := ResolveMultiSlot(bids, 100, 2)
	if !errors.Is(err, ErrNoValidBids) {
		t.Fatalf("expected ErrNoValidBids, got %v", err)
	}
}

// TestResolveMultiSlot_FloorClearingPriceEqualFloor exercises the branch where
// the next bid's price is exactly the floor (clearing stays = floor, not the
// next price). This is the `valid[i+1].Price > floor` false-path inside the loop.
func TestResolveMultiSlot_FloorClearingPriceEqualFloor(t *testing.T) {
	bids := []Bid{
		{"high", 500, nil},
		{"floor-bid", 100, nil}, // price == floor → clearing must stay 100 (floor), not advance
	}
	results, err := ResolveMultiSlot(bids, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 slots, got %d", len(results))
	}
	// Slot 1's clearing is the second bid's price; since 100 == floor, both
	// branches yield 100 — but we still exercise the `> floor` comparison.
	if results[0].ClearingPrice != 100 {
		t.Fatalf("slot 1 clearing = %d, want 100 (floor)", results[0].ClearingPrice)
	}
	// Slot 2 has no third bid → clearing = floor.
	if results[1].ClearingPrice != 100 {
		t.Fatalf("slot 2 clearing = %d, want 100 (floor fallback)", results[1].ClearingPrice)
	}
	if results[1].Winner.Bidder != "floor-bid" {
		t.Fatalf("slot 2 winner = %s, want floor-bid", results[1].Winner.Bidder)
	}
}

// TestResolveMultiSlot_MoreSlotsThanBids covers the loop bound
// `i < slots && i < len(valid)` when slots exceeds the number of valid bids.
func TestResolveMultiSlot_MoreSlotsThanBids(t *testing.T) {
	bids := []Bid{{"only", 500, nil}}
	results, err := ResolveMultiSlot(bids, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result (capped by valid bids), got %d", len(results))
	}
	if results[0].Winner.Bidder != "only" {
		t.Fatalf("winner = %s, want only", results[0].Winner.Bidder)
	}
}

// TestResolveMultiSlot_SingleValidFloorClearing covers the case where there is
// exactly one valid bid and slots >= 1: clearing must equal the floor (the
// `i+1 < len(valid)` condition is false).
func TestResolveMultiSlot_SingleValidFloorClearing(t *testing.T) {
	bids := []Bid{{"solo", 700, nil}}
	results, err := ResolveMultiSlot(bids, 250, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1, got %d", len(results))
	}
	if results[0].ClearingPrice != 250 {
		t.Fatalf("clearing = %d, want floor 250", results[0].ClearingPrice)
	}
}
