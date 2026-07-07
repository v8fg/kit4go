package fanout_test

import (
	"fmt"

	"github.com/v8fg/kit4go/fanout"
)

// ExampleFanout broadcasts a message to every subscriber. Publish is
// non-blocking and drops to a full subscriber channel, returning the number of
// subscribers that received the message. With a single subscriber the delivered
// count is deterministic.
func ExampleFanout() {
	f := fanout.New[int]()
	defer f.Close()

	sub := f.Subscribe()
	delivered := f.Publish(42) // broadcast to 1 subscriber
	msg := <-sub.Ch

	fmt.Println(delivered, msg, f.Published())
	// Output: 1 42 1
}
