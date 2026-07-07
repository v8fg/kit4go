package limiter

import "time"

// Algorithm identifiers accepted by [NewLimiter]. Use them via [LimiterOptions].
const (
	// AlgorithmTokenBucket selects the token-bucket implementation: continuous
	// refill at Rate up to Burst capacity. Requires Burst > 0.
	AlgorithmTokenBucket = "token_bucket"

	// AlgorithmSlidingWindow selects the sliding-window counter implementation:
	// at most Rate requests per Window. Burst is ignored.
	AlgorithmSlidingWindow = "sliding_window"

	// AlgorithmFixedWindow selects the fixed-window counter implementation:
	// at most Rate requests per Window, resetting at each window boundary.
	// Simple and fast, but allows bursts at window edges. Burst is ignored.
	AlgorithmFixedWindow = "fixed_window"

	// AlgorithmLeakyBucket selects the leaky-bucket implementation: requests
	// "fill" a bucket that drains at Rate; if the bucket is full (capacity =
	// Burst), the request is denied. Smooths outflow. Requires Burst > 0.
	AlgorithmLeakyBucket = "leaky_bucket"

	// AlgorithmGCRA selects the Generic Cell Rate Algorithm (local GCRA):
	// equivalent to the distributed GCRA in kit4go/rate, but in-process.
	// Tracks a single "theoretical arrival time" per limiter. Requires Burst > 0.
	AlgorithmGCRA = "gcra"
)

// LimiterOptions configures a [Limiter]. Pass it to [NewLimiter].
//
// JSON/mapstructure tags are included so the struct loads cleanly from config
// files (e.g. viper), matching the convention used across kit4go.
type LimiterOptions struct {
	// Algorithm selects the limiter implementation. One of
	// [AlgorithmTokenBucket] or [AlgorithmSlidingWindow]. Required.
	Algorithm string `json:"algorithm" mapstructure:"algorithm"`

	// Rate is the steady-state allow rate:
	//   - token bucket: tokens added per second (QPS).
	//   - sliding window: max requests allowed inside Window.
	// Must be > 0.
	Rate float64 `json:"rate" mapstructure:"rate"`

	// Burst is the token-bucket capacity — the largest spike absorbed in one go.
	// Token bucket only; ignored by the sliding window. Must be > 0 for a token
	// bucket (it is clamped to >= 1 by [withDefaults]).
	Burst int `json:"burst" mapstructure:"burst"`

	// Window is the sliding-window size. Sliding window only; ignored by the
	// token bucket. Defaults to 1 second; rounded down to whole seconds with a
	// minimum of 1s by [withDefaults].
	Window time.Duration `json:"window" mapstructure:"window"`
}

// defaultLimiterOptions returns the baseline options applied when a field is
// unset. Tests can rely on these as the documented defaults.
func defaultLimiterOptions() LimiterOptions {
	return LimiterOptions{
		Algorithm: AlgorithmTokenBucket,
		Rate:      1,
		Burst:     1,
		Window:    time.Second,
	}
}

// withDefaults returns a copy of o with zero values replaced by defaults:
//   - Algorithm "" -> [AlgorithmTokenBucket] (UNSET only; a non-empty but
//     unrecognised value is left untouched so [NewLimiter]'s switch can reject
//     it via its default arm — see the contract on [NewLimiter])
//   - Rate <= 0         -> 1
//   - Burst <= 0        -> 1 (token bucket only)
//   - Window <= 0       -> 1s (sliding window only, then rounded to >= 1s)
//
// Burst and Window are clamped regardless of algorithm so the struct is always
// internally consistent if the caller flips Algorithm later.
//
// Note: only the empty string is treated as "unset". A typo like "tokn_bucket"
// is preserved and surfaces as a nil limiter from [NewLimiter] rather than
// silently degrading to token-bucket semantics.
func (o LimiterOptions) withDefaults() LimiterOptions {
	d := defaultLimiterOptions()
	if o.Algorithm == "" {
		o.Algorithm = d.Algorithm
	}
	if o.Rate <= 0 {
		o.Rate = d.Rate
	}
	if o.Burst <= 0 {
		o.Burst = d.Burst
	}
	if o.Window <= 0 {
		o.Window = d.Window
	}
	return o
}
