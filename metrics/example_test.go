package metrics_test

import (
	"fmt"

	"github.com/v8fg/kit4go/metrics"
)

// ExampleRegistry builds an isolated registry, registers a counter, and reads
// its value back via Gather. The counter value is deterministic after a known
// number of Add calls.
func ExampleRegistry() {
	reg := metrics.NewRegistry()
	requests := reg.NewCounter("requests_total", "total requests", "method")

	requests.WithLabelValues("GET").Add(5)
	requests.WithLabelValues("GET").Inc()

	families, _ := reg.Gather()
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			fmt.Printf("%s GET %g\n", mf.GetName(), m.GetCounter().GetValue())
		}
	}
	// Output:
	// requests_total GET 6
}
