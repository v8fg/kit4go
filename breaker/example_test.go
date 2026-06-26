package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/v8fg/kit4go/breaker"
)

// ExampleNewBreaker shows the default construction and that a freshly built
// breaker starts in the closed state.
func ExampleNewBreaker() {
	b := breaker.NewBreaker[string](breaker.BreakerOptions{
		Name:         "billing",
		MaxRequests:  5,
		Interval:     60 * time.Second,
		OpenDuration: 30 * time.Second,
		FailRate:     0.5,
		MinRequests:  10,
	})

	fmt.Println(b.State())

	// output:
	// closed
}

// ExampleBreaker_Execute wraps a flaky operation. When fn succeeds the breaker
// forwards its value; when fn fails the error is returned verbatim while the
// breaker records the failure internally.
func ExampleBreaker_Execute() {
	b := breaker.NewBreaker[int](breaker.BreakerOptions{
		Interval:     time.Second,
		OpenDuration: 50 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
		MaxRequests:  2,
	})

	// A successful call returns the value and nil error.
	v, err := b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 42, nil
	})
	fmt.Println("success:", v, err)

	// A failing call returns its error unchanged.
	_, err = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errors.New("downstream error")
	})
	fmt.Println("failure:", err)

	// output:
	// success: 42 <nil>
	// failure: downstream error
}

// ExampleBreaker_State shows the state lifecycle: closed under normal load,
// open once the failure rate trips the breaker, half_open while probing.
func ExampleBreaker_State() {
	b := breaker.NewBreaker[int](breaker.BreakerOptions{
		Interval:     time.Second,
		OpenDuration: 30 * time.Millisecond,
		FailRate:     0.5,
		MinRequests:  4,
		MaxRequests:  2,
	})

	fmt.Println(b.State()) // closed at startup

	// Drive the failure rate past the threshold to trip the breaker.
	for i := 0; i < 4; i++ {
		_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errors.New("boom")
		})
	}
	fmt.Println(b.State()) // now open

	// Once OpenDuration elapses the next call moves it to half_open.
	time.Sleep(60 * time.Millisecond)
	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return 1, nil })
	fmt.Println(b.State()) // probing

	// output:
	// closed
	// open
	// half_open
}

// ExampleBreaker_Metrics reads the lifetime counters after a small burst of
// traffic. The snapshot is a best-effort observation of total/success/failures
// plus the current consecutive-failure run.
func ExampleBreaker_Metrics() {
	b := breaker.NewBreaker[int](breaker.BreakerOptions{
		Interval:    time.Second,
		FailRate:    0.9, // high: don't trip in this example
		MinRequests: 100,
	})

	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return 1, nil })
	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) { return 2, nil })
	_, _ = b.Execute(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errors.New("boom")
	})

	m := b.Metrics()
	fmt.Printf("total=%d success=%d failures=%d consecutive=%d state=%s\n",
		m.Total, m.Success, m.Failures, m.ConsecutiveFail, m.State)

	// output:
	// total=3 success=2 failures=1 consecutive=1 state=closed
}
