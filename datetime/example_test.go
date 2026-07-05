package datetime_test

import (
	"fmt"

	"github.com/v8fg/kit4go/datetime"
)

func ExampleDurationStrToDuration() {
	d, _ := datetime.DurationStrToDuration("500ms")
	fmt.Println(d)
	// Output: 500ms
}
