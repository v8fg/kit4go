package debounce_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/debounce"
)

// ExampleNew coalesces rapid calls into a single execution. This example has no
// // Output: comment (the debounced fn fires on a timer goroutine), so go test
// compiles but does not execute it.
func ExampleNew() {
	d := debounce.New(50*time.Millisecond, func() {
		fmt.Println("saved")
	})

	d.Call()
	d.Call()  // reschedules — only one "saved" fires after the quiet period
	d.Flush() // or run it immediately, cancelling the pending timer
	d.Close()
}
