package hyperloglog_test

import (
	"fmt"

	"github.com/v8fg/kit4go/hyperloglog"
)

// Estimate is approximate (~0.8% at precision 14), so the example has no
// deterministic Output assertion — it shows the usage, not a checked result.
func ExampleHyperLogLog() {
	h, _ := hyperloglog.New(14) // precision 14: ~16 KB registers, ~0.8% error
	for i := 0; i < 100_000; i++ {
		h.AddString(fmt.Sprintf("user-%d", i))
	}
	fmt.Printf("estimated distinct users: %.0f\n", h.Estimate())
}
