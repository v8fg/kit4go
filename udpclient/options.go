package udpclient

import (
	"context"
	"time"
)

// CircuitBreaker is the interface used by [Client] to optionally wrap each call
// in a circuit breaker. The breaker package implements an equivalent shape;
// udpclient does NOT import breaker (that would create a hard dependency for
// every caller). Users pass an implementation (e.g. an adapter wrapping a
// *breaker.Breaker[T], whose Execute returns (T, error)) or any other
// implementation. A nil breaker on [ClientOptions] disables the integration and
// calls are issued directly.
type CircuitBreaker interface {
	// Execute runs fn under the breaker's protection. Implementations should
	// short-circuit with their own ErrCircuitOpen when the breaker is open
	// rather than invoking fn, and record the outcome of fn for their sliding
	// window when it is invoked. fn must honour ctx.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// ClientOptions configures a [Client]. Zero values are replaced with sensible
// defaults by withDefaults at construction time, so the zero ClientOptions is
// usable (it yields a client with all defaults). Address is the one truly
// required field — without it NewClient returns an error. Breaker is the only
// field that opts into extra behaviour beyond the socket; everything else is a
// tunable.
//
// Field tags carry both json and mapstructure names so the struct can be loaded
// from either a JSON config or a Viper-style mapstructure source. Breaker is
// tagged "-" because a live breaker object cannot be (de)serialised.
type ClientOptions struct {
	// Address is the remote UDP peer to dial (host:port, or a literal IP). It is
	// resolved once at construction time via net.ResolveUDPAddr and the result
	// is wired into a connected [net.UDPConn]. Required.
	Address string `json:"address" mapstructure:"address"`

	// LocalAddress, when non-empty, binds the outbound socket's source address
	// (host:port) before connecting to Address. Empty (the default) lets the OS
	// pick an ephemeral source port, which is almost always what you want. Set
	// it when a NAT, firewall or ACL pins a specific source port. The address is
	// resolved at construction time.
	LocalAddress string `json:"local_address" mapstructure:"local_address"`

	// ReadTimeout bounds every datagram read issued by [Client.SendReceive],
	// applied as a [net.UDPConn.SetReadDeadline]. Default 5s. A value <= 0 keeps
	// the package default. A read that exceeds the deadline surfaces as a
	// retryable error (typically the sign of a dropped/unanswered datagram).
	ReadTimeout time.Duration `json:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout bounds every datagram write, applied as a
	// [net.UDPConn.SetWriteDeadline]. Default 2s. UDP writes to a reachable peer
	// are normally near-instant, so this primarily guards against a wedged
	// kernel/socket; a value <= 0 keeps the package default.
	WriteTimeout time.Duration `json:"write_timeout" mapstructure:"write_timeout"`

	// BufferSize is the size of the read buffer used by [Client.SendReceive].
	// Datagrams larger than this are silently truncated by the kernel, so size
	// it to your protocol's MTU (the common 1472-byte UDP-over-Ethernet payload
	// is well under the 4096 default). Default 4096.
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`

	// RetryMax is the maximum number of retry attempts after the first call,
	// i.e. the total number of sends is RetryMax+1. Default 2. UDP being
	// unreliable, a small retry count is a cheap way to absorb transient
	// packet/timeout loss; set 0 to send exactly once.
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
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 2 * time.Second,
		BufferSize:   4096,
		RetryMax:     2,
		RetryWaitMin: 100 * time.Millisecond,
		RetryWaitMax: 1 * time.Second,
	}
}

// withDefaults returns a copy of o with every zero field replaced by the
// corresponding default. Non-zero fields are preserved, so callers can override
// only what they need. Address and Breaker are never defaulted — Address stays
// empty (and NewClient will reject it) and Breaker stays nil unless explicitly
// set.
func (o ClientOptions) withDefaults() ClientOptions {
	d := defaultClientOptions()
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = d.ReadTimeout
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = d.WriteTimeout
	}
	if o.BufferSize <= 0 {
		o.BufferSize = d.BufferSize
	}
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
