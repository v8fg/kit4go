package etcd

import (
	"crypto/tls"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Options configures the etcd client. Zero-valued tuning fields defer to
// client/v3's own defaults; only Endpoints is required.
type Options struct {
	Endpoints         []string
	DialTimeout       time.Duration
	DialKeepAliveTime time.Duration
	TLS               *tls.Config
	Username          string
	Password          string
	AutoSyncInterval  time.Duration
	RejectOldCluster  bool
}

// Option configures Options.
type Option func(*Options)

// WithEndpoints sets the etcd cluster URLs (e.g. "http://localhost:2379").
// Copies the slice so later caller mutation cannot affect the client. Required.
func WithEndpoints(endpoints ...string) Option {
	return func(o *Options) { o.Endpoints = append([]string(nil), endpoints...) }
}

// WithDialTimeout sets the connection dial timeout (0 -> client default 5s).
func WithDialTimeout(d time.Duration) Option { return func(o *Options) { o.DialTimeout = d } }

// WithDialKeepAliveTime sets the gRPC keepalive ping interval (0 -> client
// default 10s).
func WithDialKeepAliveTime(d time.Duration) Option {
	return func(o *Options) { o.DialKeepAliveTime = d }
}

// WithTLSConfig enables TLS.
func WithTLSConfig(c *tls.Config) Option { return func(o *Options) { o.TLS = c } }

// WithUsername sets the auth username (pair with WithPassword).
func WithUsername(u string) Option { return func(o *Options) { o.Username = u } }

// WithPassword sets the auth password.
func WithPassword(p string) Option { return func(o *Options) { o.Password = p } }

// WithAutoSyncInterval sets how often endpoints are re-resolved from the cluster
// (0 disables auto-sync).
func WithAutoSyncInterval(d time.Duration) Option {
	return func(o *Options) { o.AutoSyncInterval = d }
}

// WithRejectOldCluster rejects connecting to an incompatible (old) cluster.
func WithRejectOldCluster(b bool) Option { return func(o *Options) { o.RejectOldCluster = b } }

// withDefaults applies the option chain and fills defaults. DialTimeout defaults
// to 5s so a forgotten timeout never hangs startup against a dead endpoint.
func withDefaults(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	if o.DialTimeout == 0 {
		o.DialTimeout = 5 * time.Second
	}
	return o
}

// toConfig maps Options onto client/v3's Config. Zero-valued tuning fields are
// forwarded as-is so the client applies its own defaults for the unset ones.
func (o Options) toConfig() clientv3.Config {
	cfg := clientv3.Config{
		Endpoints:        o.Endpoints,
		DialTimeout:      o.DialTimeout,
		TLS:              o.TLS,
		Username:         o.Username,
		Password:         o.Password,
		AutoSyncInterval: o.AutoSyncInterval,
		RejectOldCluster: o.RejectOldCluster,
	}
	if o.DialKeepAliveTime > 0 {
		cfg.DialKeepAliveTime = o.DialKeepAliveTime
	}
	return cfg
}
