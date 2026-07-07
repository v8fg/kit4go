package budget_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/budget"
)

// ExamplePacer shows budget pacing: distribute a total budget evenly across a
// day, then decide whether to throttle (ahead of plan) or push (behind). The
// target is the planned cumulative spend at a fixed point in the day.
func ExamplePacer() {
	pacer, _ := budget.New(1000.0, 24*time.Hour) // 1000 units over a day

	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) // half-way through the day
	fmt.Printf("%.0f\n", pacer.TargetSpend(noon))        // even plan -> 500 at noon
	fmt.Println(pacer.OnPlan(510.0, noon))               // within 5% tolerance -> true
	fmt.Println(pacer.ShouldThrottle(510.0, noon))       // not over tolerance -> false
	fmt.Println(pacer.ShouldThrottle(700.0, noon))       // well ahead of plan -> true
	// Output:
	// 500
	// true
	// false
	// true
}
