package hotkey_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/hotkey"
)

// ExampleNew shows a heavy-hitter detector in a 1s window. This example has no
// // Output: comment (Top uses the wall clock and map ordering), so go test
// compiles but does not execute it.
func ExampleNew() {
	d := hotkey.New(time.Second, 3)

	// Touch three keys with different frequencies.
	d.Touch("ssp-a")
	d.Touch("ssp-a")
	d.Touch("ssp-b")
	d.Touch("ssp-c")
	d.Touch("ssp-c")
	d.Touch("ssp-c")

	for _, h := range d.Top() {
		fmt.Println(h.Key, h.Count)
	}
}
