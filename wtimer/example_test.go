package wtimer_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/wtimer"
)

// ExampleNew schedules a one-shot callback on the timer wheel. This example has
// no // Output: comment (the callback fires on a background goroutine after a
// real delay), so go test compiles but does not execute it.
func ExampleNew() {
	w := wtimer.New()
	defer w.Close()

	_, _ = w.Add(10*time.Millisecond, func() {
		fmt.Println("fired")
	})

	time.Sleep(30 * time.Millisecond) // allow the wheel goroutine to fire
}
