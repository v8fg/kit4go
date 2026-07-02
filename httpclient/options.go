package httpclient

import (
	"context"
	"time"
)

// CircuitBreaker is the interface used by [Client] to optionally wrap each call
// in a circuit breaker. The breaker package implements this; httpclient does
// NOT import breaker (that would create a hard dependency for every caller).
// Users pass a *breaker.Breaker[T] which satisfies this interface, or any other
// implementation. A nil breaker on [ClientOptions] disables the integration and
// calls are issued directly.
type CircuitBreaker interface {
	// Execute runs fn under the breaker's protection. Implementations should
	// short-circuit with their own ErrCircuitOpen when the breaker is open
	// rather than invoking fn, and record the outcome of fn for their sliding
	// window when it is invoked. fn must honour ctx.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// LatencyObserver receives the end-to-end duration of a call. httpclient does
// not import the latency package; pass a *latency.Histogram (which satisfies
// this interface) or any other implementation. A nil Latency on ClientOptions
// disables observation — the call site is a single nil check, with no time.Now
// and no defer, so the disabled path is free.
type LatencyObserver interface {
	// Observe records a single latency sample. Must be safe for concurrent use.
	Observe(time.Duration)
}

// ClientOptions configures a [Client]. Zero values are replaced with sensible
// defaults by withDefaults at construction time, so the zero ClientOptions is
// usable (it yields a client with all defaults). Breaker is the only field that
// truly opts into extra behaviour; everything else is a tunable.
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from either a JSON config or a Viper-style mapstructure source. Breaker is
// tagged "-" because a live breaker object cannot be (de)serialised.
type ClientOptions struct {
	// ConnectTimeout bounds the TCP dial applied via the transport's
	// DialContext. Default 5s.
	ConnectTimeout time.Duration `json:"connect_timeout" mapstructure:"connect_timeout"`

	// RequestTimeout bounds the whole request (DNS + connect + write + read),
	// applied via context.WithTimeout on every call. Default 30s. A value <= 0
	// keeps the package default.
	RequestTimeout time.Duration `json:"request_timeout" mapstructure:"request_timeout"`

	// MaxIdleConns is the connection-pool-wide cap on idle connections kept
	// across all hosts. Default 100.
	MaxIdleConns int `json:"max_idle_conns" mapstructure:"max_idle_conns"`

	// IdleConnTimeout is how long an idle connection lives in the pool before
	// being closed. Default 90s (matches net/http's own default).
	IdleConnTimeout time.Duration `json:"idle_conn_timeout" mapstructure:"idle_conn_timeout"`

	// MaxIdlePerHost caps the idle connections kept per host. Default 10. Raise
	// this for high-fanout clients talking to a single host.
	MaxIdlePerHost int `json:"max_idle_per_host" mapstructure:"max_idle_per_host"`

	// RetryMax is the maximum number of retry attempts after the first call,
	// i.e. the total number of sends is RetryMax+1. Default 3.
	RetryMax int `json:"retry_max" mapstructure:"retry_max"`

	// RetryWaitMin is the lower bound of the exponential backoff applied between
	// retries (before jitter). Default 100ms.
	RetryWaitMin time.Duration `json:"retry_wait_min" mapstructure:"retry_wait_min"`

	// RetryWaitMax is the upper bound of the backoff, and the cap above which
	// the exponential growth stops. Default 2s.
	RetryWaitMax time.Duration `json:"retry_wait_max" mapstructure:"retry_wait_max"`

	// FollowRedirect controls HTTP redirect handling. The zero value (false)
	// is treated as "use the default" which follows redirects — see
	// [ClientOptions.withDefaults]. To actually disable redirect following,
	// construct the options via [WithNoRedirect] or set RedirectEnabled to
	// false after parsing config. This tri-state dance exists because Go has no
	// nullable bool; see FollowRedirectSet.
	FollowRedirect bool `json:"follow_redirect" mapstructure:"follow_redirect"`

	// FollowRedirectSet records whether FollowRedirect was explicitly set. When
	// false, withDefaults applies the default (true). When true, the explicit
	// FollowRedirect value is honoured as-is. This field is populated by config
	// loaders (Viper/JSON unmarshalling set it whenever the key is present) and
	// by [WithRedirect]; hand-written literals should use the [WithRedirect] /
	// [WithNoRedirect] helpers for clarity.
	FollowRedirectSet bool `json:"-" mapstructure:"-"`

	// EnableHTTP2 enables HTTP/2 over the transport (h2c for cleartext, ALPN
	// for TLS). Default false — HTTP/1.1 only. Set true to negotiate HTTP/2
	// when the server supports it (most modern servers do over TLS).
	// HTTP/2 multiplexes multiple requests over a single TCP connection, which
	// can significantly reduce latency for high-fanout calls to the same host.
	EnableHTTP2 bool `json:"enable_http2" mapstructure:"enable_http2"`

	// Breaker, when non-nil, wraps every call via Breaker.Execute. nil (the
	// default) disables circuit-breaker integration.
	//
	// Scope to be aware of: the breaker covers the round-trip through receipt of
	// the response HEADERS, not the response-body read that follows. A downstream
	// that returns headers promptly then stalls or truncates the body is seen as
	// healthy by the breaker. Also, a call that succeeds only after retries is
	// recorded as a single success — per-attempt failures inside the retry
	// budget are invisible to the breaker. For per-attempt visibility, wrap a
	// single attempt in the breaker and retry outside it.
	Breaker CircuitBreaker `json:"-"`

	// Latency, when non-nil, receives the end-to-end duration of every call
	// (including retries and body read). nil (the default) disables latency
	// observation.
	Latency LatencyObserver `json:"-"`
}

// defaultClientOptions returns the package defaults used to fill zero option
// fields. It is the single source of truth for "what is the default for X".
func defaultClientOptions() ClientOptions {
	return ClientOptions{
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  30 * time.Second,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
		MaxIdlePerHost:  10,
		RetryMax:        3,
		RetryWaitMin:    100 * time.Millisecond,
		RetryWaitMax:    2 * time.Second,
		FollowRedirect:  true,
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved, so callers can override
// only what they need. FollowRedirect is set to its default (true) unless the
// caller signalled an explicit choice via FollowRedirectSet, in which case the
// explicit value is kept.
func (o ClientOptions) withDefaults() ClientOptions {
	d := defaultClientOptions()
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = d.ConnectTimeout
	}
	if o.RequestTimeout <= 0 {
		o.RequestTimeout = d.RequestTimeout
	}
	if o.MaxIdleConns <= 0 {
		o.MaxIdleConns = d.MaxIdleConns
	}
	if o.IdleConnTimeout <= 0 {
		o.IdleConnTimeout = d.IdleConnTimeout
	}
	if o.MaxIdlePerHost <= 0 {
		o.MaxIdlePerHost = d.MaxIdlePerHost
	}
	if o.RetryMax <= 0 {
		o.RetryMax = d.RetryMax
	}
	if o.RetryWaitMin <= 0 {
		o.RetryWaitMin = d.RetryWaitMin
	}
	if o.RetryWaitMax <= 0 {
		o.RetryWaitMax = d.RetryWaitMax
	}
	if !o.FollowRedirectSet {
		o.FollowRedirect = d.FollowRedirect
	}
	return o
}

// WithRedirect returns a copy of o with redirect following explicitly enabled
// (and flagged as set, so withDefaults will not override it).
func (o ClientOptions) WithRedirect() ClientOptions {
	o.FollowRedirect = true
	o.FollowRedirectSet = true
	return o
}

// WithNoRedirect returns a copy of o with redirect following explicitly
// disabled (and flagged as set, so withDefaults will not override it). The
// resulting client surfaces the raw 3xx response to the caller.
func (o ClientOptions) WithNoRedirect() ClientOptions {
	o.FollowRedirect = false
	o.FollowRedirectSet = true
	return o
}

// WithRedirect is a package-level convenience that mirrors
// [ClientOptions.WithRedirect]. It is provided so callers can write
// httpclient.WithRedirect(opts) at construction sites that read options from a
// plain ClientOptions value.
func WithRedirect(o ClientOptions) ClientOptions { return o.WithRedirect() }

// WithNoRedirect is a package-level convenience that mirrors
// [ClientOptions.WithNoRedirect].
func WithNoRedirect(o ClientOptions) ClientOptions { return o.WithNoRedirect() }
