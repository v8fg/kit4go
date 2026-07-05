package fsm_test

import (
	"fmt"

	"github.com/v8fg/kit4go/fsm"
)

func ExampleNew() {
	m, _ := fsm.New("idle",
		fsm.Rule{From: "idle", Event: "submit", To: "pending"},
		fsm.Rule{From: "pending", Event: "pay", To: "paid"},
		fsm.Rule{From: "pending", Event: "cancel", To: "cancelled"},
	)
	m.Send("submit", nil)
	m.Send("pay", nil)
	fmt.Println(m.Current())
	// Output: paid
}
