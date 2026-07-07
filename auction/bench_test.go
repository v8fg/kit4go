package auction

import "testing"

func BenchmarkResolve_Small(b *testing.B) {
	bids := []Bid{{"a", 100, nil}, {"b", 300, nil}, {"c", 200, nil}}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Resolve(bids, 0)
	}
}

func BenchmarkResolve_Large(b *testing.B) {
	bids := make([]Bid, 100)
	for i := range bids {
		bids[i] = Bid{Bidder: "dsp", Price: int64(i), Payload: nil}
	}
	b.ReportAllocs()

	for b.Loop() {
		_, _ = Resolve(bids, 0)
	}
}
