package reservoir_test

import (
	"testing"

	"github.com/v8fg/kit4go/reservoir"
)

// FuzzSampleSizeInvariant verifies the Algorithm R size contract for any
// (capacity, stream-length): after N offers, Count == N and len(Sample()) ==
// min(N, k). A regression in the fill-vs-replace boundary (the first k fill by
// append, each later item replaces a slot with probability k/count) would
// violate the size or count.
func FuzzSampleSizeInvariant(f *testing.F) {
	f.Add(1, 100) // k=1, many offers — every slot is contested
	f.Add(5, 10)
	f.Add(10, 3)  // fewer offers than k — partial fill
	f.Add(50, 50) // exactly k offers — full, no replacement

	f.Fuzz(func(t *testing.T, k, offers int) {
		if k < 1 || k > 100 || offers < 0 || offers > 10000 {
			t.Skip("bounded for a fast fuzz")
		}
		s := reservoir.New[int](k)
		for i := range offers {
			s.Offer(i)
		}
		if got := s.Count(); got != offers {
			t.Fatalf("Count=%d want %d (k=%d)", got, offers, k)
		}
		want := k
		if offers < k {
			want = offers
		}
		sample := s.Sample()
		if len(sample) != want {
			t.Fatalf("Sample len=%d want %d (k=%d offers=%d)", len(sample), want, k, offers)
		}
	})
}
