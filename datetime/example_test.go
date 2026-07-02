package datetime_test

import (
	"fmt"

	"github.com/v8fg/kit4go/datetime"
)

func ExampleDurationStrToDuration() {
	fmt.Println(datetime.DurationStrToDuration("500ms"))
	// Output: 500ms
}
