package retry_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/v8fg/kit4go/retry"
)

func ExampleDo() {
	// Fail the first two attempts, then succeed on the third.
	calls := 0
	fn := func(_ context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("transient")
		}
		return fmt.Sprintf("ok after %d", calls), nil
	}

	r := retry.Do(context.Background(), fn,
		retry.WithMaxAttempts(3),
		retry.WithBackoff(retry.NoBackoff()),
	)
	fmt.Println(r.Value, r.Err, r.Tries)
	// Output: ok after 3 <nil> 3
}
