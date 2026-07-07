package retry

import (
	"context"
	"errors"
	"testing"
)

// BenchmarkDoSuccess measures Do on the first-attempt-success path (no backoff
// wait). Do is stateless — the cost is config build + one fn invocation.
func BenchmarkDoSuccess(b *testing.B) {
	ctx := context.Background()
	fn := func(context.Context) (int, error) { return 1, nil }
	b.ReportAllocs()

	for b.Loop() {
		r := Do(ctx, fn, WithMaxAttempts(3))
		if r.Err != nil {
			b.Fatal(r.Err)
		}
	}
}

// BenchmarkDoRetryableFail measures Do when every attempt fails with a
// retryable error and immediate backoff (no sleep) — the max-attempts loop.
func BenchmarkDoRetryableFail(b *testing.B) {
	ctx := context.Background()
	fail := errors.New("transient")
	fn := func(context.Context) (int, error) { return 0, fail }
	b.ReportAllocs()

	for b.Loop() {
		_ = Do(ctx, fn, WithMaxAttempts(3), WithBackoff(NoBackoff()))
	}
}

// BenchmarkDoPermanentFail measures Do when the error is non-retryable (aborts
// after the first attempt).
func BenchmarkDoPermanentFail(b *testing.B) {
	ctx := context.Background()
	fail := errors.New("permanent")
	fn := func(context.Context) (int, error) { return 0, fail }
	b.ReportAllocs()

	for b.Loop() {
		_ = Do(ctx, fn, WithMaxAttempts(3), WithRetryable(func(error) bool { return false }))
	}
}

// BenchmarkExponentialBackoff measures the pure backoff-function computation.
func BenchmarkExponentialBackoff(b *testing.B) {
	bf := ExponentialBackoff(0, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bf((i % 10) + 1)
	}
}
