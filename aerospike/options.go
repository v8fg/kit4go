package aerospike

import (
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
)

// Options configures the aerospike client. Zero-valued tuning fields defer to
// the driver's own defaults; only Host is required.
type Options struct {
	Host        string
	Port        int
	Timeout     time.Duration // connection timeout (client policy)
	Namespace   string        // default namespace (documentation only; ops take a *Key which carries it)
	UserName    string
	Password    string
	ClusterName string
}

// Option configures Options.
type Option func(*Options)

// WithHost sets the cluster host. Required.
func WithHost(host string) Option { return func(o *Options) { o.Host = host } }

// WithPort sets the port (default 3000).
func WithPort(port int) Option { return func(o *Options) { o.Port = port } }

// WithTimeout sets the connection/transaction timeout (0 -> driver default 30s).
func WithTimeout(d time.Duration) Option { return func(o *Options) { o.Timeout = d } }

// WithCredentials sets auth username/password (for a security-enabled cluster).
func WithCredentials(user, password string) Option {
	return func(o *Options) { o.UserName = user; o.Password = password }
}

// WithClusterName constrains the connection to a cluster with this name.
func WithClusterName(name string) Option { return func(o *Options) { o.ClusterName = name } }

// WithNamespace records the default namespace for documentation; the actual
// namespace is carried by each *as.Key passed to ops.
func WithNamespace(ns string) Option { return func(o *Options) { o.Namespace = ns } }

// withDefaults applies the option chain. Port defaults to 3000; Timeout to 5s.
func withDefaults(opts []Option) Options {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	if o.Port == 0 {
		o.Port = 3000
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Second
	}
	return o
}

// toClientPolicy maps Options onto the driver's *as.ClientPolicy.
func (o Options) toClientPolicy() *as.ClientPolicy {
	cp := as.NewClientPolicy()
	cp.Timeout = o.Timeout // ClientPolicy.Timeout is a time.Duration
	if o.UserName != "" || o.Password != "" {
		cp.User = o.UserName
		cp.Password = o.Password
	}
	if o.ClusterName != "" {
		cp.ClusterName = o.ClusterName
	}
	return cp
}
