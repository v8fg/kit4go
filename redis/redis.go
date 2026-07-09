// Package redis is a thin, option-configured wrapper around go-redis/v9 that
// picks single-node vs cluster wiring from the address list and exposes the
// underlying redis.Cmdable for the full command surface.
//
// It stays deliberately small: ergonomic construction (functional options),
// a health check (Ping), and graceful Close. Everything else is the standard
// go-redis API reached via Client.Cmdable(). Ad-tech uses: real-time budget /
// pacing state, frequency capping, user/session lookups, leader election
// (paired with a distributed lock).
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Mode selects the connection topology.
type Mode int

const (
	// ModeAuto uses cluster when more than one address is given, else single
	// node. This is the default.
	ModeAuto Mode = iota
	// ModeSingle forces a single-node client.
	ModeSingle
	// ModeCluster forces a cluster client.
	ModeCluster
)

// Options configures the wrapped go-redis client. Zero values are left to
// go-redis's own defaults except where an option sets them.
type Options struct {
	Addrs        []string
	Mode         Mode
	Username     string
	Password     string
	DB           int
	MasterName   string // Sentinel master name; switches Sentinel topology when set with >0 Addrs
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	ClientName   string
	TLSConfig    *tls.Config
}

// Option configures Options.
type Option func(*Options)

// WithAddrs sets the seed address(es). At least one is required for New.
func WithAddrs(addrs ...string) Option { return func(o *Options) { o.Addrs = addrs } }

// WithMode forces the connection topology.
func WithMode(m Mode) Option { return func(o *Options) { o.Mode = m } }

// WithPassword sets the AUTH password.
func WithPassword(p string) Option { return func(o *Options) { o.Password = p } }

// WithUsername sets the AUTH username (Redis ACL / Redis 6+).
func WithUsername(u string) Option { return func(o *Options) { o.Username = u } }

// WithDB selects the logical database (single-node only; ignored by cluster).
func WithDB(db int) Option { return func(o *Options) { o.DB = db } }

// WithMasterName switches to Sentinel topology using the named master.
func WithMasterName(name string) Option { return func(o *Options) { o.MasterName = name } }

// WithDialTimeout sets the connection establishment timeout.
func WithDialTimeout(d time.Duration) Option { return func(o *Options) { o.DialTimeout = d } }

// WithReadTimeout sets the per-command socket read timeout.
func WithReadTimeout(d time.Duration) Option { return func(o *Options) { o.ReadTimeout = d } }

// WithWriteTimeout sets the per-command socket write timeout.
func WithWriteTimeout(d time.Duration) Option { return func(o *Options) { o.WriteTimeout = d } }

// WithPoolSize sets the maximum socket connections per node.
func WithPoolSize(n int) Option { return func(o *Options) { o.PoolSize = n } }

// WithMinIdleConns sets the minimum idle connections kept open per node.
func WithMinIdleConns(n int) Option { return func(o *Options) { o.MinIdleConns = n } }

// WithMaxRetries sets the number of retries for failing commands.
func WithMaxRetries(n int) Option { return func(o *Options) { o.MaxRetries = n } }

// WithClientName runs CLIENT SETNAME on each connection.
func WithClientName(name string) Option { return func(o *Options) { o.ClientName = name } }

// WithTLSConfig enables TLS using the supplied config.
func WithTLSConfig(c *tls.Config) Option { return func(o *Options) { o.TLSConfig = c } }

// Client wraps a redis.Cmdable. Construct with New (owns the underlying client)
// or Wrap (adopts an existing Cmdable, e.g. for tests).
type Client struct {
	cmd  goredis.Cmdable
	own  bool // true when New created the underlying client (Close will close it)
	opts Options
}

// ErrNoAddrs is returned by New when no addresses are configured.
var ErrNoAddrs = errors.New("redis: at least one address required (WithAddrs)")

