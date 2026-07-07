package freqcap_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/freqcap"
)

// ExampleCounter shows a per-key frequency cap: a key may produce at most
// maxEvents within the sliding window. Allow returns true while under the cap
// and false once it is reached. A fixed clock keeps the example deterministic.
func ExampleCounter() {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	// At most 3 events per key per 1h window.
	c := freqcap.New(time.Hour, 3, freqcap.WithClock(clock))

	fmt.Println(c.Allow("user-1")) // 1st — recorded
	fmt.Println(c.Allow("user-1")) // 2nd — recorded
	fmt.Println(c.Allow("user-1")) // 3rd — recorded
	fmt.Println(c.Allow("user-1")) // 4th — rejected, cap reached

	// Advance past the window: the oldest events expire, the key may act again.
	now = now.Add(2 * time.Hour)
	fmt.Println(c.Allow("user-1")) // window rolled — recorded
	// Output:
	// true
	// true
	// true
	// false
	// true
}
