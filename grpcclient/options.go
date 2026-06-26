package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
)

// CircuitBreaker is the interface used by [Middleware] to optionally wrap each
// call in a circuit breaker. The breaker package implements this; grpcclient
// does NOT import breaker (that would create a hard dependency for every
// caller). Users pass a *breaker.Breaker[T] which satisfies this interface, or
// any other implementation. A nil breaker on [ClientOptions] disables the
// integration and calls are issued directly.
type CircuitBreaker interface {
	// Execute runs fn under the breaker's protection. Implementations should
	// short-circuit with their own ErrCircuitOpen when the breaker is open
	// rather than invoking fn, and record the outcome of fn for their sliding
	// window when it is invoked. fn must honour ctx.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// ClientOptions configures a [Middleware]. Zero values are replaced with
// sensible defaults by withDefaults at construction time, so the zero
// ClientOptions is usable (it yields a middleware with all defaults). Breaker
// and RetryCodes are the only fields that opt into extra behaviour; everything
// else is a tunable.
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from either a JSON config or a Viper-style mapstructure source. Breaker and
// RetryCodes are tagged "-" because a live breaker object and a codes.Code
// slice cannot be cleanly (de)serialised.
type ClientOptions struct {
	// Target is the gRPC server address dialled by [DialConn], e.g.
	// "localhost:50051" or "dns:///srv.example.com:443". It is not consumed by
	// the interceptors themselves (they only see per-call method names); it is
	// stored on the options purely so DialConn has everything it needs.
	Target string `json:"target" mapstructure:"target"`

	// ConnectTimeout bounds the initial grpc.Dial connection establishment.
	// Applied by [DialConn] via grpc.WithConnectParams. Default 5s. A value <= 0
	// keeps the package default.
	ConnectTimeout time.Duration `json:"connect_timeout" mapstructure:"connect_timeout"`

	// RequestTimeout is the per-RPC timeout applied via context.WithTimeout on
	// every unary and stream call. Default 10s. A caller-supplied context
	// deadline tighter than RequestTimeout always wins.
	RequestTimeout time.Duration `json:"request_timeout" mapstructure:"request_timeout"`

	// RetryMax is the maximum number of retry attempts after the first call,
	// i.e. the total number of sends for a unary RPC is RetryMax+1. Default 2.
	// Retries only apply to unary RPCs whose status code is in RetryCodes.
	RetryMax int `json:"retry_max" mapstructure:"retry_max"`

	// RetryCodes is the set of gRPC status codes that trigger a unary-RPC
	// retry. Default [codes.Unavailable, codes.DeadlineExceeded]. A nil/empty
	// slice is replaced with the default by withDefaults. Pass a single-element
	// slice containing codes.OK to effectively disable retry (no real RPC ever
	// returns OK as an error).
	RetryCodes []codes.Code `json:"-"`

	// RetryWaitMin is the lower bound of the exponential backoff applied
	// between unary retries (before jitter). Default 100ms.
	RetryWaitMin time.Duration `json:"retry_wait_min" mapstructure:"retry_wait_min"`

	// RetryWaitMax is the upper bound of the backoff, and the cap above which
	// the exponential growth stops. Default 1s.
	RetryWaitMax time.Duration `json:"retry_wait_max" mapstructure:"retry_wait_max"`

	// Breaker, when non-nil, wraps every unary and stream call via
	// Breaker.Execute. nil (the default) disables circuit-breaker integration.
	Breaker CircuitBreaker `json:"-"`
}

// defaultClientOptions returns the package defaults used to fill zero option
// fields. It is the single source of truth for "what is the default for X".
func defaultClientOptions() ClientOptions {
	return ClientOptions{
		ConnectTimeout: 5 * time.Second,
		RequestTimeout: 10 * time.Second,
		RetryMax:       2,
		RetryCodes:     []codes.Code{codes.Unavailable, codes.DeadlineExceeded},
		RetryWaitMin:   100 * time.Millisecond,
		RetryWaitMax:   1 * time.Second,
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved, so callers can override
// only what they need. RetryCodes is replaced with the default only when nil or
// empty, so a caller that deliberately empties it (len 0 via append) gets the
// default; to truly disable retries, set RetryMax to 0.
func (o ClientOptions) withDefaults() ClientOptions {
	d := defaultClientOptions()
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = d.ConnectTimeout
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = d.RequestTimeout
	}
	if o.RetryMax <= 0 {
		o.RetryMax = d.RetryMax
	}
	if len(o.RetryCodes) == 0 {
		o.RetryCodes = d.RetryCodes
	}
	if o.RetryWaitMin <= 0 {
		o.RetryWaitMin = d.RetryWaitMin
	}
	if o.RetryWaitMax <= 0 {
		o.RetryWaitMax = d.RetryWaitMax
	}
	// Target has no useful "is it set?" zero value to default, so leave it as is.
	return o
}

// retryable reports whether a gRPC status code warrants a unary-RPC retry. The
// check is a linear scan over RetryCodes; the slice is tiny (1-3 codes in
// practice) so a map would be over-engineering.
func (o ClientOptions) retryable(c codes.Code) bool {
	for _, rc := range o.RetryCodes {
		if rc == c {
			return true
		}
	}
	return false
}
