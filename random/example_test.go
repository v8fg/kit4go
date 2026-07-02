package random_test

import (
	"fmt"

	"github.com/v8fg/kit4go/random"
)

func ExampleNumericCode() {
	// A 6-digit SMS/email verification code (length is deterministic; the
	// digits themselves are random, so only the length is asserted).
	fmt.Println(len(random.NumericCode(6)))
	// Output: 6
}
