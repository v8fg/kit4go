// Package retry provides a composable retry helper: call a function with
// configurable backoff, max attempts, and error classification (retryable vs
// not). Pure standard library.
//
// It composes with kit4go/backoff for the delay strategy and lets callers
// classify errors via a RetryableFunc predicate — only retryable errors trigger
// another attempt; permanent errors abort immediately.
//
// Ad-tech uses: retrying transient SSP/broker/DB failures without hammering
// (backoff), while aborting immediately on permanent errors (400, auth, schema
// mismatch). Pairs naturally with kit4go/breaker for circuit-level protection.
package retry

import (
	"context"
	"errors"
	"time"
)

// ErrMaxAttempts is returned when all attempts are exhausted.
var ErrMaxAttempts = errors.New("retry: max attempts reached")

// Result holds the outcome of a Do call.
type Result[T any] struct {
	Value T
	Err   error
	Tries int
}

// Config controls retry behavior.
type Config struct {
	MaxAttempts int              // total attempts (including the first); 0 = unlimited
	Backoff     Backoff          // delay strategy (default: none/immediate)
	IsRetryable func(error) bool // classify errors; nil = retry all
}

// Backoff returns the delay before the next attempt (0 = retry immediately).
type Backoff func(attempt int) time.Duration

// ConstantBackoff returns a fixed delay between attempts.
func ConstantBackoff(d time.Duration) Backoff {
	return func(_ int) time.Duration { return d }
}

// ExponentialBackoff returns base * 2^(attempt-1), capped at max.
func ExponentialBackoff(base, max time.Duration) Backoff {
	return func(attempt int) time.Duration {
		if attempt > 30 {
			return max // 2^30 * any ns base saturates; avoid overflow
		}
		d := base << (attempt - 1)
		if d > max || d <= 0 {
			return max
		}
		return d
	}
}

// NoBackoff retries immediately (delay = 0).
func NoBackoff() Backoff { return func(_ int) time.Duration { return 0 } }

// Option configures Config.
type Option func(*Config)

// WithMaxAttempts sets the total attempt count (including the first).
func WithMaxAttempts(n int) Option { return func(c *Config) { c.MaxAttempts = n } }

// WithBackoff sets the backoff strategy.
func WithBackoff(b Backoff) Option { return func(c *Config) { c.Backoff = b } }

// WithRetryable sets the error classifier (nil = retry all errors).
func WithRetryable(fn func(error) bool) Option { return func(c *Config) { c.IsRetryable = fn } }

// Do calls fn with retry. It blocks between attempts (respecting ctx) and
// returns the first success or the last error after exhausting attempts.
//
// Concurrency: safe for concurrent use. Do is a stateless function — each call
// builds its own Config from the options and shares no mutable state, so any
// number of goroutines can call Do (with the same or different options)
// concurrently. The Backoff and IsRetryable funcs supplied via options are
// invoked from the calling goroutine and must themselves be concurrency-safe if
// shared.
func Do[T any](ctx context.Context, fn func(ctx context.Context) (T, error), opts ...Option) Result[T] {
	cfg := Config{
		MaxAttempts: 3,
		Backoff:     NoBackoff(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	var lastErr error
	var zero T
	for attempt := 1; cfg.MaxAttempts == 0 || attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return Result[T]{Value: zero, Err: ctx.Err(), Tries: attempt - 1}
		default:
		}
		val, err := fn(ctx)
		if err == nil {
			return Result[T]{Value: val, Tries: attempt}
		}
		lastErr = err
		// Check if error is retryable.
		if cfg.IsRetryable != nil && !cfg.IsRetryable(err) {
			return Result[T]{Value: zero, Err: err, Tries: attempt}
		}
		// Wait before next attempt (unless this is the last allowed attempt).
		if cfg.MaxAttempts == 0 || attempt < cfg.MaxAttempts {
			delay := cfg.Backoff(attempt)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return Result[T]{Value: zero, Err: ctx.Err(), Tries: attempt}
				case <-timer.C:
				}
			}
		}
	}
	return Result[T]{Value: zero, Err: lastErr, Tries: cfg.MaxAttempts}
}
