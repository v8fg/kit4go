package tcpclient

import (
	"context"
	"time"
)

// CircuitBreaker is the interface used by [Client] to optionally wrap each call
// in a circuit breaker. The breaker package implements a generic Execute; this
// package uses the non-generic, error-only form (the same shape httpclient
// adopts) so callers can bridge a *breaker.Breaker[T] with a thin adapter, or
// supply any other implementation. tcpclient does NOT import breaker (that would
// create a hard dependency for every caller). A nil Breaker on [ClientOptions]
// disables the integration and calls are issued directly.
type CircuitBreaker interface {
	// Execute runs fn under the breaker's protection. Implementations should
	// short-circuit with their own ErrCircuitOpen when the breaker is open
	// rather than invoking fn, and record the outcome of fn for their sliding
	// window when it is invoked. fn must honour ctx.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// ClientOptions configures a [Client]. Zero values are replaced with sensible
// defaults by withDefaults at construction time, so the zero ClientOptions is
// usable (it yields a client with all defaults). The one exception is RetryMax:
// its zero value disables retrying (see the field comment); pass a negative
// value to fall back to the default. Breaker is the only field that truly opts
// into extra behaviour; everything else is a tunable.
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from either a JSON config or a Viper-style mapstructure source. Breaker is
// tagged "-" because a live breaker object cannot be (de)serialised.
type ClientOptions struct {
	// Network is the dial network: "tcp", "tcp4", "tcp6" or "unix". Default
	// "tcp". For Unix-domain sockets set this to "unix" and Address to the
	// socket filesystem path.
	Network string `json:"network" mapstructure:"network"`

	// Address is the dial target: host:port for TCP networks, or an absolute
	// filesystem path for Unix sockets. There is no default — a client with an
	// empty Address fails every call with a dial error.
	Address string `json:"address" mapstructure:"address"`

	// ConnectTimeout bounds the net.Dialer dial applied when a new connection
	// is needed. Default 5s. A value <= 0 keeps the package default.
	ConnectTimeout time.Duration `json:"connect_timeout" mapstructure:"connect_timeout"`

	// ReadTimeout bounds each read from the connection, applied via
	// SetReadDeadline at the start of the read. Default 10s. A slow or silent
	// peer that sends nothing within this window yields an i/o timeout error.
	ReadTimeout time.Duration `json:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout bounds each write to the connection, applied via
	// SetWriteDeadline before the write. Default 5s.
	WriteTimeout time.Duration `json:"write_timeout" mapstructure:"write_timeout"`

	// PoolSize is the maximum number of idle connections kept per address.
	// Default 10. When the pool is full, an extra connection may be dialled for
	// the in-flight call and closed when returned (rather than pooled).
	PoolSize int `json:"pool_size" mapstructure:"pool_size"`

	// IdleTimeout is how long an idle connection lives in the pool before being
	// closed. Default 90s (matches net/http's own default). Idle connections
	// older than this are discarded on checkout, so a caller never receives a
	// stale socket.
	IdleTimeout time.Duration `json:"idle_timeout" mapstructure:"idle_timeout"`

	// RetryMax is the maximum number of retry attempts after the first call,
	// i.e. the total number of sends is RetryMax+1. Default 2. A value of 0
	// disables retry entirely (exactly one attempt); only a negative value is
	// treated as "unset" and replaced with the default. This lets callers
	// explicitly opt out of retrying.
	RetryMax int `json:"retry_max" mapstructure:"retry_max"`

	// RetryWaitMin is the lower bound of the exponential backoff applied between
	// retries (before jitter). Default 100ms.
	RetryWaitMin time.Duration `json:"retry_wait_min" mapstructure:"retry_wait_min"`

	// RetryWaitMax is the upper bound of the backoff, and the cap above which
	// the exponential growth stops. Default 1s.
	RetryWaitMax time.Duration `json:"retry_wait_max" mapstructure:"retry_wait_max"`

	// Breaker, when non-nil, wraps every call via Breaker.Execute. nil (the
	// default) disables circuit-breaker integration.
	Breaker CircuitBreaker `json:"-"`
}

// defaultClientOptions returns the package defaults used to fill zero option
// fields. It is the single source of truth for "what is the default for X".
func defaultClientOptions() ClientOptions {
	return ClientOptions{
		Network:        "tcp",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   5 * time.Second,
		PoolSize:       10,
		IdleTimeout:    90 * time.Second,
		RetryMax:       2,
		RetryWaitMin:   100 * time.Millisecond,
		RetryWaitMax:   1 * time.Second,
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved, so callers can override
// only what they need. Address is never defaulted (there is no sensible default
// dial target) and Breaker is passed through unchanged.
func (o ClientOptions) withDefaults() ClientOptions {
	d := defaultClientOptions()
	if o.Network == "" {
		o.Network = d.Network
	}
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = d.ConnectTimeout
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = d.ReadTimeout
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = d.WriteTimeout
	}
	if o.PoolSize <= 0 {
		o.PoolSize = d.PoolSize
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = d.IdleTimeout
	}
	// RetryMax == 0 is a meaningful value (disable retries: one attempt
	// total). Only a negative (unset) value falls back to the default, so
	// callers can explicitly opt out of retrying by setting RetryMax to 0.
	if o.RetryMax < 0 {
		o.RetryMax = d.RetryMax
	}
	if o.RetryWaitMin <= 0 {
		o.RetryWaitMin = d.RetryWaitMin
	}
	if o.RetryWaitMax <= 0 {
		o.RetryWaitMax = d.RetryWaitMax
	}
	return o
}