// New builds a go-redis client from the options and wraps it. The topology is
// chosen by Mode (default ModeAuto: cluster when >1 address, else single node;
// Sentinel when MasterName is set).
func New(opts ...Option) (*Client, error) {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}
	if len(o.Addrs) == 0 {
		return nil, ErrNoAddrs
	}

	switch {
	case o.MasterName != "": // Sentinel
		failover := &goredis.FailoverOptions{
			MasterName:    o.MasterName,
			SentinelAddrs: o.Addrs,
			Username:      o.Username,
			Password:      o.Password,
			DB:            o.DB,
			DialTimeout:   o.DialTimeout,
			ReadTimeout:   o.ReadTimeout,
			WriteTimeout:  o.WriteTimeout,
			PoolSize:      o.PoolSize,
			MinIdleConns:  o.MinIdleConns,
			MaxRetries:    o.MaxRetries,
			TLSConfig:     o.TLSConfig,
			ClientName:    o.ClientName,
		}
		c := goredis.NewFailoverClient(failover)
		return &Client{cmd: c, own: true, opts: o}, nil
	case isCluster(o):
		co := &goredis.ClusterOptions{
			Addrs:        o.Addrs,
			Username:     o.Username,
			Password:     o.Password,
			DialTimeout:  o.DialTimeout,
			ReadTimeout:  o.ReadTimeout,
			WriteTimeout: o.WriteTimeout,
			PoolSize:     o.PoolSize,
			MinIdleConns: o.MinIdleConns,
			MaxRetries:   o.MaxRetries,
			TLSConfig:    o.TLSConfig,
			ClientName:   o.ClientName,
		}
		c := goredis.NewClusterClient(co)
		return &Client{cmd: c, own: true, opts: o}, nil
	default:
		so := &goredis.Options{
			Addr:         o.Addrs[0],
			Username:     o.Username,
			Password:     o.Password,
			DB:           o.DB,
			DialTimeout:  o.DialTimeout,
			ReadTimeout:  o.ReadTimeout,
			WriteTimeout: o.WriteTimeout,
			PoolSize:     o.PoolSize,
			MinIdleConns: o.MinIdleConns,
			MaxRetries:   o.MaxRetries,
			ClientName:   o.ClientName,
			TLSConfig:    o.TLSConfig,
		}
		c := goredis.NewClient(so)
		return &Client{cmd: c, own: true, opts: o}, nil
	}
}

func isCluster(o Options) bool {
	switch o.Mode {
	case ModeCluster:
		return true
	case ModeSingle:
		return false
	default: // ModeAuto
		return len(o.Addrs) > 1
	}
}

// Wrap adopts an existing redis.Cmdable. The wrapper does NOT own it: Close is
// a no-op and the caller remains responsible for closing the underlying client.
// Useful for injecting a miniredis-backed client in tests.
func Wrap(cmd goredis.Cmdable) *Client {
	return &Client{cmd: cmd, own: false}
}

// Cmdable returns the underlying go-redis command interface for direct access
// to the full command surface (Set, Get, HSet, Pipelines, Pub/Sub, ...).
func (c *Client) Cmdable() goredis.Cmdable { return c.cmd }

// Ping checks connectivity with a single PING command. It does not validate
// authorization beyond what go-redis already does on connect.
func (c *Client) Ping(ctx context.Context) error {
	return c.cmd.Ping(ctx).Err()
}

// Close closes the underlying client when New created it; for a wrapped client
// it is a no-op. Always returns nil for wrapped clients.
func (c *Client) Close() error {
	if !c.own {
		return nil
	}
	if closer, ok := c.cmd.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// Options returns the resolved options the client was built with.
func (c *Client) Options() Options { return c.opts }

// PoolStats returns connection-pool statistics when the underlying client
// exposes them (single-node, failover, and cluster clients do). Returns zero
// value for a wrapped Cmdable that does not.
func (c *Client) PoolStats() goredis.PoolStats {
	// go-redis exposes PoolStats as a *pointer* (*goredis.PoolStats). The
	// assertion must use the pointer signature — a value-returning one never
	// matches *Client and would silently return the zero value for every real
	// client. Dereference defensively (the pointer can be nil on a fresh pool).
	type statter interface{ PoolStats() *goredis.PoolStats }
	if s, ok := c.cmd.(statter); ok {
		if p := s.PoolStats(); p != nil {
			return *p
		}
	}
	return goredis.PoolStats{}
}
