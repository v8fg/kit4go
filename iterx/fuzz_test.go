package iterx_test

import (
	"slices"
	"testing"

	"github.com/v8fg/kit4go/iterx"
)

// FuzzPipeline encodes the lazy-combinator contract: a composed pipeline
// Filter -> Map -> Take -> Collect must yield exactly the elements the same
// operations applied eagerly would. The fuzzer supplies the input slice and the
// Take count. E10 invariant-encoding fuzz target.
func FuzzPipeline(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6}, 3)
	f.Fuzz(func(t *testing.T, data []byte, take int) {
		if take < 0 {
			t.Skip()
		}
		s := make([]int, len(data))
		for i, b := range data {
			s[i] = int(b)
		}

		// Pipeline: keep even elements, double them, take the first `take`.
		got := iterx.Collect(
			iterx.Take(
				iterx.Map(
					iterx.Filter(slices.Values(s), func(x int) bool { return x%2 == 0 }),
					func(x int) int { return x * 2 },
				),
				take,
			),
		)

		// Reference: the same operations applied eagerly. Check the take bound
		// BEFORE appending so take=0 yields nothing (matching iterx.Take(n<=0)).
		var want []int
		for _, x := range s {
			if len(want) >= take {
				break
			}
			if x%2 != 0 {
				continue
			}
			want = append(want, x*2)
		}

		if !slices.Equal(got, want) {
			t.Errorf("pipeline mismatch:\n got %v\nwant %v", got, want)
		}
	})
}
