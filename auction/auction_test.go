package auction

import (
	"errors"
	"testing"
)

func TestResolve_SecondPrice(t *testing.T) {
	bids := []Bid{
		{"dsp-a", 300, nil},
		{"dsp-b", 500, nil},
		{"dsp-c", 200, nil},
	}
	r, err := Resolve(bids, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Winner.Bidder != "dsp-b" {
		t.Fatalf("winner = %s, want dsp-b", r.Winner.Bidder)
	}
	if r.ClearingPrice != 300 {
		t.Fatalf("clearing = %d, want 300 (second-highest)", r.ClearingPrice)
	}
	if r.ValidCount != 3 {
		t.Fatalf("valid = %d, want 3", r.ValidCount)
	}
}

func TestResolve_SingleBidClearingIsFloor(t *testing.T) {
	bids := []Bid{{"dsp-a", 500, nil}}
	r, err := Resolve(bids, 100)
	if err != nil {
		t.Fatal(err)
	}
	if r.ClearingPrice != 100 {
		t.Fatalf("single bid clearing = %d, want floor 100", r.ClearingPrice)
	}
}

func TestResolve_FloorFilters(t *testing.T) {
	bids := []Bid{
		{"dsp-a", 300, nil},
		{"dsp-b", 500, nil},
		{"dsp-c", 50, nil}, // below floor
	}
	r, err := Resolve(bids, 100)
	if err != nil {
		t.Fatal(err)
	}
	if r.ValidCount != 2 {
		t.Fatalf("valid = %d, want 2", r.ValidCount)
	}
	if r.Winner.Bidder != "dsp-b" {
		t.Fatalf("winner = %s, want dsp-b", r.Winner.Bidder)
	}
	if r.ClearingPrice != 300 {
		t.Fatalf("clearing = %d, want 300", r.ClearingPrice)
	}
}

func TestResolve_NoValidBids(t *testing.T) {
	bids := []Bid{{"dsp-a", 50, nil}}
	_, err := Resolve(bids, 100)
	if !errors.Is(err, ErrNoValidBids) {
		t.Fatalf("expected ErrNoValidBids, got %v", err)
	}
}

func TestResolve_TieStableOrder(t *testing.T) {
	bids := []Bid{
		{"dsp-a", 500, nil},
		{"dsp-b", 500, nil},
	}
	r, err := Resolve(bids, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Stable sort preserves input order for ties → dsp-a wins.
	if r.Winner.Bidder != "dsp-a" {
		t.Fatalf("tie winner = %s, want dsp-a (stable)", r.Winner.Bidder)
	}
	if r.ClearingPrice != 500 {
		t.Fatalf("tie clearing = %d, want 500", r.ClearingPrice)
	}
}

func TestResolveMultiSlot(t *testing.T) {
	bids := []Bid{
		{"a", 100, nil},
		{"b", 300, nil},
		{"c", 200, nil},
		{"d", 50, nil},
	}
	results, err := ResolveMultiSlot(bids, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("slots = %d, want 2", len(results))
	}
	if results[0].Winner.Bidder != "b" || results[0].ClearingPrice != 200 {
		t.Fatalf("slot 1: winner=%s clearing=%d, want b/200", results[0].Winner.Bidder, results[0].ClearingPrice)
	}
	if results[1].Winner.Bidder != "c" || results[1].ClearingPrice != 100 {
		t.Fatalf("slot 2: winner=%s clearing=%d, want c/100", results[1].Winner.Bidder, results[1].ClearingPrice)
	}
}

func TestResolveMultiSlot_FloorClearing(t *testing.T) {
	bids := []Bid{
		{"a", 100, nil},
		{"b", 300, nil},
	}
	results, err := ResolveMultiSlot(bids, 50, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Slot 2 has no third bid → clearing = floor.
	if results[1].ClearingPrice != 50 {
		t.Fatalf("slot 2 clearing = %d, want floor 50", results[1].ClearingPrice)
	}
}

func TestResolve_Payload(t *testing.T) {
	type markup struct{ Ad string }
	bids := []Bid{
		{"dsp-a", 100, markup{Ad: "<html>win</html>"}},
		{"dsp-b", 200, markup{Ad: "<html>higher</html>"}},
	}
	r, _ := Resolve(bids, 0)
	m, ok := r.Winner.Payload.(markup)
	if !ok {
		t.Fatal("payload type mismatch")
	}
	if m.Ad != "<html>higher</html>" {
		t.Fatalf("payload ad = %q", m.Ad)
	}
}

func TestResolve_EmptyBids(t *testing.T) {
	_, err := Resolve(nil, 0)
	if !errors.Is(err, ErrNoValidBids) {
		t.Fatalf("expected ErrNoValidBids, got %v", err)
	}
}
