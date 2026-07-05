package backoff_test

import (
	"fmt"
	"time"

	"github.com/v8fg/kit4go/backoff"
)

func ExampleBackoff_Next() {
	b := backoff.New(
		backoff.WithBase(10*time.Millisecond),
		backoff.WithFactor(2.0),
		backoff.WithMax(time.Hour),
		backoff.WithJitter(backoff.JitterNone),
		backoff.WithMaxAttempts(4),
	)
	for {
		d, ok := b.Next()
		if !ok {
			break
		}
		fmt.Println(d)
	}
	// Output:
	// 10ms
	// 20ms
	// 40ms
	// 80ms
}
