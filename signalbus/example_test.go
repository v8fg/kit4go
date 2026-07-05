package signalbus_test

import (
	"fmt"

	"github.com/v8fg/kit4go/signalbus"
)

// ExampleNew demonstrates the publish/subscribe lifecycle: connect two handlers
// to the same signal, send one event, and observe that dispatch happens in
// registration order. Handlers are synchronous — Send returns only after both
// have printed.
func ExampleNew() {
	bus := signalbus.New()

	// Connect in order; Send invokes in this same order.
	bus.Connect("user.signed_up", func(args ...any) {
		fmt.Println("metrics:", args[0])
	})
	bus.Connect("user.signed_up", func(args ...any) {
		fmt.Println("welcome-email:", args[0])
	})

	// Synchronous dispatch on this goroutine — both prints happen before Send
	// returns.
	bus.Send("user.signed_up", "alice@example.com")

	// Output:
	// metrics: alice@example.com
	// welcome-email: alice@example.com
}
