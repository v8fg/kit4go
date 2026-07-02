package money_test

import (
	"fmt"

	"github.com/v8fg/kit4go/money"
)

func ExampleMustFromMinor() {
	// 1234 minor units (cents) of USD renders in major units.
	m := money.MustFromMinor(1234, "USD")
	fmt.Println(m)
	// Output: 12.34 USD
}
