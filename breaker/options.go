package breaker

import (
	"errors"
	"time"
)

// BreakerState is the lifecycle stage of a Breaker.
type BreakerState int32

const (
	// StateClosed lets traffic through and records every call in the sliding
	// window. The breaker trips to StateOpen when the in-window failure rate
	// reaches FailRate over at least MinRequests calls.
	StateClosed BreakerState = iota
	// StateOpen blocks traffic: Execute returns ErrCircuitOpen without calling
	// fn. After OpenDuration elapses the next call moves the breaker to
	// StateHalfOpen.
	StateOpen
	// StateHalfOpen admits up to MaxRequests probe calls. All probes
	// succeeding returns the breaker to StateClosed; any failure sends it
	// back to StateOpen.
	StateHalfOpen
)

// BreakerOptions configures a Breaker. Zero values are replaced with sensible
// defaults by withDefaults at construction time, so the zero BreakerOptions is
// usable (it yields a breaker with all defaults).
//
// Field tags carry both json and mapstructure names so the struct can be
// loaded from either a JSON config or a Viper-style mapstructure source.
type BreakerOptions struct {
	// Name is an optional human-readable identifier surfaced in logs/metrics.
	Name string `json:"name" mapstructure:"name"`

	// MaxRequests is the number of probe calls permitted in StateHalfOpen.
	// The breaker returns to StateClosed once that many consecutive probes
	// succeed. Default 5. Clamped to >= 1.
	MaxRequests uint32 `json:"max_requests" mapstructure:"max_requests"`

	// Interval is the span of the sliding-window used to evaluate the
	// failure rate in StateClosed, rounded down to whole seconds (min 1s).
	// Default 60s.
	Interval time.Duration `json:"interval" mapstructure:"interval"`

	// OpenDuration is how long StateOpen blocks traffic before transitioning
	// to StateHalfOpen on the next call. Default 30s.
	OpenDuration time.Duration `json:"open_duration" mapstructure:"open_duration"`

	// FailRate is the in-window failure-rate threshold that trips the
	// breaker from StateClosed to StateOpen. Default 0.5.
	//
	// Note: because the zero value 0.0 is treated as "unset" by withDefaults
	// (and replaced with 0.5), callers who want trip-on-any-failure semantics
	// must pass a negative value. A negative FailRate trips as soon as
	// MinRequests calls have landed with at least one failure; a value > 1
	// never trips (useful for disabling a breaker at runtime).
	FailRate float64 `json:"fail_rate" mapstructure:"fail_rate"`

	// MinRequests is the minimum in-window call count required before the
	// failure rate is evaluated, so a few early failures don't trip the
	// breaker on a cold start. Default 10. Clamped to >= 1.
	MinRequests uint32 `json:"min_requests" mapstructure:"min_requests"`
}

// defaultBreakerOptions returns the package defaults used to fill zero option
// fields. It is the single source of truth for "what is the default for X".
func defaultBreakerOptions() BreakerOptions {
	return BreakerOptions{
		Name:         "",
		MaxRequests:  5,
		Interval:     60 * time.Second,
		OpenDuration: 30 * time.Second,
		FailRate:     0.5,
		MinRequests:  10,
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved, so callers can override
// only what they need. MaxRequests and MinRequests are clamped to >= 1, and
// Interval is normalised to whole seconds (min 1s).
func (o BreakerOptions) withDefaults() BreakerOptions {
	d := defaultBreakerOptions()
	if o.MaxRequests == 0 {
		o.MaxRequests = d.MaxRequests
	}
	if o.Interval <= 0 {
		o.Interval = d.Interval
	}
	if o.OpenDuration <= 0 {
		o.OpenDuration = d.OpenDuration
	}
	if o.FailRate == 0 {
		o.FailRate = d.FailRate
	}
	if o.MinRequests == 0 {
		o.MinRequests = d.MinRequests
	}
	// Name has no useful "is it set?" zero value to default, so leave it as is.
	if o.MaxRequests < 1 {
		o.MaxRequests = 1
	}
	if o.MinRequests < 1 {
		o.MinRequests = 1
	}
	// Normalise Interval to whole seconds (>= 1s) so the window ring has a
	// stable length. Mirrors RateAlerter's window rounding.
	if secs := int64(o.Interval.Seconds()); secs < 1 {
		o.Interval = time.Second
	} else {
		o.Interval = time.Duration(secs) * time.Second
	}
	if o.OpenDuration < 0 {
		o.OpenDuration = d.OpenDuration
	}
	return o
}

// String returns the lower-case name of the state: "closed", "open", or
// "half_open". Unknown values render as "unknown".
func (s BreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned by [Breaker.Execute] when the breaker is Open (or
// HalfOpen with all probe slots taken) and the call is therefore not attempted.
// Callers can branch on it with errors.Is.
var ErrCircuitOpen = errors.New("breaker: circuit is open")
